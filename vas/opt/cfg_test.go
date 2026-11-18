package opt

import (
	"strings"
	"testing"
)

func TestRemoveUnreachableAfterJmp(t *testing.T) {
	lines := []string{
		"\tJMP\tdone",
		"\tMOVI\tv0, 99",
		"done:",
		"\tMOVI\tv0, 1",
	}
	out := strings.Join(removeUnreachable(lines), "\n")
	if strings.Contains(out, "MOVI\tv0, 99") {
		t.Errorf("unreachable instruction survived:\n%s", out)
	}
	if !strings.Contains(out, "done:") || !strings.Contains(out, "MOVI\tv0, 1") {
		t.Errorf("referenced label or live code was removed:\n%s", out)
	}
}

func TestRemoveUnreachableKeepsConditionalFallthrough(t *testing.T) {
	lines := []string{
		"\tJE\tlbl",
		"\tMOVI\tv0, 1",
		"lbl:",
		"\tMOVI\tv0, 2",
	}
	out := strings.Join(removeUnreachable(lines), "\n")
	if !strings.Contains(out, "MOVI\tv0, 1") {
		t.Errorf("fallthrough after a conditional branch was removed:\n%s", out)
	}
}

func TestRemoveUnreachableKeepsGlobalRoot(t *testing.T) {
	lines := []string{
		"\tRET",
		"exported:",
		"\tMOVI\tv0, 1",
		"",
		"\tGLOBAL\texported",
	}
	out := strings.Join(removeUnreachable(lines), "\n")
	if !strings.Contains(out, "exported:") {
		t.Errorf("global root label was removed:\n%s", out)
	}
}

func TestRemoveUnreachableKeepsDataSection(t *testing.T) {
	lines := []string{
		"\tRET",
		"\tsection .data",
		"msg:",
		"\tdb 'x', 0",
	}
	out := strings.Join(removeUnreachable(lines), "\n")
	if !strings.Contains(out, "msg:") || !strings.Contains(out, "db 'x', 0") {
		t.Errorf("data section was treated as unreachable text:\n%s", out)
	}
}

func TestThreadJumpsThroughLabel(t *testing.T) {
	lines := []string{
		"\tJE\ta",
		"\tMOVI\tv0, 1",
		"a:",
		"\tJMP\tb",
		"b:",
		"\tMOVI\tv0, 2",
	}
	out := strings.Join(threadJumps(lines), "\n")
	if !strings.Contains(out, "\tJE\tb") {
		t.Errorf("jump was not threaded through a:\n%s", out)
	}
}

func TestRemoveUnreachableKeepsIndirectReference(t *testing.T) {
	lines := []string{
		"\tJMP\tafter",
		"fn:",
		"\tMOVI\tv0, 1",
		"\tRET",
		"after:",
		"\tLEA\tv0, [fn]",
	}
	out := strings.Join(removeUnreachable(lines), "\n")
	if !strings.Contains(out, "fn:") || !strings.Contains(out, "MOVI\tv0, 1") {
		t.Errorf("indirectly referenced block was removed:\n%s", out)
	}
}

func TestRemoveDeadLabelsKeepsMemoryReference(t *testing.T) {
	lines := []string{"\tLEA\tv0, [msg]", "msg:", "\tdb 'x', 0", "dead:", "\tNOP"}
	out := strings.Join(removeDeadLabels(lines), "\n")
	if !strings.Contains(out, "msg:") {
		t.Errorf("memory-referenced label was removed:\n%s", out)
	}
	if strings.Contains(out, "dead:") {
		t.Errorf("unreferenced text label survived:\n%s", out)
	}
}

func TestRemoveDeadLabelsKeepsGlobalRoot(t *testing.T) {
	lines := []string{"\tGLOBAL\texported", "exported:", "\tNOP"}
	out := strings.Join(removeDeadLabels(lines), "\n")
	if !strings.Contains(out, "exported:") {
		t.Errorf("global label was removed:\n%s", out)
	}
}

func TestThreadJumpsDoesNotLoopForever(t *testing.T) {
	lines := []string{"a:", "\tJMP\tb", "b:", "\tJMP\ta"}
	out := threadJumps(lines)
	if len(out) != len(lines) {
		t.Fatalf("line count changed: %v", out)
	}
}
