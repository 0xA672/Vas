// Package opt implements the -O1 and -O2 optimization passes for VAS.
//
// Entry points:
//   - Optimize(input, level)        string-based pipeline
//   - OptimizeLines(lines, level)   []string-based pipeline (no join overhead)
//   - PeepholeOnly(input)           string-based peephole only
//   - PeepholeOnlyLines(lines)      []string-based peephole only
//   - FoldConstants(lines)          constant folding only
//   - SetExplain(bool)              emit [OPT] diagnostics
package opt

import (
	"strings"
	"sync"
)

// regTo32 returns the 32-bit name for a 64-bit register.
func regTo32(reg string) string {
	switch reg {
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
	return reg
}

// regTo64 returns the 64-bit name for a 32-bit register.
func regTo64(reg string) string {
	switch reg {
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
	return reg
}

// explainEnabled controls whether peephole passes insert explanatory
// comments. Guarded by a sync.RWMutex for concurrent callers.
var explainEnabled bool
var explainMu sync.RWMutex

// SetExplain atomically updates the explain mode flag.
func SetExplain(v bool) {
	explainMu.Lock()
	explainEnabled = v
	explainMu.Unlock()
}

// isExplain returns the current explain mode.
func isExplain() bool {
	explainMu.RLock()
	defer explainMu.RUnlock()
	return explainEnabled
}

// hasVirtualReg reports whether s contains any virtual register token (v0..v12).
func hasVirtualReg(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 'v' && i+1 < len(s) {
			c := s[i+1]
			if c >= '0' && c <= '9' {
				if i+2 >= len(s) {
					return true
				}
				if i+2 < len(s) && s[i+2] >= '0' && s[i+2] <= '9' {
					// Two digits: check v10..v12
					if i+3 >= len(s) || (s[i+3] < '0' || s[i+3] > '9') {
						n := int(c-'0')*10 + int(s[i+2]-'0')
						if n <= 12 {
							return true
						}
						i++
						continue
					}
					i++
					continue
				}
				return true
			}
		}
	}
	return false
}

// hasVirtualRegLine reports whether any line contains a virtual register token.
func hasVirtualRegLine(lines []string) bool {
	for _, line := range lines {
		if hasVirtualReg(line) {
			return true
		}
	}
	return false
}

// PeepholeOnlyLines is the []string version of PeepholeOnly.
func PeepholeOnlyLines(lines []string) []string {
	return peephole(lines)
}

// PeepholeOnly runs only peephole optimizations on fully assembled output.
func PeepholeOnly(input string) string {
	return strings.Join(PeepholeOnlyLines(strings.Split(input, "\n")), "\n")
}

// runPipeline executes the optimization pipeline for the given level.
func runPipeline(lines []string, level int) []string {
	if !hasVirtualRegLine(lines) {
		return lines
	}

	// O1: dataflow + peephole
	lines = copyPropagate(lines)
	lines = constPropagate(lines)
	lines = strengthReduce(lines)
	lines = storeLoadFwd(lines)
	lines = deadStoreElim(lines)
	lines = deadCodeElim(lines)

	lines = peephole(lines)
	lines = peephole(lines)

	if level >= 2 {
		lines = cse(lines)
		lines = licm(lines)
		lines = redundantLoadElim(lines)
		lines = pushPopElim(lines)
		lines = tailCallOpt(lines)
	}
	return lines
}

// OptimizeLines runs the optimization pipeline directly on []string input.
func OptimizeLines(lines []string, level int) []string {
	if level <= 0 {
		return lines
	}
	return runPipeline(lines, level)
}

// Optimize runs all enabled optimization passes on VAS source.
// level 0 = no optimization, level >=1 = -O1.
// IMPORTANT: only works on VAS source (with virtual registers v0..v12),
// not on raw NASM output.
func Optimize(input string, level int) string {
	if level <= 0 {
		return input
	}
	if !hasVirtualReg(input) {
		return input
	}
	return strings.Join(runPipeline(strings.Split(input, "\n"), level), "\n")
}
