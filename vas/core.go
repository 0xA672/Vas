package vas

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"vas/vas/internal/asm"
	"vas/vas/lint"
	"vas/vas/opt"
)

// OptConfig controls optimization and lint behaviour.
// All fields are optional; zero values produce safe defaults.
type OptConfig struct {
	Level    int    // 0=no opt, 1=-O1, 2=-O2
	Explain  bool   // annotate output with [OPT] comments
	WarnOnly bool   // downgrade lint errors to warnings
	Target   string // elf64 (default) or win64; affects platform defines
	BaseDir  string // base directory for .include resolution

	// FailOnLint is retained for source compatibility. Lint errors are fatal
	// by default now, so the zero value is safe; set WarnOnly to opt out.
	// Deprecated: use WarnOnly.
	FailOnLint bool
}

// AssembleWithOpt assembles VAS source with the given optimization level.
// Lint errors are fatal by default; set cfg.WarnOnly to downgrade them to
// warnings and continue assembly.
func AssembleWithOpt(input string, cfg OptConfig) (string, error) {
	return AssembleWithOptWithHook(input, cfg, nil, nil)
}

// AssembleWithOptWithHook is the same as AssembleWithOpt but allows pre/post hooks.
func AssembleWithOptWithHook(input string, cfg OptConfig, preHook func(string) (string, error), postHook func(string) (string, error)) (string, error) {
	// Preprocessing: if the source contains any preprocessor directive,
	// run the full preprocessor before anything else.
	if hasPreprocessorDirectives(input) {
		var opts []PreprocessOption
		if cfg.Target != "" {
			opts = append(opts, WithTarget(cfg.Target))
		}
		preprocessed, err := Preprocess(input, cfg.BaseDir, opts...)
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
	if hasError && !cfg.WarnOnly {
		return "", fmt.Errorf("%w (use --warn-only to suppress)", ErrLintError)
	}

	lines := strings.Split(input, "\n")
	// Pre-expansion optimization: constant folding + full pipeline on []string
	if cfg.Level >= 1 {
		lines = opt.FoldConstants(lines)
		lines = opt.OptimizeLinesWithExplain(lines, cfg.Level, cfg.Explain)
	}

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

	// Peephole runs on the fully expanded NASM output. Every rewrite that
	// changes EFLAGS is gated by the optimizer's flag-liveness analysis, so it
	// is safe to run this at both -O1 and -O2.
	if cfg.Level >= 1 {
		outLines = opt.PeepholeOnlyLinesWithExplain(outLines, cfg.Explain)
	}
	output := strings.Join(outLines, "\n")

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
		trimmed := strings.TrimSpace(asm.StripComment(line))
		switch {
		case strings.HasPrefix(trimmed, ".include"),
			strings.HasPrefix(trimmed, ".const"),
			strings.HasPrefix(trimmed, ".macro"),
			strings.HasPrefix(trimmed, ".ifdef"),
			strings.HasPrefix(trimmed, ".ifndef"),
			strings.HasPrefix(trimmed, ".once"),
			strings.HasPrefix(trimmed, ".rept"),
			strings.HasPrefix(trimmed, ".endm"),
			strings.HasPrefix(trimmed, ".endr"),
			strings.HasPrefix(trimmed, ".endif"),
			strings.HasPrefix(trimmed, ".else"),
			strings.HasPrefix(trimmed, ".author"),
			strings.HasPrefix(trimmed, ".error"),
			strings.HasPrefix(trimmed, ".test"),
			strings.HasPrefix(trimmed, ".endtest"):
			return true
		}
	}
	return false
}

// StripComment removes an inline ; or # comment while preserving quoted text.
// It is exported for CLI tooling that needs the same lexer as the assembler.
func StripComment(line string) string { return asm.StripComment(line) }

// stripComment removes trailing comments from line, respecting string literals.
func stripComment(line string) string {
	return asm.StripComment(line)
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
	code := strings.TrimSpace(line)
	if code == "" {
		return nil
	}
	i := 0
	for i < len(code) && code[i] != ' ' && code[i] != '\t' {
		i++
	}
	tokens := []string{code[:i]}
	return append(tokens, asm.SplitOperands(code[i:])...)
}

