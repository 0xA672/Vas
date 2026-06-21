package vas

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"vas/vas/lint"
	"vas/vas/opt"
)

// OptConfig controls optimization and lint behaviour.
// All fields are optional; zero values produce safe defaults.
type OptConfig struct {
	Level      int  // 0=no opt, 1=-O1, 2=-O2
	Explain    bool // annotate output with [OPT] comments
	FailOnLint bool // treat lint errors as fatal (default true)
}

// AssembleWithOpt assembles VAS source with the given optimization level.
// Assembles VAS source with the given optimization configuration.
// Lint errors are fatal by default; set cfg.FailOnLint=false to downgrade
// them to warnings and continue assembly.
func AssembleWithOpt(input string, cfg OptConfig) (string, error) {
	return AssembleWithOptWithHook(input, cfg, nil, nil)
}

// AssembleWithOptWithHook is the same as AssembleWithOpt but allows pre/post hooks.
func AssembleWithOptWithHook(input string, cfg OptConfig, preHook func(string) (string, error), postHook func(string) (string, error)) (string, error) {
	// Preprocessing: if the source contains any preprocessor directive,
	// run the full preprocessor before anything else.
	if hasPreprocessorDirectives(input) {
		preprocessed, err := Preprocess(input, "")
		if err != nil {
			return "", fmt.Errorf("preprocessing: %w", err)
		}
		input = preprocessed
	}

	if preHook != nil {
		var err error
		input, err = preHook(input)
		if err != nil {
			return "", err
		}
	}

	// ── Semantic lint for dangerous instructions ─────────────────────────
	// Runs after preprocessing so we can see the expanded source.
	violations := lint.Run(input)
	hasError := false
	for _, v := range violations {
		if v.Severity == "error" {
			hasError = true
			fmt.Fprintf(os.Stderr, "lint error at line %d: %s\n  Suggested fix: %s\n", v.Line, v.Message, v.Fix)
		} else {
			fmt.Fprintf(os.Stderr, "lint warning at line %d: %s\n  Suggested fix: %s\n", v.Line, v.Message, v.Fix)
		}
	}
	if hasError && cfg.FailOnLint {
		return "", fmt.Errorf("lint errors found (use --warn-only or set FailOnLint=false to suppress)")
	}

	// Set explain mode before optimization
	if cfg.Explain {
		opt.SetExplain(true)
		defer opt.SetExplain(false)
	}

	lines := strings.Split(input, "\n")
	// Pre-expansion optimization: constant folding
	if cfg.Level >= 1 {
		lines = opt.FoldConstants(lines)
		input = strings.Join(lines, "\n")
	}

	// Pre-expansion optimization: dead code elimination and other passes
	if cfg.Level >= 1 {
		input = opt.Optimize(input, cfg.Level)
	}

	lines = strings.Split(input, "\n")
	var outLines []string

	for lineNum, line := range lines {
		original := line
		stripped := strings.TrimSpace(stripComment(original))
		if stripped == "" {
			outLines = append(outLines, original)
			continue
		}

		if strings.HasSuffix(stripped, ":") && !isInstruction(stripped) {
			checkNasmKeyword(stripped)
			mapped, err := mapReg(stripped)
			if err != nil {
				return "", fmt.Errorf("line %d: %q: %w", lineNum+1, strings.TrimRight(original, "\r"), err)
			}
			// Preserve original indentation/prefix before the label
			prefix := original[:len(original)-len(strings.TrimLeft(original, " \t"))]
			outLines = append(outLines, prefix+mapped)
			continue
		}

		result, err := processInstruction(stripped)
		if err != nil {
			return "", fmt.Errorf("line %d: %q: %w", lineNum+1, strings.TrimRight(original, "\r"), err)
		}
		outLines = append(outLines, result...)
	}

	output := strings.Join(outLines, "\n")

	// The peephole passes inside Optimize run on VAS source lines and
	// use regexes that require tab-prefixed lowercase opcodes — they are
	// effectively no-ops on VAS source. Run PeepholeOnly here on the fully
	// expanded nasm output so passes like pushModPopElim can eliminate
	// patterns such as  "push rbx; add rbx, r8; pop rbx".
	// These aggressive peephole optimizations are gated at O2 and above
	// to match the opt_showcase behavior where O2 output is strictly
	// shorter than O1.
	if cfg.Level >= 2 {
		output = opt.PeepholeOnly(output)
	}

	if postHook != nil {
		var err error
		output, err = postHook(output)
		if err != nil {
			return "", err
		}
	}

	return output, nil
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

// Assemble is a convenience wrapper that assembles with no optimization.
func Assemble(input string) (string, error) {
	return AssembleWithOpt(input, OptConfig{})
}

// hasPreprocessorDirectives checks whether src contains any preprocessor directive.
func hasPreprocessorDirectives(src string) bool {
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ".include") ||
			strings.HasPrefix(trimmed, ".const") ||
			strings.HasPrefix(trimmed, ".macro") ||
			strings.HasPrefix(trimmed, ".ifdef") ||
			strings.HasPrefix(trimmed, ".ifndef") ||
			strings.HasPrefix(trimmed, ".once") ||
			strings.HasPrefix(trimmed, ".rept") ||
			strings.HasPrefix(trimmed, ".endm") ||
			strings.HasPrefix(trimmed, ".endr") ||
			strings.HasPrefix(trimmed, ".endif") ||
			strings.HasPrefix(trimmed, ".else") ||
			strings.HasPrefix(trimmed, ".include_bytes") {
			return true
		}
	}
	return false
}

