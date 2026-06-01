package vas

import (
 "fmt"
 "strconv"
 "strings"
)

var regMap = map[string]string{
	"v0": "rax",
	"v1": "rbx",
	"v2": "rcx",
	"v3": "rdx",
	"v4": "rsi",
	"v5": "rdi",
	"v6": "r8",
	"v7": "r9",
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
	upper = strings.TrimLeft(upper, ".")
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
	case "LEA":
		return expandLea(args)
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
		out := mapReg(line)
		t := strings.TrimLeft(out, " \t")
		if strings.HasPrefix(t, ".") {
			// GAS → NASM: strip leading dot from directive keyword
			// .section → section, .global → global
			t = t[1:]
			ws := out[:len(out)-len(strings.TrimLeft(out, " \t"))]
			out = ws + t
		}
		return []string{"\t" + out}, nil
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
		var lines []string
		if dst != src1 {
			lines = append(lines, fmt.Sprintf("\tmov\t%s, %s", dst, src1))
		}
		lines = append(lines, fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src2))
		return lines, nil
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
		var lines []string
		if dst != src1 {
			lines = append(lines, fmt.Sprintf("\tmov\t%s, %s", dst, src1))
		}
		lines = append(lines, fmt.Sprintf("\timul\t%s, %s", dst, src2))
		return lines, nil
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

func expandLea(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("LEA expects 2 operands, got %d", len(args))
	}
	dst := mapReg(args[0])
	mem := mapReg(args[1])
	return []string{fmt.Sprintf("\tlea\t%s, %s", dst, mem)}, nil
}

// AssembleStandalone assembles VAS and wraps with a minimal ELF64 skeleton
// if the input contains no section/global/extern boilerplate.
func AssembleStandalone(input string) (string, error) {
	return AssembleStandaloneTarget(input, "elf64")
}

func AssembleStandaloneTarget(input, target string) (string, error) {
	asm, err := Assemble(input)
	if err != nil {
		return "", err
	}
	if hasBoilerplate(asm) {
		return asm, nil
	}
	switch target {
	case "win64":
		return wrapStandaloneWin64(input, asm), nil
	default:
		return wrapStandalone(input, asm), nil
	}
}

func hasBoilerplate(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "\tsection ") ||
		strings.Contains(lower, "\tglobal ") ||
		strings.Contains(lower, "\textern ")
}

func wrapStandalone(vasInput, asmOutput string) string {
	memRefs := collectMemRefs(vasInput)

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")
	sb.WriteString("\tglobal _start\n")
	sb.WriteString("_start:\n")
	sb.WriteString(asmOutput)
	sb.WriteString("\n")

	// Only append exit if the last instruction isn't already a syscall
	trimmed := strings.TrimSpace(asmOutput)
	if !strings.HasSuffix(trimmed, "syscall") {
		sb.WriteString("\txor\tedi, edi\n")
		sb.WriteString("\tmov\teax, 60\n")
		sb.WriteString("\tsyscall\n")
	}

	if len(memRefs) > 0 {
		sb.WriteString("\n\tsection .data\n")
		for _, ref := range memRefs {
			sb.WriteString(fmt.Sprintf("%s:\tdq 0\n", ref))
		}
	}

	return sb.String()
}

func wrapStandaloneWin64(vasInput, asmOutput string) string {
	memRefs := collectMemRefs(vasInput)

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")
	sb.WriteString("\tglobal main\n")
	sb.WriteString("main:\n")
	sb.WriteString(asmOutput)
	sb.WriteString("\n")

	// On Windows, exit via ret (rax = 0)
	trimmed := strings.TrimSpace(asmOutput)
	if !strings.HasSuffix(trimmed, "ret") {
		sb.WriteString("\txor\teax, eax\n")
		sb.WriteString("\tret\n")
	}

	if len(memRefs) > 0 {
		sb.WriteString("\n\tsection .data\n")
		for _, ref := range memRefs {
			sb.WriteString(fmt.Sprintf("%s:\tdq 0\n", ref))
		}
	}

	return sb.String()
}

func collectMemRefs(input string) []string {
	var refs []string
	seen := map[string]bool{}

	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(stripComment(line))
		if trimmed == "" {
			continue
		}
		// Find all [...] patterns
		for {
			start := strings.Index(trimmed, "[")
			if start == -1 {
				break
			}
			end := strings.Index(trimmed[start:], "]")
			if end == -1 {
				break
			}
			inner := strings.TrimSpace(trimmed[start+1 : start+end])
			trimmed = trimmed[start+end+1:]

			// Extract the base symbol (before + - or *)
			sym := inner
			for _, sep := range []string{"+", "-", "*"} {
				if idx := strings.Index(sym, sep); idx > 0 {
					sym = strings.TrimSpace(sym[:idx])
					break
				}
			}
			if sym == "" || seen[sym] {
				continue
			}
			if isRegister(sym) {
				continue
			}
			seen[sym] = true
			refs = append(refs, sym)
		}
	}
	return refs
}

func isRegister(s string) bool {
	if strings.HasPrefix(s, "v") {
		_, err := strconv.Atoi(s[1:])
		return err == nil
	}
	phys := []string{"rax", "rbx", "rcx", "rdx", "rsi", "rdi", "rbp", "rsp",
		"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
		"eax", "ebx", "ecx", "edx", "esi", "edi", "ebp", "esp",
		"ax", "bx", "cx", "dx", "si", "di", "bp", "sp",
		"al", "bl", "cl", "dl", "ah", "bh", "ch", "dh"}
	for _, r := range phys {
		if s == r {
			return true
		}
	}
	return false
}
