package opt

import (
	"fmt"
	"strconv"
	"strings"
)

func copyPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, propagateBlock(block)...)
	}
	return result
}

func propagateBlock(lines []string) []string {
	alias := make([]int, 13)
	for i := range alias {
		alias[i] = -1
	}

	resolve := func(ri int) int {
		for ri >= 0 && alias[ri] >= 0 {
			ri = alias[ri]
		}
		return ri
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		code := strings.TrimSpace(line)
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result[i] = line
			continue
		}

		fields := strings.Fields(code)
		if len(fields) == 0 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(fields[0])
		args := fields[1:]

		hasVReg := false
		for _, a := range args {
			if regIndex(a) >= 0 {
				hasVReg = true
				break
			}
		}
		if !hasVReg {
			result[i] = line
			continue
		}

		dst := dstReg(op, args)

		propagated := make([]string, len(args))
		for j, a := range args {
			if j == 0 && dst >= 0 {
				propagated[j] = a
				continue
			}
			ri := regIndex(a)
			resolved := resolve(ri)
			if resolved >= 0 && resolved != ri {
				comma := ""
				if strings.HasSuffix(a, ",") {
					comma = ","
				}
				propagated[j] = fmt.Sprintf("v%d%s", resolved, comma)
			} else {
				propagated[j] = a
			}
		}

		newLine := fmt.Sprintf("\t%s\t%s", op, strings.Join(propagated, " "))
		if idx := strings.IndexAny(line, ";#"); idx >= 0 {
			newLine += line[idx:]
		}

		if dst >= 0 {
			for j := range alias {
				if alias[j] == dst {
					alias[j] = -1
				}
			}
		}

		if op == "MOV" && dst >= 0 && len(args) >= 2 {
			srcRi := resolve(regIndex(args[1]))
			if srcRi >= 0 {
				alias[dst] = srcRi
			}
		} else if dst >= 0 {
			alias[dst] = -1
		}
		result[i] = newLine
	}
	return result
}

// ---------------------------------------------------------------------------
// Pre-expansion: constant propagation (MOVI vX, imm -> fold subsequent uses)
// ---------------------------------------------------------------------------

func constPropagate(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, constBlock(block)...)
	}
	return result
}

func constBlock(lines []string) []string {
	constVal := make([]int64, 13)
	constKnown := make([]bool, 13)
	used := map[int]bool{}
	moviLine := map[int]int{}

	parseArg := func(a string) (int64, bool) {
		a = strings.TrimRight(a, ",")
		ri := regIndex(a)
		if ri >= 0 && constKnown[ri] {
			return constVal[ri], true
		}
		n, err := strconv.ParseInt(a, 0, 64)
		if err == nil {
			return n, true
		}
		return 0, false
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result[i] = line
			continue
		}

		tokens := tokenizeFold(code)
		if len(tokens) < 2 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(tokens[0])
		args := tokens[1:]

		reads := readRegs(op, args)
		folded := false

		switch op {
		case "MOVI":
			if len(args) >= 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				imm, err := strconv.ParseInt(args[1], 0, 64)
				if dstRi >= 0 && err == nil {
					constVal[dstRi] = imm
					constKnown[dstRi] = true
					moviLine[dstRi] = i
					delete(used, dstRi)
				}
			}
			reads = nil
		case "ADD", "SUB":
			if len(args) == 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					if constKnown[dstRi] {
						if imm, ok := parseArg(args[1]); ok {
							var val int64
							switch op {
							case "ADD":
								val = constVal[dstRi] + imm
							case "SUB":
								val = constVal[dstRi] - imm
							}
							constVal[dstRi] = val
							constKnown[dstRi] = true
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
							reads = nil
							folded = true
							continue
						}
					}
					constKnown[dstRi] = false
				}
			} else if len(args) == 3 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi < 0 {
					result[i] = line
					continue
				}
				v1, ok1 := parseArg(args[1])
				v2, ok2 := parseArg(args[2])
				if ok1 && ok2 {
					var val int64
					switch op {
					case "ADD":
						val = v1 + v2
					case "SUB":
						val = v1 - v2
					}
					constVal[dstRi] = val
					constKnown[dstRi] = true
					comment := ""
					if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
						comment = " " + trimmed[idx:]
					}
					result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
					folded = true
					continue
				}
				constKnown[dstRi] = false
			}
		case "MUL":
			if len(args) == 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					if constKnown[dstRi] {
						if imm, ok := parseArg(args[1]); ok {
							val := constVal[dstRi] * imm
							constVal[dstRi] = val
							constKnown[dstRi] = true
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
							reads = nil
							folded = true
							continue
						}
					}
					constKnown[dstRi] = false
				}
			} else if len(args) == 3 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					v1, ok1 := parseArg(args[1])
					v2, ok2 := parseArg(args[2])
					if ok1 && ok2 {
						val := v1 * v2
						constVal[dstRi] = val
						constKnown[dstRi] = true
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
						folded = true
						continue
					}
					constKnown[dstRi] = false
				}
			}
		case "MOV":
			if len(args) >= 2 {
				dst := args[0]
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					src := strings.TrimRight(args[1], ",")
					srcRi := regIndex(src)
					if srcRi >= 0 && constKnown[srcRi] {
						cp := constVal[srcRi]
						constVal[dstRi] = cp
						constKnown[dstRi] = true
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result[i] = fmt.Sprintf("\tMOVI\t%s, %d%s", dst, cp, comment)
						reads = nil
						folded = true
						continue
					} else {
						constKnown[dstRi] = false
					}
				}
			}
		case "SYSCALL", "INT":
			for _, r := range []int{0, 3, 4, 5, 6, 7, 8} {
				constKnown[r] = false
			}
		default:
			dst := dstReg(op, args)
			if dst >= 0 {
				constKnown[dst] = false
			}
		}
		if !folded {
			for _, r := range reads {
				used[r] = true
			}
		}
		result[i] = line
	}
	return result
}