// stripComment removes trailing comments from line, respecting string literals.
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

var nasmKeywords = map[string]bool{
	"ptr":  true,
	"byte": true, "word": true, "dword": true, "qword": true, "tword": true, "oword": true,
	"short": true, "near": true, "far": true,
	"to": true, "strict": true, "nosplit": true, "rel": true, "abs": true,
	"seg": true, "wrt": true,
}

func checkNasmKeyword(line string) {
	trimmed := strings.TrimSpace(line)
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if trimmed == "" {
		return
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return
	}
	upper := strings.ToUpper(fields[0])
	switch upper {
	case "SECTION", "GLOBAL", "EXTERN", "ALIGN", "DB", "DW", "DD", "DQ",
		"BYTE", "WORD", "DWORD", "QWORD", "TYPE", "SIZE", "LENGTH", "START",
		"RESB", "RESW", "RESD", "RESQ", "EQU", "TIMES", "INCBIN":
		return
	}
	first := fields[0]
	hasColon := strings.HasSuffix(first, ":")
	if hasColon {
		first = first[:len(first)-1]
	}
	if nasmKeywords[strings.ToLower(first)] {
		fmt.Fprintf(os.Stderr, "warning: %q is a NASM reserved keyword", first)
		if hasColon {
			fmt.Fprintf(os.Stderr, " (used as label)")
		}
		fmt.Fprintf(os.Stderr, " - may cause assembly errors\n")
	}
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
		checkNasmKeyword(line)
		mapped, err := mapReg(line)
		if err != nil {
			return nil, err
		}
		t := strings.TrimLeft(mapped, " \t")
		if strings.HasPrefix(t, ".") {
			t = t[1:]
			if strings.HasPrefix(t, "globl") {
				t = "global" + t[5:]
			}
			if t == "data" || strings.HasPrefix(t, "data ") || strings.HasPrefix(t, "data\t") {
				t = "section .data"
			} else if t == "bss" || strings.HasPrefix(t, "bss ") || strings.HasPrefix(t, "bss\t") {
				t = "section .bss"
			} else if t == "text" || strings.HasPrefix(t, "text ") || strings.HasPrefix(t, "text\t") {
				t = "section .text"
			}
			ws := mapped[:len(mapped)-len(strings.TrimLeft(mapped, " \t"))]
			mapped = ws + t
		}
		return []string{"\t" + mapped}, nil
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
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	if len(args) == 3 {
		src1, err := mapReg(args[1])
		if err != nil {
			return nil, err
		}
		src2, err := mapReg(args[2])
		if err != nil {
			return nil, err
		}
		var lines []string
		if dst == src2 {
			if mnemonic == "add" || mnemonic == "imul" {
				lines = append(lines, fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src1))
			} else {
				lines = append(lines, fmt.Sprintf("\tmov\tr10, %s", src2))
				if dst != src1 {
					lines = append(lines, fmt.Sprintf("\tmov\t%s, %s", dst, src1))
				}
				lines = append(lines, fmt.Sprintf("\t%s\t%s, r10", mnemonic, dst))
			}
		} else {
			if dst != src1 {
				lines = append(lines, fmt.Sprintf("\tmov\t%s, %s", dst, src1))
			}
			lines = append(lines, fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src2))
		}
		return lines, nil
	}
	src, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\t%s\t%s, %s", mnemonic, dst, src)}, nil
}

