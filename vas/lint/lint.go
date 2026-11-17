package lint

import (
	"fmt"
	"sort"
	"strings"

	"vas/vas/internal/asm"
)

// Violation represents a detected dangerous instruction pattern.
type Violation struct {
	Line     int // 1-based
	Message  string
	Severity string // "error" or "warning"
	Fix      string // suggested fix (one line)
}

// Rule is the interface for a lint check.
type Rule interface {
	Check(lines []string) []Violation
}

// Rules returns the active lint rules.
func Rules() []Rule {
	return []Rule{
		&divCheck{},
		&stackBalanceCheck{},
		&uninitRegCheck{},
		&callerSaveCheck{},
		&storeByteCheck{},
		&cmpMemSizeCheck{},
		&infiniteLoopCheck{},
	}
}

// Run applies all rules to the source and returns all violations.
// Rules run sequentially over one parsed line array; results are sorted by
// line number for stable output.
func Run(source string) []Violation {
	lines := strings.Split(source, "\n")
	var all []Violation
	for _, rule := range Rules() {
		all = append(all, rule.Check(lines)...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Line < all[j].Line })
	return all
}

// ── div/idiv check ──────────────────────────────────────────────────────

type divCheck struct{}

func (d *divCheck) Check(lines []string) []Violation {
	var violations []Violation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "IDIV") {
			if !isRDXPreparedSigned(lines, i) {
				violations = append(violations, Violation{
					Line:     i + 1,
					Message:  fmt.Sprintf("%s used at line %d but RDX (v3) is not sign-extended", trimmed, i+1),
					Severity: "error",
					Fix:      "insert cqo or cdq before idiv",
				})
			}
		}
		if strings.HasPrefix(upper, "DIV") {
			if !isRDXPreparedUnsigned(lines, i) {
				violations = append(violations, Violation{
					Line:     i + 1,
					Message:  fmt.Sprintf("%s used at line %d but RDX (v3) is not zeroed", trimmed, i+1),
					Severity: "error",
					Fix:      "insert xor v3, v3 or movi v3, 0 before div",
				})
			}
		}
	}
	return violations
}

// isRDXPreparedSigned checks whether RDX (v3) is sign-extended for IDIV.
// CQO and CDQ are the only valid preparations.
func isRDXPreparedSigned(lines []string, idx int) bool {
	for j := idx - 1; j >= 0; j-- {
		prev := strings.TrimSpace(lines[j])
		if prev == "" || strings.HasPrefix(prev, ";") || strings.HasPrefix(prev, "#") {
			continue
		}
		upper := strings.ToUpper(prev)
		if isControlBoundary(upper) {
			return false
		}
		if strings.HasPrefix(upper, "CQO") || strings.HasPrefix(upper, "CDQ") {
			return true
		}
		if writesV3(upper) {
			return false
		}
	}
	return false
}

// isRDXPreparedUnsigned checks whether RDX (v3) is zeroed for DIV.
// CQO/CDQ sign-extend and are therefore wrong for unsigned division.
func isRDXPreparedUnsigned(lines []string, idx int) bool {
	for j := idx - 1; j >= 0; j-- {
		prev := strings.TrimSpace(lines[j])
		if prev == "" || strings.HasPrefix(prev, ";") || strings.HasPrefix(prev, "#") {
			continue
		}
		upper := strings.ToUpper(prev)
		if isControlBoundary(upper) {
			return false
		}
		switch {
		case strings.HasPrefix(upper, "XOR V3, V3"),
			strings.HasPrefix(upper, "MOVI V3, 0"),
			strings.HasPrefix(upper, "XOR EDX, EDX"),
			strings.HasPrefix(upper, "MOV EDX, 0"),
			strings.HasPrefix(upper, "MOV V3, 0"):
			return true
		}
		if writesV3(upper) {
			return false
		}
	}
	return false
}

func writesV3(upper string) bool {
	return strings.Contains(upper, "V3,") || strings.Contains(upper, " V3") || strings.HasSuffix(upper, " V3")
}

