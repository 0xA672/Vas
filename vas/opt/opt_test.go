package opt

import (
	"strings"
	"testing"
)

func TestConstantFoldAdd(t *testing.T) {
	lines := []string{"\tADD\tv1, 1, 2"}
	result := FoldConstants(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	// FoldConstants produces lines with leading tab, but TrimSpace strips it
	got := result[0]
	if !strings.Contains(got, "MOVI\tv1, 3") {
		t.Errorf("FoldConstants = %q, want MOVI v1, 3", got)
	}
}

func TestConstantFoldSub(t *testing.T) {
	lines := []string{"\tSUB\tv1, 10, 3"}
	result := FoldConstants(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	got := result[0]
	if !strings.Contains(got, "MOVI\tv1, 7") {
		t.Errorf("FoldConstants = %q, want MOVI v1, 7", got)
	}
}

func TestConstantFoldMul(t *testing.T) {
	lines := []string{"\tMUL\tv1, 6, 7"}
	result := FoldConstants(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	got := result[0]
	if !strings.Contains(got, "MOVI\tv1, 42") {
		t.Errorf("FoldConstants = %q, want MOVI v1, 42", got)
	}
}

func TestConstantFoldWithComment(t *testing.T) {
	lines := []string{"\tADD\tv1, 1, 2\t; sum = 3"}
	result := FoldConstants(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	got := result[0]
	if !strings.Contains(got, "MOVI\tv1, 3") || !strings.Contains(got, "; sum = 3") {
		t.Errorf("FoldConstants = %q, want MOVI v1, 3 with comment", got)
	}
}

func TestConstantFoldNoFold(t *testing.T) {
	// Should not fold when operands are registers
	lines := []string{"\tADD\tv1, v2, v3"}
	result := FoldConstants(lines)
	if result[0] != lines[0] {
		t.Errorf("expected unchanged line, got %q", result[0])
	}
}

func TestDeadCodeElimUnusedWrite(t *testing.T) {
	// Without a final SYSCALL/SYSCALL, the DCE eliminates all writes
	// because no register is read at block end.
	// Use a SYSCALL to make registers live at block end.
	input := "\tMOVI\tv1, 1\n\tMOVI\tv2, 2\n\tMOVI\tv1, 3\n\tSYSCALL"
	output := Optimize(input, 1)
	// v1 = 1 should be eliminated (overwritten by v1 = 3 before being read)
	if strings.Contains(output, "MOVI\tv1, 1") {
		t.Errorf("unused write to v1 should have been eliminated: %q", output)
	}
	if !strings.Contains(output, "MOVI\tv2, 2") {
		t.Errorf("write to v2 should remain (SYSCALL reads v0)")
	}
	if !strings.Contains(output, "MOVI\tv1, 3") {
		t.Errorf("last write to v1 should remain")
	}
}

func TestDeadCodeElimKeepUsed(t *testing.T) {
	input := "\tMOVI\tv1, 1\n\tADD\tv2, v2, v1\n\tMOVI\tv1, 2\n\tSYSCALL"
	output := Optimize(input, 1)
	// v1 = 1 should NOT be eliminated because it's read by ADD
	if !strings.Contains(output, "MOVI\tv1, 1") {
		t.Errorf("write to v1 should be kept (read by ADD): %q", output)
	}
}

func TestDeadCodeElimBetweenBlocks(t *testing.T) {
	// Control flow should not confuse DCE
	input := "\tMOVI\tv1, 1\nloop:\n\tMOVI\tv1, 2\n\tSYSCALL"
	output := Optimize(input, 1)
	// v1=1 and v1=2 are in different blocks, both should survive
	if !strings.Contains(output, "MOVI\tv1, 1") {
		t.Errorf("v1=1 should survive (different block from v1=2)")
	}
	if !strings.Contains(output, "MOVI\tv1, 2") {
		t.Errorf("v1=2 should survive")
	}
}

func TestXorZero(t *testing.T) {
	lines := []string{"\tmov\trax, 0", "\tmov\trbx, 1", "\tmov\trcx, 0"}
	result := xorZero(lines)
	expected := []string{"\txor\teax, eax", "\tmov\trbx, 1", "\txor\tecx, ecx"}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("xorZero[%d] = %q, want %q", i, result[i], expected[i])
		}
	}
}

func TestTestCmp(t *testing.T) {
	lines := []string{"\tcmp\trax, 0", "\tcmp\trbx, 1", "\tcmp\trcx, 0"}
	result := testCmp(lines)
	expected := []string{"\ttest\teax, eax", "\tcmp\trbx, 1", "\ttest\tecx, ecx"}
	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("testCmp[%d] = %q, want %q", i, result[i], expected[i])
		}
	}
}

func TestNopMerge(t *testing.T) {
	lines := []string{"\tnop", "\tnop", "\tnop", "\tmov\trax, 1", "\tnop"}
	result := nopMerge(lines)
	// First three nops merged into one
	if !strings.Contains(result[0], "merged 3") {
		t.Errorf("expected merged nop line, got %q", result[0])
	}
	if result[1] != "\tmov\trax, 1" {
		t.Errorf("expected mov line unchanged, got %q", result[1])
	}
	// Last single nop should stay as-is
	if result[2] != "\tnop" {
		t.Errorf("expected single nop, got %q", result[2])
	}
}

func TestLeaFuse(t *testing.T) {
	lines := []string{"\tmov\trax, rbx", "\tadd\trax, rcx"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx+rcx]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseNoMatch(t *testing.T) {
	// Different destination registers -> no fusion
	lines := []string{"\tmov\trax, rbx", "\tadd\trcx, rdx"}
	result := leaFuse(lines)
	if len(result) != 2 {
		t.Errorf("expected 2 lines unchanged, got %d", len(result))
	}
}

func TestRegTo32(t *testing.T) {
	tests := map[string]string{
		"rax": "eax", "rbx": "ebx", "rcx": "ecx", "rdx": "edx",
		"rsi": "esi", "rdi": "edi", "r8": "r8d", "r9": "r9d",
		"r10": "r10d", "r15": "r15d",
	}
	for in, want := range tests {
		got := regTo32(in)
		if got != want {
			t.Errorf("regTo32(%q) = %q, want %q", in, got, want)
		}
	}
}

// Full pipeline test: VAS source -> assembled with -O1
func TestOptPipelineFull(t *testing.T) {
	input := "\tMOVI\tv0, 1\n\tMOVI\tv0, 60\n\tSYSCALL"
	output := Optimize(input, 1)
	// After DCE: first MOVI v0, 1 eliminated (overwritten by MOVI v0, 60)
	if strings.Contains(output, "MOVI\tv0, 1") {
		t.Errorf("unused write to v0 should be eliminated: %q", output)
	}
	if !strings.Contains(output, "MOVI\tv0, 60") {
		t.Errorf("v0 = 60 should remain")
	}
	if !strings.Contains(output, "SYSCALL") {
		t.Errorf("SYSCALL should remain")
	}
}