func expandMul(args []string) ([]string, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("MUL expects 2 or 3 operands, got %d", len(args))
	}
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	if len(args) == 3 {
		src1, err := mapReg(args[1])
		if err != nil {
			return nil, err
		}
		src2, err := mapReg(args[2])
		if err != nil {
			return nil, err
		}
		var lines []string
		if dst == src2 {
			lines = append(lines, fmt.Sprintf("\timul\t%s, %s", dst, src1))
		} else {
			if dst != src1 {
				lines = append(lines, fmt.Sprintf("\tmov\t%s, %s", dst, src1))
			}
			lines = append(lines, fmt.Sprintf("\timul\t%s, %s", dst, src2))
		}
		return lines, nil
	}
	src, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\timul\t%s, %s", dst, src)}, nil
}

func expandLoad(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("LOAD expects 2 operands, got %d", len(args))
	}
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	mem, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, mem)}, nil
}

func expandStore(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("STORE expects 2 operands, got %d", len(args))
	}
	src, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	mem, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tmov\t%s, %s", mem, src)}, nil
}

func expandMovi(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("MOVI expects 2 operands, got %d", len(args))
	}
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	imm := args[1]
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, imm)}, nil
}

func expandMov(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("MOV expects 2 operands, got %d", len(args))
	}
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	src, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tmov\t%s, %s", dst, src)}, nil
}

func expandCmp(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("CMP expects 2 operands, got %d", len(args))
	}
	a, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	b, err := mapReg(args[1])
	if err != nil {
		if _, parseErr := strconv.ParseInt(args[1], 0, 64); parseErr == nil {
			b = args[1]
		} else {
			return nil, err
		}
	}
	return []string{fmt.Sprintf("\tcmp\t%s, %s", a, b)}, nil
}

func expandJump(opcode string, args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%s expects 1 operand, got %d", opcode, len(args))
	}
	target, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	mnemonic := strings.ToLower(opcode)
	return []string{fmt.Sprintf("\t%s\t%s", mnemonic, target)}, nil
}

func expandPush(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("PUSH expects 1 operand, got %d", len(args))
	}
	reg, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tpush\t%s", reg)}, nil
}

func expandPop(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("POP expects 1 operand, got %d", len(args))
	}
	reg, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tpop\t%s", reg)}, nil
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
	dst, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	mem, err := mapReg(args[1])
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(mem, "[") {
		mem = "[" + mem + "]"
	}
	return []string{fmt.Sprintf("\tlea\t%s, %s", dst, mem)}, nil
}

// AssembleStandalone assembles VAS and wraps with a minimal ELF64 skeleton
// if the input contains no section/global/extern boilerplate.
func AssembleStandalone(input string) (string, error) {
	return AssembleStandaloneTarget(input, "elf64")
}

func AssembleStandaloneTarget(input, target string) (string, error) {
	return AssembleStandaloneTargetOpt(input, target, 0)
}

// AssembleStandaloneTargetOpt assembles VAS with optimization level and wraps
// with a platform-appropriate skeleton.
func AssembleStandaloneTargetOpt(input, target string, optLevel int) (string, error) {
	asm, err := AssembleWithOpt(input, OptConfig{Level: optLevel})
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

// hasBoilerplate checks if the assembled output already defines a .text section.
func hasBoilerplate(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if strings.ToLower(trimmed) == "section .text" {
			return true
		}
	}
	return false
}

