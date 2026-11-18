package opt

import (
	"fmt"
	"strings"

	"vas/vas/internal/asm"
)

// cfgLine is the control-flow-relevant view of one source line.
type cfgLine struct {
	raw     string
	code    string
	op      string
	args    []string
	label   string
	section string
}

func analyzeCFG(lines []string) []cfgLine {
	out := make([]cfgLine, len(lines))
	for i, line := range lines {
		code := asm.TrimCode(line)
		cl := cfgLine{raw: line, code: code}
		if code == "" {
			out[i] = cl
			continue
		}
		fields := strings.Fields(code)
		if len(fields) > 0 {
			switch fields[0] {
			case "section", "SECTION":
				if len(fields) > 1 {
					cl.section = strings.ToLower(fields[1])
				}
			}
		}
		if strings.HasSuffix(code, ":") && !isInstructionFold(code) {
			cl.label = strings.TrimSpace(strings.TrimSuffix(code, ":"))
		} else if op, args, ok := asm.Parse(code); ok {
			cl.op, cl.args = op, args
		}
		out[i] = cl
	}
	return out
}

func isTextSection(name string) bool {
	if name == "" {
		return true
	}
	return strings.HasPrefix(name, ".text")
}

func isUnconditionalTerminator(op string) bool {
	switch strings.ToLower(op) {
	case "jmp", "jmpq", "ret", "retf", "iret", "iretq", "hlt":
		return true
	}
	return false
}

func isThreadableBranch(op string) bool {
	return strings.EqualFold(op, "jmp") || asm.IsCondJump(op)
}

// directSymbol returns a direct label operand, or "" for registers, memory,
// numbers and modifier forms.
func directSymbol(op string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	if !isThreadableBranch(op) {
		return ""
	}
	target := strings.TrimSpace(strings.TrimRight(args[0], ","))
	if target == "" || strings.HasPrefix(target, "[") || asm.IsReg(target) {
		return ""
	}
	if strings.ContainsAny(target, " \t:+*") {
		return ""
	}
	return target
}

func branchLine(op, target, raw string) string {
	return fmt.Sprintf("\t%s\t%s%s", strings.ToUpper(op), target, commentSuffix(raw))
}

// cleanCFG performs two conservative CFG cleanups:
//  1. drops instructions that follow an unconditional JMP/RET/HLT until a
//     referenced or root label starts a live block;
//  2. threads jumps through a label whose first instruction is another JMP.
//
// It never removes data sections or global/extern metadata.
func cleanCFG(lines []string) []string {
	lines = removeUnreachable(lines)
	lines = removeDeadLabels(lines)
	lines = threadJumps(lines)
	return lines
}

// removeDeadLabels removes text-section labels that are neither roots
// (GLOBAL/EXTERN/entry points) nor referenced anywhere in the remaining code,
// including memory operands and data declarations. Data labels are preserved.
func removeDeadLabels(lines []string) []string {
	info := analyzeCFG(lines)
	defined := map[string]bool{}
	roots := map[string]bool{"_start": true, "main": true, "vas_main": true}
	for _, cl := range info {
		if cl.label != "" {
			defined[cl.label] = true
		}
		if cl.op == "GLOBAL" || cl.op == "EXTERN" || cl.op == "COMMON" {
			for _, a := range cl.args {
				name := strings.TrimSuffix(strings.TrimSpace(a), ",")
				if name != "" {
					roots[name] = true
				}
			}
		}
	}
	refs := map[string]bool{}
	for _, cl := range info {
		if cl.label != "" || cl.code == "" {
			continue
		}
		for _, tok := range symbolTokens(cl.code) {
			if defined[tok] {
				refs[tok] = true
			}
		}
	}

	var out []string
	inText := true
	for _, cl := range info {
		if cl.section != "" {
			inText = isTextSection(cl.section)
			out = append(out, cl.raw)
			continue
		}
		if inText && cl.label != "" && !roots[cl.label] && !refs[cl.label] {
			continue
		}
		out = append(out, cl.raw)
	}
	return out
}

