//go:build !wasm

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"vas/vas"
	"vas/vas/lint"
)

// Version is set at build time via -ldflags "-X main.Version=v0.2.0".
var Version = "dev"

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		// No arguments – read from stdin (supports pipelines on all platforms)
		cmdAssemble(nil)
		return
	}

	// Subcommand dispatch
	switch args[0] {
	case "list":
		cmdList()
	case "diff":
		cmdDiff(args[1:])
	case "stats":
		cmdStats(args[1:])
	case "version":
		fmt.Println("vas " + Version)
	case "check":
		cmdCheck(args[1:])
	case "build":
		cmdBuild(args[1:])
	case "prep":
		cmdPrep(args[1:])
	case "-v", "--version":
		fmt.Println("vas " + Version)
	case "-h", "--help":
		fmt.Print(helpText)
	default:
		cmdAssemble(args)
	}
}

// ── assemble (existing behaviour) ──────────────────────────────────────────

func cmdAssemble(args []string) {
	var inputFile, outFile, target string
	optLevel := 0

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-o" && i+1 < len(args):
			outFile = args[i+1]
			i++
		case args[i] == "-target" && i+1 < len(args):
			target = args[i+1]
			i++
		case args[i] == "-O1":
			optLevel = 1
		case args[i] == "-O2":
			optLevel = 2
		case args[i] == "-h" || args[i] == "--help":
			fmt.Print(helpText)
			return
		case !strings.HasPrefix(args[i], "-"):
			inputFile = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	if target == "" {
		target = "elf64"
	}

	var input string

	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read file: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
		if input == "" {
			printUsage()
			os.Exit(1)
		}
	}

	output, err := vas.AssembleStandaloneTargetOpt(input, target, optLevel)
	if err != nil {
		// Try to show source context for line-numbered errors
		errMsg := err.Error()
		lineNum := 0
		if _, err2 := fmt.Sscanf(errMsg, "line %d:", &lineNum); err2 == nil && lineNum > 0 && inputFile != "" {
			lines := strings.Split(input, "\n")
			if lineNum-1 < len(lines) {
				fmt.Fprintf(os.Stderr, "error at line %d:\n", lineNum)
				fmt.Fprintf(os.Stderr, "  %s\n", strings.TrimRight(lines[lineNum-1], "\r"))
				fmt.Fprintf(os.Stderr, "  ^\n")
				fmt.Fprintf(os.Stderr, "%s\n", errMsg)
				os.Exit(1)
			}
		}
		fmt.Fprintf(os.Stderr, "assembly error: %v\n", err)
		os.Exit(1)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(output)
	}
}

// ── MinGW / linker detection ────────────────────────────────────────────────

// findLinker locates a suitable linker for the target platform.
// For win64 it searches common MinGW-w64 installations; for elf64 it looks for ld.
// Returns the linker executable path and whether it was found.
func findLinker(target string) (string, error) {
	if target == "win64" {
		// Candidate linker names in order of preference
		candidates := []string{
			"x86_64-w64-mingw32-gcc", // MinGW-w64 cross compiler (best)
			"x86_64-w64-mingw32-ld",  // MinGW-w64 raw linker
		}
		// Check PATH first
		for _, name := range candidates {
			if p, err := exec.LookPath(name); err == nil {
				return p, nil
			}
		}
		// Search common MinGW installation paths on Windows
		// (only searched when running on Windows — on Linux the cross compiler
		//  must be in PATH or configured by the user)
		if runtime.GOOS == "windows" {
			commonRoots := []string{
				"C:\\msys64\\mingw64\\bin",
				"C:\\msys64\\ucrt64\\bin",
				"C:\\msys64\\clang64\\bin",
				"C:\\MinGW\\bin",
				"C:\\mingw-w64\\x86_64-8.1.0-posix-seh-rt_v6-rev0\\mingw64\\bin",
				"D:\\msys64\\mingw64\\bin",
				"D:\\msys64\\ucrt64\\bin",
			}
			for _, root := range commonRoots {
				for _, name := range candidates {
					p := filepath.Join(root, name+".exe")
					if _, err := os.Stat(p); err == nil {
						return p, nil
					}
				}
			}
		}
		return "", fmt.Errorf("MinGW-w64 not found\n" +
			"  Install it:\n" +
			"    - Via MSYS2:    pacman -S mingw-w64-ucrt-x86_64-gcc\n" +
			"    - Via Chocolatey: choco install mingw\n" +
			"    - Via winget:     winget install MSYS2.MSYS2 (then pacman inside)\n" +
			"  Or add the MinGW bin/ directory to your PATH")
	}

	// elf64 (Linux): plain ld or gcc
	if p, err := exec.LookPath("ld"); err == nil {
		return p, nil
	}
	if p, err := exec.LookPath("gcc"); err == nil {
		// gcc can serve as linker driver for elf64 via -nostdlib
		return p, nil
	}
	return "", fmt.Errorf("linker not found — install binutils (ld) or gcc")
}