func wrapStandalone(vasInput, asmOutput string) string {
	memRefs := collectMemRefs(vasInput)

	lines := strings.Split(asmOutput, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "global ") {
			continue
		}
		filtered = append(filtered, line)
	}
	asmOutput = strings.Join(filtered, "\n")

	hasStart := strings.Contains(asmOutput, "_start:")
	hasMain := strings.Contains(asmOutput, "main:")

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")

	if !hasStart && !hasMain {
		sb.WriteString("\tglobal _start\n")
		sb.WriteString("_start:\n")
		sb.WriteString("\tcall\tvas_main\n")
		sb.WriteString("\tmov\tedi, eax\n")
		sb.WriteString("\tmov\teax, 60\n")
		sb.WriteString("\tsyscall\n")
	} else if hasMain && !hasStart {
		sb.WriteString("\tglobal main\n")
	} else if hasStart {
		sb.WriteString("\tglobal _start\n")
	}

	sb.WriteString("vas_main:\n")
	sb.WriteString(asmOutput)
	sb.WriteString("\n")

	lastInst := lastInstructionLine(asmOutput)
	if !strings.HasSuffix(lastInst, "ret") &&
		!strings.HasPrefix(lastInst, "syscall") &&
		!strings.HasPrefix(lastInst, "jmp") &&
		!strings.HasPrefix(lastInst, "hlt") {
		sb.WriteString("\tret\n")
	}

	if len(memRefs) > 0 {
		var dataLines []string
		for _, ref := range memRefs {
			if !strings.Contains(asmOutput, ref+":") {
				dataLines = append(dataLines, fmt.Sprintf("%s:\tdq 0\n", ref))
			}
		}
		if len(dataLines) > 0 {
			sb.WriteString("\n\tsection .data\n")
			for _, line := range dataLines {
				sb.WriteString(line)
			}
		}
	}

	return sb.String()
}

func wrapStandaloneWin64(vasInput, asmOutput string) string {
	memRefs := collectMemRefs(vasInput)

	lines := strings.Split(asmOutput, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "global ") {
			continue
		}
		filtered = append(filtered, line)
	}
	asmOutput = strings.Join(filtered, "\n")

	hasStart := strings.Contains(asmOutput, "_start:")
	hasMain := strings.Contains(asmOutput, "main:")

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")

	if !hasStart && !hasMain {
		sb.WriteString("\tglobal main\n")
		sb.WriteString("main:\n")
	} else if hasMain && !hasStart {
		sb.WriteString("\tglobal main\n")
	} else if hasStart {
		sb.WriteString("\tglobal _start\n")
	}

	sb.WriteString(asmOutput)
	sb.WriteString("\n")

	lastInst := lastInstructionLine(asmOutput)
	if !strings.HasSuffix(lastInst, "ret") {
		sb.WriteString("\txor\teax, eax\n")
		sb.WriteString("\tret\n")
	}

	if len(memRefs) > 0 {
		var dataLines []string
		for _, ref := range memRefs {
			if !strings.Contains(asmOutput, ref+":") {
				dataLines = append(dataLines, fmt.Sprintf("%s:\tdq 0\n", ref))
			}
		}
		if len(dataLines) > 0 {
			sb.WriteString("\n\tsection .data\n")
			for _, line := range dataLines {
				sb.WriteString(line)
			}
		}
	}

	return sb.String()
}

// lastInstructionLine returns the last line that looks like an instruction.
func lastInstructionLine(asmOutput string) string {
	lines := strings.Split(asmOutput, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "section ") || strings.HasPrefix(line, "global ") ||
			strings.HasPrefix(line, "extern ") || strings.HasPrefix(line, "default ") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			continue
		}
		if strings.Contains(line, ":\t") || strings.Contains(line, ": ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				switch strings.ToLower(fields[1]) {
				case "dq", "db", "dd", "dw", "resq", "resb", "equ", "times":
					continue
				}
			}
		}
		return line
	}
	return ""
}

func collectMemRefs(input string) []string {
	var refs []string
	seen := map[string]bool{}

	var collectFrom func(text string)
	collectFrom = func(text string) {
	loop:
		for {
			start := strings.Index(text, "[")
			if start == -1 {
				break loop
			}
			depth := 1
			end := -1
		scan:
			for j := start + 1; j < len(text); j++ {
				switch text[j] {
				case '[':
					depth++
				case ']':
					depth--
					if depth == 0 {
						end = j
						break scan
					}
				}
			}
			if end == -1 {
				break loop
			}
			inner := strings.TrimSpace(text[start+1 : end])
			text = text[end+1:]

			collectFrom(inner)

			sym := inner
			if idx := strings.Index(sym, "+"); idx > 0 {
				sym = strings.TrimSpace(sym[:idx])
			} else if idx := strings.Index(sym, "-"); idx > 0 {
				sym = strings.TrimSpace(sym[:idx])
			} else if idx := strings.Index(sym, "*"); idx > 0 {
				sym = strings.TrimSpace(sym[:idx])
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

	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(stripComment(line))
		if trimmed == "" {
			continue
		}
		collectFrom(trimmed)
	}
	return refs
}

func isRegister(s string) bool { return isPhysReg(s) || IsValidVirtualReg(s) }
