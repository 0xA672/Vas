// Package opt implements the -O1 optimization passes for VAS.
//
// Optimizations are divided into two categories:
//
// Pre-expansion (operates on VAS source before instruction expansion):
//   - ConstantFolding: compute ADD/SUB v1, imm, imm at assembly time
//   - DeadCodeElim: remove writes to v-regs that are overwritten before being read
//
// Post-expansion (peephole optimization on generated assembly text):
//   - XorZero:  mov reg, 0  =>  xor reg, reg  (smaller encoding, zeroes flags)
//   - TestCmp:  cmp reg, 0  =>  test reg, reg  (smaller encoding)
//   - NopMerge: consecutive NOP lines => longer efficient NOP
//   - LeaFuse:  mov r1, r2; add r1, r3  =>  lea r1, [r2+r3]
package opt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Optimize runs all enabled optimization passes on the assembled output.
// level 0 = no optimization, level >=1 = -O1.
func Optimize(input string, level int) string {
	if level <= 0 {
		return input
	}

	lines := strings.Split(input, "\n")
	lines = copyPropagate(lines)
	lines = strengthReduce(lines)
	lines = storeLoadFwd(lines)
	lines = deadCodeElim(lines)
	lines = peephole(lines)
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------------
// Pre-expansion: constant folding
// ---------------------------------------------------------------------------

// FoldConstants scans VAS source lines and folds arithmetic on immediates.
// e.g. "ADD v1, 1, 2" => "MOVI v1, 3"
func FoldConstants(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, foldLine(line))
	}
	return result
}

// foldLine attempts to fold a single VAS line.
func foldLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Strip inline comment for parsing
	code := trimmed
	if idx := strings.IndexAny(code, ";#"); idx >= 0 {
		code = strings.TrimSpace(code[:idx])
	}
	if code == "" {
		return line
	}

	tokens := tokenizeFold(code)
	if len(tokens) < 4 {
		return line
	}

	op := strings.ToUpper(tokens[0])
	dst := tokens[1]

	// 3-operand ADD/SUB with two immediate operands: dst = imm op imm
	if len(tokens) == 4 {
		src1, err1 := strconv.ParseInt(tokens[2], 0, 64)
		src2, err2 := strconv.ParseInt(tokens[3], 0, 64)
		if err1 == nil && err2 == nil {
			var val int64
			switch op {
			case "ADD":
				val = src1 + src2
			case "SUB":
				val = src1 - src2
			case "MUL":
				val = src1 * src2
			default:
				return line
			}
			// Preserve comment
			comment := ""
			if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
				comment = trimmed[idx:]
			}
			return fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
		}
	}

	return line
}

