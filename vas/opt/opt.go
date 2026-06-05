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
	lines = constPropagate(lines)
	lines = strengthReduce(lines)
	lines = storeLoadFwd(lines)
	lines = deadStoreElim(lines)
	lines = deadCodeElim(lines)

	// Peephole runs post-expansion too, so keep it last.
	lines = peephole(lines)

	// -O2: more aggressive optimizations
	if level >= 2 {
		lines = cse(lines)
		lines = licm(lines)
		lines = redundantLoadElim(lines)
		lines = pushPopElim(lines)
		lines = tailCallOpt(lines)
	}

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
			a := trimBrackets(args[1])
			if r := regIndex(a); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "STORE":
		if len(args) >= 2 {
			if r := regIndex(args[0]); r >= 0 {
				regs = append(regs, r)
			}
			a := trimBrackets(args[1])
			if r := regIndex(a); r >= 0 {
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
	case "SYSCALL":
		// Linux x86-64 syscall ABI: rax(v0)=number, rdi(v5)=arg1, rsi(v4)=arg2, rdx(v3)=arg3, r10(v8)=arg4, r8(v6)=arg5, r9(v7)=arg6
		regs = append(regs, 0, 3, 4, 5, 6, 7, 8)
	case "INT":
		// int 0x80 uses the same ABI regs in 32-bit mode
		regs = append(regs, 0, 3, 4, 5, 6, 7, 8)
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
		// Don't propagate the destination register (args[0]) — doing so
		// would make the instruction write to the aliased register instead,
		// potentially clobbering a live value (e.g., loop counter).
		propagated := make([]string, len(args))
		for j, a := range args {
			// Skip destination register
			if j == 0 && dst >= 0 {
				propagated[j] = a
				continue
			}
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
// Pre-expansion: constant propagation (MOVI vX, imm -> fold subsequent uses)
// ---------------------------------------------------------------------------

// constPropagate tracks MOVI assignments and folds known-constant register
// references into immediate operands within each basic block.
func constPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, constBlock(block)...)
	}
	return result
}

func constBlock(lines []string) []string {
	// const[v] = known constant value (false = unknown)
	constVal := make([]int64, 13)
	constKnown := make([]bool, 13)
	// used[reg] tracks registers whose value is read after the last MOVI to them
	used := map[int]bool{}
	// moviLine[reg] = line index of the last MOVI to this register in this block
	moviLine := map[int]int{}

	// parseArg extracts the integer value from an argument token.
	parseArg := func(a string) (int64, bool) {
		a = strings.TrimRight(a, ",")
		ri := regIndex(a)
		if ri >= 0 && constKnown[ri] {
			return constVal[ri], true
		}
		n, err := strconv.ParseInt(a, 0, 64)
		if err == nil {
			return n, true
		}
		return 0, false
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result[i] = line
			continue
		}

		tokens := tokenizeFold(code)
		if len(tokens) < 2 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(tokens[0])
		args := tokens[1:]

		reads := readRegs(op, args)
		folded := false

		switch op {
		case "MOVI":
			if len(args) >= 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				imm, err := strconv.ParseInt(args[1], 0, 64)
				if dstRi >= 0 && err == nil {
					constVal[dstRi] = imm
					constKnown[dstRi] = true
					moviLine[dstRi] = i
					delete(used, dstRi)
				}
			}
			reads = nil
		case "ADD", "SUB":
			if len(args) == 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					if constKnown[dstRi] {
						if imm, ok := parseArg(args[1]); ok {
							var val int64
							switch op {
							case "ADD":
								val = constVal[dstRi] + imm
							case "SUB":
								val = constVal[dstRi] - imm
							}
							constVal[dstRi] = val
							constKnown[dstRi] = true
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
							reads = nil
							folded = true
							continue
						}
					}
					constKnown[dstRi] = false
				}
			} else if len(args) == 3 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi < 0 {
					result[i] = line
					continue
				}
				v1, ok1 := parseArg(args[1])
				v2, ok2 := parseArg(args[2])
				if ok1 && ok2 {
					var val int64
					switch op {
					case "ADD":
						val = v1 + v2
					case "SUB":
						val = v1 - v2
					}
					constVal[dstRi] = val
					constKnown[dstRi] = true
					comment := ""
					if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
						comment = " " + trimmed[idx:]
					}
					result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
					folded = true
					continue
				}
				constKnown[dstRi] = false
			}
		case "MUL":
			if len(args) == 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					if constKnown[dstRi] {
						if imm, ok := parseArg(args[1]); ok {
							val := constVal[dstRi] * imm
							constVal[dstRi] = val
							constKnown[dstRi] = true
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
							reads = nil
							folded = true
							continue
						}
					}
					constKnown[dstRi] = false
				}
			} else if len(args) == 3 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					v1, ok1 := parseArg(args[1])
					v2, ok2 := parseArg(args[2])
					if ok1 && ok2 {
						val := v1 * v2
						constVal[dstRi] = val
						constKnown[dstRi] = true
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
						folded = true
						continue
					}
					constKnown[dstRi] = false
				}
			}
		case "MOV":
			if len(args) >= 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					src := strings.TrimRight(args[1], ",")
					srcRi := regIndex(src)
					if srcRi >= 0 && constKnown[srcRi] {
						cp := constVal[srcRi]
						constVal[dstRi] = cp
						constKnown[dstRi] = true
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, cp, comment)
						reads = nil
						folded = true
						continue
					} else {
						constKnown[dstRi] = false
					}
				}
			}
		case "SYSCALL", "INT":
			for _, r := range []int{0, 3, 4, 5, 6, 7, 8} {
				constKnown[r] = false
			}
		default:
			dst := dstReg(op, args)
			if dst >= 0 {
				constKnown[dst] = false
			}
		}
		if !folded {
			for _, r := range reads {
				used[r] = true
			}
		}
		result[i] = line
	}
	return result
}