// deadStoreElim removes STORE instructions whose target label is stored again
// before any LOAD within the same basic block.
func deadStoreElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, elimDeadStoreBlock(block)...)
	}
	return result
}

func elimDeadStoreBlock(lines []string) []string {
	lastAccess := map[string]string{} // label -> "STORE" or "LOAD"
	remove := make([]bool, len(lines))

	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}
		fields := strings.Fields(code)
		if len(fields) < 2 {
			continue
		}
		op := strings.ToUpper(fields[0])
		if op != "STORE" && op != "LOAD" {
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				lastAccess = map[string]string{}
			}
			continue
		}
		args := fields[1:]
		if len(args) < 2 {
			continue
		}
		label := extractLabel(args[1])
		if label == "" {
			continue
		}

		prev := lastAccess[label]
		if op == "STORE" && prev == "STORE" {
			remove[i] = true
		}
		lastAccess[label] = op
	}

	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if !remove[i] {
			result = append(result, line)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Pre-expansion: strength reduction (MUL by power-of-2 -> SHL)
// ---------------------------------------------------------------------------
func strengthReduce(lines []string) []string {
	var result []string
	for _, line := range lines {
		reduced := reduceLine(line)
		if strings.Contains(reduced, "\n") {
			result = append(result, strings.Split(reduced, "\n")...)
		} else {
			result = append(result, reduced)
		}
	}
	return result
}

func reduceLine(line string) string {
	trimmed := strings.TrimSpace(line)
	code := trimmed
	if idx := strings.IndexAny(code, ";#"); idx >= 0 {
		code = strings.TrimSpace(code[:idx])
	}
	if code == "" {
		return line
	}
	fields := strings.Fields(code)
	if len(fields) < 2 {
		return line
	}
	op := strings.ToUpper(fields[0])
	if op != "MUL" {
		return line
	}

	arg := func(i int) string {
		s := fields[i]
		s = strings.TrimRight(s, ",")
		return s
	}

	if len(fields) == 3 {
		dst := arg(1)
		imm, err := strconv.ParseInt(arg(2), 0, 64)
		if err != nil || imm <= 0 {
			return line
		}
		if result := decomposeMul2Op(dst, imm, trimmed); result != "" {
			return result
		}
		return line
	} else if len(fields) == 4 {
		dst := arg(1)
		src := arg(2)
		imm, err := strconv.ParseInt(arg(3), 0, 64)
		if err != nil || imm <= 0 {
			return line
		}
		if result := decomposeMul3Op(dst, src, imm, trimmed); result != "" {
			return result
		}
		return line
	}
	return line
}

func decomposeMul2Op(dst string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tshl\t%s, %d%s", dst, shift, comment)
	}

	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, dst, dst, scale, comment)
	}

	shift := int64(0)
	odd := C
	for odd%2 == 0 {
		odd /= 2
		shift++
	}

	if shift > 0 {
		oddScale := odd - 1
		switch oddScale {
		case 1, 2, 4, 8:
			return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, dst, dst, oddScale, comment, dst, shift, comment)
		}
	}

	return ""
}

