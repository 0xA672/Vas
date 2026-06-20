// Package opt implements the -O1 and -O2 optimization passes for VAS.
//
// Optimizations are divided into two categories:
//
// Pre-expansion (operates on VAS source before instruction expansion):
//   - ConstantFolding: compute ADD/SUB v1, imm, imm at assembly time
//   - DeadCodeElim: remove writes to v-regs that are overwritten before being read
//   - CopyPropagate: replace MOV v1, v0 references with v0 where possible
//   - ConstPropagate: track MOVI constants and fold them into subsequent instructions
//   - StoreLoadFwd: replace LOAD from a label with MOV from the last STORE to that label
//   - DeadStoreElim: remove STORE instructions whose target is stored again before any LOAD
//   - StrengthReduce: replace MUL by power-of-2 or small constants with SHL/LEA sequences
//
// Post-expansion (peephole optimization on generated assembly text):
//   - XorZero:  mov reg, 0  =>  xor reg, reg  (smaller encoding, zeroes flags)
//   - TestCmp:  cmp reg, 0  =>  test reg, reg  (smaller encoding)
//   - NopMerge: consecutive NOP lines => longer efficient NOP
//   - LeaFuse:  mov r1, r2; add r1, r3  =>  lea r1, [r2+r3]
//   - NoopElim: remove mov r1,r1, add r1,0, sub r1,0, imul r1,1
//   - PushPopMov: push r1; pop r2  =>  mov r2, r1
//   - XorMovElim: xor r1,r1; mov r1,r2  =>  mov r1, r2
//   - ShlAddFuse: mov r1,r2; shl r1,k; add r1,r2  =>  lea r1,[r2+r2*2^k]
//   - AddNegFuse: add r1,1; neg r1  =>  not r1
//   - CancelPairElim: not r1; not r1, neg r1; neg r1, inc r1; dec r1, etc. => delete
//   - PushModPopElim: push reg; modify reg; pop reg (result unused) => delete
//
// -O2 additions (more aggressive, operate on VAS source with virtual registers):
//   - CSE: common subexpression elimination
//   - LICM: loop invariant code motion (LEA with label operands)
//   - RedundantLoadElim: remove LOAD from same address without intervening STORE
//   - PushPopElim: remove balanced PUSH/POP pairs when register is unmodified
//   - TailCallOpt: CALL label; RET => JMP label
//
// Explain mode: set ExplainEnabled = true to emit [OPT] comments describing
// each applied optimization.  Vas diff automatically enables this.
package opt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Package-level precompiled regexps and lookup tables.
// Recompiling these on every call was the single largest source of allocations.
// ---------------------------------------------------------------------------

