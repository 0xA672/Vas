package opt

import (
	"strconv"
	"strings"

	"vas/vas/internal/asm"
)

// deadCodeElim removes writes to virtual registers that are overwritten
// before being read. It tracks one linear stream and resets state at labels,
// branches and clobbers, so FLAGS consumers in a following block are visible
// to the liveness check.
func deadCodeElim(lines []string) []string {
	return elimBlock(lines)
}

// splitBlocks splits lines at labels and at every control-flow instruction,
// including all conditional jumps (JA/JB/JBE/JS/JO/LOOP…), not just the seven
// educational mnemonics.
func splitBlocks(lines []string) [][]string {
	var blocks [][]string
	var current []string
	for _, line := range lines {
		code := asm.TrimCode(line)
		if code == "" {
			current = append(current, line)
			continue
		}
		isLabel := strings.HasSuffix(code, ":") && !isInstructionFold(code)
		isControlFlow := false
		if !isLabel {
			if op, _, ok := asm.Parse(code); ok {
				isControlFlow = asm.IsBranch(op)
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
	op, _, ok := asm.Parse(s)
	if !ok {
		return false
	}
	switch op {
	case "ADD", "SUB", "MUL", "LOAD", "STORE", "LEA", "MOV", "MOVI",
		"CMP", "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE",
		"JA", "JB", "JAE", "JBE", "JS", "JNS", "JO", "JNO",
		"JC", "JNC", "JZ", "JNZ", "JP", "JNP", "JCXZ", "JECXZ", "JRCXZ",
		"LOOP", "LOOPE", "LOOPNE", "LOOPZ", "LOOPNZ",
		"CALL", "RET", "NOP", "PUSH", "POP", "INT", "SYSCALL",
		"SECTION", "GLOBAL", "EXTERN", "DATA", "TEXT", "BSS",
		"ALIGN", "BYTE", "WORD", "DWORD", "QWORD", "DD", "DQ", "DB",
		"TYPE", "SIZE", "LENGTH", "START":
		return true
	}
	return false
}

// hasSideEffect reports whether an instruction has observable side effects
// beyond writing its destination register.
func hasSideEffect(op string) bool {
	switch strings.ToUpper(op) {
	case "POP", "PUSH", "CALL", "STORE", "INT", "SYSCALL", "RET":
		return true
	}
	return false
}

type writtenReg struct {
	line        int
	writesFlags bool
}

func elimBlock(lines []string) []string {
	lastWrite := map[int]writtenReg{}
	remove := make([]bool, len(lines))

	for i, line := range lines {
		p, ok := parseOptLine(line)
		if !ok {
			// A label or blank line ends the register dataflow region.
			if strings.HasSuffix(p.code, ":") {
				lastWrite = map[int]writtenReg{}
			}
			continue
		}
		op := p.op
		args := p.args

		if touchesPhysicalReg(line) {
			// Raw physical-register instructions may alias any v-register
			// (v0 is rax, v1 is rbx, …). Do not reason across them.
			lastWrite = map[int]writtenReg{}
			continue
		}

		if hasSideEffect(op) || isUnknownOrClobber(p) {
			for _, r := range readRegs(op, args) {
				delete(lastWrite, r)
			}
			// Calls and syscalls clobber the whole virtual register file.
			if isUnknownOrClobber(p) {
				lastWrite = map[int]writtenReg{}
			}
			if asm.IsBranch(op) {
				lastWrite = map[int]writtenReg{}
			}
			continue
		}

		if !knownDstOp(op) {
			// Passthrough instructions are not modelled. Treat every virtual
			// register they mention as read, then drop all knowledge.
			for _, a := range args {
				for _, r := range virtualRegsIn(a) {
					delete(lastWrite, r)
				}
			}
			lastWrite = map[int]writtenReg{}
			continue
		}

		for _, r := range readRegs(op, args) {
			delete(lastWrite, r)
		}

		dst := dstReg(op, args)
		if dst >= 0 {
			if prev, exists := lastWrite[dst]; exists {
				// The value is dead only when the FLAGS it may have written
				// are also dead. The scan is over the full line stream and
				// therefore sees a JE/JNE in a following block.
				if !prev.writesFlags || !flagsLiveAfter(lines, prev.line) {
					remove[prev.line] = true
				}
			}
			lastWrite[dst] = writtenReg{line: i, writesFlags: asm.WritesFlags(strings.ToLower(op))}
		}
		if asm.IsBranch(op) {
			lastWrite = map[int]writtenReg{}
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

func knownDstOp(op string) bool {
	switch op {
	case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
		return true
	}
	return false
}

func dstReg(op string, args []string) int {
	if len(args) == 0 {
		return -1
	}
	switch strings.ToUpper(op) {
	case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
		return regIndex(args[0])
	}
	return -1
}

func readRegs(op string, args []string) []int {
	var regs []int
	add := func(a string) {
		regs = append(regs, virtualRegsIn(a)...)
	}
	switch strings.ToUpper(op) {
	case "MOVI":
		// The immediate/symbol is not a register read. In particular the
		// destination must not be treated as reading itself, or DCE can never
		// remove an overwritten MOVI.
		if len(args) >= 2 {
			add(args[1])
		}
	case "MOV":
		for _, a := range args[1:] {
			add(a)
		}
	case "ADD", "SUB", "MUL":
		if len(args) >= 2 {
			if len(args) == 2 {
				add(args[0]) // two-operand form reads its destination
			}
			for _, a := range args[1:] {
				add(a)
			}
		}
	case "LOAD", "LEA":
		if len(args) >= 2 {
			add(args[1])
		}
	case "STORE":
		if len(args) >= 2 {
			add(args[0])
			add(args[1])
		}
	case "CMP":
		for _, a := range args {
			add(a)
		}
	case "PUSH":
		if len(args) >= 1 {
			add(args[0])
		}
	case "SYSCALL", "INT", "CALL":
		// Calls and syscalls can read every argument register and are
		// capable of destroying any register. Conservatively mark all
		// virtual registers as read so DCE keeps argument setup.
		regs = append(regs, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12)
	default:
		for _, a := range args {
			add(a)
		}
	}
	return uniqueInts(regs)
}

func uniqueInts(in []int) []int {
	if len(in) < 2 {
		return in
	}
	seen := [13]bool{}
	out := in[:0]
	for _, v := range in {
		if v >= 0 && v < len(seen) && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func trimBrackets(s string) string {
	s = strings.TrimLeft(strings.TrimSpace(s), "[")
	s = strings.TrimRight(s, "]")
	return s
}

// extractLabel returns the static symbol at the base of a memory operand, or
// "" when the address is register-based, numeric, or otherwise dynamic.
func extractLabel(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return ""
	}
	inner := trimBrackets(s)
	if inner == "" {
		return ""
	}
	base := inner
	if idx := strings.IndexAny(base, "+-*/"); idx >= 0 {
		base = strings.TrimSpace(base[:idx])
	}
	if base == "" || asm.IsReg(base) || strings.HasPrefix(base, ".") ||
		strings.HasPrefix(base, "$") || strings.HasPrefix(base, "rel ") ||
		strings.HasPrefix(base, "abs ") {
		return ""
	}
	if _, err := strconv.ParseInt(base, 0, 64); err == nil {
		return ""
	}
	// A base with a physical register anywhere is not a static label.
	for _, tok := range asm.OperandRegs(inner) {
		if asm.IsPhysReg(tok) {
			return ""
		}
	}
	return base
}