// deadStoreElim removes STORE instructions whose target label is stored again
// before any LOAD within the same basic block.
func deadStoreElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, elimDeadStoreBlock(block)...)
	}
	return result
}

func elimDeadStoreBlock(lines []string) []string {
	// Scan backward: track the last LOAD/STORE for each label
	// If a STORE's label is stored again before any LOAD, the first is dead.
	lastAccess := map[string]string{} // label -> "STORE" or "LOAD"
	remove := make([]bool, len(lines))

	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}
		fields := strings.Fields(code)
		if len(fields) < 2 {
			continue
		}
		op := strings.ToUpper(fields[0])
		if op != "STORE" && op != "LOAD" {
			// Calls, syscalls, and interrupts may write memory — clear all access state
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				lastAccess = map[string]string{}
			}
			continue
		}
		args := fields[1:]
		if len(args) < 2 {
			continue
		}
		label := extractLabel(args[1])
		if label == "" {
			continue
		}

		prev := lastAccess[label]
		if op == "STORE" && prev == "STORE" {
			// Another STORE follows without an intervening LOAD -> this one is dead
			remove[i] = true
		}
		lastAccess[label] = op
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
// Pre-expansion: strength reduction (MUL by power-of-2 -> SHL)
// ---------------------------------------------------------------------------
func strengthReduce(lines []string) []string {
	var result []string
	for _, line := range lines {
		reduced := reduceLine(line)
		// reduceLine may return multiple lines (e.g. 3-op MUL: "MOV dst, src\nSHL dst, shift")
		// Split them so each entry is a real line in the slice.
		if strings.Contains(reduced, "\n") {
			result = append(result, strings.Split(reduced, "\n")...)
		} else {
			result = append(result, reduced)
		}
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
		if err != nil || imm <= 0 {
			return line
		}
		if result := decomposeMul2Op(dst, imm, trimmed); result != "" {
			return result
		}
		return line
	} else if len(fields) == 4 {
		// 3-op: MUL dst, src, imm  (or MUL dst, src, reg)
		dst := arg(1)
		src := arg(2)
		imm, err := strconv.ParseInt(arg(3), 0, 64)
		if err != nil || imm <= 0 {
			return line
		}
		if result := decomposeMul3Op(dst, src, imm, trimmed); result != "" {
			return result
		}
		return line
	}
	return line
}

// decomposeMul2Op emits VAS-level instructions for dst *= C.
func decomposeMul2Op(dst string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	// 1. Power of 2: SHL
	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tshl\t%s, %d%s", dst, shift, comment)
	}

	// 2. LEA-decomposable: C = 1 + scale where scale ∈ {1,2,4,8} → C ∈ {2,3,5,9}
	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, dst, dst, scale, comment)
	}

	// 3. Factor out powers of 2: C = odd * 2^k
	shift := int64(0)
	odd := C
	for odd%2 == 0 {
		odd /= 2
		shift++
	}

	// Only decompose if odd is small enough for a single LEA
	if shift > 0 {
		oddScale := odd - 1
		switch oddScale {
		case 1, 2, 4, 8:
			// LEA dst, [dst+dst*oddScale] ; dst *= odd
			// Then SHL dst, shift            ; dst *= 2^shift
			// Total: dst *= odd * 2^shift = C
			return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, dst, dst, oddScale, comment, dst, shift, comment)
		}
	}

	// No decomposition found
	return ""
}