// ── build subcommand ──────────────────────────────────────────────────────

func cmdBuild(args []string) {
	var inputFile, outFile, target string
	optLevel := 0
	keepTemps := false
	verbose := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-o" && i+1 < len(args):
			outFile = args[i+1]
			i++
		case args[i] == "-target" && i+1 < len(args):
			target = args[i+1]
			i++
		case args[i] == "-O1":
			optLevel = 1
		case args[i] == "-O2":
			optLevel = 2
		case args[i] == "--keep-temps":
			keepTemps = true
		case args[i] == "-v" || args[i] == "--verbose":
			verbose = true
		case args[i] == "-h" || args[i] == "--help":
			fmt.Print(buildHelpText)
			return
		case !strings.HasPrefix(args[i], "-"):
			inputFile = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			fmt.Fprint(os.Stderr, buildHelpText)
			os.Exit(1)
		}
	}

	if inputFile == "" {
		fmt.Fprintln(os.Stderr, "error: no input file")
		fmt.Fprint(os.Stderr, buildHelpText)
		os.Exit(1)
	}

	if target == "" {
		target = "elf64"
	}

	// Check tools
	if _, err := exec.LookPath("nasm"); err != nil {
		fmt.Fprintln(os.Stderr, "error: nasm not found — install it from https://nasm.us")
		os.Exit(1)
	}
	linker, err := findLinker(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	// Read input
	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	input := string(data)

	// Resolve .include directives before assembling
	baseDir := filepath.Dir(inputFile)
	prepped, err := vas.Preprocess(input, baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "preprocess error: %v\n", err)
		os.Exit(1)
	}

	// Assemble
	asm, err := vas.AssembleStandaloneTargetOpt(prepped, target, optLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly error: %v\n", err)
		os.Exit(1)
	}

	// Default output name
	inputBase := strings.TrimSuffix(filepath.Base(inputFile), filepath.Ext(inputFile))
	if outFile == "" {
		outFile = inputBase
		if target == "win64" {
			outFile += ".exe"
		}
	}

	// Temp dir or working directory for intermediates
	workDir := filepath.Dir(inputFile)
	asmFile := filepath.Join(workDir, inputBase+".asm")
	objFile := filepath.Join(workDir, inputBase+".o")
	if target == "win64" {
		objFile = filepath.Join(workDir, inputBase+".obj")
	}
	binFile := outFile
	if !filepath.IsAbs(binFile) {
		binFile = filepath.Join(workDir, binFile)
	}

	// Write .asm
	if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write .asm: %v\n", err)
		os.Exit(1)
	}

	// nasm
	var nasmArgs []string
	if target == "win64" {
		nasmArgs = []string{"-f", "win64", "-o", objFile, asmFile}
	} else {
		nasmArgs = []string{"-f", "elf64", "-o", objFile, asmFile}
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "+ nasm %s\n", strings.Join(nasmArgs, " "))
	}
	if out, err := exec.Command("nasm", nasmArgs...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "nasm error:\n%s\n", out)
		os.Exit(1)
	}

	// ld
	var ldArgs []string
	if target == "win64" {
		if strings.Contains(linker, "gcc") {
			ldArgs = []string{"-o", binFile, objFile, "-nostdlib"}
		} else {
			ldArgs = []string{"-e", "main", "-o", binFile, objFile}
		}
	} else {
		ldArgs = []string{"-o", binFile, objFile}
	}
	if verbose {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", filepath.Base(linker), strings.Join(ldArgs, " "))
	}
	if out, err := exec.Command(linker, ldArgs...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "linker error:\n%s\n", out)
		os.Exit(1)
	}

	// Cleanup temp files
	if !keepTemps {
		os.Remove(asmFile)
		os.Remove(objFile)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "→ %s\n", binFile)
	}
}

