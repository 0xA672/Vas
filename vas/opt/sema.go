package opt

import (
	"strconv"
	"strings"

	"vas/vas/internal/asm"
)

// parsedLine is the optimizer's lightweight view of one source line.
type parsedLine struct {
	op   string
	args []string
	code string // comment-free, trimmed code
	raw  string
}

func parseOptLine(line string) (parsedLine, bool) {
	code := asm.TrimCode(line)
	if code == "" {
		return parsedLine{raw: line}, false
	}
	op, args, ok := asm.Parse(code)
	if !ok {
		return parsedLine{code: code, raw: line}, false
	}
	return parsedLine{op: op, args: args, code: code, raw: line}, true
}

func stripComment(line string) string { return asm.TrimCode(line) }

func tokenizeFold(line string) []string {
	op, args, ok := asm.Parse(line)
	if !ok {
		return asm.Fields(line)
	}
	return append([]string{op}, args...)
}

func regIndex(s string) int {
	return asm.VirtualRegNum(strings.TrimRight(strings.TrimSpace(s), ","))
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// virtualRegsIn returns the virtual-register indices referenced anywhere in an
// operand, including effective addresses like [v1+v2*8+16].
func virtualRegsIn(operand string) []int {
	toks := asm.OperandRegs(operand)
	out := make([]int, 0, len(toks))
	seen := make(map[int]bool, len(toks))
	for _, t := range toks {
		if n := asm.VirtualRegNum(t); n >= 0 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}

func touchesPhysicalReg(line string) bool {
	for _, tok := range asm.OperandRegs(line) {
		if asm.IsPhysReg(tok) {
			return true
		}
	}
	return false
}

func hasMemoryOperand(args []string) bool {
	for _, a := range args {
		if strings.Contains(a, "[") {
			return true
		}
	}
	return false
}

func allArgsHaveOnlyVirtualRegs(args []string) bool {
	for _, a := range args {
		if strings.ContainsAny(a, "\"'`[]") {
			return false
		}
		for _, tok := range asm.OperandRegs(a) {
			if !asm.IsVirtualReg(tok) {
				return false
			}
		}
	}
	return true
}

// flagsLiveAfter reports whether the EFLAGS value produced before line idx can
// be observed before a later instruction overwrites it. It walks forward
// across basic blocks conservatively; an unconditional transfer of control is
// treated as "live" because another path may consume the flags.
func flagsLiveAfter(lines []string, idx int) bool {
	for j := idx + 1; j < len(lines); j++ {
		p, ok := parseOptLine(lines[j])
		if !ok {
			continue
		}
		op := strings.ToLower(p.op)
		if asm.ReadsFlags(op) {
			return true
		}
		if asm.WritesFlags(op) {
			return false
		}
		switch op {
		case "jmp", "jmpq":
			return true
		}
	}
	return false
}

// flagsUsedBetween reports whether flags are consumed between from+1 and to-1
// before any instruction writes them.
func flagsUsedBetween(lines []string, from, to int) bool {
	for j := from + 1; j < to; j++ {
		p, ok := parseOptLine(lines[j])
		if !ok {
			continue
		}
		op := strings.ToLower(p.op)
		if asm.ReadsFlags(op) {
			return true
		}
		if asm.WritesFlags(op) {
			return false
		}
		if op == "jmp" || op == "jmpq" {
			return true
		}
	}
	return false
}

func writesFlags(line string) bool {
	p, ok := parseOptLine(line)
	return ok && asm.WritesFlags(strings.ToLower(p.op))
}

// memKey canonicalises a memory operand. The second result is false when the
// operand is not a bracketed memory reference.
func memKey(operand string) (string, bool) {
	operand = strings.TrimSpace(operand)
	if !strings.Contains(operand, "[") {
		return "", false
	}
	return asm.AddrKey(operand), true
}

// addressVersions returns a compact version fingerprint for all virtual
// registers appearing in an address. Physical-register addresses are
// represented by "*" and must be treated as aliasing everything.
func addressVersions(operand string, versions *[13]int) string {
	toks := asm.OperandRegs(operand)
	if len(toks) == 0 {
		return ""
	}
	var b strings.Builder
	for _, tok := range toks {
		b.WriteByte('|')
		if n := asm.VirtualRegNum(tok); n >= 0 {
			b.WriteString(strconv.Itoa(versions[n]))
		} else {
			b.WriteByte('*')
		}
	}
	return b.String()
}

// addressHasPhysical reports whether an effective address uses a physical
// register. Such addresses alias conservatively across the virtual world.
func addressHasPhysical(operand string) bool {
	for _, tok := range asm.OperandRegs(operand) {
		if asm.IsPhysReg(tok) {
			return true
		}
	}
	return false
}

func isStaticMemory(operand string) bool {
	for _, tok := range asm.OperandRegs(operand) {
		if asm.IsPhysReg(tok) || asm.IsVirtualReg(tok) {
			return false
		}
	}
	return strings.Contains(operand, "[")
}

// parseNASMInt parses NASM integer literals. Unlike strconv.ParseInt(s, 0, 64)
// a leading zero is decimal in NASM, not octal; 0o/q/J suffixes are octal and
// h/b prefix/suffix forms are supported. Character constants such as 'A' are
// also accepted.
func parseNASMInt(s string) (int64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), "_", "")
	if len(s) == 3 && (s[0] == '\'' || s[0] == '"') && s[2] == s[0] {
		return int64(s[1]), true
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	if s == "" {
		return 0, false
	}
	base := 10
	digits := s
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "0x"):
		base, digits = 16, s[2:]
	case strings.HasPrefix(lower, "0b"):
		base, digits = 2, s[2:]
	case strings.HasPrefix(lower, "0o"):
		base, digits = 8, s[2:]
	case strings.HasPrefix(lower, "0d"):
		base, digits = 10, s[2:]
	default:
		last := lower[len(lower)-1]
		switch last {
		case 'h':
			base, digits = 16, s[:len(s)-1]
		case 'b':
			// A trailing b is binary only when the remainder is 0/1.
			candidate := s[:len(s)-1]
			if candidate != "" && strings.Trim(candidate, "01") == "" {
				base, digits = 2, candidate
			}
		case 'o', 'q':
			base, digits = 8, s[:len(s)-1]
		case 'd':
			base, digits = 10, s[:len(s)-1]
		}
	}
	u, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return 0, false
	}
	v := int64(u)
	if neg {
		v = -v
	}
	return v, true
}

func isUnknownOrClobber(p parsedLine) bool {
	op := strings.ToLower(p.op)
	if asm.ClobbersAllRegs(op) {
		return true
	}
	return false
}
