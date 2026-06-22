package opt

import (
	"fmt"
	"strings"
)

func cse(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, cseBlock(block)...)
	}
	return result
}

type cseKey struct {
	op   string
	args string
}

func cseBlock(lines []string) []string {
	seen := map[cseKey]string{}
	regVals := map[string]cseKey{}

	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			result = append(result, line)
			continue
		}
		tokens := strings.Fields(code)
		if len(tokens) < 2 {
			result = append(result, line)
			continue
		}
		op := strings.ToUpper(tokens[0])
		args := tokens[1:]

		dst := dstReg(op, args)
		if dst >= 0 {
			dstName := args[0]
			if prevKey, ok := regVals[dstName]; ok {
				delete(seen, prevKey)
			}
		}

		if len(args) >= 2 && (op == "ADD" || op == "SUB" || op == "MUL") {
			cleanArgs := make([]string, len(args[1:]))
			for i, a := range args[1:] {
				cleanArgs[i] = strings.TrimRight(a, ",")
			}
			key := cseKey{op: op, args: strings.Join(cleanArgs, " ")}
			if prevDst, ok := seen[key]; ok && prevDst != args[0] {
				seen[key] = args[0]
				if dst >= 0 {
					regVals[args[0]] = key
				}
				result = append(result, line)
				continue
			}
			seen[key] = args[0]
			if dst >= 0 {
				regVals[args[0]] = key
			}
		}

		result = append(result, line)
	}
	return result
}

func licm(lines []string) []string {
	labelIdx := map[string]int{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}
		if strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSuffix(trimmed, ":")
			labelIdx[name] = i
		}
	}

	type loop struct {
		header int
		back   int
	}
	var loops []loop
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		code := trimmed
		if idx := strings.IndexAny(code, ";#"); idx >= 0 {
			code = strings.TrimSpace(code[:idx])
		}
		if code == "" {
			continue
		}
		fields := strings.Fields(code)
		if len(fields) != 2 {
			continue
		}
		op := strings.ToUpper(fields[0])
		switch op {
		case "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE":
			target := fields[1]
			if idx, ok := labelIdx[target]; ok && idx < i {
				loops = append(loops, loop{header: idx, back: i})
			}
		}
	}

	if len(loops) == 0 {
		return lines
	}

	for li := len(loops) - 1; li >= 0; li-- {
		l := loops[li]
		isNested := false
		for _, outer := range loops {
			if outer.header < l.header && outer.back > l.back {
				isNested = true
			}
		}
		if isNested {
			continue
		}

		modified := map[string]bool{}
		for j := l.header + 1; j <= l.back; j++ {
			trimmed := strings.TrimSpace(lines[j])
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
			args := fields[1:]
			if dst := dstReg(op, args); dst >= 0 && len(args) >= 1 {
				modified[args[0]] = true
			}
		}

		var hoisted []int
		var hoistedLines []string
		for j := l.header + 1; j < l.back; j++ {
			trimmed := strings.TrimSpace(lines[j])
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
			args := fields[1:]

			isInvariant := false
			if op == "LEA" && len(args) >= 2 {
				memOp := args[1]
				memOp = strings.TrimPrefix(memOp, "[")
				memOp = strings.TrimSuffix(memOp, "]")
				if plusIdx := strings.Index(memOp, "+"); plusIdx >= 0 {
					memOp = memOp[:plusIdx]
				}
				if minusIdx := strings.Index(memOp, "-"); minusIdx >= 0 {
					memOp = memOp[:minusIdx]
				}
				dstName := strings.TrimRight(args[0], ",")
				if !modified[memOp] && !modified[dstName] {
					isInvariant = true
				}
			}

			if isInvariant {
				hoisted = append(hoisted, j)
				hoistedLines = append(hoistedLines, lines[j])
			}
		}

		if len(hoistedLines) > 0 {
			var newResult []string
			for i, line := range lines {
				if i == l.header {
					newResult = append(newResult, hoistedLines...)
				}
				isHoisted := false
				for _, h := range hoisted {
					if i == h {
						isHoisted = true
						break
					}
				}
				if !isHoisted {
					newResult = append(newResult, line)
				}
			}
			lines = newResult
			break
		}
	}
	return lines
}

func redundantLoadElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, redundantLoadBlock(block)...)
	}
	return result
}

func redundantLoadBlock(lines []string) []string {
	lastLoad := map[string]string{}
	addrModified := map[string]bool{}

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
		fields := strings.Fields(code)
		if len(fields) < 2 {
			result[i] = line
			continue
		}
		op := strings.ToUpper(fields[0])
		args := fields[1:]

		if op == "LOAD" && len(args) >= 2 {
			addr := args[1]
			addrReg := strings.TrimRight(strings.TrimLeft(addr, "["), "]")
			if prevDst, ok := lastLoad[addr]; ok && !addrModified[addrReg] {
				dst := strings.TrimRight(args[0], ",")
				src := strings.TrimRight(prevDst, ",")
				comment := ""
				if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
					comment = " " + trimmed[idx:]
				}
				result[i] = fmt.Sprintf("\tMOV\t%s, %s%s", dst, src, comment)
				continue
			}
			lastLoad[addr] = args[0]
			addrModified[addrReg] = false
		}

		dstIdx := -1
		switch op {
		case "MOVI", "MOV", "ADD", "SUB", "MUL", "LOAD", "LEA", "POP":
			if len(args) >= 1 {
				dstIdx = 0
			}
		}
		if dstIdx >= 0 {
			modifiedReg := strings.TrimRight(args[dstIdx], ",")
			addrModified[modifiedReg] = true
		}

		if op == "STORE" && len(args) >= 2 {
			delete(lastLoad, args[1])
		}

		result[i] = line
	}
	return result
}

