package vas

import (
	"fmt"
	"strings"
)

var regMap = map[string]string{
	"v0": "rax",
	"v1": "rdi",
	"v2": "rsi",
	"v3": "rdx",
	"v4": "rcx",
	"v5": "r8",
	"v6": "r9",
	"v7": "r10",
}

func mapReg(s string) string {
	for i := 19; i >= 0; i-- {
		old := fmt.Sprintf("v%d", i)
		if phys, ok := regMap[old]; ok {
			s = strings.ReplaceAll(s, old, phys)
		}
	}
	return s
}

func stripComment(line string) string {
	inQuote := false
	for i, ch := range line {
		if ch == '"' || ch == '\'' {
			inQuote = !inQuote
		}
		if !inQuote && (ch == '#' || ch == ';') {
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}

func Assemble(input string) (string, error) {
	lines := strings.Split(input, "\n")
	var outLines []string

	for _, line := range lines {
		original := line
		line = stripComment(line)
		line = strings.TrimSpace(line)

		if line == "" {
			outLines = append(outLines, original)
			continue
		}

		stripped := strings.TrimSpace(stripComment(original))
		if stripped == "" {
			outLines = append(outLines, original)
			continue
		}

		if strings.HasSuffix(line, ":") && !isInstruction(line) {
			outLines = append(outLines, mapReg(line))
			continue
		}

		result, err := processInstruction(line)
		if err != nil {
			return "", fmt.Errorf("line %q: %w", original, err)
		}
		outLines = append(outLines, result...)
	}

	return strings.Join(outLines, "\n"), nil
}

func isInstruction(s string) bool {
	upper := strings.ToUpper(strings.Fields(s)[0])
	switch upper {
	case "ADD", "SUB", "MUL", "LOAD", "STORE", "MOV", "MOVI",
		"CMP", "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE",
		"CALL", "RET", "NOP", "PUSH", "POP", "INT", "SYSCALL",
		"SECTION", "GLOBAL", "EXTERN", "DATA", "TEXT", "BSS",
		"ALIGN", "BYTE", "WORD", "DWORD", "QWORD", "DD", "DQ", "DB",
		"TYPE", "SIZE", "LENGTH", "START":
		return true
	}
	return false
}

func processInstruction(line string) ([]string, error) {
	tokens := splitTokens(line)
	if len(tokens) == 0 {
		return nil, nil
	}

	opcode := strings.ToUpper(tokens[0])
	args := tokens[1:]

	switch opcode {
	case "ADD":
		return expand2op("add", args)
	case "SUB":
		return expand2op("sub", args)
	case "MUL":
		return expandMul(args)
	case "LOAD":
		return expandLoad(args)
	case "STORE":
		return expandStore(args)
	case "MOVI":
		return expandMovi(args)
	case "MOV":
		return expandMov(args)
	case "CMP":
		return expandCmp(args)
	case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE", "CALL":
		return expandJump(opcode, args)
	case "RET":
		return []string{"\tret"}, nil
	case "NOP":
		return []string{"\tnop"}, nil
	case "PUSH":
		return expandPush(args)
	case "POP":
		return expandPop(args)
	case "SYSCALL":
		return []string{"\tsyscall"}, nil
	case "INT":
		return expandInt(args)
	default:
		return []string{"\t" + mapReg(line)}, nil
	}
}

func splitTokens(line string) []string {
	var tokens []string
	var cur strings.Builder
	inBracket := false

	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, strings.TrimSpace(cur.String()))
			cur.Reset()
		}
	}

	for _, ch := range line {
		switch {
		case ch == '[':
			inBracket = true
			cur.WriteRune(ch)
		case ch == ']':
			inBracket = false
			cur.WriteRune(ch)
		case (ch == ',' || ch == '\t' || ch == ' ') && !inBracket:
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	flush()
	return tokens
}


func expand2op(mnemonic string, args []string) ([]string, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("%s expects 2 or 3 operands, got %d", mnemonic, len(args))
	}
	dst := mapReg(args[0])
	if len(args) == 3 {
		src1 := mapReg(args[1])
		src2 := mapReg(args[2])
		return []string{
			fmt.Sprintf("\tmov\t%s, %s", dst, src1),
			fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src2),
		}, nil
	}
	src := mapReg(args[1])
	return []string{fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src)}, nil
}

func expandMul(args []string) ([]string, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("MUL expects 2 or 3 operands, got %d", len(args))
	}
	dst := mapReg(args[0])
	if len(args) == 3 {
		src1 := mapReg(args[1])
		src2 := mapReg(args[2])
		return []string{
			fmt.Sprintf("\tmov\t%s, %s", dst, src1),
			fmt.Sprintf("\timul\t%s, %s", dst, src2),
		}, nil
	}
	src := mapReg(args[1])
	return []string{fmt.Sprintf("\timul\t%s, %s", dst, src)}, nil
}

func expandLoad(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("LOAD expects 2 operands, got %d", len(args))
	}
	dst := mapReg(args[0])
	mem := mapReg(args[1])
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, mem)}, nil
}

func expandStore(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("STORE expects 2 operands, got %d", len(args))
	}
	src := mapReg(args[0])
	mem := mapReg(args[1])
	return []string{fmt.Sprintf("\tmov\t%s, %s", mem, src)}, nil
}

func expandMovi(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("MOVI expects 2 operands, got %d", len(args))
	}
	dst := mapReg(args[0])
	imm := args[1]
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, imm)}, nil
}

func expandMov(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("MOV expects 2 operands, got %d", len(args))
	}
	dst := mapReg(args[0])
	src := mapReg(args[1])
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, src)}, nil
}

func expandCmp(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("CMP expects 2 operands, got %d", len(args))
	}
	a := mapReg(args[0])
	b := mapReg(args[1])
	return []string{fmt.Sprintf("\tcmp\t%s, %s", a, b)}, nil
}

func expandJump(opcode string, args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 operand, got %d", opcode, len(args))
	}
	target := mapReg(args[0])
	mnemonic := strings.ToLower(opcode)
	return []string{fmt.Sprintf("\t%s\t%s", mnemonic, target)}, nil
}

func expandPush(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("PUSH expects 1 operand, got %d", len(args))
	}
	return []string{fmt.Sprintf("\tpush\t%s", mapReg(args[0]))}, nil
}

func expandPop(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("POP expects 1 operand, got %d", len(args))
	}
	return []string{fmt.Sprintf("\tpop\t%s", mapReg(args[0]))}, nil
}

func expandInt(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("INT expects 1 operand, got %d", len(args))
	}
	return []string{fmt.Sprintf("\tint\t%s", args[0])}, nil
}