func tokenizeFold(line string) []string {
	var tokens []string
	var cur strings.Builder
	for _, ch := range line {
		if ch == ',' || ch == '\t' || ch == ' ' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// ---------------------------------------------------------------------------
// Pre-expansion: dead code elimination
// ---------------------------------------------------------------------------

// deadCodeElim removes writes to virtual registers that are immediately
// overwritten within the same basic block before being read.
//
// This is a forward scan: for each register write, if the same register
// was written before without being read in between, the earlier write
// is removed. Writes at the end of a block are always kept (conservative).
func deadCodeElim(lines []string) []string {
	// Group lines into basic blocks (split at labels, jumps, calls, ret, syscall)
	blocks := splitBlocks(lines)

	var result []string
	for _, block := range blocks {
		result = append(result, elimBlock(block)...)
	}
	return result
}

// splitBlocks splits lines at labels and control-flow instructions.
func splitBlocks(lines []string) [][]string {
	var blocks [][]string
	var current []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			current = append(current, line)
			continue
		}
		// Strip comment for detection
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			current = append(current, line)
			continue
		}
		// Labels and control flow break the block
		isLabel := strings.HasSuffix(code, ":") && !isInstructionFold(code)
		isControlFlow := false
		if !isLabel {
			fields := strings.Fields(code)
			if len(fields) == 0 {
				current = append(current, line)
				continue
			}
			upper := strings.ToUpper(fields[0])
			switch upper {
			case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE",
				"CALL", "RET", "SYSCALL", "INT":
				isControlFlow = true
			}
		}
		if isLabel || isControlFlow {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			current = append(current, line)
			if isControlFlow {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

func isInstructionFold(s string) bool {
	upper := strings.ToUpper(strings.Fields(s)[0])
	switch upper {
	case "ADD", "SUB", "MUL", "LOAD", "STORE", "LEA", "MOV", "MOVI",
		"CMP", "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE",
		"CALL", "RET", "NOP", "PUSH", "POP", "INT", "SYSCALL",
		"SECTION", "GLOBAL", "EXTERN", "DATA", "TEXT", "BSS",
		"ALIGN", "BYTE", "WORD", "DWORD", "QWORD", "DD", "DQ", "DB",
		"TYPE", "SIZE", "LENGTH", "START":
		return true
	}
	return false
}

// hasSideEffect reports whether an instruction has observable side effects
// beyond writing its destination register (e.g., stack or memory operations).
// Such instructions can never be removed by dead code elimination.
func hasSideEffect(op string) bool {
	switch op {
	case "POP", "PUSH", "CALL", "STORE", "INT", "SYSCALL", "RET":
		return true
	}
	return false
}

func elimBlock(lines []string) []string {
	// Last write position for each virtual register
	lastWrite := map[int]int{} // reg index => line index in this block
	// Mark lines for removal
	remove := make([]bool, len(lines))

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}

		tokens := tokenizeFold(code)
		if len(tokens) < 2 {
			continue
		}

		op := strings.ToUpper(tokens[0])
		args := tokens[1:]

		// Instructions with side effects (POP, PUSH, CALL, STORE, etc.)
		// can never be removed, even if their destination register is unused.
		if hasSideEffect(op) {
			// Mark all read registers as fresh (can't remove prior writes to them)
			for _, r := range readRegs(op, args) {
				delete(lastWrite, r)
			}
			// Don't track any destination register — side-effect instructions
			// must always be preserved so they can't be used to justify removals.
			continue
		}

		// Determine which v-regs are read by this instruction
		reads := readRegs(op, args)
		// Mark all read registers as "fresh" (can't remove writes to them anymore)
		for _, r := range reads {
			delete(lastWrite, r)
		}

		dst := dstReg(op, args)
		if dst >= 0 {
			if prev, exists := lastWrite[dst]; exists {
				// Same register was written before without being read => remove previous
				remove[prev] = true
			}
			lastWrite[dst] = i
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

// dstReg returns the virtual register index written by this instruction, or -1.
func dstReg(op string, args []string) int {
	if len(args) == 0 {
		return -1
	}
	switch op {
	case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
		return regIndex(args[0])
	}
	return -1
}

// readRegs returns the virtual register indices read by this instruction.
func readRegs(op string, args []string) []int {
	var regs []int
	switch op {
	case "MOV":
		if len(args) >= 2 {
			if r := regIndex(args[1]); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "ADD", "SUB", "MUL":
		if len(args) >= 2 {
			// 2-operand form also reads destination (x86: add dst, src => dst = dst + src)
			if r := regIndex(args[0]); r >= 0 {
				regs = append(regs, r)
			}
			if r := regIndex(args[1]); r >= 0 {
				regs = append(regs, r)
			}
			if len(args) >= 3 {
				if r := regIndex(args[2]); r >= 0 {
					regs = append(regs, r)
				}
			}
		}
	case "LOAD", "LEA":
		if len(args) >= 2 {
			if r := regIndex(args[1]); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "STORE":
		if len(args) >= 2 {
			if r := regIndex(args[0]); r >= 0 {
				regs = append(regs, r)
			}
			if r := regIndex(args[1]); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "CMP":
		for _, a := range args {
			if r := regIndex(a); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "PUSH":
		if len(args) >= 1 {
			if r := regIndex(args[0]); r >= 0 {
				regs = append(regs, r)
			}
		}
	}
	return regs
}

// ---------------------------------------------------------------------------
// Pre-expansion: copy propagation (MOV vX, vY => use vY instead of vX)
// ---------------------------------------------------------------------------

// copyPropagate replaces references to copy-destination registers with their
// source register within a basic block. After propagation, dead MOVs can be
// eliminated by the subsequent DCE pass.
func copyPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, propagateBlock(block)...)
	}
	return result
}

// propagateBlock performs copy propagation within a single basic block.
func propagateBlock(lines []string) []string {
	// alias[v] = the v-reg index that v is an alias for (-1 = no alias)
	alias := make([]int, 13) // v0-v12
	for i := range alias {
		alias[i] = -1
	}

	// resolve follows the alias chain transitively.
	resolve := func(ri int) int {
		for ri >= 0 && alias[ri] >= 0 {
			ri = alias[ri]
		}
		return ri
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		code := strings.TrimSpace(line)
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result[i] = line
			continue
		}

		fields := strings.Fields(code)
		if len(fields) == 0 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(fields[0])
		args := fields[1:]

		// Only process instructions with at least one v-reg argument
		hasVReg := false
		for _, a := range args {
			if regIndex(a) >= 0 {
				hasVReg = true
				break
			}
		}
		if !hasVReg {
			result[i] = line
			continue
		}

		dst := dstReg(op, args)

		// Step 1: replace source operands with their propagated alias
		propagated := make([]string, len(args))
		for j, a := range args {
			ri := regIndex(a)
			resolved := resolve(ri)
			if resolved >= 0 && resolved != ri {
				comma := ""
				if strings.HasSuffix(a, ",") {
					comma = ","
				}
				propagated[j] = fmt.Sprintf("v%d%s", resolved, comma)
			} else {
				propagated[j] = a
			}
		}

		// Rebuild the line with propagated args
		newLine := fmt.Sprintf("\t%s\t%s", op, strings.Join(propagated, " "))
		if idx := strings.IndexAny(line, ";#"); idx >= 0 {
			newLine += line[idx:]
		}

		// Step 2: update alias map (resolve source transitively)
		if op == "MOV" && dst >= 0 && len(args) >= 2 {
			srcRi := resolve(regIndex(args[1]))
			if srcRi >= 0 {
				alias[dst] = srcRi
			}
		} else if dst >= 0 {
			alias[dst] = -1
		}

		result[i] = newLine
	}
	return result
}

// ---------------------------------------------------------------------------
// Pre-expansion: strength reduction (MUL by power-of-2 -> SHL)
// ---------------------------------------------------------------------------

// strengthReduce replaces MUL by a power-of-2 constant with a left shift.
func strengthReduce(lines []string) []string {
	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = reduceLine(line)
	}
	return result
}

func reduceLine(line string) string {
	trimmed := strings.TrimSpace(line)
	code := trimmed
	if idx := strings.IndexAny(code, ";#"); idx >= 0 {
		code = strings.TrimSpace(code[:idx])
	}
	if code == "" {
		return line
	}
	fields := strings.Fields(code)
	if len(fields) < 2 {
		return line
	}
	op := strings.ToUpper(fields[0])
	if op != "MUL" {
		return line
	}

	// arg helper: strip trailing comma
	arg := func(i int) string {
		s := fields[i]
		s = strings.TrimRight(s, ",")
		return s
	}

	if len(fields) == 3 {
		// 2-op: MUL dst, imm
		dst := arg(1)
		imm, err := strconv.ParseInt(arg(2), 0, 64)
		if err == nil && isPowerOf2(imm) && imm > 0 && imm <= 0x80000000 {
			shift := log2(imm)
			comment := ""
			if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
				comment = " " + trimmed[idx:]
			}
			return fmt.Sprintf("\tshl\t%s, %d%s", dst, shift, comment)
		}
	} else if len(fields) == 4 {
		// 3-op: MUL dst, src, imm  (or MUL dst, src, reg)
		dst := arg(1)
		src := arg(2)
		imm, err := strconv.ParseInt(arg(3), 0, 64)
		if err == nil && isPowerOf2(imm) && imm > 0 && imm <= 0x80000000 {
			shift := log2(imm)
			comment := ""
			if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
				comment = " " + trimmed[idx:]
			}
			// MOV dst, src; SHL dst, shift
			return fmt.Sprintf("\tMOV\t%s, %s%s\n\tshl\t%s, %d%s", dst, src, comment, dst, shift, comment)
		}
	}
	return line
}

func isPowerOf2(n int64) bool {
	return n > 0 && (n&(n-1)) == 0
}

func log2(n int64) int {
	r := 0
	for n > 1 {
		n >>= 1
		r++
	}
	return r
}

// ---------------------------------------------------------------------------
// Pre-expansion: STORE-LOAD forwarding
// ---------------------------------------------------------------------------

// storeLoadFwd replaces LOAD from a label with MOV from the last STORE to
// that label within the same basic block.
func storeLoadFwd(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, fwdBlock(block)...)
	}
	return result
}