// ── prep subcommand ────────────────────────────────────────────────────────

func cmdPrep(args []string) {
	var inputFile, outFile string
	verbose := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "-o" && i+1 < len(args):
			outFile = args[i+1]
			i++
		case args[i] == "-v" || args[i] == "--verbose":
			verbose = true
		case args[i] == "-h" || args[i] == "--help":
			fmt.Print(prepHelpText)
			return
		case !strings.HasPrefix(args[i], "-"):
			inputFile = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	var input string
	var baseDir string

	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
		baseDir = filepath.Dir(inputFile)
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
		baseDir, _ = os.Getwd()
	}

	start := time.Now()
	output, err := vas.Preprocess(input, baseDir)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		if strings.Contains(err.Error(), "circular include") {
			fmt.Fprintf(os.Stderr, "Tip: use 'vas prep -v' to trace included files.\n")
		}
		os.Exit(1)
	}

	if verbose {
		inLines := strings.Count(input, "\n") + 1
		outLines := strings.Count(output, "\n") + 1
		fmt.Fprintf(os.Stderr, "preprocessing: %d lines -> %d lines, took %v\n", inLines, outLines, elapsed)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(output)
	}
}

const prepHelpText = `usage: vas prep [options] <input.vas>

Resolves all preprocessor directives (.include, .macro, .const, .ifdef, .once, …)
and prints the fully expanded source. This is the same step that 'vas build' performs
internally before assembly. Use 'vas prep' when you want to inspect the expanded code
without assembling it.

Options:
  -o <file>   Write output to file instead of stdout
  -v, --verbose  Print processing statistics (line counts, time)
  -h, --help  Show this help message

Examples:
  vas prep app.vas               show expanded source
  vas prep -v app.vas            show expansion stats
  vas prep -o expanded.vas app.vas   write to file
`

const buildHelpText = `usage: vas build <input.vas> [options]

Build a .vas file into an executable (via nasm + ld).

Options:
  -o <file>         Output filename (default: <input>.exe on win64, <input> on elf64)
  -target <arch>    Target platform: elf64 (default) or win64
  -O1               Enable -O1 optimisations (const folding, DCE, peephole)
  -O2               Enable -O2 optimisations (CSE, LICM, redundant load elim, …)
  --keep-temps      Keep intermediate .asm and .o/.obj files
  -v, --verbose     Print tool commands and progress
  -h, --help        Show this help message

Examples:
  vas build hello.vas           ->  ./hello (ELF64)
  vas build hello.vas -O1       ->  ./hello (optimised)
  vas build hello.vas -O2       ->  ./hello (more optimised)
  vas build app.vas -target win64 ->  ./app.exe (Windows PE)
`

// ── diff subcommand ───────────────────────────────────────────────────────

func cmdDiff(args []string) {
	if len(args) < 1 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "usage: vas diff <input.vas>")
		os.Exit(1)
	}

	inputFile := args[0]
	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read file: %v\n", err)
		os.Exit(1)
	}

	source := string(data)
	output, err := vas.Assemble(source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly error: %v\n", err)
		os.Exit(1)
	}

	srcLines := strings.Split(strings.TrimRight(source, "\n"), "\n")
	asmLines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	name := filepath.Base(inputFile)
	outName := strings.TrimSuffix(name, filepath.Ext(name)) + ".asm"

	fmt.Printf("=== %s (VAS source) ===\n", name)
	for _, l := range srcLines {
		fmt.Println(l)
	}
	fmt.Println()
	fmt.Printf("=== %s (NASM output) ===\n", outName)
	for _, l := range asmLines {
		fmt.Println(l)
	}
}

// ── check subcommand ─────────────────────────────────────────────────────

