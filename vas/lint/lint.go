package lint

import (
	"fmt"
	"strings"
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
func Run(source string) []Violation {
	lines := strings.Split(source, "\n")
	var all []Violation
	for _, rule := range Rules() {
		all = append(all, rule.Check(lines)...)
	}
	return all
}

// ── div/idiv check (existing) ───────────────────────────────────────────

type divCheck struct{}

func (d *divCheck) Check(lines []string) []Violation {
	var violations []Violation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "IDIV") || strings.HasPrefix(upper, "DIV") {
			if !isRDXPrepared(lines, i) {
				msg := fmt.Sprintf("%s used at line %d but RDX (v3) is not prepared", trimmed, i+1)
				fix := "insert cqo before idiv (signed) or xor v3, v3 before div (unsigned)"
				violations = append(violations, Violation{
					Line:     i + 1,
					Message:  msg,
					Severity: "error",
					Fix:      fix,
				})
			}
		}
	}
	return violations
}

func isRDXPrepared(lines []string, idx int) bool {
	for j := idx - 1; j >= 0; j-- {
		prev := strings.TrimSpace(lines[j])
		if prev == "" || strings.HasPrefix(prev, ";") || strings.HasPrefix(prev, "#") {
			continue
		}
		upper := strings.ToUpper(prev)
		accepted := []string{"CQO", "CDQ", "XOR V3, V3", "MOVI V3, 0", "XOR EDX, EDX"}
		for _, a := range accepted {
			if strings.HasPrefix(upper, a) {
				return true
			}
		}
		if strings.Contains(upper, "V3,") || strings.Contains(upper, " V3") || strings.HasSuffix(upper, " V3") {
			return false
		}
		return false
	}
	return false
}

// ── stack balance check ─────────────────────────────────────────────────

type stackBalanceCheck struct{}

func (s *stackBalanceCheck) Check(lines []string) []Violation {
	var violations []Violation
	// split into functions by labels (naive: detect _start or any label:)
	var funcs [][]int // each slice holds line indices of a function
	var current []int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// simple function detection: a label that looks like a function (ends with ":")
		if strings.HasSuffix(trimmed, ":") && !isInstruction(trimmed) {
			if len(current) > 0 {
				funcs = append(funcs, current)
			}
			current = []int{i}
			continue
		}
		current = append(current, i)
	}
	if len(current) > 0 {
		funcs = append(funcs, current)
	}

	for _, fn := range funcs {
		balance := 0
		for _, idx := range fn {
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
			case "RET", "RETURN", "SYSCALL":
				if balance != 0 {
					violations = append(violations, Violation{
						Line:     idx + 1,
						Message:  fmt.Sprintf("stack imbalance at function exit: push/pop mismatch of %d", balance),
						Severity: "warning",
						Fix:      "ensure every push has a corresponding pop before return",
					})
					balance = 0 // reset after reporting
				}
			}
		}
		// If function ends without RET/SYSCALL, still report imbalance
		if balance != 0 {
			lastIdx := fn[len(fn)-1]
			violations = append(violations, Violation{
				Line:     lastIdx + 1,
				Message:  fmt.Sprintf("stack imbalance at end of function: push/pop mismatch of %d", balance),
				Severity: "warning",
				Fix:      "ensure every push has a corresponding pop",
			})
		}
	}
	return violations
}

func isInstruction(s string) bool {
	// same logic as in vas package, copied to avoid import cycle
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
	// simple intra-block analysis; ignore cross-block for now
	blocks := splitBlocks(lines)
	for _, block := range blocks {
		written := map[string]bool{}
		// first pass: collect writes
		for _, idx := range block {
			line := strings.TrimSpace(lines[idx])
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			dst := dstReg(op, fields[1:])
			if dst >= 0 {
				written[fields[1]] = true
			}
		}
		// second pass: check reads before writes
		seen := map[string]bool{}
		for _, idx := range block {
			line := strings.TrimSpace(lines[idx])
			if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			op := strings.ToUpper(fields[0])
			for _, reg := range readRegs(op, fields[1:]) {
				regName := fmt.Sprintf("v%d", reg)
				if !seen[regName] && !written[regName] {
					violations = append(violations, Violation{
						Line:     idx + 1,
						Message:  fmt.Sprintf("register %s may be used uninitialized at line %d", regName, idx+1),
						Severity: "warning",
						Fix:      fmt.Sprintf("initialize %s before use", regName),
					})
					seen[regName] = true // only report once per block
				}
			}
			// mark destination as written (in this block, after this line)
			dst := dstReg(op, fields[1:])
			if dst >= 0 {
				written[fields[1]] = true
			}
		}
	}
	return violations
}

