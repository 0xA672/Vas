package opt

import (
	"fmt"
	"strconv"
	"strings"
)

// FoldConstants scans VAS source lines and folds arithmetic on immediates.
// e.g. "ADD v1, 1, 2" => "MOVI v1, 3"
func FoldConstants(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, foldLine(line))
	}
	return result
}

// foldLine attempts to fold a single VAS line.
func foldLine(line string) string {
	trimmed := strings.TrimSpace(line)

	// Strip inline comment for parsing
	code := trimmed
	if idx := strings.IndexAny(code, ";#"); idx >= 0 {
		code = strings.TrimSpace(code[:idx])
	}
	if code == "" {
		return line
	}

	tokens := tokenizeFold(code)
	if len(tokens) < 4 {
		return line
	}

	op := strings.ToUpper(tokens[0])
	dst := tokens[1]

	// 3-operand ADD/SUB with two immediate operands
	if len(tokens) == 4 {
		src1, err1 := strconv.ParseInt(tokens[2], 0, 64)
		src2, err2 := strconv.ParseInt(tokens[3], 0, 64)
		if err1 == nil && err2 == nil {
			var val int64
			switch op {
			case "ADD":
				val = src1 + src2
			case "SUB":
				val = src1 - src2
			case "MUL":
				val = src1 * src2
			default:
				return line
			}
			// Preserve comment
			comment := ""
			if idx := strings.IndexAny(trimmed, ";#"); idx >= 0 {
				comment = trimmed[idx:]
			}
			return fmt.Sprintf("\tMOVI\t%s, %d%s", dst, val, comment)
		}
	}

	return line
}

func tokenizeFold(line string) []string {
	var tokens []string
	var cur strings.Builder
	for _, ch := range line {
		if ch == ',' || ch == '\t' || ch == ' ' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(ch)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