func cmdCheck(args []string) {
	var inputFile string
	strict := false

	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--strict":
			strict = true
		case args[i] == "-h" || args[i] == "--help":
			fmt.Println("usage: vas check [--strict] <input.vas>")
			fmt.Println("       --strict  treat dangerous instruction warnings as errors (exit 1)")
			return
		case !strings.HasPrefix(args[i], "-"):
			inputFile = args[i]
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	if inputFile == "" {
		fmt.Fprintln(os.Stderr, "usage: vas check [--strict] <input.vas>")
		os.Exit(1)
	}

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check error: %v\n", err)
		os.Exit(1)
	}
	input := string(data)

	// Run lint checks first
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

	if strict && hasError {
		fmt.Fprintln(os.Stderr, "strict mode: lint errors found")
		os.Exit(1)
	}

	// Proceed with normal assembly check
	_, err = vas.Assemble(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check error: %v\n", err)
		os.Exit(1)
	}
}

// ── stats subcommand ──────────────────────────────────────────────────────

func cmdStats(args []string) {
	if len(args) < 1 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "usage: vas stats <input.vas>")
		os.Exit(1)
	}

	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read file: %v\n", err)
		os.Exit(1)
	}

	s := analyzeVAS(string(data))
	s.file = filepath.Base(args[0])
	s.print()
}

// vasStats holds aggregate statistics for a VAS source file.
type vasStats struct {
	file       string
	lines      int
	instrTotal int

	// Instruction categories
	arithmetic  int // ADD, SUB, MUL
	move        int // MOV, MOVI
	memory      int // LOAD, STORE
	controlFlow int // JMP, JE, JNE, JG, JL, JGE, JLE, CALL, RET
	stack       int // PUSH, POP
	other       int // NOP, INT, SYSCALL, LEA, CMP, CMPI, SECTION, GLOBAL, …

	// Virtual registers used
	regsUsed map[string]bool
	// Labels defined
	labels int
	// Errors
	errCount int
}

func (s *vasStats) print() {
	fmt.Printf("File: %s\n", s.file)
	fmt.Printf("  Lines:         %d\n", s.lines)
	fmt.Printf("  Labels:        %d\n", s.labels)
	fmt.Println()
	fmt.Printf("  Instructions:  %d\n", s.instrTotal)
	fmt.Printf("    Arithmetic:  %d  (ADD, SUB, MUL)\n", s.arithmetic)
	fmt.Printf("    Move:        %d  (MOV, MOVI)\n", s.move)
	fmt.Printf("    Memory:      %d  (LOAD, STORE)\n", s.memory)
	fmt.Printf("    Control:     %d  (JMP/Jcc, CALL, RET)\n", s.controlFlow)
	fmt.Printf("    Stack:       %d  (PUSH, POP)\n", s.stack)
	fmt.Printf("    Other:       %d\n", s.other)
	fmt.Println()

	used := make([]string, 0, len(s.regsUsed))
	for r := range s.regsUsed {
		used = append(used, r)
	}
	sort.Strings(used)
	fmt.Printf("  Virtual regs used: %d of 8: %s\n", len(used), strings.Join(used, ", "))

	if s.errCount > 0 {
		fmt.Printf("\n  Warnings: %d\n", s.errCount)
	}
}

// knownArithmetic, knownMove, etc. — instruction classification set
var instrClass = map[string]string{
	"ADD": "arith", "SUB": "arith", "MUL": "arith",
	"MOV": "move", "MOVI": "move",
	"LOAD": "mem", "STORE": "mem",
	"JMP": "ctrl", "JE": "ctrl", "JNE": "ctrl",
	"JG": "ctrl", "JL": "ctrl", "JGE": "ctrl", "JLE": "ctrl",
	"CALL": "ctrl", "RET": "ctrl",
	"PUSH": "stack", "POP": "stack",
}

// stripComment removes trailing ;...# comments (duplicated from vas package
// so analyzeVAS can use it without an export).
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