var (
	// Peephole pass regexes
	xorZeroRe   = regexp.MustCompile(`^\txor\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	movZeroRe   = regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*0$`)
	testCmpZero = regexp.MustCompile(`^\tcmp\t([a-z][a-z0-9]+),\s*0$`)

	// LEA fusion regexes
	peepMovRe    = regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	peepAddRe    = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	peepSubImmRe = regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*(-?\d+)$`)
	peepImulOneRe = regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*(\d+)$`)
	peepShlRe    = regexp.MustCompile(`^\tshl\t([a-z][a-z0-9]+),\s*(\d+)$`)

	// No-op elimination regexes
	noopAddZeroRe  = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*0$`)
	noopSubZeroRe  = regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*0$`)
	noopImulOneRe2 = regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*1$`)

	// Push/pop fusion
	pushRe = regexp.MustCompile(`^\tpush\t([a-z][a-z0-9]+)$`)
	popRe  = regexp.MustCompile(`^\tpop\t([a-z][a-z0-9]+)$`)

	// XOR zero detection
	xorSelfRe = regexp.MustCompile(`^\txor\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)

	// Pair cancellation
	notRe = regexp.MustCompile(`^\tnot\t([a-z][a-z0-9]+)$`)
	negRe = regexp.MustCompile(`^\tneg\t([a-z][a-z0-9]+)$`)
	incRe = regexp.MustCompile(`^\tinc\t([a-z][a-z0-9]+)$`)
	decRe = regexp.MustCompile(`^\tdec\t([a-z][a-z0-9]+)$`)

	// Push-mod-pop elimination
	addNegFuseRe = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*1$`)
	pushModRe    = regexp.MustCompile(`^\t(add|sub|imul|mov|lea)\t([a-z][a-z0-9]+),.*$`)

	// Precomputed register conversion tables
	regTo32Map = map[string]string{
		"rax": "eax", "rbx": "ebx", "rcx": "ecx", "rdx": "edx",
		"rsi": "esi", "rdi": "edi", "rbp": "ebp", "rsp": "esp",
		"r8": "r8d", "r9": "r9d", "r10": "r10d", "r11": "r11d",
		"r12": "r12d", "r13": "r13d", "r14": "r14d", "r15": "r15d",
	}
	regTo64Map = map[string]string{
		"eax": "rax", "ebx": "rbx", "ecx": "rcx", "edx": "rdx",
		"esi": "rsi", "edi": "rdi", "ebp": "rbp", "esp": "rsp",
		"r8d": "r8", "r9d": "r9", "r10d": "r10", "r11d": "r11",
		"r12d": "r12", "r13d": "r13", "r14d": "r14", "r15d": "r15",
	}
)

func regTo32(reg string) string {
	if r, ok := regTo32Map[reg]; ok {
		return r
	}
	return reg
}

func regTo64(reg string) string {
	if r, ok := regTo64Map[reg]; ok {
		return r
	}
	return reg
}

// Fast virtual register detector: checks for v0..v12 using simple
// byte-oriented scanning instead of strings.Split + 13 separate Contains calls.
func hasVirtualReg(s string) bool {
	// Scan for "v" followed by a digit
	for i := 0; i < len(s); i++ {
		if s[i] == 'v' && i+1 < len(s) {
			c := s[i+1]
			if c >= '0' && c <= '9' {
				// It's vN or vNN — confirm v0..v12
				if c == '0' || c == '1' {
					if i+2 >= len(s) {
						// v0 or v1 at end
						return true
					}
					next := s[i+2]
					if next < '0' || next > '9' {
						return true
					}
					// Two digit: v10, v11, v12
					if c == '1' && (next == '0' || next == '1' || next == '2') {
						if i+3 >= len(s) || s[i+3] < '0' || s[i+3] > '9' {
							return true
						}
					}
					// Not v0..v12 (e.g. v13+), keep scanning
					i++
					continue
				}
				// v2..v9 single digit
				return true
			}
		}
	}
	return false
}

// Pre-split helper: avoids redundant strings.Split calls in the pipeline.
// A small pool of line slices keeps GC pressure low on repeated invocations.
var linePool = sync.Pool{
	New: func() interface{} {
		buf := make([]string, 0, 32)
		return &buf
	},
}

func borrowLines(n int) []string {
	p := linePool.Get().(*[]string)
	s := *p
	if cap(s) < n {
		s = make([]string, 0, n)
	}
	return s[:0]
}

func releaseLines(s []string) {
	// Don't keep huge slices in the pool
	if cap(s) <= 4096 {
		s = s[:0]
		linePool.Put(&s)
	}
}

// ExplainEnabled controls whether peephole passes insert explanatory
// comments when an optimization is applied.
var ExplainEnabled bool

// PeepholeOnly runs only peephole optimizations (safe for NASM output with physical registers).
func PeepholeOnly(input string) string {
	lines := strings.Split(input, "\n")
	lines = peephole(lines)
	return strings.Join(lines, "\n")
}

// Optimize runs all enabled optimization passes on the assembled output.
// level 0 = no optimization, level >=1 = -O1.
// IMPORTANT: This function only works correctly on VAS source code (with virtual registers v0-v12).
// It should NOT be called on NASM output (with physical registers like rax, rbx, etc.).
func Optimize(input string, level int) string {
	if level <= 0 {
		return input
	}

	// Fast-path: check if input contains any virtual register at all.
	// Avoids the cost of running all passes on physical-register output.
	if !hasVirtualReg(input) {
		return input
	}

	lines := strings.Split(input, "\n")

	// Pre-expansion optimizations (operate on VAS virtual registers)
	lines = copyPropagate(lines)
	lines = constPropagate(lines)
	lines = strengthReduce(lines)
	lines = storeLoadFwd(lines)
	lines = deadStoreElim(lines)
	lines = deadCodeElim(lines)

	// Peephole runs post-expansion too, so keep it last.
	lines = peephole(lines)
	lines = peephole(lines)

	// -O2: more aggressive optimizations (only on VAS source, NOT on NASM output)
	// These passes only understand virtual registers (v0-v12), not physical registers
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
func deadCodeElim(lines []string) []string {
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
func hasSideEffect(op string) bool {
	switch op {
	case "POP", "PUSH", "CALL", "STORE", "INT", "SYSCALL", "RET":
		return true
	}
	return false
}

func elimBlock(lines []string) []string {
	lastWrite := map[int]int{} // reg index => line index in this block
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

		if hasSideEffect(op) {
			for _, r := range readRegs(op, args) {
				delete(lastWrite, r)
			}
			continue
		}

		reads := readRegs(op, args)
		for _, r := range reads {
			delete(lastWrite, r)
		}

		dst := dstReg(op, args)
		if dst >= 0 {
			if prev, exists := lastWrite[dst]; exists {
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
		regs = append(regs, 0, 3, 4, 5, 6, 7, 8)
	case "INT":
		regs = append(regs, 0, 3, 4, 5, 6, 7, 8)
	}
	return regs
}

// ---------------------------------------------------------------------------
// Pre-expansion: copy propagation (MOV vX, vY => use vY instead of vX)
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
	alias := make([]int, 13)
	for i := range alias {
		alias[i] = -1
	}

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

		propagated := make([]string, len(args))
		for j, a := range args {
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

		newLine := fmt.Sprintf("\t%s\t%s", op, strings.Join(propagated, " "))
		if idx := strings.IndexAny(line, ";#"); idx >= 0 {
			newLine += line[idx:]
		}

		if dst >= 0 {
			for j := range alias {
				if alias[j] == dst {
					alias[j] = -1
				}
			}
		}

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

func constPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, constBlock(block)...)
	}
	return result
}

func constBlock(lines []string) []string {
	constVal := make([]int64, 13)
	constKnown := make([]bool, 13)
	used := map[int]bool{}
	moviLine := map[int]int{}

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

	arg := func(i int) string {
		s := fields[i]
		s = strings.TrimRight(s, ",")
		return s
	}

	if len(fields) == 3 {
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

func decomposeMul2Op(dst string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tshl\t%s, %d%s", dst, shift, comment)
	}

	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, dst, dst, scale, comment)
	}

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
			return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, dst, dst, oddScale, comment, dst, shift, comment)
		}
	}

	return ""
}

func decomposeMul3Op(dst, src string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tMOV\t%s, %s%s\n\tshl\t%s, %d%s", dst, src, comment, dst, shift, comment)
	}

	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, src, src, scale, comment)
	}

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

	for k := int64(2); k <= 3; k++ {
		if C == (1<<k)-1 {
			return fmt.Sprintf("\tLEA\t%s, [%s*%d]%s\n\tSUB\t%s, %s%s",
				dst, src, 1<<k, comment, dst, src, comment)
		}
	}

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

func storeLoadFwd(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, fwdBlock(block)...)
	}
	return result
}

func fwdBlock(lines []string) []string {
	lastStore := map[string]int{}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			lastStore = map[string]int{}
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
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				lastStore = map[string]int{}
			} else if len(fields) >= 2 {
				firstArg := strings.TrimRight(args[0], ",")
				if strings.HasPrefix(firstArg, "[") {
					lastStore = map[string]int{}
				}
			}
		}

		result[i] = line
	}
	return result
}

func extractLabel(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return ""
	}
	s = s[1:]
	if idx := strings.IndexAny(s, "+-*/]"); idx >= 0 {
		before := strings.TrimSpace(s[:idx])
		if regIndex(before) >= 0 {
			return ""
		}
		if _, err := strconv.ParseInt(before, 0, 64); err == nil {
			return ""
		}
		return before
	}
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
func regIndex(s string) int {
	s = strings.TrimRight(s, ",")
	if len(s) >= 2 && s[0] == 'v' {
		rest := s[1:]
		if len(rest) == 0 {
			return -1
		}
		if len(rest) == 1 && rest[0] >= '0' && rest[0] <= '9' {
			return int(rest[0] - '0')
		}
		if len(rest) == 2 && rest[0] == '1' && rest[1] >= '0' && rest[1] <= '2' {
			return 10 + int(rest[1]-'0')
		}
	}
	return -1
}

func trimBrackets(s string) string {
	s = strings.TrimLeft(s, "[")
	s = strings.TrimRight(s, "]")
	return s
}

// ---------------------------------------------------------------------------
// Peephole orchestration
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
	lines = pushModPopElim(lines)
	return lines
}

func xorZero(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := movZeroRe.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			r32 := regTo32(reg)
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] xor %s, %s  replaces mov %s, 0 (shorter encoding, sets ZF=1 CF=0 OF=0)",
					r32, r32, reg))
			}
			result = append(result, fmt.Sprintf("\txor\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

func testCmp(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := testCmpZero.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			r32 := regTo32(reg)
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] test %s, %s  replaces cmp %s, 0 (shorter encoding)",
					r32, r32, reg))
			}
			result = append(result, fmt.Sprintf("\ttest\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

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
				if ExplainEnabled {
					result = append(result, fmt.Sprintf("; [OPT] merged %d nops into one", count))
				}
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

func leaFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			fused, ok := tryLeaFuse(lines[i], lines[i+1])
			if ok {
				if ExplainEnabled {
					// Build description from the two fused lines
					line1 := strings.TrimSpace(lines[i])
					line2 := strings.TrimSpace(lines[i+1])
					result = append(result, fmt.Sprintf("; [OPT] lea fused from: %s ; %s", line1, line2))
				}
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

func tryLeaFuse(line1, line2 string) (string, bool) {
	m1 := peepMovRe.FindStringSubmatch(strings.TrimRight(line1, " \t\r"))
	if m1 == nil {
		return "", false
	}
	dst, src1 := m1[1], m1[2]

	if m2 := peepAddRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		addDst, src2 := m2[1], m2[2]
		if addDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s+%s]", dst, src1, src2), true
		}
	}

	if m2 := peepSubImmRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		subDst, imm := m2[1], m2[2]
		if subDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s-%s]", dst, src1, imm), true
		}
	}

	if m2 := peepImulOneRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		imulDst, kStr := m2[1], m2[2]
		if imulDst == dst {
			k, err := strconv.Atoi(kStr)
			if err == nil {
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

func cseBlock(lines []string) []string {
	seen := map[cseKey]string{}
	regVals := map[string]cseKey{}

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

		dst := dstReg(op, args)
		if dst >= 0 {
			dstName := args[0]
			if prevKey, ok := regVals[dstName]; ok {
				delete(seen, prevKey)
			}
		}

		if len(args) >= 2 && (op == "ADD" || op == "SUB" || op == "MUL") {
			cleanArgs := make([]string, len(args[1:]))
			for i, a := range args[1:] {
				cleanArgs[i] = strings.TrimRight(a, ",")
			}
			key := cseKey{op: op, args: strings.Join(cleanArgs, " ")}
			if prevDst, ok := seen[key]; ok && prevDst != args[0] {
				seen[key] = args[0]
				if dst >= 0 {
					regVals[args[0]] = key
				}
				result = append(result, line)
				continue
			}
			seen[key] = args[0]
			if dst >= 0 {
				regVals[args[0]] = key
			}
		}

		result = append(result, line)
	}
	return result
}

func licm(lines []string) []string {
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

	type loop struct {
		header int
		back   int
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

	for li := len(loops) - 1; li >= 0; li-- {
		l := loops[li]
		isNested := false
		for _, outer := range loops {
			if outer.header < l.header && outer.back > l.back {
				isNested = true
			}
		}
		if isNested {
			continue
		}

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
			if dst := dstReg(op, args); dst >= 0 && len(args) >= 1 {
				modified[args[0]] = true
			}
		}

		var hoisted []int
		var hoistedLines []string
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

			isInvariant := false
			if op == "LEA" && len(args) >= 2 {
				memOp := args[1]
				memOp = strings.TrimPrefix(memOp, "[")
				memOp = strings.TrimSuffix(memOp, "]")
				if plusIdx := strings.Index(memOp, "+"); plusIdx >= 0 {
					memOp = memOp[:plusIdx]
				}
				if minusIdx := strings.Index(memOp, "-"); minusIdx >= 0 {
					memOp = memOp[:minusIdx]
				}
				dstName := strings.TrimRight(args[0], ",")
				if !modified[memOp] && !modified[dstName] {
					isInvariant = true
				}
			}

			if isInvariant {
				hoisted = append(hoisted, j)
				hoistedLines = append(hoistedLines, lines[j])
			}
		}

		if len(hoistedLines) > 0 {
			var newResult []string
			for i, line := range lines {
				if i == l.header {
					newResult = append(newResult, hoistedLines...)
				}
				isHoisted := false
				for _, h := range hoisted {
					if i == h {
						isHoisted = true
						break
					}
				}
				if !isHoisted {
					newResult = append(newResult, line)
				}
			}
			lines = newResult
			break
		}
	}
	return lines
}

func redundantLoadElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, redundantLoadBlock(block)...)
	}
	return result
}

func redundantLoadBlock(lines []string) []string {
	lastLoad := map[string]string{}
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
			addrReg := strings.TrimRight(strings.TrimLeft(addr, "["), "]")
			if prevDst, ok := lastLoad[addr]; ok && !addrModified[addrReg] {
				dst := strings.TrimRight(args[0], ",")
				src := strings.TrimRight(prevDst, ",")
				comment := ""
				if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
					comment = " " + trimmed[idx:]
				}
				result[i] = fmt.Sprintf("\tMOV\t%s, %s%s", dst, src, comment)
				continue
			}
			lastLoad[addr] = args[0]
			addrModified[addrReg] = false
		}

		dstIdx := -1
		switch op {
		case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
			if len(args) >= 1 {
				dstIdx = 0
			}
		}
		if dstIdx >= 0 {
			modifiedReg := strings.TrimRight(args[dstIdx], ",")
			addrModified[modifiedReg] = true
		}

		if op == "STORE" && len(args) >= 2 {
			delete(lastLoad, args[1])
		}

		result[i] = line
	}
	return result
}

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

			if jOp == "POP" && jReg == reg {
				remove[i] = true
				remove[j] = true
				break
			}

			if d := dstReg(jOp, jFields[1:]); d >= 0 && jReg == reg {
				break
			}

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

func noopElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if m := peepMovRe.FindStringSubmatch(trimmed); m != nil && m[1] == m[2] {
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: mov %s, %s", m[1], m[2]))
			}
			continue
		}
		if noopAddZeroRe.MatchString(trimmed) {
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		if noopSubZeroRe.MatchString(trimmed) {
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		if noopImulOneRe2.MatchString(trimmed) {
			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		result = append(result, line)
	}
	return result
}

func pushPopMov(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := pushRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			m2 := popRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m1 != nil && m2 != nil {
				if ExplainEnabled {
					result = append(result, fmt.Sprintf("; [OPT] push %s; pop %s  replaced by mov %s, %s",
						m1[1], m2[1], m2[1], m1[1]))
				}
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

func xorMovElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := xorSelfRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil && m1[1] == m1[2] {
				reg32 := m1[1]
				reg64 := regTo64(reg32)
				// Build a regex that matches: mov <reg64>, <src-reg>
				// We construct this manually by checking the string prefix instead of a regex
				// to avoid per-call regex compilation.
				prefix := fmt.Sprintf("\tmov\t%s,", reg64)
				next := strings.TrimRight(lines[i+1], " \t\r")
				if strings.HasPrefix(next, prefix) {
					// Extract src register
					rest := strings.TrimSpace(next[len(prefix):])
					if rest != "" {
						srcField := strings.Fields(rest)
						if len(srcField) > 0 && len(srcField[0]) > 0 && srcField[0] != "" {
							srcReg := srcField[0]
							// Validate it looks like a register
							valid := true
							for _, c := range srcReg {
								if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
									valid = false
									break
								}
							}
							if valid {
								if ExplainEnabled {
									result = append(result, fmt.Sprintf("; [OPT] xor %s,%s; mov %s,%s  replaced by mov %s, %s",
										m1[1], m1[2], reg64, srcReg, reg64, srcReg))
								}
								result = append(result, fmt.Sprintf("\tmov\t%s, %s", reg64, srcReg))
								i += 2
								continue
							}
						}
					}
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func shlAddFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+2 < len(lines) {
			m1 := peepMovRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			movDst, movSrc := m1[1], m1[2]

			m2 := peepShlRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 == nil || m2[1] != movDst {
				result = append(result, lines[i])
				i++
				continue
			}
			shiftStr := m2[2]

			m3 := peepAddRe.FindStringSubmatch(strings.TrimRight(lines[i+2], " \t\r"))
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
			scale := 1 << uint(shift)

			if ExplainEnabled {
				result = append(result, fmt.Sprintf("; [OPT] shl+add fused into lea %s, [%s+%s*%d]",
					movDst, movSrc, movSrc, scale))
			}
			result = append(result, fmt.Sprintf("\tlea\t%s, [%s+%s*%d]", movDst, movSrc, movSrc, scale))
			i += 3
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func addNegFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := addNegFuseRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			reg := m1[1]

			m2 := negRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 != nil && m2[1] == reg {
				if ExplainEnabled {
					result = append(result, fmt.Sprintf("; [OPT] add %s,1; neg %s  replaced by not %s", reg, reg, reg))
				}
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

func cancelPairElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := notRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := notRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if ExplainEnabled {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant not %s; not %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = negRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := negRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if ExplainEnabled {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant neg %s; neg %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = incRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := decRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if ExplainEnabled {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant inc %s; dec %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = decRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := incRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if ExplainEnabled {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant dec %s; inc %s", reg, reg))
					}
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

// pushModPopElim removes push; instr reg, ... ; pop reg triples
// when the pop restores the old value and the intermediate result dies.
func pushModPopElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+2 < len(lines) {
			m1 := pushRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			m3 := popRe.FindStringSubmatch(strings.TrimRight(lines[i+2], " \t\r"))
			if m1 != nil && m3 != nil && m1[1] == m3[1] {
				reg := m1[1]
				m2 := pushModRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[2] == reg {
					if ExplainEnabled {
						result = append(result, fmt.Sprintf("; [OPT] removed dead push %s; %s %s; pop %s",
							m1[1], m2[1], m2[2], m3[1]))
					}
					i += 3
					continue
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

