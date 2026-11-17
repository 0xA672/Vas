package vas

import (
	"fmt"
	"strings"

	"vas/vas/internal/asm"
)

// RegInfo holds the physical register names for each size variant.
type RegInfo struct {
	Name64 string // e.g. "rax"
	Name32 string // e.g. "eax"
	Name16 string // e.g. "ax"
	Name8  string // e.g. "al"
}

// AllRegs maps VAS virtual register names to physical register info.
var AllRegs = map[string]RegInfo{
	"v0":  {Name64: "rax", Name32: "eax", Name16: "ax", Name8: "al"},
	"v1":  {Name64: "rbx", Name32: "ebx", Name16: "bx", Name8: "bl"},
	"v2":  {Name64: "rcx", Name32: "ecx", Name16: "cx", Name8: "cl"},
	"v3":  {Name64: "rdx", Name32: "edx", Name16: "dx", Name8: "dl"},
	"v4":  {Name64: "rsi", Name32: "esi", Name16: "si", Name8: "sil"},
	"v5":  {Name64: "rdi", Name32: "edi", Name16: "di", Name8: "dil"},
	"v6":  {Name64: "r8", Name32: "r8d", Name16: "r8w", Name8: "r8b"},
	"v7":  {Name64: "r9", Name32: "r9d", Name16: "r9w", Name8: "r9b"},
	"v8":  {Name64: "r11", Name32: "r11d", Name16: "r11w", Name8: "r11b"},
	"v9":  {Name64: "r12", Name32: "r12d", Name16: "r12w", Name8: "r12b"},
	"v10": {Name64: "r13", Name32: "r13d", Name16: "r13w", Name8: "r13b"},
	"v11": {Name64: "r14", Name32: "r14d", Name16: "r14w", Name8: "r14b"},
	"v12": {Name64: "r15", Name32: "r15d", Name16: "r15w", Name8: "r15b"},
}

// MaxVirtualReg is the highest valid virtual register number.
const MaxVirtualReg = 12

// IsValidVirtualReg reports whether s is a valid virtual register name (v0..v12).
func IsValidVirtualReg(s string) bool { return asm.IsVirtualReg(s) }

// VirtualRegNum returns the register number from "v0".."v12", or -1.
func VirtualRegNum(s string) int { return asm.VirtualRegNum(s) }

// PhysReg64 returns the 64-bit physical register for a virtual register name,
// or ("", false) if not a valid virtual register.
func PhysReg64(v string) (string, bool) {
	if info, ok := AllRegs[v]; ok {
		return info.Name64, true
	}
	return "", false
}

// PhysReg32 returns the 32-bit physical register for a 64-bit physical name,
// or the same string if not a known register.
func PhysReg32(r64 string) string {
	switch r64 {
	case "rax":
		return "eax"
	case "rbx":
		return "ebx"
	case "rcx":
		return "ecx"
	case "rdx":
		return "edx"
	case "rsi":
		return "esi"
	case "rdi":
		return "edi"
	case "rbp":
		return "ebp"
	case "rsp":
		return "esp"
	case "r8":
		return "r8d"
	case "r9":
		return "r9d"
	case "r10":
		return "r10d"
	case "r11":
		return "r11d"
	case "r12":
		return "r12d"
	case "r13":
		return "r13d"
	case "r14":
		return "r14d"
	case "r15":
		return "r15d"
	}
	return r64
}

// PhysReg64From32 returns the 64-bit physical register name from a 32-bit name.
func PhysReg64From32(r32 string) string {
	switch r32 {
	case "eax":
		return "rax"
	case "ebx":
		return "rbx"
	case "ecx":
		return "rcx"
	case "edx":
		return "rdx"
	case "esi":
		return "rsi"
	case "edi":
		return "rdi"
	case "ebp":
		return "rbp"
	case "esp":
		return "rsp"
	case "r8d":
		return "r8"
	case "r9d":
		return "r9"
	case "r10d":
		return "r10"
	case "r11d":
		return "r11"
	case "r12d":
		return "r12"
	case "r13d":
		return "r13"
	case "r14d":
		return "r14"
	case "r15d":
		return "r15"
	}
	return r32
}

