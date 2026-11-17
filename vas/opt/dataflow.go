package opt

import (
	"fmt"
	"strconv"
	"strings"

	"vas/vas/internal/asm"
)

// ---------------------------------------------------------------------------
// Copy propagation
// ---------------------------------------------------------------------------

func copyPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, propagateBlock(block)...)
	}
	return result
}

func propagateBlock(lines []string) []string {
	alias := [13]int{}
	for i := range alias {
		alias[i] = -1
	}
	resolve := func(ri int) int {
		if ri < 0 {
			return -1
		}
		for steps := 0; steps < len(alias) && ri >= 0 && alias[ri] >= 0; steps++ {
			ri = alias[ri]
		}
		return ri
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			result[i] = line
			continue
		}
		if touchesPhysicalReg(line) {
			for j := range alias {
				alias[j] = -1
			}
			result[i] = line
			continue
		}
		op := p.op
		args := p.args
		if !knownDstOp(op) && op != "CMP" {
			for j := range alias {
				alias[j] = -1
			}
			result[i] = line
			continue
		}

		dst := dstReg(op, args)
		propagated := make([]string, len(args))
		for j, a := range args {
			if j == 0 && dst >= 0 {
				propagated[j] = a
				continue
			}
			propagated[j] = replaceVirtualTokens(a, resolve)
		}
		if dst >= 0 {
			for j := range alias {
				if alias[j] == dst {
					alias[j] = -1
				}
			}
		}
		if op == "MOV" && dst >= 0 && len(args) >= 2 {
			srcRi := regIndex(args[1])
			if srcRi >= 0 {
				srcRi = resolve(srcRi)
				if srcRi >= 0 && srcRi != dst {
					alias[dst] = srcRi
				}
			}
		} else if dst >= 0 {
			alias[dst] = -1
		}
		result[i] = formatOptimizedLine(op, propagated, line)
	}
	return result
}

func replaceVirtualTokens(s string, resolve func(int) int) string {
	if !strings.Contains(s, "v") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 'v' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			name := s[i:j]
			beforeOK := i == 0 || !isIdentChar(s[i-1])
			afterOK := j == len(s) || !isIdentChar(s[j])
			if beforeOK && afterOK {
				if n := asm.VirtualRegNum(name); n >= 0 {
					if r := resolve(n); r >= 0 && r != n {
						fmt.Fprintf(&b, "v%d", r)
						i = j
						continue
					}
				}
			}
			b.WriteString(name)
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func commentSuffix(raw string) string {
	if idx := strings.IndexAny(raw, ";#"); idx >= 0 {
		return " " + raw[idx:]
	}
	return ""
}

func formatOptimizedLine(op string, args []string, raw string) string {
	return "\t" + strings.ToUpper(op) + "\t" + strings.Join(args, ", ") + commentSuffix(raw)
}

// ---------------------------------------------------------------------------
// Constant propagation and folding
// ---------------------------------------------------------------------------

func constPropagate(lines []string) []string {
	return constBlock(lines)
}

func constBlock(lines []string) []string {
	var vals [13]int64
	var known [13]bool

	argValue := func(a string) (int64, bool) {
		if ri := regIndex(a); ri >= 0 {
			return vals[ri], known[ri]
		}
		return parseNASMInt(strings.TrimRight(a, ","))
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			if strings.HasSuffix(strings.TrimSpace(line), ":") {
				known = [13]bool{}
			}
			result[i] = line
			continue
		}
		if touchesPhysicalReg(line) {
			known = [13]bool{}
			result[i] = line
			continue
		}
		op := p.op
		args := p.args
		if asm.IsBranch(op) {
			// A branch ends the local dataflow region. Process earlier
			// instructions normally, then discard constant knowledge.
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				known = [13]bool{}
			}
			result[i] = line
			known = [13]bool{}
			continue
		}

		switch op {
		case "MOVI":
			if len(args) >= 2 {
				if ri := regIndex(args[0]); ri >= 0 {
					if v, ok := parseNASMInt(args[1]); ok {
						vals[ri], known[ri] = v, true
					} else {
						// MOVI v0, symbol: previous numeric value is dead.
						known[ri] = false
					}
				}
			}
			result[i] = line
			continue

		case "MOV":
			if len(args) >= 2 {
				if ri := regIndex(args[0]); ri >= 0 {
					if src := regIndex(args[1]); src >= 0 && known[src] {
						vals[ri], known[ri] = vals[src], true
						comment := ""
						if idx := strings.IndexAny(line, ";#"); idx >= 0 {
							comment = " " + line[idx:]
						}
						result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", args[0], vals[src], comment)
						continue
					}
					known[ri] = false
				}
			}
			result[i] = line
			continue

		case "ADD", "SUB", "MUL":
			var dst int
			var v int64
			folded := false
			if len(args) == 2 {
				dst = regIndex(args[0])
				if dst >= 0 && known[dst] {
					if imm, ok := argValue(args[1]); ok {
						v = foldArith(op, vals[dst], imm)
						folded = true
					}
				}
			} else if len(args) >= 3 {
				dst = regIndex(args[0])
				v1, ok1 := argValue(args[1])
				v2, ok2 := argValue(args[2])
				if dst >= 0 && ok1 && ok2 {
					v = foldArith(op, v1, v2)
					folded = true
				}
			}
			if dst >= 0 {
				if folded && !flagsLiveAfter(lines, i) {
					comment := ""
					if idx := strings.IndexAny(line, ";#"); idx >= 0 {
						comment = " " + line[idx:]
					}
					result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", args[0], v, comment)
					vals[dst], known[dst] = v, true
					continue
				}
				known[dst] = false
			}
			result[i] = line
			continue

		case "CALL", "SYSCALL", "INT":
			known = [13]bool{}
			result[i] = line
			continue
		}

		if dst := dstReg(op, args); dst >= 0 {
			known[dst] = false
		}
		result[i] = line
	}
	return result
}