// decomposeMul3Op emits VAS-level instructions for dst = src * C.
func decomposeMul3Op(dst, src string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	// 1. Power of 2: MOV dst, src; SHL dst, shift
	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tMOV\t%s, %s%s\n\tshl\t%s, %d%s", dst, src, comment, dst, shift, comment)
	}

	// 2. LEA-decomposable: C = 1 + scale → dst = src * (1+scale)
	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, src, src, scale, comment)
	}

	// 3. C = (1+scale) * 2^k → LEA then SHL
	shift := int64(0)
	odd := C
	for odd%2 == 0 {
		odd /= 2
		shift++
	}
	if shift > 0 {
		oddScale := odd - 1
		switch oddScale {
		case 1, 2, 4, 8:
			return fmt.Sprintf("\tMOV\t%s, %s%s\n\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, src, comment, dst, dst, dst, oddScale, comment, dst, shift, comment)
		}
	}

	// 4. C = (2^k) - 1 → LEA then SUB (e.g., 3 = 4-1, 7 = 8-1)
	// dst = src * (2^k - 1)
	// LEA dst, [src*2^k]  → dst = src * 2^k
	// SUB dst, src          → dst = src * 2^k - src = src*(2^k-1)
	for k := int64(2); k <= 3; k++ {
		if C == (1<<k)-1 {
			return fmt.Sprintf("\tLEA\t%s, [%s*%d]%s\n\tSUB\t%s, %s%s",
				dst, src, 1<<k, comment, dst, src, comment)
		}
	}

	// No decomposition found
	return ""
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
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				lastStore = map[string]int{}
			} else if len(fields) >= 2 {
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

// trimBrackets strips surrounding brackets from a memory operand like "[v5]" -> "v5".
func trimBrackets(s string) string {
	s = strings.TrimLeft(s, "[")
	s = strings.TrimRight(s, "]")
	return s
}

// ---------------------------------------------------------------------------
// Post-expansion: peephole optimizations on asm output
// ---------------------------------------------------------------------------

func peephole(lines []string) []string {
	lines = xorZero(lines)
	lines = testCmp(lines)
	lines = nopMerge(lines)
	lines = leaFuse(lines)
	lines = noopElim(lines)
	lines = pushPopMov(lines)
	lines = xorMovElim(lines)
	lines = shlAddFuse(lines)
	lines = addNegFuse(lines)
	lines = cancelPairElim(lines)
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
// Also fuses:
//
//	"mov r1, r2; sub r1, N"  → "lea r1, [r2-N]"
//	"mov r1, r2; imul r1, K" → "lea r1, [r2+r2*(K-1)]" (K ∈ {3,5,9})
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

// subImmRe matches: sub\tr1,\s*N  (register, immediate)
var subImmRe = regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*(-?\d+)$`)

// imulImmRe matches: imul\tr1,\s*K  (register, immediate)
var imulImmRe = regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*(\d+)$`)

func tryLeaFuse(line1, line2 string) (string, bool) {
	m1 := movRe.FindStringSubmatch(strings.TrimRight(line1, " \t\r"))
	if m1 == nil {
		return "", false
	}
	dst, src1 := m1[1], m1[2]

	// Try: mov dst, src1 ; add dst, src2  →  lea dst, [src1+src2]
	if m2 := addRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		addDst, src2 := m2[1], m2[2]
		if addDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s+%s]", dst, src1, src2), true
		}
	}

	// Try: mov dst, src1 ; sub dst, N  →  lea dst, [src1-N]
	if m2 := subImmRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		subDst, imm := m2[1], m2[2]
		if subDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s-%s]", dst, src1, imm), true
		}
	}

	// Try: mov dst, src1 ; imul dst, K  (K ∈ {3,5,9}) → lea dst, [src1+src1*(K-1)]
	if m2 := imulImmRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		imulDst, kStr := m2[1], m2[2]
		if imulDst == dst {
			k, err := strconv.Atoi(kStr)
			if err == nil {
				// LEA supports scale 1,2,4,8. Decompose K as 1+scale.
				// Valid K: 2 (1+1), 3 (1+2), 5 (1+4), 9 (1+8)
				scale := k - 1
				switch scale {
				case 1, 2, 4, 8:
					return fmt.Sprintf("\tlea\t%s, [%s+%s*%d]", dst, src1, src1, scale), true
				}
			}
		}
	}

	return "", false
}