// fwdBlock performs STORE-LOAD forwarding within a single basic block.
func fwdBlock(lines []string) []string {
	// lastStore[labelName] = v-reg index that was stored
	lastStore := map[string]int{}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			lastStore = map[string]int{} // clear on empty line
			result[i] = line
			continue
		}

		fields := strings.Fields(code)
		if len(fields) < 2 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(fields[0])
		args := fields[1:]

		switch op {
		case "STORE":
			if len(args) >= 2 {
				src := strings.TrimRight(args[0], ",")
				srcRi := regIndex(src)
				if srcRi >= 0 {
					label := extractLabel(args[1])
					if label != "" {
						lastStore[label] = srcRi
					}
				}
			}
		case "LOAD":
			if len(args) >= 2 {
				dst := strings.TrimRight(args[0], ",")
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					label := extractLabel(args[1])
					if label != "" {
						if srcRi, ok := lastStore[label]; ok {
							// Forward: replace LOAD with MOV
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOV\t%s, v%d%s", dst, srcRi, comment)
							continue
						}
					}
				}
			}
		default:
			// Any non-STORE/LOAD instruction that could modify memory
			// clears the store map for safety.
			if len(fields) >= 2 {
				firstArg := strings.TrimRight(args[0], ",")
				if strings.HasPrefix(firstArg, "[") {
					// Writing to memory (e.g., passthrough). Clear all.
					lastStore = map[string]int{}
				}
			}
		}

		result[i] = line
	}
	return result
}

