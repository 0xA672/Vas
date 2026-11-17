// Package asm contains the lexical and semantic primitives shared by the VAS
// assembler, optimizer, preprocessor and linter.
//
// The package deliberately knows nothing about files or preprocessing: it is a
// small, allocation-conscious lexer for one line of x86-64/NASM-style source.
// Keeping it in one place is important because previous passes each had their
// own subtly different quote, comment and operand rules.
package asm

import "strings"

// StripComment returns the code part of line, with an inline ; or # comment
// removed. Semicolons and hashes inside ', " or ` quoted strings are preserved.
// A backslash escapes the next byte inside quoted strings, matching NASM.
func StripComment(line string) string {
	quote := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case ';', '#':
			return strings.TrimRight(line[:i], " \t\r")
		}
	}
	return strings.TrimRight(line, " \t\r")
}

// TrimCode trims surrounding whitespace from the code part of line.
func TrimCode(line string) string {
	return strings.TrimSpace(StripComment(line))
}

// SplitOperands splits s at top-level commas. Commas inside quotes and square
// brackets are preserved. When s contains no top-level comma, it is split on
// whitespace at bracket depth zero, which supports both NASM
// ("jmp label") and pseudo forms ("MOVI v0, 1").
func SplitOperands(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	hasComma := false
	depth := 0
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
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
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				hasComma = true
				i = len(s)
			}
		}
	}

	var out []string
	var cur strings.Builder
	depth = 0
	quote = 0
	flush := func() {
		if v := strings.TrimSpace(cur.String()); v != "" {
			out = append(out, v)
		}
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			cur.WriteByte(c)
		case '[':
			depth++
			cur.WriteByte(c)
		case ']':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case ',':
			if depth == 0 && hasComma {
				flush()
				continue
			}
			cur.WriteByte(c)
		default:
			if !hasComma && depth == 0 && (c == ' ' || c == '\t') {
				flush()
			} else {
				cur.WriteByte(c)
			}
		}
	}
	flush()
	return out
}

// Parse splits a source line into an opcode and its operands. The opcode is
// upper-cased. ok is false for labels, blank lines and directives beginning
// with '.'.
func Parse(line string) (op string, args []string, ok bool) {
	code := TrimCode(line)
	if code == "" || strings.HasPrefix(code, ".") {
		return "", nil, false
	}
	i := 0
	for i < len(code) && code[i] != ' ' && code[i] != '\t' && code[i] != ',' {
		i++
	}
	op = strings.ToUpper(code[:i])
	if op == "" || strings.HasSuffix(op, ":") {
		return "", nil, false
	}
	args = SplitOperands(code[i:])
	return op, args, true
}

// Fields splits code into whitespace-separated fields while keeping bracketed
// expressions and quoted strings intact.
func Fields(s string) []string {
	var out []string
	var cur strings.Builder
	depth := 0
	quote := byte(0)
	flush := func() {
		if v := strings.TrimSpace(cur.String()); v != "" {
			out = append(out, v)
		}
		cur.Reset()
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			cur.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			cur.WriteByte(c)
		case '[':
			depth++
			cur.WriteByte(c)
		case ']':
			if depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		default:
			if depth == 0 && (c == ' ' || c == '\t') {
				flush()
			} else {
				cur.WriteByte(c)
			}
		}
	}
	flush()
	return out
}

// VirtualRegNum returns 0..12 for an exact virtual-register token and -1 for
// any other string. Prefixes such as "v0_table" or "v13_loop" are not
// registers.
func VirtualRegNum(s string) int {
	if len(s) < 2 || s[0] != 'v' {
		return -1
	}
	n := len(s)
	if n == 2 && s[1] >= '0' && s[1] <= '9' {
		return int(s[1] - '0')
	}
	if n == 3 && s[1] == '1' && s[2] >= '0' && s[2] <= '2' {
		return 10 + int(s[2]-'0')
	}
	return -1
}

// IsVirtualReg reports whether s is exactly v0..v12.
func IsVirtualReg(s string) bool { return VirtualRegNum(s) >= 0 }

