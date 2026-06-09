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
		// more rules can be added here
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

// ── div/idiv check ──────────────────────────────────────────────────────

type divCheck struct{}

func (d *divCheck) Check(lines []string) []Violation {
	var violations []Violation
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// skip comments and empty
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

// isRDXPrepared checks backwards from idx for a preparation of RDX (v3).
func isRDXPrepared(lines []string, idx int) bool {
	for j := idx - 1; j >= 0; j-- {
		prev := strings.TrimSpace(lines[j])
		if prev == "" || strings.HasPrefix(prev, ";") || strings.HasPrefix(prev, "#") {
			continue
		}
		upper := strings.ToUpper(prev)
		// accepted preparations
		accepted := []string{"CQO", "CDQ", "XOR V3, V3", "MOVI V3, 0", "XOR EDX, EDX"}
		for _, a := range accepted {
			if strings.HasPrefix(upper, a) {
				return true
			}
		}
		// if we hit any instruction that modifies v3 (rdx), we can't guarantee preparation
		if strings.Contains(upper, "V3,") || strings.Contains(upper, " V3") || strings.HasSuffix(upper, " V3") {
			return false
		}
		// stop at other non-empty lines that are not comments (assume they clobber or are unrelated)
		return false
	}
	return false
}