func isControlBoundary(upper string) bool {
	fields := strings.Fields(upper)
	if len(fields) == 0 {
		return false
	}
	op := fields[0]
	if strings.HasSuffix(op, ":") {
		return true
	}
	switch op {
	case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE", "JA", "JB", "JAE", "JBE",
		"JS", "JNS", "JO", "JNO", "JC", "JNC", "JZ", "JNZ", "JP", "JNP",
		"CALL", "RET", "SYSCALL", "INT", "LOOP", "LOOPE", "LOOPNE":
		return true
	}
	return false
}

// ── stack balance check ─────────────────────────────────────────────────

type stackBalanceCheck struct{}

func (s *stackBalanceCheck) Check(lines []string) []Violation {
	var violations []Violation
	funcs := splitFuncs(lines)

	for _, fn := range funcs {
		balance := 0
		inFunction := true
		for _, idx := range fn {
			if !inFunction {
				break
			}
			line := strings.TrimSpace(lines[idx])
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			upper := strings.ToUpper(strings.Fields(line)[0])
			switch upper {
			case "PUSH":
				balance++
			case "POP":
				balance--
			case "RET", "RETURN":
				if balance != 0 {
					violations = append(violations, Violation{
						Line:     idx + 1,
						Message:  fmt.Sprintf("stack imbalance at function exit: push/pop mismatch of %d", balance),
						Severity: "warning",
						Fix:      "ensure every push has a corresponding pop before return",
					})
				}
				inFunction = false // Stop tracking after return
			case "SYSCALL":
				if balance != 0 {
					violations = append(violations, Violation{
						Line:     idx + 1,
						Message:  fmt.Sprintf("stack imbalance at syscall: push/pop mismatch of %d", balance),
						Severity: "warning",
						Fix:      "ensure every push has a corresponding pop before syscall",
					})
					balance = 0
				}
			}
		}
	}
	return violations
}