func expand2op(mnemonic string, args []string) ([]string, error) {
	if len(args) < 2 || len(args) > 3 {
		return nil, fmt.Errorf("%w: %s expects 2 or 3 operands, got %d", ErrOperandCount, mnemonic, len(args))
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
		return nil, fmt.Errorf("%w: MUL expects 2 or 3 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: LOAD expects 2 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: STORE expects 2 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: MOVI expects 2 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: MOV expects 2 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: CMP expects 2 operands, got %d", ErrOperandCount, len(args))
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
		return nil, fmt.Errorf("%w: %s expects 1 operand, got %d", ErrOperandCount, opcode, len(args))
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
		return nil, fmt.Errorf("%w: PUSH expects 1 operand, got %d", ErrOperandCount, len(args))
	}
	reg, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tpush\t%s", reg)}, nil
}

func expandPop(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: POP expects 1 operand, got %d", ErrOperandCount, len(args))
	}
	reg, err := mapReg(args[0])
	if err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("\tpop\t%s", reg)}, nil
}

func expandInt(args []string) ([]string, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("%w: INT expects 1 operand, got %d", ErrOperandCount, len(args))
	}
	return []string{fmt.Sprintf("\tint\t%s", args[0])}, nil
}

func expandLea(args []string) ([]string, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: LEA expects 2 operands, got %d", ErrOperandCount, len(args))
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
	return AssembleStandaloneTargetOptConfig(input, target, OptConfig{Level: optLevel})
}

// AssembleStandaloneTargetOptConfig is AssembleStandaloneTargetOpt with the
// full OptConfig (warn-only lint, target defines, include base directory).
func AssembleStandaloneTargetOptConfig(input, target string, cfg OptConfig) (string, error) {
	if target != "" {
		cfg.Target = target
	}
	asm, err := AssembleWithOpt(input, cfg)
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

// hasBoilerplate checks if the assembled output already defines a .text
// section, including forms with attributes such as "section .text align=16".
func hasBoilerplate(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(asm.StripComment(line))
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(strings.ToLower(trimmed))
		if len(fields) >= 2 && fields[0] == "section" && strings.HasPrefix(fields[1], ".text") {
			return true
		}
	}
	return false
}

func hasLabel(asmOutput, name string) bool {
	for _, line := range strings.Split(asmOutput, "\n") {
		code := strings.TrimSpace(asm.StripComment(line))
		if strings.HasPrefix(strings.ToLower(code), strings.ToLower(name)+":") {
			return true
		}
	}
	return false
}

