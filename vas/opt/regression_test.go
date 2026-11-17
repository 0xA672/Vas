package opt

import (
	"strings"
	"testing"
)

func TestDCEKeepsAddressRegister(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 5", "\tLOAD\tv0, [v1+8]", "\tMOVI\tv1, 6"}
	out := strings.Join(OptimizeLines(lines, 1), "\n")
	if !strings.Contains(out, "MOVI\tv1, 5") {
		t.Errorf("address register v1 was treated as dead:\n%s", out)
	}
}

func TestConstPropagationDoesNotDropLiveFlags(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 3", "\tADD\tv1, 1", "\tJE\tlbl", "lbl:"}
	out := strings.Join(constPropagate(lines), "\n")
	if !strings.Contains(out, "ADD") {
		t.Errorf("ADD was folded even though JE consumes its FLAGS:\n%s", out)
	}
}

func TestStoreLoadForwardingInvalidatedBySourceWrite(t *testing.T) {
	lines := []string{"\tSTORE\tv0, [x]", "\tMOVI\tv0, 42", "\tLOAD\tv1, [x]"}
	out := strings.Join(storeLoadFwd(lines), "\n")
	if strings.Contains(out, "MOV\tv1, v0") {
		t.Errorf("forwarded a clobbered source register:\n%s", out)
	}
}

func TestRedundantLoadOffsetInvalidated(t *testing.T) {
	lines := []string{"\tLOAD\tv7, [v5+8]", "\tMOVI\tv5, 0", "\tLOAD\tv8, [v5+8]"}
	out := strings.Join(redundantLoadElim(lines), "\n")
	if strings.Contains(out, "MOV\tv8, v7") {
		t.Errorf("reused a load through a modified address register:\n%s", out)
	}
}

func TestExtractLabelRejectsPhysicalRegisters(t *testing.T) {
	if got := extractLabel("[rax+8]"); got != "" {
		t.Errorf("extractLabel([rax+8]) = %q, want empty", got)
	}
	if got := extractLabel("[x+8]"); got != "x" {
		t.Errorf("extractLabel([x+8]) = %q, want x", got)
	}
}

func TestTestCmpKeepsWidth(t *testing.T) {
	lines := []string{"\tcmp\trax, 0"}
	got := testCmp(lines)[0]
	if got != "\ttest\trax, rax" {
		t.Errorf("testCmp = %q, want same-width test rax, rax", got)
	}
}