func foldArith(op string, a, b int64) int64 {
	switch op {
	case "ADD":
		return a + b
	case "SUB":
		return a - b
	case "MUL":
		return a * b
	}
	return 0
}

// ---------------------------------------------------------------------------
// Dead store elimination
// ---------------------------------------------------------------------------

func deadStoreElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, elimDeadStoreBlock(block)...)
	}
	return result
}

func elimDeadStoreBlock(lines []string) []string {
	// key -> "STORE" or "LOAD" seen at a later point in the block.
	lastAccess := map[string]string{}
	remove := make([]bool, len(lines))

	clear := func() { lastAccess = map[string]string{} }

	for i := len(lines) - 1; i >= 0; i-- {
		p, ok := parseOptLine(lines[i])
		if !ok {
			continue
		}
		op := p.op
		args := p.args
		if op == "CALL" || op == "SYSCALL" || op == "INT" || touchesPhysicalReg(lines[i]) {
			clear()
			continue
		}
		if op != "STORE" && op != "LOAD" {
			continue
		}
		if len(args) < 2 {
			continue
		}
		if !isStaticMemory(args[1]) {
			// A register-based address may alias any static symbol.
			clear()
			continue
		}
		key, ok := memKey(args[1])
		if !ok {
			continue
		}
		prev := lastAccess[key]
		if op == "STORE" && prev == "STORE" {
			remove[i] = true
		}
		lastAccess[key] = op
	}

	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if !remove[i] {
			result = append(result, line)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Strength reduction
// ---------------------------------------------------------------------------

func strengthReduce(lines []string) []string {
	var result []string
	for i, line := range lines {
		if writesFlags(line) && flagsLiveAfter(lines, i) {
			result = append(result, line)
			continue
		}
		reduced := reduceLine(line)
		if strings.Contains(reduced, "\n") {
			result = append(result, strings.Split(reduced, "\n")...)
		} else {
			result = append(result, reduced)
		}
	}
	return result
}

func reduceLine(line string) string {
	p, ok := parseOptLine(line)
	if !ok || p.op != "MUL" {
		return line
	}
	args := p.args
	comment := commentSuffix(line)
	if len(args) == 2 {
		imm, ok := parseNASMInt(args[1])
		if !ok || imm <= 0 {
			return line
		}
		if result := decomposeMul2Op(args[0], imm, comment); result != "" {
			return result
		}
		return line
	}
	if len(args) >= 3 {
		imm, ok := parseNASMInt(args[2])
		if !ok || imm <= 0 {
			return line
		}
		if result := decomposeMul3Op(args[0], args[1], imm, comment); result != "" {
			return result
		}
	}
	return line
}

func decomposeMul2Op(dst string, c int64, comment string) string {
	if isPowerOf2(c) && c <= 0x80000000 {
		return fmt.Sprintf("\tshl\t%s, %d%s", dst, log2(c), comment)
	}
	switch c - 1 {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, dst, dst, c-1, comment)
	}
	shift := int64(0)
	odd := c
	for odd%2 == 0 {
		odd /= 2
		shift++
	}
	if shift > 0 {
		switch odd - 1 {
		case 1, 2, 4, 8:
			return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, dst, dst, odd-1, comment, dst, shift, comment)
		}
	}
	return ""
}