// ---------------------------------------------------------------------------
// -O2 optimizations
// ---------------------------------------------------------------------------

// cse eliminates common subexpressions within each basic block.
// If the same (op, arg1, arg2) appears twice, the second is replaced
// with a MOV from the first result.
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
	args string // joined args (without dst)
}

func cseBlock(lines []string) []string {
	seen := map[cseKey]string{} // key → dst register name
	// Track which registers are overwritten (kill entries that use them)
	regVals := map[string]cseKey{} // reg → last key it computed

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result = append(result, line)
			continue
		}
		tokens := strings.Fields(code)
		if len(tokens) < 2 {
			result = append(result, line)
			continue
		}
		op := strings.ToUpper(tokens[0])
		args := tokens[1:]

		// Clear entries whose source registers have been overwritten
		dst := dstReg(op, args)
		if dst >= 0 {
			dstName := args[0]
			// If this dst was tracking a previous expression, remove that expression from seen
			if prevKey, ok := regVals[dstName]; ok {
				delete(seen, prevKey)
			}
		}

		// For 2- or 3-operand arithmetic: check if this computation is already seen
		if len(args) >= 2 && (op == "ADD" || op == "SUB" || op == "MUL") {
			// Strip trailing commas from args for clean comparison
			cleanArgs := make([]string, len(args[1:]))
			for i, a := range args[1:] {
				cleanArgs[i] = strings.TrimRight(a, ",")
			}
			// Key = (op, remaining args after dst)
			key := cseKey{op: op, args: strings.Join(cleanArgs, " ")}
			if prevDst, ok := seen[key]; ok && prevDst != args[0] {
				// Same computation exists — emit MOV instead
				dst := strings.TrimRight(args[0], ",")
				src := strings.TrimRight(prevDst, ",")
				comment := ""
				if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
					comment = " " + trimmed[idx:]
				}
				result = append(result, fmt.Sprintf("\tMOV\t%s, %s%s", dst, src, comment))
				// Track that args[0] now holds the same value as prevDst
				regVals[args[0]] = key
				continue
			}
			// First time seeing this computation — record it
			seen[key] = args[0]
			if dst >= 0 {
				regVals[args[0]] = key
			}
		}

		result = append(result, line)
	}
	return result
}