func isInstruction(s string) bool {
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

// ── uninitialized register check ─────────────────────────────────────────

type uninitRegCheck struct{}

func (u *uninitRegCheck) Check(lines []string) []Violation {
	var violations []Violation
	funcs := splitFuncs(lines)
	for _, fn := range funcs {
		seen := map[string]bool{}
		warned := map[string]bool{}
		for _, idx := range fn {
			trimmed := asm.TrimCode(lines[idx])
			if trimmed == "" {
				continue
			}
			fields := asm.Fields(trimmed)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			args := fields[1:]
			for _, reg := range readRegs(op, args) {
				regName := fmt.Sprintf("v%d", reg)
				if !seen[regName] && !warned[regName] {
					violations = append(violations, Violation{
						Line:     idx + 1,
						Message:  fmt.Sprintf("register %s may be used uninitialized at line %d", regName, idx+1),
						Severity: "warning",
						Fix:      fmt.Sprintf("initialize %s before use", regName),
					})
					warned[regName] = true
				}
			}
			if dst := dstReg(op, args); dst >= 0 && len(args) > 0 {
				seen[fmt.Sprintf("v%d", dst)] = true
				delete(warned, fmt.Sprintf("v%d", dst))
			}
		}
	}
	return violations
}

// ── caller-save register check ───────────────────────────────────────────

type callerSaveCheck struct{}

var callerSaveRegs = map[string]bool{
	"v0": true, "v2": true, "v3": true, "v4": true, "v5": true,
	"v6": true, "v7": true, "v8": true,
}

func (c *callerSaveCheck) Check(lines []string) []Violation {
	var violations []Violation
	funcs := splitFuncs(lines)
	for _, fn := range funcs {
		saved := map[string]bool{}
		clobbered := map[string]bool{}
		hasCall := false
		for _, idx := range fn {
			line := strings.TrimSpace(lines[idx])
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			if op == "PUSH" {
				reg := strings.ToLower(strings.TrimRight(fields[1], ","))
				if callerSaveRegs[reg] {
					saved[reg] = true
				}
			} else if op == "CALL" {
				hasCall = true
				for reg := range callerSaveRegs {
					if !saved[reg] {
						clobbered[reg] = true
					}
				}
				saved = map[string]bool{}
			} else if op == "POP" {
				reg := strings.ToLower(strings.TrimRight(fields[1], ","))
				if callerSaveRegs[reg] {
					delete(clobbered, reg)
				}
			} else {
				if hasCall {
					for _, reg := range readRegs(op, fields[1:]) {
						regName := fmt.Sprintf("v%d", reg)
						if callerSaveRegs[regName] && clobbered[regName] {
							violations = append(violations, Violation{
								Line:     idx + 1,
								Message:  fmt.Sprintf("register %s (caller-saved) may be used after call without being preserved", regName),
								Severity: "warning",
								Fix:      fmt.Sprintf("push %s before call and pop after, or reload after call", regName),
							})
						}
					}
				}
				dst := dstReg(op, fields[1:])
				if dst >= 0 {
					dstName := strings.ToLower(strings.TrimRight(fields[1], ","))
					delete(clobbered, dstName)
				}
			}
		}
	}
	return violations
}

// ── store byte check ─────────────────────────────────────────────────────

type storeByteCheck struct{}

func (s *storeByteCheck) Check(lines []string) []Violation {
	var violations []Violation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || strings.ToUpper(fields[0]) != "STORE" {
			continue
		}
		rest := strings.TrimSpace(trimmed[len("STORE"):])
		parts := strings.SplitN(rest, ",", 2)
		if len(parts) < 2 {
			continue
		}
		src := strings.TrimSpace(parts[0])
		if isByteValue(src) {
			violations = append(violations, Violation{
				Line:     i + 1,
				Message:  fmt.Sprintf("STORE at line %d writes 8 bytes but the value appears to be a single byte", i+1),
				Severity: "warning",
				Fix:      "consider using 'mov byte [addr], value' instead of STORE for byte writes",
			})
		}
	}
	return violations
}

func isByteValue(s string) bool {
	if len(s) == 3 && (s[0] == '\'' || s[0] == '"') && (s[2] == '\'' || s[2] == '"') {
		return true
	}
	val := 0
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return val >= 0 && val <= 255
	}
	return false
}

// ── cmp memory size check ───────────────────────────────────────────────

type cmpMemSizeCheck struct{}

func (c *cmpMemSizeCheck) Check(lines []string) []Violation {
	var violations []Violation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := asm.Fields(trimmed)
		if len(fields) < 3 || strings.ToUpper(fields[0]) != "CMP" {
			continue
		}
		for argIdx, arg := range fields[1:] {
			low := strings.ToLower(arg)
			if low != "byte" && low != "word" && low != "dword" && low != "qword" &&
				low != "tword" && low != "oword" {
				continue
			}
			// Only a genuine size prefix on a memory operand is a problem;
			// symbols such as byte_count must not trigger this rule.
			if argIdx+2 < len(fields) && strings.Contains(fields[argIdx+2], "[") {
				violations = append(violations, Violation{
					Line:     i + 1,
					Message:  fmt.Sprintf("CMP with memory operand size prefix may generate invalid NASM syntax: %s", trimmed),
					Severity: "error",
					Fix:      "load the value into a register first, e.g., 'movzx reg, byte [addr]; cmp reg, imm'",
				})
			}
		}
	}
	return violations
}

// ── infinite loop detection ──────────────────────────────────────────────

type infiniteLoopCheck struct{}

