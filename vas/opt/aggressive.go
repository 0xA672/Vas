package opt

import (
	"fmt"
	"strconv"
	"strings"

	"vas/vas/internal/asm"
)

// ---------------------------------------------------------------------------
// Common subexpression elimination (block-local, versioned)
// ---------------------------------------------------------------------------

func cse(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, cseBlock(block)...)
	}
	return result
}

type cseKey struct {
	op   string
	args string
}

type cseEntry struct {
	dst int
}

func cseBlock(lines []string) []string {
	seen := map[cseKey]cseEntry{}
	var versions [13]int
	result := make([]string, 0, len(lines))

	clear := func() { seen = map[cseKey]cseEntry{} }

	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			result = append(result, line)
			continue
		}
		if touchesPhysicalReg(line) || isUnknownOrClobber(p) {
			clear()
			result = append(result, line)
			continue
		}
		op := p.op
		args := p.args
		dst := dstReg(op, args)

		// A write to dst invalidates every cached expression that produced it.
		if dst >= 0 {
			for key, entry := range seen {
				if entry.dst == dst {
					delete(seen, key)
				}
			}
		}

		pure := (op == "ADD" || op == "SUB" || op == "MUL") && len(args) >= 2 &&
			!hasMemoryOperand(args) && allArgsHaveOnlyVirtualRegs(args)
		if pure && dst >= 0 {
			key := cseKey{op: op, args: versionedArgs(args[1:], &versions)}
			if entry, found := seen[key]; found && entry.dst != dst && !flagsLiveAfter(lines, i) {
				result = append(result, fmt.Sprintf("\tMOV\t%s, v%d%s", args[0], entry.dst, commentSuffix(line)))
				if dst >= 0 {
					versions[dst]++
				}
				continue
			}
			seen[key] = cseEntry{dst: dst}
		}

		if dst >= 0 {
			versions[dst]++
		}
		result = append(result, line)
	}
	return result
}

func versionedArgs(args []string, versions *[13]int) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte('|')
		}
		b.WriteString(asm.AddrKey(a))
		for _, r := range virtualRegsIn(a) {
			fmt.Fprintf(&b, "@%d", versions[r])
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Loop-invariant code motion
// ---------------------------------------------------------------------------

func licm(lines []string) []string {
	labelIdx := map[string]int{}
	jumpTargets := map[string][]int{}
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			code := asm.TrimCode(line)
			if strings.HasSuffix(code, ":") {
				labelIdx[strings.TrimSuffix(code, ":")] = i
			}
			continue
		}
		if asm.IsBranch(p.op) && len(p.args) > 0 {
			target := strings.TrimRight(strings.TrimSpace(p.args[0]), ",")
			if target != "" {
				jumpTargets[target] = append(jumpTargets[target], i)
			}
		}
	}

	type loop struct {
		header int
		back   int
	}
	var loops []loop
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok || !asm.IsBranch(p.op) || len(p.args) == 0 {
			continue
		}
		target := strings.TrimRight(strings.TrimSpace(p.args[0]), ",")
		if idx, ok := labelIdx[target]; ok && idx < i {
			loops = append(loops, loop{header: idx, back: i})
		}
	}
	if len(loops) == 0 {
		return lines
	}

	for _, l := range loops {
		isNested := false
		for _, outer := range loops {
			if outer.header < l.header && outer.back > l.back {
				isNested = true
				break
			}
		}
		if isNested {
			continue
		}
		// The hoisted code executes only on fall-through into the header. If
		// any jump from outside the loop targets the header, skip the loop.
		headerName := ""
		for name, idx := range labelIdx {
			if idx == l.header {
				headerName = name
				break
			}
		}
		externalEntry := false
		for _, j := range jumpTargets[headerName] {
			if j < l.header || j > l.back {
				externalEntry = true
				break
			}
		}
		if headerName == "" || externalEntry {
			continue
		}

		modified := map[int]bool{}
		writes := map[int]int{}
		for j := l.header + 1; j <= l.back; j++ {
			p, ok := parseOptLine(lines[j])
			if !ok {
				continue
			}
			if touchesPhysicalReg(lines[j]) || isUnknownOrClobber(p) {
				for r := 0; r <= 12; r++ {
					modified[r] = true
				}
				continue
			}
			if dst := dstReg(p.op, p.args); dst >= 0 {
				modified[dst] = true
				writes[dst]++
			}
			// Passthrough instructions that mention a virtual register may
			// write it; treat it as modified.
			if !knownDstOp(p.op) && p.op != "CMP" {
				for _, a := range p.args {
					for _, r := range virtualRegsIn(a) {
						modified[r] = true
					}
				}
			}
		}

		var hoisted []int
		var hoistedLines []string
		for j := l.header + 1; j < l.back; j++ {
			p, ok := parseOptLine(lines[j])
			if !ok || p.op != "LEA" || len(p.args) < 2 {
				continue
			}
			dst := regIndex(p.args[0])
			if dst < 0 || writes[dst] != 1 || modified[dst] && writes[dst] > 1 {
				continue
			}
			// Destination must not be read before this LEA in the loop.
			readBefore := false
			for k := l.header + 1; k < j; k++ {
				pp, ok := parseOptLine(lines[k])
				if !ok {
					continue
				}
				for _, r := range readRegs(pp.op, pp.args) {
					if r == dst {
						readBefore = true
						break
					}
				}
				if readBefore {
					break
				}
			}
			if readBefore {
				continue
			}
			// Every register in the effective address must be loop-invariant
			// and must not be the destination itself.
			invariant := true
			for _, r := range virtualRegsIn(p.args[1]) {
				if r == dst || modified[r] {
					invariant = false
					break
				}
			}
			if !invariant || addressHasPhysical(p.args[1]) {
				continue
			}
			hoisted = append(hoisted, j)
			hoistedLines = append(hoistedLines, lines[j])
		}

		if len(hoistedLines) > 0 {
			remove := map[int]bool{}
			for _, h := range hoisted {
				remove[h] = true
			}
			var out []string
			for i, line := range lines {
				if i == l.header {
					out = append(out, hoistedLines...)
				}
				if !remove[i] {
					out = append(out, line)
				}
			}
			return out
		}
	}
	return lines
}