// extractLabel extracts a label name from a memory operand like "[label]"
// or "[label+8]". Returns "" if no simple label is found.
func extractLabel(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return ""
	}
	s = s[1:] // strip '['
	if idx := strings.IndexAny(s, "+-*/]"); idx >= 0 {
		before := strings.TrimSpace(s[:idx])
		// Check if it's a v-reg or number
		if regIndex(before) >= 0 {
			return ""
		}
		if _, err := strconv.ParseInt(before, 0, 64); err == nil {
			return ""
		}
		return before
	}
	// Simple case: [label]
	s = strings.TrimRight(s, "]")
	s = strings.TrimSpace(s)
	if regIndex(s) >= 0 {
		return ""
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return ""
	}
	return s
}

// ---------------------------------------------------------------------------
// Post-expansion: peephole optimizations on asm output
// ---------------------------------------------------------------------------
// regIndex returns the v-register index (0-12) or -1 if not a v-reg.
func regIndex(s string) int {
	s = strings.TrimRight(s, ",")
	if len(s) >= 2 && s[0] == 'v' {
		rest := s[1:]
		if len(rest) == 0 {
			return -1
		}
		// Single digit: v0-v9
		if len(rest) == 1 && rest[0] >= '0' && rest[0] <= '9' {
			return int(rest[0] - '0')
		}
		// Two digits: v10, v11, v12
		if len(rest) == 2 && rest[0] == '1' && rest[1] >= '0' && rest[1] <= '2' {
			return 10 + int(rest[1]-'0')
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Post-expansion: peephole optimizations on asm output
// ---------------------------------------------------------------------------

func peephole(lines []string) []string {
	lines = xorZero(lines)
	lines = testCmp(lines)
	lines = nopMerge(lines)
	lines = leaFuse(lines)
	return lines
}

// xorZero replaces "mov <reg>, 0" with "xor <reg>, <reg>".
func xorZero(lines []string) []string {
	// Match: mov\tr(a-z0-9)+,\s*0
	re := regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*0$`)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := re.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			// Use 32-bit register (eax, ebx, etc.) for smaller encoding
			r32 := regTo32(reg)
			result = append(result, fmt.Sprintf("\txor\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

// testCmp replaces "cmp <reg>, 0" with "test <reg>, <reg>".
func testCmp(lines []string) []string {
	re := regexp.MustCompile(`^\tcmp\t([a-z][a-z0-9]+),\s*0$`)
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := re.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			r32 := regTo32(reg)
			result = append(result, fmt.Sprintf("\ttest\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

// nopMerge merges consecutive NOP lines into one (with comment showing count).
// NASM handles single-byte NOPs; multi-byte NOP encoding is deferred to the
// assembler for now. A future arch-specific pass can emit db sequences.
func nopMerge(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "nop" {
			count := 1
			for i+count < len(lines) && strings.TrimSpace(lines[i+count]) == "nop" {
				count++
			}
			if count > 1 {
				result = append(result, "\tnop\t; merged "+strconv.Itoa(count)+" nops")
			} else {
				result = append(result, lines[i])
			}
			i += count
		} else {
			result = append(result, lines[i])
			i++
		}
	}
	return result
}

// leaFuse replaces "mov r1, r2; add r1, r3" with "lea r1, [r2+r3]"
// when r2 is not used between the two instructions.
func leaFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			fused, ok := tryLeaFuse(lines[i], lines[i+1])
			if ok {
				result = append(result, fused)
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// movRe matches: mov\tr1,\s*r2
var movRe = regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)

// addRe matches: add\tr1,\s*r3
var addRe = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)

func tryLeaFuse(line1, line2 string) (string, bool) {
	m1 := movRe.FindStringSubmatch(strings.TrimRight(line1, " \t\r"))
	if m1 == nil {
		return "", false
	}
	dst, src1 := m1[1], m1[2]

	m2 := addRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r"))
	if m2 == nil {
		return "", false
	}
	addDst, src2 := m2[1], m2[2]

	if addDst != dst {
		return "", false
	}

	// lea dst, [src1+src2]
	return fmt.Sprintf("\tlea\t%s, [%s+%s]", dst, src1, src2), true
}

// regTo32 converts a 64-bit register name to 32-bit (for XOR/TEST).
func regTo32(reg string) string {
	m := map[string]string{
		"rax": "eax", "rbx": "ebx", "rcx": "ecx", "rdx": "edx",
		"rsi": "esi", "rdi": "edi", "rbp": "ebp", "rsp": "esp",
		"r8": "r8d", "r9": "r9d", "r10": "r10d", "r11": "r11d",
		"r12": "r12d", "r13": "r13d", "r14": "r14d", "r15": "r15d",
	}
	if r, ok := m[reg]; ok {
		return r
	}
	return reg
}