func (l *infiniteLoopCheck) Check(lines []string) []Violation {
	var violations []Violation
	labels := map[string]int{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") && !isInstruction(trimmed) {
			name := strings.TrimSuffix(trimmed, ":")
			labels[name] = i
		}
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) != 2 || strings.ToUpper(fields[0]) != "JMP" {
			continue
		}
		target := fields[1]
		targetIdx, ok := labels[target]
		if !ok || targetIdx >= i {
			continue
		}

		hasExit := false
		for j := targetIdx; j < i; j++ {
			l := strings.TrimSpace(lines[j])
			if l == "" || strings.HasPrefix(l, ";") || strings.HasPrefix(l, "#") {
				continue
			}
			f := strings.Fields(l)
			if len(f) == 0 {
				continue
			}
			op := strings.ToUpper(f[0])
			switch op {
			case "RET", "SYSCALL", "CALL", "HLT",
				"JE", "JNE", "JG", "JL", "JGE", "JLE":
				hasExit = true
			}
			if hasExit {
				break
			}
		}
		if !hasExit {
			violations = append(violations, Violation{
				Line:     i + 1,
				Message:  fmt.Sprintf("infinite loop detected: unconditional jump to %q at line %d with no exit", target, targetIdx+1),
				Severity: "warning",
				Fix:      "add a condition to leave the loop (e.g., CMP + JE) or a SYSCALL/RET inside the loop body",
			})
		}
	}
	return violations
}

// ── helpers ─────────────────────────────────────────────────────────────

// splitFuncs partitions the source into function bodies. Call targets,
// underscore-prefixed labels, main/vas_main, and non-local labels following a
// RET are treated as entry points. Loop labels inside a function are not.
func splitFuncs(lines []string) [][]int {
	callTargets := map[string]bool{}
	for _, line := range lines {
		code := asm.TrimCode(line)
		if code == "" {
			continue
		}
		op, args, ok := asm.Parse(code)
		if ok && op == "CALL" && len(args) > 0 {
			callTargets[strings.TrimRight(args[0], ",")] = true
		}
	}
	isFuncLabel := func(name string) bool {
		if name == "" || strings.HasPrefix(name, ".") {
			return false
		}
		return callTargets[name] || strings.HasPrefix(name, "_") || name == "main" || name == "vas_main"
	}

	var funcs [][]int
	var current []int
	sawRet := false
	for i, line := range lines {
		code := asm.TrimCode(line)
		if strings.HasSuffix(code, ":") && !isInstruction(code) {
			name := strings.TrimSpace(strings.TrimSuffix(code, ":"))
			if isFuncLabel(name) || sawRet {
				if len(current) > 0 {
					funcs = append(funcs, current)
				}
				current = []int{i}
				sawRet = false
				continue
			}
		}
		if op, args, ok := asm.Parse(code); ok {
			_ = args
			if op == "RET" {
				sawRet = true
			}
		}
		current = append(current, i)
	}
	if len(current) > 0 {
		funcs = append(funcs, current)
	}
	return funcs
}

func splitBlocks(lines []string) [][]int {
	var blocks [][]int
	var current []int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			current = append(current, i)
			continue
		}
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			current = append(current, i)
			continue
		}
		isLabel := strings.HasSuffix(code, ":") && !isInstruction(code)
		isControlFlow := false
		if !isLabel {
			fields := strings.Fields(code)
			if len(fields) == 0 {
				current = append(current, i)
				continue
			}
			upper := strings.ToUpper(fields[0])
			switch upper {
			case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE", "CALL", "RET", "SYSCALL", "INT":
				isControlFlow = true
			}
		}
		if isLabel || isControlFlow {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			current = append(current, i)
			if isControlFlow {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, i)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
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
	add := func(a string) {
		for _, tok := range asm.OperandRegs(a) {
			if n := asm.VirtualRegNum(tok); n >= 0 {
				regs = append(regs, n)
			}
		}
	}
	switch strings.ToUpper(op) {
	case "MOVI":
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
				add(args[0])
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
	case "SYSCALL", "INT":
		regs = append(regs, 0, 3, 4, 5, 6, 7, 8)
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

func regIndex(s string) int {
	return asm.VirtualRegNum(strings.TrimRight(strings.TrimSpace(s), ","))
}