func decomposeMul3Op(dst, src string, c int64, comment string) string {
	if isPowerOf2(c) && c <= 0x80000000 {
		return fmt.Sprintf("\tMOV\t%s, %s%s\n\tshl\t%s, %d%s",
			dst, src, comment, dst, log2(c), comment)
	}
	switch c - 1 {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, src, src, c-1, comment)
	}
	shift := int64(0)
	odd := c
	for odd%2 == 0 {
		odd /= 2
		shift++
	}
	if shift > 0 {
		switch odd - 1 {
		case 1, 2, 4, 8:
			return fmt.Sprintf("\tMOV\t%s, %s%s\n\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, src, comment, dst, dst, dst, odd-1, comment, dst, shift, comment)
		}
	}
	for k := int64(2); k <= 3; k++ {
		if c == (1<<k)-1 {
			return fmt.Sprintf("\tLEA\t%s, [%s*%d]%s\n\tSUB\t%s, %s%s",
				dst, src, 1<<k, comment, dst, src, comment)
		}
	}
	return ""
}

func isPowerOf2(n int64) bool { return n > 0 && (n&(n-1)) == 0 }

func log2(n int64) int {
	r := 0
	for n > 1 {
		n >>= 1
		r++
	}
	return r
}

// ---------------------------------------------------------------------------
// STORE -> LOAD forwarding
// ---------------------------------------------------------------------------

func storeLoadFwd(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, fwdBlock(block)...)
	}
	return result
}

type forwardedStore struct {
	reg     int
	regVer  int
	addrKey string
}

func fwdBlock(lines []string) []string {
	var versions [13]int
	stores := map[string]forwardedStore{}

	clear := func() { stores = map[string]forwardedStore{} }

	result := make([]string, len(lines))
	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			result[i] = line
			continue
		}
		op := p.op
		args := p.args
		if touchesPhysicalReg(line) || isUnknownOrClobber(p) {
			clear()
			result[i] = line
			continue
		}

		// Any register write kills forwarding whose source register it is,
		// and changes the address-version fingerprint.
		dst := dstReg(op, args)
		if dst >= 0 {
			delete(stores, "dst:"+strconv.Itoa(dst))
			for k, st := range stores {
				if st.reg == dst {
					delete(stores, k)
				}
			}
		}

		switch op {
		case "STORE":
			if len(args) >= 2 {
				if !isStaticMemory(args[1]) || addressHasPhysical(args[1]) {
					clear()
					break
				}
				src := regIndex(args[0])
				key, ok := memKey(args[1])
				if !ok {
					clear()
					break
				}
				full := key + addressVersions(args[1], &versions)
				if src >= 0 {
					stores[full] = forwardedStore{reg: src, regVer: versions[src], addrKey: key}
				} else {
					delete(stores, full)
				}
			}
		case "LOAD":
			if len(args) >= 2 {
				if !isStaticMemory(args[1]) || addressHasPhysical(args[1]) {
					clear()
					break
				}
				key, ok := memKey(args[1])
				if !ok {
					break
				}
				full := key + addressVersions(args[1], &versions)
				if st, ok := stores[full]; ok && st.reg != dst && versions[st.reg] == st.regVer {
					result[i] = fmt.Sprintf("\tMOV\t%s, v%d%s", args[0], st.reg, commentSuffix(line))
					continue
				}
			}
		}

		if dst >= 0 {
			versions[dst]++
		}
		result[i] = line
	}
	return result
}