var physRegs = func() map[string]bool {
	names := []string{
		"rax", "rbx", "rcx", "rdx", "rsi", "rdi", "rbp", "rsp",
		"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15",
		"eax", "ebx", "ecx", "edx", "esi", "edi", "ebp", "esp",
		"r8d", "r9d", "r10d", "r11d", "r12d", "r13d", "r14d", "r15d",
		"ax", "bx", "cx", "dx", "si", "di", "bp", "sp",
		"r8w", "r9w", "r10w", "r11w", "r12w", "r13w", "r14w", "r15w",
		"al", "bl", "cl", "dl", "sil", "dil", "bpl", "spl", "ah", "bh", "ch", "dh",
		"r8b", "r9b", "r10b", "r11b", "r12b", "r13b", "r14b", "r15b",
		"rip", "eip", "ip", "cs", "ds", "es", "fs", "gs", "ss",
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}()

// IsPhysReg reports whether s is a recognised physical register or segment
// register. Matching is case-insensitive.
func IsPhysReg(s string) bool { return physRegs[strings.ToLower(s)] }

// IsReg reports whether s is a virtual or physical register.
func IsReg(s string) bool { return IsVirtualReg(s) || IsPhysReg(s) }

// OperandRegs returns every register token referenced by operand, including
// registers nested in an effective address such as "[v0+v1*8+16]". Tokens that
// are not registers (labels, numbers, scale factors) are ignored.
func OperandRegs(operand string) []string {
	var out []string
	seen := make(map[string]bool, 4)
	start := -1
	for i := 0; i <= len(operand); i++ {
		var c byte
		if i < len(operand) {
			c = operand[i]
		}
		ident := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_'
		if ident {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			tok := operand[start:i]
			if (IsVirtualReg(tok) || IsPhysReg(tok)) && !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
			start = -1
		}
	}
	return out
}

// AddrKey canonicalises a memory operand for conservative alias bookkeeping:
// whitespace is removed and the string is lower-cased. It is intentionally
// only an equality key; alias analysis still compares register sets.
func AddrKey(operand string) string {
	var b strings.Builder
	for i := 0; i < len(operand); i++ {
		c := operand[i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// IsCondJump reports whether op is a conditional jump or loop instruction.
func IsCondJump(op string) bool {
	op = strings.ToLower(op)
	if strings.HasPrefix(op, "j") {
		switch op {
		case "jmp", "jmpq":
			return false
		}
		return true
	}
	switch op {
	case "loop", "loope", "loopne", "loopz", "loopnz", "jcxz", "jecxz", "jrcxz":
		return true
	}
	return false
}

// ReadsFlags reports whether op consumes EFLAGS as an input.
func ReadsFlags(op string) bool {
	op = strings.ToLower(op)
	if IsCondJump(op) {
		return true
	}
	if strings.HasPrefix(op, "set") || strings.HasPrefix(op, "cmov") {
		return true
	}
	switch op {
	case "adc", "sbb", "rcl", "rcr", "lahf", "pushf", "pushfq", "into",
		"loop", "loope", "loopne", "loopz", "loopnz", "ret", "retf", "iret", "iretq":
		return true
	}
	return false
}

// WritesFlags reports whether op writes EFLAGS.
func WritesFlags(op string) bool {
	op = strings.ToLower(op)
	switch op {
	case "add", "sub", "adc", "sbb", "and", "or", "xor", "cmp", "test",
		"inc", "dec", "neg", "shl", "shr", "sar", "sal", "rol", "ror",
		"rcl", "rcr", "imul", "mul", "idiv", "div", "bsf", "bsr",
		"popcnt", "lzcnt", "tzcnt", "bt", "bts", "btr", "btc",
		"cmpxchg", "xadd", "xchg", "crc32", "paddb", "paddw", "paddd", "paddq",
		"psubb", "psubw", "psubd", "psubq", "pand", "por", "pxor",
		"call", "syscall", "sysenter", "int", "int3", "into":
		return true
	}
	return false
}

// ClobbersAllRegs reports whether op may destroy arbitrary register state.
func ClobbersAllRegs(op string) bool {
	switch strings.ToLower(op) {
	case "call", "syscall", "sysenter", "int", "int3", "iret", "iretq":
		return true
	}
	return false
}

// IsBranch reports whether op ends a basic block.
func IsBranch(op string) bool {
	op = strings.ToLower(op)
	switch op {
	case "jmp", "jmpq", "ret", "retf", "iret", "iretq", "call", "syscall", "int", "int3", "hlt":
		return true
	}
	return IsCondJump(op)
}