// licm hoists loop-invariant instructions before the loop header.
// A loop is identified as a label targeted by a backward JMP/JE/etc.
func licm(lines []string) []string {
	// Collect label positions: label → line index
	labelIdx := map[string]int{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSuffix(trimmed, ":")
			labelIdx[name] = i
		}
	}

	// Find backward jumps: JMP/JE/etc. to a label before the jump
	type loop struct {
		header int // label line index
		back   int // jump line index
	}
	var loops []loop
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}
		fields := strings.Fields(code)
		if len(fields) != 2 {
			continue
		}
		op := strings.ToUpper(fields[0])
		switch op {
		case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE":
			target := fields[1]
			if idx, ok := labelIdx[target]; ok && idx < i {
				loops = append(loops, loop{header: idx, back: i})
			}
		}
	}

	if len(loops) == 0 {
		return lines
	}

	// For each loop (innermost first), hoist invariant instructions
	// Sort loops so innermost (largest header) comes first
	// Simple approach: process in reverse order of header
	for li := len(loops) - 1; li >= 0; li-- {
		l := loops[li]
		// Check if this loop is inside another (nested) — skip, outer pass handles it
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

		// Determine which registers are modified inside the loop body
		modified := map[string]bool{}
		for j := l.header + 1; j <= l.back; j++ {
			trimmed := strings.TrimSpace(lines[j])
			code := trimmed
			if idx := strings.IndexAny(code, ";#"); idx >= 0 {
				code = strings.TrimSpace(code[:idx])
			}
			if code == "" {
				continue
			}
			fields := strings.Fields(code)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			args := fields[1:]
			// Simple check: any register that appears as arg[0] of a non-MOVI instruction is modified
			if op == "MOVI" && len(args) >= 1 {
				modified[args[0]] = true
			} else if dst := dstReg(op, args); dst >= 0 {
				modified[args[0]] = true
			}
		}

		// Scan the loop body for instructions to hoist (currently only LEA with label operand)
		var hoisted []int
		for j := l.header + 1; j < l.back; j++ {
			trimmed := strings.TrimSpace(lines[j])
			code := trimmed
			if idx := strings.IndexAny(code, ";#"); idx >= 0 {
				code = strings.TrimSpace(code[:idx])
			}
			if code == "" {
				continue
			}
			fields := strings.Fields(code)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			args := fields[1:]

			// Hoist LEA with label operand if its dst is not modified elsewhere in the loop
			if op == "LEA" && len(args) >= 2 {
				dst := args[0]
				// Check that the source operand references a label, not a v-reg
				src := args[1]
				if !strings.HasPrefix(src, "[v") && strings.HasPrefix(src, "[") {
					// Check dst is not modified by any OTHER instruction in the loop
					modifiedElsewhere := false
					for jj := l.header + 1; jj < l.back; jj++ {
						if jj == j {
							continue
						}
						jjTrimmed := strings.TrimSpace(lines[jj])
						jjCode := jjTrimmed
						if idx := strings.IndexAny(jjCode, ";#"); idx >= 0 {
							jjCode = strings.TrimSpace(jjCode[:idx])
						}
						if jjCode == "" {
							continue
						}
						jjFields := strings.Fields(jjCode)
						if len(jjFields) < 2 {
							continue
						}
						jjOp := strings.ToUpper(jjFields[0])
						jjArgs := jjFields[1:]
						if jjOp == "MOVI" && len(jjArgs) >= 1 && jjArgs[0] == dst {
							modifiedElsewhere = true
							break
						}
						if d := dstReg(jjOp, jjArgs); d >= 0 && jjArgs[0] == dst {
							modifiedElsewhere = true
							break
						}
					}
					if !modifiedElsewhere {
						hoisted = append(hoisted, j)
					}
				}
			}
		}

		if len(hoisted) == 0 {
			continue
		}

		// Build new lines with hoisted instructions moved before the loop header
		newLines := make([]string, 0, len(lines)+len(hoisted))
		hoistSet := map[int]bool{}
		for _, h := range hoisted {
			hoistSet[h] = true
		}
		for i, line := range lines {
			if i == l.header {
				// Insert hoisted instructions just before the label
				for _, h := range hoisted {
					newLines = append(newLines, lines[h])
				}
			}
			if !hoistSet[i] {
				newLines = append(newLines, line)
			}
		}
		lines = newLines
	}

	return lines
}

// redundantLoadElim replaces LOAD from an address that was recently loaded
// from the same address (without intervening STORE) with a MOV from the earlier dst.
func redundantLoadElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, redundantLoadBlock(block)...)
	}
	return result
}

func redundantLoadBlock(lines []string) []string {
	// lastLoad[operand_string] = dst_register that holds the loaded value
	lastLoad := map[string]string{}
	// Track which address registers have been modified since their last load
	addrModified := map[string]bool{}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
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

		if op == "LOAD" && len(args) >= 2 {
			addr := args[1]
			// Extract the address register (strip brackets)
			addrReg := strings.TrimRight(strings.TrimLeft(addr, "["), "]")
			if prevDst, ok := lastLoad[addr]; ok && !addrModified[addrReg] {
				// Same address, no intervening modification — reuse previous value
				dst := strings.TrimRight(args[0], ",")
				src := strings.TrimRight(prevDst, ",")
				comment := ""
				if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
					comment = " " + trimmed[idx:]
				}
				result[i] = fmt.Sprintf("\tMOV\t%s, %s%s", dst, src, comment)
				continue
			}
			// Record this load
			lastLoad[addr] = args[0]
			addrModified[addrReg] = false
		}

		// Track modifications to registers (including LOAD dst itself)
		dstIdx := -1
		switch op {
		case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
			if len(args) >= 1 {
				dstIdx = 0
			}
		}
		if dstIdx >= 0 {
			modifiedReg := strings.TrimRight(args[dstIdx], ",")
			// Mark this register as modified (may be used as address in future loads)
			addrModified[modifiedReg] = true
		}

		// STORE to an address invalidates loads from that address
		if op == "STORE" && len(args) >= 2 {
			delete(lastLoad, args[1])
		}

		result[i] = line
	}
	return result
}