// extractAddressRegs returns every register referenced in an effective-address
// expression such as "v1+v2*4-8". Constants and scale factors are ignored.
func extractAddressRegs(expr string) []string {
	toks := asm.OperandRegs(expr)
	out := make([]string, 0, len(toks))
	for _, tok := range toks {
		if asm.IsReg(tok) {
			out = append(out, tok)
		}
	}
	return out
}

func isRegName(s string) bool { return asm.IsReg(s) }

// ---------------------------------------------------------------------------
// Redundant load elimination
// ---------------------------------------------------------------------------

func redundantLoadElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, redundantLoadBlock(block)...)
	}
	return result
}

type cachedLoad struct {
	reg    int
	regVer int
}

func redundantLoadBlock(lines []string) []string {
	var versions [13]int
	loads := map[string]cachedLoad{}
	clear := func() { loads = map[string]cachedLoad{} }

	result := make([]string, len(lines))
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			result[i] = line
			continue
		}
		if touchesPhysicalReg(line) || isUnknownOrClobber(p) {
			clear()
			result[i] = line
			continue
		}
		op := p.op
		args := p.args
		dst := dstReg(op, args)

		if op == "STORE" && len(args) >= 2 {
			if addressHasPhysical(args[1]) || !isStaticMemory(args[1]) {
				clear()
				result[i] = line
				continue
			}
			key, ok := memKey(args[1])
			if ok {
				delete(loads, key+addressVersions(args[1], &versions))
			}
			if dst >= 0 {
				versions[dst]++
			}
			result[i] = line
			continue
		}

		if op == "LOAD" && len(args) >= 2 {
			if addressHasPhysical(args[1]) {
				clear()
				result[i] = line
				continue
			}
			key, ok := memKey(args[1])
			if !ok {
				result[i] = line
				continue
			}
			full := key + addressVersions(args[1], &versions)
			if c, ok := loads[full]; ok && c.reg != dst && versions[c.reg] == c.regVer {
				result[i] = fmt.Sprintf("\tMOV\t%s, v%d%s", args[0], c.reg, commentSuffix(line))
				if dst >= 0 {
					versions[dst]++
					loads[full] = cachedLoad{reg: dst, regVer: versions[dst]}
				}
				continue
			}
		}

		if dst >= 0 {
			for key, c := range loads {
				if c.reg == dst {
					delete(loads, key)
				}
			}
			versions[dst]++
		}
		// A LOAD defines a new cached value. Invalidate the cache first so a
		// register that was written concurrently cannot be reused.
		if op == "LOAD" && len(args) >= 2 && !addressHasPhysical(args[1]) {
			if key, ok := memKey(args[1]); ok {
				loads[key+addressVersions(args[1], &versions)] = cachedLoad{reg: dst, regVer: versions[dst]}
			}
		}
		result[i] = line
	}
	return result
}

// ---------------------------------------------------------------------------
// PUSH/POP elimination
// ---------------------------------------------------------------------------

func pushPopElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, pushPopBlock(block)...)
	}
	return result
}

func pushPopBlock(lines []string) []string {
	remove := make([]bool, len(lines))
	for i, line := range lines {
		if remove[i] {
			continue
		}
		p, ok := parseOptLine(line)
		if !ok || p.op != "PUSH" || len(p.args) < 1 {
			continue
		}
		reg := strings.TrimRight(p.args[0], ",")

		if removed, mid, pop := scanPushModPop(i, reg, lines, remove); removed {
			remove[i], remove[mid], remove[pop] = true, true, true
			continue
		}

		for j := i + 1; j < len(lines); j++ {
			if remove[j] {
				continue
			}
			pp, ok := parseOptLine(lines[j])
			if !ok {
				continue
			}
			if pp.op == "POP" {
				if len(pp.args) >= 1 && strings.TrimRight(pp.args[0], ",") == reg {
					remove[i], remove[j] = true, true
				}
				break
			}
			// Reading the saved value, changing the stack pointer, or
			// touching the register makes the pair unsafe.
			if instrTouchesReg(pp.op, pp.args, reg) || usesStackPointer(lines[j]) {
				break
			}
			if pp.op == "CALL" || pp.op == "SYSCALL" || pp.op == "INT" {
				break
			}
		}
	}

	var result []string
	for i, line := range lines {
		if !remove[i] {
			result = append(result, line)
		}
	}
	return result
}

func usesStackPointer(line string) bool {
	for _, tok := range asm.OperandRegs(line) {
		switch strings.ToLower(tok) {
		case "rsp", "esp", "sp", "rbp", "ebp", "bp":
			return true
		}
	}
	return false
}

func instrTouchesReg(op string, args []string, reg string) bool {
	for _, a := range args {
		if containsRegToken(a, reg) {
			return true
		}
	}
	return false
}

func containsRegToken(s, reg string) bool {
	want := strings.ToLower(reg)
	for _, tok := range asm.OperandRegs(s) {
		if strings.ToLower(tok) == want {
			return true
		}
	}
	return false
}

func scanPushModPop(i int, reg string, lines []string, remove []bool) (bool, int, int) {
	nextCode := func(from int) int {
		for j := from; j < len(lines); j++ {
			if !remove[j] && asm.TrimCode(lines[j]) != "" {
				return j
			}
		}
		return -1
	}
	midIdx := nextCode(i + 1)
	if midIdx < 0 {
		return false, 0, 0
	}
	mid, ok := parseOptLine(lines[midIdx])
	if !ok || len(mid.args) < 2 || strings.TrimRight(mid.args[0], ",") != reg {
		return false, 0, 0
	}
	if dstReg(mid.op, mid.args) < 0 {
		return false, 0, 0
	}
	switch mid.op {
	case "MOV", "MOVI", "LOAD", "LEA":
		// These do not set FLAGS. STORE writes memory and is not a
		// register-defining instruction for this pattern.
	default:
		return false, 0, 0
	}
	popIdx := nextCode(midIdx + 1)
	if popIdx < 0 {
		return false, 0, 0
	}
	pop, ok := parseOptLine(lines[popIdx])
	if !ok || pop.op != "POP" || len(pop.args) < 1 ||
		strings.TrimRight(pop.args[0], ",") != reg {
		return false, 0, 0
	}
	return true, midIdx, popIdx
}

// ---------------------------------------------------------------------------
// Tail-call optimization
// ---------------------------------------------------------------------------

func tailCallOpt(lines []string) []string {
	result := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if i+1 < len(lines) {
			p, ok := parseOptLine(lines[i])
			if ok && p.op == "CALL" {
				next, ok := parseOptLine(lines[i+1])
				if ok && next.op == "RET" && safeTailCallContext(lines, i) {
					target := p.args[0]
					result = append(result, fmt.Sprintf("\tJMP\t%s%s", target, commentSuffix(lines[i])))
					i += 2
					continue
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// safeTailCallContext rejects tail calls in functions that adjust the stack.
func safeTailCallContext(lines []string, callIdx int) bool {
	for j := callIdx - 1; j >= 0; j-- {
		p, ok := parseOptLine(lines[j])
		if !ok {
			if strings.HasSuffix(asm.TrimCode(lines[j]), ":") {
				break
			}
			continue
		}
		op := strings.ToLower(p.op)
		switch op {
		case "push", "pop", "enter", "leave", "ret":
			return false
		case "add", "sub":
			if usesStackPointer(lines[j]) {
				return false
			}
		}
	}
	return true
}

var _ = strconv.Itoa