func symbolTokens(s string) []string {
	var out []string
	quote := byte(0)
	for i := 0; i < len(s); {
		c := s[i]
		if quote != 0 {
			if c == '\\' && i+1 < len(s) {
				i += 2
			} else if c == quote {
				quote = 0
				i++
			} else {
				i++
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			i++
		case ';', '#':
			return out
		default:
			if isIdentStart(c) {
				j := i + 1
				for j < len(s) && isIdentChar(s[j]) {
					j++
				}
				out = append(out, s[i:j])
				i = j
			} else {
				i++
			}
		}
	}
	return out
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '.' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func removeUnreachable(lines []string) []string {
	info := analyzeCFG(lines)
	refs := map[string]bool{}
	roots := map[string]bool{"_start": true, "main": true, "vas_main": true}
	defined := map[string]bool{}

	for _, cl := range info {
		if cl.label != "" {
			defined[cl.label] = true
		}
		if cl.op == "GLOBAL" || cl.op == "EXTERN" || cl.op == "COMMON" {
			for _, a := range cl.args {
				name := strings.TrimSuffix(strings.TrimSpace(a), ",")
				if name != "" {
					roots[name] = true
				}
			}
		}
		if t := directSymbol(cl.op, cl.args); t != "" {
			refs[t] = true
		}
		if cl.op == "CALL" && len(cl.args) > 0 {
			t := strings.TrimSuffix(strings.TrimSpace(cl.args[0]), ",")
			if t != "" && !asm.IsReg(t) {
				refs[t] = true
			}
		}
		// A label may be reached indirectly through LEA or a data pointer.
		// Treat every textual reference as a root so its block is preserved.
		if cl.label == "" && cl.code != "" {
			for _, tok := range symbolTokens(cl.code) {
				if defined[tok] {
					refs[tok] = true
				}
			}
		}
	}

	var out []string
	reachable := true
	inText := true
	for _, cl := range info {
		if cl.section != "" {
			inText = isTextSection(cl.section)
			if inText {
				reachable = true
			}
			out = append(out, cl.raw)
			continue
		}
		if !inText {
			out = append(out, cl.raw)
			continue
		}
		if cl.label != "" {
			if !reachable && (refs[cl.label] || roots[cl.label]) {
				reachable = true
			}
			if !reachable {
				continue
			}
			out = append(out, cl.raw)
			continue
		}
		if !reachable {
			// Preserve blank/comment-only lines: they carry no execution and
			// keeping them avoids gratuitous formatting churn in output.
			if cl.code == "" {
				out = append(out, cl.raw)
			}
			continue
		}
		out = append(out, cl.raw)
		if isUnconditionalTerminator(cl.op) {
			reachable = false
		}
	}
	return out
}

func threadJumps(lines []string) []string {
	for pass := 0; pass < 8; pass++ {
		info := analyzeCFG(lines)
		labelJmp := map[string]string{}
		for i, cl := range info {
			if cl.label == "" {
				continue
			}
			// Find the first real instruction after the label.
			for j := i + 1; j < len(info); j++ {
				if info[j].code == "" {
					continue
				}
				if info[j].op == "JMP" {
					if t := directSymbol(info[j].op, info[j].args); t != "" && t != cl.label {
						labelJmp[cl.label] = t
					}
				}
				break
			}
		}
		if len(labelJmp) == 0 {
			return lines
		}

		changed := false
		for i := range lines {
			cl := info[i]
			if !isThreadableBranch(cl.op) {
				continue
			}
			target := directSymbol(cl.op, cl.args)
			if target == "" {
				continue
			}
			final := target
			seen := map[string]bool{}
			for steps := 0; steps < len(labelJmp) && !seen[final]; steps++ {
				seen[final] = true
				next, ok := labelJmp[final]
				if !ok || next == final {
					break
				}
				final = next
			}
			if final != target {
				lines[i] = branchLine(cl.op, final, cl.raw)
				changed = true
			}
		}
		if !changed {
			return lines
		}
	}
	return lines
}
