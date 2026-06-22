package opt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	movZeroRe     = regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*0$`)
	testCmpZeroRe = regexp.MustCompile(`^\tcmp\t([a-z][a-z0-9]+),\s*0$`)
	peepMovRe     = regexp.MustCompile(`^\tmov\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	peepAddRe     = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	peepSubImmRe  = regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*(-?\d+)$`)
	peepImulOneRe = regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*(\d+)$`)
	peepShlRe     = regexp.MustCompile(`^\tshl\t([a-z][a-z0-9]+),\s*(\d+)$`)
	noopAddZeroRe = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*0$`)
	noopSubZeroRe = regexp.MustCompile(`^\tsub\t([a-z][a-z0-9]+),\s*0$`)
	noopImulOneRe = regexp.MustCompile(`^\timul\t([a-z][a-z0-9]+),\s*1$`)
	pushRe        = regexp.MustCompile(`^\tpush\t([a-z][a-z0-9]+)$`)
	popRe         = regexp.MustCompile(`^\tpop\t([a-z][a-z0-9]+)$`)
	xorSelfRe     = regexp.MustCompile(`^\txor\t([a-z][a-z0-9]+),\s*([a-z][a-z0-9]+)$`)
	notRe         = regexp.MustCompile(`^\tnot\t([a-z][a-z0-9]+)$`)
	negRe         = regexp.MustCompile(`^\tneg\t([a-z][a-z0-9]+)$`)
	incRe         = regexp.MustCompile(`^\tinc\t([a-z][a-z0-9]+)$`)
	decRe         = regexp.MustCompile(`^\tdec\t([a-z][a-z0-9]+)$`)
	addNegFuseRe  = regexp.MustCompile(`^\tadd\t([a-z][a-z0-9]+),\s*1$`)
	pushModRe     = regexp.MustCompile(`^\t(add|sub|imul|mov|lea)\t([a-z][a-z0-9]+),.*$`)
)

func peephole(lines []string) []string {
	lines = xorZero(lines)
	lines = testCmp(lines)
	lines = nopMerge(lines)
	lines = leaFuse(lines)
	lines = noopElim(lines)
	lines = pushPopMov(lines)
	lines = xorMovElim(lines)
	lines = shlAddFuse(lines)
	lines = addNegFuse(lines)
	lines = cancelPairElim(lines)
	lines = pushModPopElim(lines)
	return lines
}

func xorZero(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := movZeroRe.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			r32 := regTo32(reg)
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] xor %s, %s  replaces mov %s, 0 (shorter encoding, sets ZF=1 CF=0 OF=0)",
					r32, r32, reg))
			}
			result = append(result, fmt.Sprintf("\txor\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

func testCmp(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		m := testCmpZeroRe.FindStringSubmatch(strings.TrimRight(line, " \t\r"))
		if m != nil {
			reg := m[1]
			r32 := regTo32(reg)
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] test %s, %s  replaces cmp %s, 0 (shorter encoding)",
					r32, r32, reg))
			}
			result = append(result, fmt.Sprintf("\ttest\t%s, %s", r32, r32))
		} else {
			result = append(result, line)
		}
	}
	return result
}

func nopMerge(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "nop" {
			count := 1
			for i+count < len(lines) && strings.TrimSpace(lines[i+count]) == "nop" {
				count++
			}
			if count > 1 {
				if isExplain() {
					result = append(result, fmt.Sprintf("; [OPT] merged %d nops into one", count))
				}
				result = append(result, "\tnop\t; merged "+strconv.Itoa(count)+" nops")
			} else {
				result = append(result, lines[i])
			}
			i += count
		} else {
			result = append(result, lines[i])
			i++
		}
	}
	return result
}

func leaFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			fused, ok := tryLeaFuse(lines[i], lines[i+1])
			if ok {
				if isExplain() {
					// Build description from the two fused lines
					line1 := strings.TrimSpace(lines[i])
					line2 := strings.TrimSpace(lines[i+1])
					result = append(result, fmt.Sprintf("; [OPT] lea fused from: %s ; %s", line1, line2))
				}
				result = append(result, fused)
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func tryLeaFuse(line1, line2 string) (string, bool) {
	m1 := peepMovRe.FindStringSubmatch(strings.TrimRight(line1, " \t\r"))
	if m1 == nil {
		return "", false
	}
	dst, src1 := m1[1], m1[2]

	if m2 := peepAddRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		addDst, src2 := m2[1], m2[2]
		if addDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s+%s]", dst, src1, src2), true
		}
	}

	if m2 := peepSubImmRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		subDst, imm := m2[1], m2[2]
		if subDst == dst {
			return fmt.Sprintf("\tlea\t%s, [%s-%s]", dst, src1, imm), true
		}
	}

	if m2 := peepImulOneRe.FindStringSubmatch(strings.TrimRight(line2, " \t\r")); m2 != nil {
		imulDst, kStr := m2[1], m2[2]
		if imulDst == dst {
			k, err := strconv.Atoi(kStr)
			if err == nil {
				scale := k - 1
				switch scale {
				case 1, 2, 4, 8:
					return fmt.Sprintf("\tlea\t%s, [%s+%s*%d]", dst, src1, src1, scale), true
				}
			}
		}
	}

	return "", false
}

func noopElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t\r")
		if m := peepMovRe.FindStringSubmatch(trimmed); m != nil && m[1] == m[2] {
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: mov %s, %s", m[1], m[2]))
			}
			continue
		}
		if noopAddZeroRe.MatchString(trimmed) {
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		if noopSubZeroRe.MatchString(trimmed) {
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		if noopImulOneRe.MatchString(trimmed) {
			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] removed no-op: %s", trimmed))
			}
			continue
		}
		result = append(result, line)
	}
	return result
}

func pushPopMov(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := pushRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			m2 := popRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m1 != nil && m2 != nil {
				if isExplain() {
					result = append(result, fmt.Sprintf("; [OPT] push %s; pop %s  replaced by mov %s, %s",
						m1[1], m2[1], m2[1], m1[1]))
				}
				result = append(result, fmt.Sprintf("\tmov\t%s, %s", m2[1], m1[1]))
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func xorMovElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := xorSelfRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil && m1[1] == m1[2] {
				reg32 := m1[1]
				reg64 := regTo64(reg32)
				// Build a regex that matches: mov <reg64>, <src-reg>
				// We construct this manually by checking the string prefix instead of a regex
				// to avoid per-call regex compilation.
				prefix := fmt.Sprintf("\tmov\t%s,", reg64)
				next := strings.TrimRight(lines[i+1], " \t\r")
				if strings.HasPrefix(next, prefix) {
					// Extract src register
					rest := strings.TrimSpace(next[len(prefix):])
					if rest != "" {
						srcField := strings.Fields(rest)
						if len(srcField) > 0 && len(srcField[0]) > 0 && srcField[0] != "" {
							srcReg := srcField[0]
							// Validate it looks like a register
							valid := true
							for _, c := range srcReg {
								if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
									valid = false
									break
								}
							}
							if valid {
								if isExplain() {
									result = append(result, fmt.Sprintf("; [OPT] xor %s,%s; mov %s,%s  replaced by mov %s, %s",
										m1[1], m1[2], reg64, srcReg, reg64, srcReg))
								}
								result = append(result, fmt.Sprintf("\tmov\t%s, %s", reg64, srcReg))
								i += 2
								continue
							}
						}
					}
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func shlAddFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+2 < len(lines) {
			m1 := peepMovRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			movDst, movSrc := m1[1], m1[2]

			m2 := peepShlRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 == nil || m2[1] != movDst {
				result = append(result, lines[i])
				i++
				continue
			}
			shiftStr := m2[2]

			m3 := peepAddRe.FindStringSubmatch(strings.TrimRight(lines[i+2], " \t\r"))
			if m3 == nil || m3[1] != movDst || m3[2] != movSrc {
				result = append(result, lines[i])
				i++
				continue
			}

			shift, err := strconv.Atoi(shiftStr)
			if err != nil || shift < 1 || shift > 3 {
				result = append(result, lines[i])
				i++
				continue
			}
			scale := 1 << uint(shift)

			if isExplain() {
				result = append(result, fmt.Sprintf("; [OPT] shl+add fused into lea %s, [%s+%s*%d]",
					movDst, movSrc, movSrc, scale))
			}
			result = append(result, fmt.Sprintf("\tlea\t%s, [%s+%s*%d]", movDst, movSrc, movSrc, scale))
			i += 3
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func addNegFuse(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := addNegFuseRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 == nil {
				result = append(result, lines[i])
				i++
				continue
			}
			reg := m1[1]

			m2 := negRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
			if m2 != nil && m2[1] == reg {
				if isExplain() {
					result = append(result, fmt.Sprintf("; [OPT] add %s,1; neg %s  replaced by not %s", reg, reg, reg))
				}
				result = append(result, fmt.Sprintf("\tnot\t%s", reg))
				i += 2
				continue
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

func cancelPairElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			m1 := notRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := notRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if isExplain() {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant not %s; not %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = negRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := negRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if isExplain() {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant neg %s; neg %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = incRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := decRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if isExplain() {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant inc %s; dec %s", reg, reg))
					}
					i += 2
					continue
				}
			}

			m1 = decRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			if m1 != nil {
				reg := m1[1]
				m2 := incRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[1] == reg {
					if isExplain() {
						result = append(result, fmt.Sprintf("; [OPT] removed redundant dec %s; inc %s", reg, reg))
					}
					i += 2
					continue
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}

// pushModPopElim removes push; instr reg, ... ; pop reg triples
// when the pop restores the old value and the intermediate result dies.
func pushModPopElim(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+2 < len(lines) {
			m1 := pushRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t\r"))
			m3 := popRe.FindStringSubmatch(strings.TrimRight(lines[i+2], " \t\r"))
			if m1 != nil && m3 != nil && m1[1] == m3[1] {
				reg := m1[1]
				m2 := pushModRe.FindStringSubmatch(strings.TrimRight(lines[i+1], " \t\r"))
				if m2 != nil && m2[2] == reg {
					if isExplain() {
						result = append(result, fmt.Sprintf("; [OPT] removed dead push %s; %s %s; pop %s",
							m1[1], m2[1], m2[2], m3[1]))
					}
					i += 3
					continue
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}
