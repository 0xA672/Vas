package opt

import (
	"fmt"
	"strings"
)

// FoldConstants scans VAS source lines and folds arithmetic on immediates.
// e.g. "ADD v1, 1, 2" => "MOVI v1, 3".
func FoldConstants(lines []string) []string {
	result := make([]string, 0, len(lines))
	for i, line := range lines {
		if writesFlags(line) && flagsLiveAfter(lines, i) {
			result = append(result, line)
			continue
		}
		result = append(result, foldLine(line))
	}
	return result
}

// foldLine attempts to fold a single VAS line.
func foldLine(line string) string {
	p, ok := parseOptLine(line)
	if !ok || len(p.args) != 3 {
		return line
	}
	dst := p.args[0]
	if regIndex(dst) < 0 || !allArgsHaveOnlyVirtualRegs([]string{dst}) {
		return line
	}
	v1, ok1 := parseNASMInt(p.args[1])
	v2, ok2 := parseNASMInt(p.args[2])
	if !ok1 || !ok2 {
		return line
	}
	var v int64
	switch p.op {
	case "ADD":
		v = v1 + v2
	case "SUB":
		v = v1 - v2
	case "MUL":
		v = v1 * v2
	default:
		return line
	}
	return fmt.Sprintf("\tMOVI\t%s, %d%s", dst, v, commentSuffix(line))
}

// Keep strings imported in this file for older callers that used its private
// tokenizer helper; the implementation now lives in sema.go.
var _ = strings.TrimSpace