func stripGlobalDirectives(asmOutput string) string {
	lines := strings.Split(asmOutput, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "global ") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func autoDataSection(asmOutput string) string {
	refs := collectMemRefs(asmOutput)
	var dataLines []string
	for _, ref := range refs {
		if !hasLabel(asmOutput, ref) {
			dataLines = append(dataLines, fmt.Sprintf("%s:\tdq 0\n", ref))
		}
	}
	if len(dataLines) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\tsection .data\n")
	for _, line := range dataLines {
		sb.WriteString(line)
	}
	return sb.String()
}

func wrapStandalone(vasInput, asmOutput string) string {
	asmOutput = stripGlobalDirectives(asmOutput)
	hasStart := hasLabel(asmOutput, "_start")
	hasMain := hasLabel(asmOutput, "main")
	autoData := autoDataSection(asmOutput)

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")

	switch {
	case hasStart:
		// The user owns the entry point; do not synthesize another one.
		sb.WriteString("\tglobal _start\n")
		sb.WriteString(asmOutput)
		sb.WriteString("\n")
	case hasMain:
		// entry for a user-provided main()
		sb.WriteString("\tglobal _start\n")
		sb.WriteString("_start:\n")
		sb.WriteString("\tcall\tmain\n")
		sb.WriteString("\tmov\trdi, rax\n")
		sb.WriteString("\tmov\teax, 60\n")
		sb.WriteString("\tsyscall\n\n")
		sb.WriteString(asmOutput)
		sb.WriteString("\n")
	default:
		sb.WriteString("\tglobal _start\n")
		sb.WriteString("_start:\n")
		sb.WriteString("\tcall\tvas_main\n")
		sb.WriteString("\tmov\trdi, rax\n")
		sb.WriteString("\tmov\teax, 60\n")
		sb.WriteString("\tsyscall\n\n")
		sb.WriteString("vas_main:\n")
		sb.WriteString(asmOutput)
		sb.WriteString("\n")
	}

	lastInst := lastInstructionLine(asmOutput)
	if lastInst == "" ||
		(!strings.HasSuffix(lastInst, "ret") &&
			!strings.HasPrefix(lastInst, "syscall") &&
			!strings.HasPrefix(lastInst, "jmp") &&
			!strings.HasPrefix(lastInst, "hlt")) {
		sb.WriteString("\tret\n")
	}

	sb.WriteString(autoData)
	return sb.String()
}

func wrapStandaloneWin64(vasInput, asmOutput string) string {
	asmOutput = stripGlobalDirectives(asmOutput)
	hasStart := hasLabel(asmOutput, "_start")
	hasMain := hasLabel(asmOutput, "main")
	autoData := autoDataSection(asmOutput)

	var sb strings.Builder
	sb.WriteString("\tdefault rel\n\n")
	sb.WriteString("\tsection .text\n")
	if hasStart && !hasMain {
		sb.WriteString("\tglobal main\n")
		sb.WriteString("main:\n")
		sb.WriteString("\tcall\t_start\n")
		sb.WriteString("\txor\teax, eax\n")
		sb.WriteString("\tret\n\n")
	} else {
		sb.WriteString("\tglobal main\n")
	}
	if !hasMain && !hasStart {
		sb.WriteString("main:\n")
	}
	sb.WriteString(asmOutput)
	sb.WriteString("\n")

	lastInst := lastInstructionLine(asmOutput)
	if lastInst != "" && !strings.HasSuffix(lastInst, "ret") {
		sb.WriteString("\txor\teax, eax\n")
		sb.WriteString("\tret\n")
	}

	sb.WriteString(autoData)
	return sb.String()
}

// lastInstructionLine returns the last line that looks like an instruction.
func lastInstructionLine(asmOutput string) string {
	lines := strings.Split(asmOutput, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(asm.StripComment(lines[i]))
		if line == "" {
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

// collectMemRefs returns static symbols referenced by bracketed memory
// operands. It scans the expanded assembly, skips quoted text, register-only
// addresses and NASM modifiers, and therefore works with macros and includes.
func collectMemRefs(input string) []string {
	var refs []string
	seen := map[string]bool{}

	for _, line := range strings.Split(input, "\n") {
		code := asm.StripComment(line)
		quote := byte(0)
		for i := 0; i < len(code); i++ {
			c := code[i]
			if quote != 0 {
				if c == '\\' {
					i++
				} else if c == quote {
					quote = 0
				}
				continue
			}
			switch c {
			case '\'', '"', '`':
				quote = c
			case '[':
				depth := 1
				j := i + 1
				for ; j < len(code); j++ {
					switch code[j] {
					case '[':
						depth++
					case ']':
						depth--
					}
					if depth == 0 {
						break
					}
				}
				if depth != 0 {
					return refs
				}
				collectAddressSymbols(code[i+1:j], &refs, seen)
				i = j
			}
		}
	}
	return refs
}

func collectAddressSymbols(inner string, refs *[]string, seen map[string]bool) {
	// Replace address operators with spaces, then inspect every remaining
	// token. Nested brackets are handled recursively by the caller's scanner.
	var normalized strings.Builder
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch c {
		case '+', '-', '*', '/', '?', '(', ')', ',', '[', ']':
			normalized.WriteByte(' ')
		default:
			normalized.WriteByte(c)
		}
	}
	for _, tok := range strings.Fields(normalized.String()) {
		tok = strings.TrimSpace(tok)
		if tok == "" || seen[tok] || strings.ContainsAny(tok, ":[]\"'`$%") {
			continue
		}
		if isRegister(tok) || nasmKeywords[strings.ToLower(tok)] {
			continue
		}
		if strings.HasPrefix(tok, ".") {
			continue
		}
		if _, err := strconv.ParseInt(tok, 0, 64); err == nil {
			continue
		}
		valid := true
		for i := 0; i < len(tok); i++ {
			c := tok[i]
			if !(c == '_' || c == '.' || (c >= 'a' && c <= 'z') ||
				(c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		seen[tok] = true
		*refs = append(*refs, tok)
	}
}

func isRegister(s string) bool { return isPhysReg(s) || IsValidVirtualReg(s) }