// ── caller-save register check ───────────────────────────────────────────

type callerSaveCheck struct{}

var callerSaveRegs = map[string]bool{
	"v0": true, "v2": true, "v3": true, "v6": true, "v7": true, "v8": true,
}

func (c *callerSaveCheck) Check(lines []string) []Violation {
	var violations []Violation
	blocks := splitBlocks(lines)
	for _, block := range blocks {
		saved := map[string]bool{}
		lastWrite := map[string]int{}
		for _, idx := range block {
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
				reg := fields[1]
				if callerSaveRegs[reg] {
					saved[reg] = true
				}
			} else if op == "CALL" {
				// after call, all caller-saved regs are considered "modified" unless saved
				for reg := range callerSaveRegs {
					if !saved[reg] {
						lastWrite[reg] = 0 // mark as potentially clobbered
					}
				}
				// clear saved after call (they are now restored by corresponding pop)
				saved = map[string]bool{}
			} else if op == "POP" {
				// pop after call restores the register
				reg := fields[1]
				if callerSaveRegs[reg] {
					delete(lastWrite, reg)
				}
			} else {
				// check if we read a caller-saved reg that hasn't been written after the last call
				for _, reg := range readRegs(op, fields[1:]) {
					regName := fmt.Sprintf("v%d", reg)
					if callerSaveRegs[regName] {
						if _, exists := lastWrite[regName]; !exists {
							violations = append(violations, Violation{
								Line:     idx + 1,
								Message:  fmt.Sprintf("register %s (caller-saved) may be used after call without being preserved", regName),
								Severity: "warning",
								Fix:      fmt.Sprintf("push %s before call and pop after, or reload after call", regName),
							})
						}
					}
				}
				// update writes
				dst := dstReg(op, fields[1:])
				if dst >= 0 {
					lastWrite[fields[1]] = idx
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
		// Re-join the rest of the line after "STORE" to properly split by comma
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
	// single character in quotes, e.g., 'a'
	if len(s) == 3 && (s[0] == '\'' || s[0] == '"') && (s[2] == '\'' || s[2] == '"') {
		return true
	}
	// small immediate (0-255)
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
		fields := strings.Fields(trimmed)
		if len(fields) < 3 || strings.ToUpper(fields[0]) != "CMP" {
			continue
		}
		for _, arg := range fields[1:] {
			// check for size prefixes
			if strings.Contains(arg, "byte") || strings.Contains(arg, "word") || strings.Contains(arg, "dword") {
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

// ── helpers ─────────────────────────────────────────────────────────────

func splitBlocks(lines []string) [][]int {
	var blocks [][]int
	var current []int
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// empty lines don't break blocks but we can skip
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
			a := strings.TrimSuffix(strings.TrimPrefix(args[1], "["), "]")
			if r := regIndex(a); r >= 0 {
				regs = append(regs, r)
			}
		}
	case "STORE":
		if len(args) >= 2 {
			if r := regIndex(args[0]); r >= 0 {
				regs = append(regs, r)
			}
			a := strings.TrimSuffix(strings.TrimPrefix(args[1], "["), "]")
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
		if len(rest) == 1 && rest[0] >= '0' && rest[0] <= '9' {
			return int(rest[0] - '0')
		}
		if len(rest) == 2 && rest[0] == '1' && rest[1] >= '0' && rest[1] <= '2' {
			return 10 + int(rest[1]-'0')
		}
	}
	return -1
}

// ── infinite loop detection ─────────────────────────────────────────────

type infiniteLoopCheck struct{}

func (l *infiniteLoopCheck) Check(lines []string) []Violation {
	var violations []Violation

	// Build label -> line index map (0-based)
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
			continue // forward jump or unknown label
		}

		// Check lines between target and this JMP (exclusive) for any exit
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