func pushPopElim(lines []string) []string {
	blocks := splitBlocks(lines)
	var result []string
	for _, block := range blocks {
		result = append(result, pushPopBlock(block)...)
	}
	return result
}

func pushPopBlock(lines []string) []string {
	remove := make([]bool, len(lines))
	for i, line := range lines {
		if remove[i] {
			continue
		}
		trimmed := strings.TrimSpace(line)
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
		if op != "PUSH" {
			continue
		}
		reg := fields[1]

		// Fast 3-instruction pattern: PUSH reg; OP reg, src; POP reg
		// The middle instruction modifies reg; POP restores it — net result: no-op on reg.
		if removed, mid, pop := scanPushModPop(i, reg, lines, remove); removed {
			remove[i] = true
			remove[mid] = true
			remove[pop] = true
			continue
		}

		for j := i + 1; j < len(lines); j++ {
			if remove[j] {
				continue
			}
			jTrimmed := strings.TrimSpace(lines[j])
			jCode := jTrimmed
			if idx := strings.IndexAny(jCode, ";#"); idx >= 0 {
				jCode = strings.TrimSpace(jCode[:idx])
			}
			if jCode == "" {
				continue
			}
			jFields := strings.Fields(jCode)
			if len(jFields) < 2 {
				continue
			}
			jOp := strings.ToUpper(jFields[0])
			jReg := strings.TrimRight(jFields[1], ",")

			if jOp == "POP" && jReg == reg {
				remove[i] = true
				remove[j] = true
				break
			}

			if d := dstReg(jOp, jFields[1:]); d >= 0 && jReg == reg {
				break
			}

			if jOp == "CALL" || jOp == "SYSCALL" || jOp == "INT" {
				break
			}
		}
	}

	var result []string
	for i, line := range lines {
		if !remove[i] {
			result = append(result, line)
		}
	}
	return result
}

// scanPushModPop checks whether lines[i:i+N] is a PUSH reg; instr-writes-reg; POP reg
// pattern. If yes, returns (true, midIdx, popIdx). Otherwise (false, 0, 0).
func scanPushModPop(i int, reg string, lines []string, remove []bool) (bool, int, int) {
	// Find next non-empty, non-comment, non-removed line after PUSH
	midIdx := -1
	for j := i + 1; j < len(lines); j++ {
		if remove[j] {
			continue
		}
		if stripComment(lines[j]) == "" {
			continue
		}
		midIdx = j
		break
	}
	if midIdx < 0 {
		return false, 0, 0
	}
	// Check that the middle line writes to reg, and that the opcode
	// is one that does not set FLAGS. This is conservative: peephole
	// passes on the final nasm output will catch flag-setting variants.
	midFields := strings.Fields(stripComment(lines[midIdx]))
	if len(midFields) < 2 {
		return false, 0, 0
	}
	midOp := strings.ToUpper(midFields[0])
	midReg := strings.TrimRight(midFields[1], ",")
	if midReg != reg {
		return false, 0, 0
	}
	if dstReg(midOp, midFields[1:]) < 0 {
		return false, 0, 0
	}
	switch midOp {
	case "MOV", "MOVI", "LOAD", "LEA", "STORE":
		// These do not set FLAGS — safe to drop.
	default:
		return false, 0, 0
	}
	// Then find next non-empty/non-comment line after midIdx — must be POP reg
	popIdx := -1
	for j := midIdx + 1; j < len(lines); j++ {
		if remove[j] {
			continue
		}
		if stripComment(lines[j]) == "" {
			continue
		}
		popIdx = j
		break
	}
	if popIdx < 0 {
		return false, 0, 0
	}
	popFields := strings.Fields(stripComment(lines[popIdx]))
	if len(popFields) < 2 {
		return false, 0, 0
	}
	popOp := strings.ToUpper(popFields[0])
	popReg := strings.TrimRight(popFields[1], ",")
	if popOp != "POP" || popReg != reg {
		return false, 0, 0
	}
	return true, midIdx, popIdx
}

// stripComment trims leading/trailing whitespace; returns "" for lines that are empty or pure comments only.
func tailCallOpt(lines []string) []string {
	result := make([]string, 0, len(lines))
	i := 0
	for i < len(lines) {
		if i+1 < len(lines) {
			trimmed := strings.TrimSpace(lines[i])
			code := trimmed
			if idx := strings.IndexAny(code, ";#"); idx >= 0 {
				code = strings.TrimSpace(code[:idx])
			}
			if code != "" {
				fields := strings.Fields(code)
				if len(fields) == 2 && strings.ToUpper(fields[0]) == "CALL" {
					nextTrimmed := strings.TrimSpace(lines[i+1])
					nextCode := nextTrimmed
					if idx := strings.IndexAny(nextCode, ";#"); idx >= 0 {
						nextCode = strings.TrimSpace(nextCode[:idx])
					}
					nextFields := strings.Fields(nextCode)
					if len(nextFields) == 1 && strings.ToUpper(nextFields[0]) == "RET" {
						target := fields[1]
						comment := ""
						if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
							comment = " " + trimmed[idx:]
						}
						result = append(result, fmt.Sprintf("\tJMP\t%s%s", target, comment))
						i += 2
						continue
					}
				}
			}
		}
		result = append(result, lines[i])
		i++
	}
	return result
}