// isPhysReg64 reports whether s is a known 64-bit physical register name.
func isPhysReg64(s string) bool {
	switch s {
	case "rax", "rbx", "rcx", "rdx", "rsi", "rdi", "rbp", "rsp",
		"r8", "r9", "r10", "r11", "r12", "r13", "r14", "r15":
		return true
	}
	return false
}

// isPhysReg reports whether s is any recognized physical register (64/32/16/8-bit).
func isPhysReg(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Try 64-bit
	if isPhysReg64(s) {
		return true
	}
	// 32-bit variants
	if PhysReg64From32(s) != s {
		return true
	}
	// 16-bit
	switch s {
	case "ax", "bx", "cx", "dx", "si", "di", "bp", "sp",
		"r8w", "r9w", "r10w", "r11w", "r12w", "r13w", "r14w", "r15w":
		return true
	}
	// 8-bit
	switch s {
	case "al", "bl", "cl", "dl", "sil", "dil", "bpl", "spl",
		"r8b", "r9b", "r10b", "r11b", "r12b", "r13b", "r14b", "r15b":
		return true
	}
	return false
}

// hasVirtualReg reports whether s contains any exact virtual register token.
// This is used as a fast-path check before running heavy optimizations.
func hasVirtualReg(s string) bool {
	for _, tok := range asm.OperandRegs(s) {
		if asm.IsVirtualReg(tok) {
			return true
		}
	}
	return false
}

func isIdentByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// VirtualRegisters returns every exact v0..v12 token in s, in order and
// without duplicates. It is used by CLI statistics and documentation tooling.
func VirtualRegisters(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, tok := range asm.OperandRegs(s) {
		if asm.IsVirtualReg(tok) && !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
	}
	return out
}

// mapReg maps exact virtual-register tokens in s to their physical
// counterparts. Identifiers such as v0_loop, av0 and v13_loop are left alone;
// quoted text and comments are copied verbatim. A standalone v13 token is
// rejected because it is neither a register nor a plausible symbol.
func mapReg(s string) (string, error) {
	if !hasVirtualReg(s) && strings.IndexByte(s, 'v') < 0 {
		return s, nil
	}
	var out strings.Builder
	out.Grow(len(s))
	quote := byte(0)

	for i := 0; i < len(s); {
		c := s[i]
		if quote != 0 {
			out.WriteByte(c)
			if c == '\\' && i+1 < len(s) {
				i++
				out.WriteByte(s[i])
			} else if c == quote {
				quote = 0
			}
			i++
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			out.WriteByte(c)
			i++
			continue
		case ';', '#':
			out.WriteString(s[i:])
			return out.String(), nil
		}

		if c == 'v' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
			j := i + 1
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			// Require a token boundary before the 'v' and after the digits.
			// The '.' case is a local label suffix and is left untouched.
			beforeOK := i == 0 || !isIdentByte(s[i-1])
			afterOK := j == len(s) || (!isIdentByte(s[j]) && s[j] != '.')
			if beforeOK && afterOK {
				name := s[i:j]
				if j < len(s) && s[j] == ':' {
					// Labels named v0..v12 keep their virtual spelling.
					out.WriteString(name)
					i = j
					continue
				}
				if info, ok := AllRegs[name]; ok {
					out.WriteString(info.Name64)
					i = j
					continue
				}
				// A standalone vN outside v0..v12 is invalid.
				return "", fmt.Errorf("%w: %s", ErrInvalidRegister, name)
			}
			out.WriteString(s[i:j])
			i = j
			continue
		}
		out.WriteByte(c)
		i++
	}
	return out.String(), nil
}