func analyzeVAS(input string) *vasStats {
	s := &vasStats{regsUsed: map[string]bool{}}

	lines := strings.Split(input, "\n")
	s.lines = len(lines)

	for _, raw := range lines {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}

		// Label definition (e.g. "loop:")
		if strings.HasSuffix(line, ":") {
			// Check it's not a known directive like "section .text:" / "global _start:"
			// Actually those are not suffixed with : in VAS, so this is safe.
			s.labels++
			continue
		}

		// Section / global / extern directives
		first := strings.Fields(line)[0]
		upper := strings.ToUpper(first)

		cls, known := instrClass[upper]
		if !known {
			// Might be a directive (section, global, extern, align, db, dq, …)
			s.other++
			s.instrTotal++
			continue
		}

		switch cls {
		case "arith":
			s.arithmetic++
		case "move":
			s.move++
		case "mem":
			s.memory++
		case "ctrl":
			s.controlFlow++
		case "stack":
			s.stack++
		}
		s.instrTotal++

		// Count virtual register references in this line
		parts := strings.Fields(line)
		for _, p := range parts {
			p = strings.TrimRight(p, ",")
			if strings.HasPrefix(p, "v") && len(p) == 2 && p[1] >= '0' && p[1] <= '7' {
				s.regsUsed[p] = true
			}
		}
	}

	return s
}

// ── list ──────────────────────────────────────────────────────────────────

func cmdList() {
	fmt.Println("VAS -- supported instructions and syntax")
	fmt.Println()
	fmt.Println("Arithmetic (3-operand form, 2-operand form):")
	fmt.Println("  ADD   dst, src1, src2    ->  mov dst, src1 ; add dst, src2")
	fmt.Println("  ADD   dst, src           ->  add dst, src")
	fmt.Println("  SUB   dst, src1, src2    ->  mov dst, src1 ; sub dst, src2")
	fmt.Println("  SUB   dst, src           ->  sub dst, src")
	fmt.Println("  MUL   dst, src1, src2    ->  imul dst, src1, src2")
	fmt.Println("  MUL   dst, src           ->  imul dst, src")
	fmt.Println()
	fmt.Println("Memory:")
	fmt.Println("  LOAD  dst, [addr]        ->  mov dst, [addr]")
	fmt.Println("  LOAD  dst, [addr+off]    ->  mov dst, [addr+off]")
	fmt.Println("  LOAD  dst, [addr*scale]  ->  mov dst, [addr*scale]")
	fmt.Println("  STORE src, [addr]        ->  mov [addr], src")
	fmt.Println("  STORE src, [addr+off]    ->  mov [addr+off], src")
	fmt.Println("  LEA   dst, [addr]        ->  lea dst, [addr]")
	fmt.Println()
	fmt.Println("Data movement:")
	fmt.Println("  MOVI  dst, imm           ->  mov dst, imm")
	fmt.Println("  MOV   dst, src           ->  mov dst, src")
	fmt.Println()
	fmt.Println("Control flow:")
	fmt.Println("  JMP   label              ->  jmp label")
	fmt.Println("  JE    label              ->  je  label")
	fmt.Println("  JNE   label              ->  jne label")
	fmt.Println("  JG    label              ->  jg  label")
	fmt.Println("  JL    label              ->  jl  label")
	fmt.Println("  JGE   label              ->  jge label")
	fmt.Println("  JLE   label              ->  jle label")
	fmt.Println("  CALL  label              ->  call label")
	fmt.Println("  RET                      ->  ret")
	fmt.Println("  NOP                      ->  nop")
	fmt.Println()
	fmt.Println("Stack:")
	fmt.Println("  PUSH  src                ->  push src")
	fmt.Println("  POP   dst                ->  pop  dst")
	fmt.Println()
	fmt.Println("System:")
	fmt.Println("  CMP   a, b               ->  cmp a, b")
	fmt.Println("  CMP   a, imm             ->  cmp a, imm")
	fmt.Println("  INT   n                  ->  int n")
	fmt.Println("  SYSCALL                  ->  syscall")
	fmt.Println()
	fmt.Println("Directives (passthrough without register substitution):")
	fmt.Println("  SECTION .text / .data / .bss")
	fmt.Println("  GLOBAL / EXTERN label")
	fmt.Println("  DB, DW, DD, DQ, BYTE, WORD, DWORD, QWORD")
	fmt.Println("  ALIGN n, TYPE, SIZE, LENGTH")
	fmt.Println()
	fmt.Println("Passthrough (raw x86 instructions, registers ARE substituted):")
	fmt.Println("  Any unrecognized line passes through with v-register mapping.")
	fmt.Println("  Commonly used:")
	fmt.Println("    movzx  dst, byte [src]   - zero-extend byte load")
	fmt.Println("    div    reg               - unsigned divide rdx:rax by reg")
	fmt.Println("    shl    reg, imm          - shift left")
	fmt.Println("    shr    reg, imm          - shift right")
	fmt.Println("    and    reg, imm/reg      - bitwise AND")
	fmt.Println("    or     reg, imm/reg      - bitwise OR")
	fmt.Println("    xor    reg, reg          - zero register")
	fmt.Println("    test   reg, reg          - set flags without write")
	fmt.Println("    not    reg               - bitwise NOT")
	fmt.Println("    neg    reg               - negate (two's complement)")
	fmt.Println()
	fmt.Println("Virtual registers (v0-v12):")
	fmt.Println("  v0 -> rax    v1 -> rbx    v2 -> rcx    v3 -> rdx")
	fmt.Println("  v4 -> rsi    v5 -> rdi    v6 -> r8     v7 -> r9")
	fmt.Println("  v8 -> r11    v9 -> r12    v10 -> r13   v11 -> r14")
	fmt.Println("  v12 -> r15")
	fmt.Println()
	fmt.Println("  v-registers work in any operand position including memory:")
	fmt.Println("    MOV  v4, [v2+8]     ->  mov rsi, [rcx+8]")
	fmt.Println("    LOAD v6, [v1+v2*8]  ->  mov r8, [rbx+rcx*8]")
}

