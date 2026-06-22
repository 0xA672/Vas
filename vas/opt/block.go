package opt

import (
	"fmt"
	"strings"
)

// deadCodeElim removes writes to virtual registers that are overwritten
// before being read within the same basic block.
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
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			current = append(current, line)
			continue
		}
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
// beyond writing its destination register.
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
		if _, err := fmt.Sscanf(before, "%d", new(int)); err == nil {
			return ""
		}
		return before
	}
	s = strings.TrimRight(s, "]")
	s = strings.TrimSpace(s)
	if regIndex(s) >= 0 {
		return ""
	}
	if _, err := fmt.Sscanf(s, "%d", new(int)); err == nil {
		return ""
	}
	return s
}

func stripComment(line string) string {
	trimmed := strings.TrimSpace(line)
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}