func decomposeMul3Op(dst, src string, C int64, trimmed string) string {
	comment := ""
	if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
		comment = " " + trimmed[idx:]
	}

	if isPowerOf2(C) && C <= 0x80000000 {
		shift := log2(C)
		return fmt.Sprintf("\tMOV\t%s, %s%s\n\tshl\t%s, %d%s", dst, src, comment, dst, shift, comment)
	}

	scale := C - 1
	switch scale {
	case 1, 2, 4, 8:
		return fmt.Sprintf("\tLEA\t%s, [%s+%s*%d]%s", dst, src, src, scale, comment)
	}

	shift := int64(0)
	odd := C
	for odd%2 == 0 {
		odd /= 2
		shift++
	}
	if shift > 0 {
		oddScale := odd - 1
		switch oddScale {
		case 1, 2, 4, 8:
			return fmt.Sprintf("\tMOV\t%s, %s%s\n\tLEA\t%s, [%s+%s*%d]%s\n\tshl\t%s, %d%s",
				dst, src, comment, dst, dst, dst, oddScale, comment, dst, shift, comment)
		}
	}

	for k := int64(2); k <= 3; k++ {
		if C == (1<<k)-1 {
			return fmt.Sprintf("\tLEA\t%s, [%s*%d]%s\n\tSUB\t%s, %s%s",
				dst, src, 1<<k, comment, dst, src, comment)
		}
	}

	return ""
}

func isPowerOf2(n int64) bool {
	return n > 0 && (n&(n-1)) == 0
}

func log2(n int64) int {
	r := 0
	for n > 1 {
		n >>= 1
		r++
	}
	return r
}

// ---------------------------------------------------------------------------
// Pre-expansion: STORE-LOAD forwarding
// ---------------------------------------------------------------------------

func storeLoadFwd(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, fwdBlock(block)...)
	}
	return result
}

func fwdBlock(lines []string) []string {
	lastStore := map[string]int{}

	result := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			lastStore = map[string]int{}
			result[i] = line
			continue
		}

		fields := strings.Fields(code)
		if len(fields) < 2 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(fields[0])
		args := fields[1:]

		switch op {
		case "STORE":
			if len(args) >= 2 {
				src := strings.TrimRight(args[0], ",")
				srcRi := regIndex(src)
				if srcRi >= 0 {
					label := extractLabel(args[1])
					if label != "" {
						lastStore[label] = srcRi
					}
				}
			}
		case "LOAD":
			if len(args) >= 2 {
				dst := strings.TrimRight(args[0], ",")
				dstRi := regIndex(dst)
				if dstRi >= 0 {
					label := extractLabel(args[1])
					if label != "" {
						if srcRi, ok := lastStore[label]; ok {
							comment := ""
							if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
								comment = " " + trimmed[idx:]
							}
							result[i] = fmt.Sprintf("\tMOV\t%s, v%d%s", dst, srcRi, comment)
							continue
						}
					}
				}
			}
		default:
			if op == "CALL" || op == "SYSCALL" || op == "INT" {
				lastStore = map[string]int{}
			} else if len(fields) >= 2 {
				firstArg := strings.TrimRight(args[0], ",")
				if strings.HasPrefix(firstArg, "[") {
					lastStore = map[string]int{}
				}
			}
		}

		result[i] = line
	}
	return result
}