// ── help / usage ──────────────────────────────────────────────────────────

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: vas [options] <input.asm|.vas>")
	fmt.Fprintln(os.Stderr, "       cat input.vas | vas [options]")
	fmt.Fprintln(os.Stderr, "       vas diff <input.vas>")
	fmt.Fprintln(os.Stderr, "       vas prep <input.vas>")
	fmt.Fprintln(os.Stderr, "       vas stats <input.vas>")
	fmt.Fprintln(os.Stderr, "       vas check [--strict] <input.vas>")
	fmt.Fprintln(os.Stderr, "       vas build <input.vas> [build-options]")
	fmt.Fprintln(os.Stderr, "       vas list")
	fmt.Fprintln(os.Stderr, "       vas version")
	os.Exit(1)
}

const helpText = `VAS -- Virtual ASseMbler

Usage:
  vas [options] <input file>
  cat <input> | vas [options]
  vas diff <input.vas>      Show VAS source vs NASM output side-by-side
  vas prep <input.vas>      Resolve .include directives (print flattened source)
  vas stats <input.vas>     Show instruction and register statistics
  vas check <input.vas>     Validate VAS syntax (exit code: 0=ok, 1=error)
  vas check --strict <input.vas>  Also fail on dangerous instruction patterns
  vas build <input.vas>     Build a .vas file into an executable
  vas list                  List all supported instructions and syntax
  vas version               Print version and exit

Options:
  -o <file>       Write output to file instead of stdout
  -target <arch>  Target platform: elf64 (default) or win64
  -O1             Enable optimizations (constant folding, dead code elim, peephole)
  -O2             Enable -O2 optimizations (CSE, LICM, redundant load elim, …)
  -v, --version   Print version and exit
  -h, --help      Show this help message
  -v, --verbose   Print verbose output (for 'prep' and 'build')

Check options:
  --strict        Treat dangerous instruction warnings as errors (exit 1)

Build options (for "vas build"):
  -o <file>         Output filename
  -target <arch>    Target platform: elf64 (default) or win64
  -O1               Enable -O1 optimisations (const folding, DCE, peephole)
  -O2               Enable -O2 optimisations (CSE, LICM, redundant load elim, …)
  --keep-temps      Keep intermediate .asm and .o/.obj files
  -v, --verbose     Print tool commands and progress

Input format: .vas or .asm files with virtual registers v0-v12.
Output: x86-64 NASM assembly (.asm) or executable (vas build).

Virtual register mapping:
  v0 -> rax   v1 -> rbx   v2 -> rcx   v3 -> rdx
  v4 -> rsi   v5 -> rdi   v6 -> r8    v7 -> r9
  v8 -> r11   v9 -> r12   v10 -> r13  v11 -> r14
  v12 -> r15

Tip: run 'vas list' to see all instructions, syntax, and virtual register mapping.  
`