// pushPopElim removes balanced PUSH/POP pairs within a basic block
// when the pushed register is not modified between them.
func pushPopElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, pushPopBlock(block)...)
	}
	return result
}

func pushPopBlock(lines []string) []string {
	// Match PUSH vX followed by POP vX with no modification of vX in between
	remove := make([]bool, len(lines))
	for i, line := range lines {
		if remove[i] {
			continue
		}
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}
		fields := strings.Fields(code)
		if len(fields) < 2 {
			continue
		}
		op := strings.ToUpper(fields[0])
		if op != "PUSH" {
			continue
		}
		reg := fields[1]

		// Scan forward for matching POP
		for j := i + 1; j < len(lines); j++ {
			if remove[j] {
				continue
			}
			jTrimmed := strings.TrimSpace(lines[j])
			jCode := jTrimmed
			if idx := strings.IndexAny(jCode, ";#"); idx >= 0 {
				jCode = strings.TrimSpace(jCode[:idx])
			}
			if jCode == "" {
				continue
			}
			jFields := strings.Fields(jCode)
			if len(jFields) < 2 {
				continue
			}
			jOp := strings.ToUpper(jFields[0])
			jReg := strings.TrimRight(jFields[1], ",")

			// If this is a POP of the same register, we found our match
			if jOp == "POP" && jReg == reg {
				// Found matching POP — remove both
				remove[i] = true
				remove[j] = true
				break
			}

			// If reg is modified between PUSH and POP, abort
			if d := dstReg(jOp, jFields[1:]); d >= 0 && jReg == reg {
				break
			}

			// CALL might modify any register, so abort
			if jOp == "CALL" || jOp == "SYSCALL" || jOp == "INT" {
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

// tailCallOpt replaces "CALL label; RET" with "JMP label" when the
// caller's return value is passed through directly.
func tailCallOpt(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			code := trimmed
			if idx := strings.IndexAny(code, ";#"); idx >= 0 {
				code = strings.TrimSpace(code[:idx])
			}
			if code != "" {
				fields := strings.Fields(code)
				if len(fields) == 2 && strings.ToUpper(fields[0]) == "CALL" {
					nextTrimmed := strings.TrimSpace(lines[i+1])
					nextCode := nextTrimmed
					if idx := strings.IndexAny(nextCode, ";#"); idx >= 0 {
						nextCode = strings.TrimSpace(nextCode[:idx])
					}
					nextFields := strings.Fields(nextCode)
					if len(nextFields) == 1 && strings.ToUpper(nextFields[0]) == "RET" {
						// CALL label + RET → JMP label
						target := fields[1]
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result = append(result, fmt.Sprintf("\tJMP\t%s%s", target, comment))
						i += 2
						continue
					}
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// noopElim removes no-op instructions: mov r1,r1, add r1,0, sub r1,0, imul r1,1
func noopElim(lines []string) []string {
	movRe := regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	addZero := regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*0$`)
	subZero := regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*0$`)
	imulOne := regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*1$`)

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		// mov r1, r1 → remove (same src and dst)
		if m := movRe.FindStringSubmatch(trimmed); m != nil && m[1] == m[2] {
			continue
		}
		if addZero.MatchString(trimmed) || subZero.MatchString(trimmed) || imulOne.MatchString(trimmed) {
			continue
		}
		result = append(result, line)
	}
	return result
}

// pushPopMov replaces "push r1; pop r2" with "mov r2, r1".
func pushPopMov(lines []string) []string {
	pushRe := regexp.MustCompile(`^\tpush\t([a-z][a-z0-9]+)$`)
	popRe := regexp.MustCompile(`^\tpop\t([a-z][a-z0-9]+)$`)

	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := pushRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			m2 := popRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m1 != nil && m2 != nil {
				result = append(result, fmt.Sprintf("\tmov\t%s, %s", m2[1], m1[1]))
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// xorMovElim replaces "xor r1,r1; mov r1,r2" with "mov r1, r2".
func xorMovElim(lines []string) []string {
	xorRe := regexp.MustCompile(`^\txor\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)

	// Map 32-bit register names to 64-bit for matching against mov
	to64 := map[string]string{
		"eax": "rax", "ebx": "rbx", "ecx": "rcx", "edx": "rdx",
		"esi": "rsi", "edi": "rdi", "ebp": "rbp", "esp": "rsp",
		"r8d": "r8", "r9d": "r9", "r10d": "r10", "r11d": "r11",
		"r12d": "r12", "r13d": "r13", "r14d": "r14", "r15d": "r15",
	}

	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := xorRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			// Match xor r1, r1 (same register)
			if m1 != nil && m1[1] == m1[2] {
				reg32 := m1[1]
				reg64 := reg32
				if r, ok := to64[reg32]; ok {
					reg64 = r
				}
				// Check if next line is "mov <reg64>, something"
				movAfterXor := regexp.MustCompile(fmt.Sprintf(`^\tmov\t%s,\s*([a-z][a-z0-9]+)$`, regexp.QuoteMeta(reg64)))
				m2 := movAfterXor.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil {
					result = append(result, fmt.Sprintf("\tmov\t%s, %s", reg64, m2[1]))
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

// shlAddFuse replaces "mov r1, r2; shl r1, k; add r1, r2" with
// "lea r1, [r2+r2*2^k]" for k ∈ {1,2,3} (scale 2,4,8).
// This computes r1 = r2 * (2^k + 1) in one instruction.
func shlAddFuse(lines []string) []string {
	movRe := regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	shlRe := regexp.MustCompile(`^\tshl\t([a-z][a-z0-9]+),\s*(\d+)$`)
	addRe := regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)

	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+2 < len(lines) {
			m1 := movRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			movDst, movSrc := m1[1], m1[2]

			m2 := shlRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 == nil || m2[1] != movDst {
				result = append(result, lines[i])
				i++
				continue
			}
			shiftStr := m2[2]

			m3 := addRe.FindStringSubmatch(strings.TrimRight(lines[i+2], " \t\r"))
			if m3 == nil || m3[1] != movDst || m3[2] != movSrc {
				result = append(result, lines[i])
				i++
				continue
			}

			shift, err := strconv.Atoi(shiftStr)
			if err != nil || shift < 1 || shift > 3 {
				result = append(result, lines[i])
				i++
				continue
			}
			scale := 1 << uint(shift) // 2, 4, 8

			result = append(result, fmt.Sprintf("\tlea\t%s, [%s+%s*%d]", movDst, movSrc, movSrc, scale))
			i += 3
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// addNegFuse replaces "add r1, 1; neg r1" with "not r1".
// Verified: -(x+1) = ~x for all 64-bit values.
func addNegFuse(lines []string) []string {
	add1Re := regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*1$`)
	negRe := regexp.MustCompile(`^\tneg\t([a-z][a-z0-9]+)$`)

	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := add1Re.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			reg := m1[1]

			m2 := negRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 != nil && m2[1] == reg {
				// add r1, 1; neg r1 → not r1
				result = append(result, fmt.Sprintf("\tnot\t%s", reg))
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// cancelPairElim removes instruction pairs that cancel each other out.
// Patterns:
//
//	not r1; not r1  →  <delete>
//	neg r1; neg r1  →  <delete>
//	inc r1; dec r1  →  <delete>
//	dec r1; inc r1  →  <delete>
func cancelPairElim(lines []string) []string {
	notRe := regexp.MustCompile(`^\tnot\t([a-z][a-z0-9]+)$`)
	negRe := regexp.MustCompile(`^\tneg\t([a-z][a-z0-9]+)$`)
	incRe := regexp.MustCompile(`^\tinc\t([a-z][a-z0-9]+)$`)
	decRe := regexp.MustCompile(`^\tdec\t([a-z][a-z0-9]+)$`)

	// Helper: check if line matches a pattern and returns the register.
	matchReg := func(re *regexp.Regexp, line string) string {
		m := re.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			return m[1]
		}
		return ""
	}

	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			reg := matchReg(notRe, lines[i])
			if reg != "" && matchReg(notRe, lines[i+1]) == reg {
				i += 2 // not; not → delete both
				continue
			}

			reg = matchReg(negRe, lines[i])
			if reg != "" && matchReg(negRe, lines[i+1]) == reg {
				i += 2 // neg; neg → delete both
				continue
			}

			reg = matchReg(incRe, lines[i])
			if reg != "" && matchReg(decRe, lines[i+1]) == reg {
				i += 2 // inc; dec → delete both
				continue
			}

			reg = matchReg(decRe, lines[i])
			if reg != "" && matchReg(incRe, lines[i+1]) == reg {
				i += 2 // dec; inc → delete both
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
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
