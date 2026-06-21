package opt

import (
	"fmt"
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
	input := "\tMOVI\tv1, 1\n\tMOVI\tv2, 2\n\tMOVI\tv1, 3\n\tSYSCALL"
	output := Optimize(input, 1)
	// v1 = 1 should be eliminated (overwritten by v1 = 3 before being read)
	if strings.Contains(output, "MOVI\tv1, 1") {
		t.Errorf("unused write to v1 should have been eliminated: %q", output)
	}
	// v2 = 2 survives (only DCE runs, terminal dead writes are kept)
	if !strings.Contains(output, "MOVI\tv2, 2") {
		t.Errorf("write to v2 should remain (never overwritten)")
	}
	// v1 = 3 survives (last write, not overwritten)
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
	// v1=1 and v1=2 are in different blocks, both survive (DCE per block)
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

func TestLeaFuseSubImm(t *testing.T) {
	// mov rax, rbx; sub rax, 5  →  lea rax, [rbx-5]
	lines := []string{"\tmov\trax, rbx", "\tsub\trax, 5"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx-5]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse(mov+sub) = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseSubImmNegative(t *testing.T) {
	// mov rax, rbx; sub rax, -8  →  lea rax, [rbx--8]  (LEA equivalent)
	lines := []string{"\tmov\trax, rbx", "\tsub\trax, -8"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx--8]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse(mov+sub neg) = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseSubImmNoMatch(t *testing.T) {
	// Different destination -> no fusion
	lines := []string{"\tmov\trax, rbx", "\tsub\trcx, 5"}
	result := leaFuse(lines)
	if len(result) != 2 {
		t.Errorf("expected 2 lines unchanged, got %d", len(result))
	}
}

func TestLeaFuseSubReg(t *testing.T) {
	// mov rax, rbx; sub rax, rcx  (reg-reg sub) — NOT fused (not representable as LEA)
	lines := []string{"\tmov\trax, rbx", "\tsub\trax, rcx"}
	result := leaFuse(lines)
	if len(result) != 2 {
		t.Errorf("reg-reg sub should NOT be fused, got %d lines", len(result))
	}
}

func TestLeaFuseImul3(t *testing.T) {
	// mov rax, rbx; imul rax, 3  →  lea rax, [rbx+rbx*2]
	lines := []string{"\tmov\trax, rbx", "\timul\trax, 3"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx+rbx*2]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse(mov+imul 3) = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseImul5(t *testing.T) {
	// mov rax, rbx; imul rax, 5  →  lea rax, [rbx+rbx*4]
	lines := []string{"\tmov\trax, rbx", "\timul\trax, 5"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx+rbx*4]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse(mov+imul 5) = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseImul9(t *testing.T) {
	// mov rax, rbx; imul rax, 9  →  lea rax, [rbx+rbx*8]
	lines := []string{"\tmov\trax, rbx", "\timul\trax, 9"}
	result := leaFuse(lines)
	expected := "\tlea\trax, [rbx+rbx*8]"
	if len(result) != 1 {
		t.Fatalf("expected 1 fused line, got %d: %v", len(result), result)
	}
	if result[0] != expected {
		t.Errorf("leaFuse(mov+imul 9) = %q, want %q", result[0], expected)
	}
}

func TestLeaFuseImulNoMatch(t *testing.T) {
	// Different destination -> no fusion
	lines := []string{"\tmov\trax, rbx", "\timul\trcx, 3"}
	result := leaFuse(lines)
	if len(result) != 2 {
		t.Errorf("expected 2 lines unchanged, got %d", len(result))
	}
}

func TestLeaFuseImulNonDecomposable(t *testing.T) {
	// imul by 7 cannot be represented as 1+scale (scale must be 1,2,4,8)
	lines := []string{"\tmov\trax, rbx", "\timul\trax, 7"}
	result := leaFuse(lines)
	if len(result) != 2 {
		t.Errorf("imul by 7 should NOT be fused, got %d lines", len(result))
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

func TestOptimizeLevel0(t *testing.T) {
	input := "\tMOVI\tv0, 1\n\tMOVI\tv0, 60\n\tSYSCALL"
	output := Optimize(input, 0)
	if output != input {
		t.Errorf("Optimize with level 0 should return unchanged input, got: %q", output)
	}
}

// --- Copy propagation ---

func TestCopyPropagateSimple(t *testing.T) {
	// MOV v1, v0  →  subsequent uses of v1 replaced with v0
	lines := []string{"\tMOV\tv1, v0", "\tADD\tv2, v1, v1"}
	result := copyPropagate(lines)
	// v1 should be replaced by v0 in the ADD
	expected := "\tADD\tv2, v0, v0"
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if result[1] != expected {
		t.Errorf("copyPropagate[1] = %q, want %q", result[1], expected)
	}
}

func TestCopyPropagateChain(t *testing.T) {
	// MOV v1, v0; MOV v2, v1  →  v2 aliases v0
	lines := []string{"\tMOV\tv1, v0", "\tMOV\tv2, v1", "\tADD\tv3, v2, v0"}
	result := copyPropagate(lines)
	// v2 should be v0 (through chain)
	expected := "\tADD\tv3, v0, v0"
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	if result[2] != expected {
		t.Errorf("copyPropagate[2] = %q, want %q", result[2], expected)
	}
}

func TestCopyPropagateKilled(t *testing.T) {
	// After ADD v1, v1, v2, the alias for v1 is killed
	lines := []string{"\tMOV\tv1, v0", "\tADD\tv1, v1, v2", "\tADD\tv3, v1, v0"}
	result := copyPropagate(lines)
	// The ADD v1, v1, v2 should keep v1 (alias killed), and the second ADD should still have v1
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	// v1 was overwritten by ADD, so the alias is killed — but the source v1 in the ADD is resolved
	// First, the alias was set to v0, then ADD v1 resolves v1->v0 for its own source
	// After ADD, alias for v1 is cleared (dst of non-MOV)
	// So ADD v3, v1, v0 should keep v1 (not replaced)
	expected := "\tADD\tv3, v1, v0"
	if result[2] != expected {
		t.Errorf("copyPropagate[2] = %q, want %q", result[2], expected)
	}
}

// --- Constant propagation ---

func TestConstPropagateAdd(t *testing.T) {
	lines := []string{"\tMOVI\tv0, 5", "\tADD\tv1, v0, 3"}
	result := constPropagate(lines)
	// v0=5 is known, so ADD v1, 5, 3 → MOVI v1, 8
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 8") {
		t.Errorf("constPropagate[1] = %q, want MOVI v1, 8", result[1])
	}
}

func TestConstPropagateSub(t *testing.T) {
	lines := []string{"\tMOVI\tv0, 10", "\tSUB\tv1, v0, 3"}
	result := constPropagate(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 7") {
		t.Errorf("constPropagate[1] = %q, want MOVI v1, 7", result[1])
	}
}

func TestConstPropagateMov(t *testing.T) {
	lines := []string{"\tMOVI\tv0, 42", "\tMOV\tv1, v0", "\tMOV\tv2, v1"}
	result := constPropagate(lines)
	// constPropagate tracks constants through MOV chains
	// v1 gets v0's constant (42), v2 gets v1's constant (42)
	// But constPropagate doesn't fold MOVI into MOV directly, it just tracks const values
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	// Lines shouldn't be changed (constPropagate just tracks values for subsequent folding)
	// The tracking itself is tested indirectly through Optimize pipeline
}

func TestConstPropagateOverwrite(t *testing.T) {
	// MOVI v0, 5 overwritten by MOVI v0, 7; then ADD should use 7
	lines := []string{"\tMOVI\tv0, 5", "\tMOVI\tv0, 7", "\tADD\tv1, v0, 0"}
	result := constPropagate(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[2], "MOVI\tv1, 7") {
		t.Errorf("constPropagate[2] = %q, want MOVI v1, 7", result[2])
	}
}

// 2-op constant folding: ADD dst, imm when dst is known
func TestConstPropagate2OpAdd(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 3", "\tADD\tv1, 5"}
	result := constPropagate(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 8") {
		t.Errorf("2-op ADD should fold to MOVI v1, 8: %q", result[1])
	}
}

// 2-op constant folding: SUB dst, imm when dst is known
func TestConstPropagate2OpSub(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 10", "\tSUB\tv1, 3"}
	result := constPropagate(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 7") {
		t.Errorf("2-op SUB should fold to MOVI v1, 7: %q", result[1])
	}
}

// 2-op constant folding: MUL dst, imm when dst is known
func TestConstPropagate2OpMul(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 6", "\tMUL\tv1, 7"}
	result := constPropagate(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 42") {
		t.Errorf("2-op MUL should fold to MOVI v1, 42: %q", result[1])
	}
}

// 2-op constant folding: no folding when dst is unknown
func TestConstPropagate2OpNoFold(t *testing.T) {
	lines := []string{"\tADD\tv1, 5"}
	result := constPropagate(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	// Should remain as ADD (v1 has no known constant)
	if !strings.Contains(result[0], "ADD") {
		t.Errorf("2-op ADD with unknown dst should remain ADD: %q", result[0])
	}
}

// 2-op constant folding with comment preservation
func TestConstPropagate2OpWithComment(t *testing.T) {
	lines := []string{"\tMOVI\tv1, 3", "\tADD\tv1, 5\t; increment"}
	result := constPropagate(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOVI\tv1, 8") {
		t.Errorf("2-op ADD with comment should fold: %q", result[1])
	}
	if !strings.Contains(result[1], "; increment") {
		t.Errorf("comment should survive folding: %q", result[1])
	}
}

// 2-op constant folding through the Optimize pipeline
func TestOptimize2OpFoldPipeline(t *testing.T) {
	input := "\tMOVI\tv1, 3\n\tADD\tv1, 5\n\tMOV\tv5, v1\n\tMOVI\tv0, 60\n\tSYSCALL"
	output := Optimize(input, 1)
	// ADD v1, 5 should be folded to MOVI v1, 8
	if !strings.Contains(output, "MOVI\tv1, 8") {
		t.Errorf("expected folded MOVI v1, 8 in pipeline: %q", output)
	}
	// MOV v5, v1 propagates constant to 8
	if !strings.Contains(output, "MOVI\tv5, 8") {
		t.Errorf("expected constant propagated to v5: %q", output)
	}
}

// --- Strength reduction ---

func TestStrengthReduceMul2Op(t *testing.T) {
	lines := []string{"\tMUL\tv1, 8"}
	result := strengthReduce(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "\tshl\tv1, 3") {
		t.Errorf("strengthReduce = %q, want shl v1, 3", result[0])
	}
}

func TestStrengthReduceMul3Op(t *testing.T) {
	lines := []string{"\tMUL\tv1, v0, 16"}
	result := strengthReduce(lines)
	// Now correctly split into 2 separate lines
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (MOV + SHL), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "MOV\tv1, v0") && !strings.Contains(result[0], "mov\tv1, v0") {
		t.Errorf("strengthReduce[0] = %q, want MOV v1, v0", result[0])
	}
	if !strings.Contains(result[1], "shl\tv1, 4") {
		t.Errorf("strengthReduce[1] = %q, want shl v1, 4", result[1])
	}
}

func TestStrengthReduceNonPowerOf2(t *testing.T) {
	// MUL by 3 → LEA v1, [v1+v1*2]
	lines := []string{"\tMUL\tv1, 3"}
	result := strengthReduce(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "LEA") {
		t.Errorf("MUL by 3 should be decomposed to LEA, got %q", result[0])
	}
}

func TestStrengthReduceBy7(t *testing.T) {
	// 3-op MUL v1, v0, 7 → LEA v1, [v0*8]; SUB v1, v0
	lines := []string{"\tMUL\tv1, v0, 7"}
	result := strengthReduce(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (LEA + SUB), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "LEA") || !strings.Contains(result[1], "SUB") {
		t.Errorf("MUL by 7 should decompose to LEA + SUB, got %q, %q", result[0], result[1])
	}
}

func TestStrengthReduceBy6(t *testing.T) {
	// MUL v1, 6 → LEA v1, [v1+v1*2]; SHL v1, 1  (6 = 3*2)
	lines := []string{"\tMUL\tv1, 6"}
	result := strengthReduce(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "LEA") || !strings.Contains(result[1], "shl") {
		t.Errorf("MUL by 6 should decompose to LEA + SHL, got %q, %q", result[0], result[1])
	}
}

func TestStrengthReduceLargeNonDecomposable(t *testing.T) {
	// MUL by a large non-decomposable constant → unchanged
	lines := []string{"\tMUL\tv1, 11"}
	result := strengthReduce(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line unchanged, got %d: %v", len(result), result)
	}
	if result[0] != lines[0] {
		t.Errorf("non-decomposable MUL should be unchanged, got %q", result[0])
	}
}

func TestStrengthReduceNotMul(t *testing.T) {
	lines := []string{"\tADD\tv1, 8"}
	result := strengthReduce(lines)
	if len(result) != 1 || result[0] != lines[0] {
		t.Errorf("non-MUL instruction should be unchanged, got %q", result[0])
	}
}

// --- STORE-LOAD forwarding ---

func TestStoreLoadFwd(t *testing.T) {
	lines := []string{"\tSTORE\tv0, [x]", "\tLOAD\tv1, [x]"}
	result := storeLoadFwd(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOV\tv1, v0") {
		t.Errorf("storeLoadFwd[1] = %q, want MOV v1, v0", result[1])
	}
}

func TestStoreLoadFwdNoMatch(t *testing.T) {
	// LOAD without prior STORE should be unchanged
	lines := []string{"\tLOAD\tv1, [x]"}
	result := storeLoadFwd(lines)
	if len(result) != 1 || result[0] != lines[0] {
		t.Errorf("LOAD without prior STORE should be unchanged, got %q", result[0])
	}
}

func TestStoreLoadFwdDifferentLabel(t *testing.T) {
	// STORE to x, LOAD from y → different labels, no forwarding
	lines := []string{"\tSTORE\tv0, [x]", "\tLOAD\tv1, [y]"}
	result := storeLoadFwd(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	// LOAD should remain as LOAD
	if !strings.Contains(result[1], "LOAD") {
		t.Errorf("LOAD from different label should not be forwarded, got %q", result[1])
	}
}

// --- Dead store elimination ---

func TestDeadStoreElim(t *testing.T) {
	// Two consecutive stores to [x]: first is dead
	lines := []string{"\tSTORE\tv0, [x]", "\tSTORE\tv1, [x]"}
	result := deadStoreElim(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line (first STORE eliminated), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "v1") {
		t.Errorf("deadStoreElim should keep second STORE, got %q", result[0])
	}
}

func TestDeadStoreElimKeepWithLoad(t *testing.T) {
	// STORE, LOAD, STORE → intervening LOAD keeps the first STORE alive
	lines := []string{"\tSTORE\tv0, [x]", "\tLOAD\tv2, [x]", "\tSTORE\tv1, [x]"}
	result := deadStoreElim(lines)
	if len(result) != 3 {
		t.Fatalf("expected all 3 lines kept, got %d: %v", len(result), result)
	}
}

func TestDeadStoreElimDifferentLabels(t *testing.T) {
	// STORE to x, STORE to y → different labels, neither dead
	lines := []string{"\tSTORE\tv0, [x]", "\tSTORE\tv1, [y]"}
	result := deadStoreElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
}

// --- Peephole combined ---

func TestPeepholeCombined(t *testing.T) {
	// Combined peephole: xorZero + testCmp + nopMerge + leaFuse
	lines := []string{
		"\tmov\trax, 0",
		"\tcmp\trax, 0",
		"\tnop",
		"\tnop",
		"\tmov\trax, rbx",
		"\tadd\trax, rcx",
	}
	result := peephole(lines)
	// After all passes: xorZero(6)→6, testCmp(6)→6, nopMerge(6)→5, leaFuse(5)→4
	if len(result) != 4 {
		t.Fatalf("expected 4 lines after all peephole passes, got %d: %v", len(result), result)
	}
	// xorZero: mov rax,0 → xor eax,eax
	if result[0] != "\txor\teax, eax" {
		t.Errorf("peephole[0] = %q, want 'xor eax, eax'", result[0])
	}
	// testCmp: cmp rax,0 → test eax,eax
	if result[1] != "\ttest\teax, eax" {
		t.Errorf("peephole[1] = %q, want 'test eax, eax'", result[1])
	}
	// nopMerge: two nops merged into one
	if !strings.Contains(result[2], "merged") {
		t.Errorf("peephole[2] = %q, want merged nop", result[2])
	}
	// leaFuse: mov+add → lea
	if !strings.Contains(result[3], "lea") {
		t.Errorf("peephole[3] = %q, want lea", result[3])
	}
}

// --- isPowerOf2 / log2 (tested indirectly via strengthReduce) ---

func TestIsPowerOf2(t *testing.T) {
	tests := []struct {
		n    int64
		want bool
	}{
		{1, true}, {2, true}, {4, true}, {8, true}, {16, true},
		{0x80000000, true},
		{0, false}, {3, false}, {5, false}, {6, false}, {7, false},
		{-1, false}, {-2, false},
	}
	for _, tt := range tests {
		got := isPowerOf2(tt.n)
		if got != tt.want {
			t.Errorf("isPowerOf2(%d) = %v, want %v", tt.n, got, tt.want)
		}
	}
}

func TestLog2(t *testing.T) {
	tests := []struct {
		n    int64
		want int
	}{
		{1, 0}, {2, 1}, {4, 2}, {8, 3}, {16, 4},
		{0x80000000, 31},
	}
	for _, tt := range tests {
		got := log2(tt.n)
		if got != tt.want {
			t.Errorf("log2(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

// --- Optimize pipeline with full interaction ---

func TestOptimizeFullPipeline(t *testing.T) {
	// Combine several optimizations: const propagation + DCE + STORE-LOAD forwarding
	input := "\tMOVI\tv0, 1\n\tMOVI\tv0, 2\n\tMUL\tv1, v0, 8\n\tSTORE\tv1, [result]\n\tLOAD\tv2, [result]"
	output := Optimize(input, 1)
	// v0=1 should be eliminated (overwritten by v0=2 before being read)
	if strings.Contains(output, "MOVI\tv0, 1") {
		t.Errorf("dead MOVI v0,1 should be eliminated: %q", output)
	}
	// v0=2 is known and MUL should be constant-folded to MOVI v1, 16
	if !strings.Contains(output, "MOVI\tv1, 16") {
		t.Errorf("MUL v1, v0, 8 should be constant-folded to MOVI v1, 16: %q", output)
	}
	// LOAD v2, [result] should be forwarded to MOV v2, v1
	if strings.Contains(output, "LOAD") {
		t.Errorf("LOAD should be forwarded to MOV: %q", output)
	}
}

// Bug 2: readRegs missing INT — DCE should keep writes used by INT
func TestDeadCodeElimInt(t *testing.T) {
	// INT reads v0 as the syscall number. If readRegs("INT") returns empty,
	// DCE would incorrectly remove MOVI v0, 1 (thinking it's overwritten by MOVI v0, 2).
	input := "\tMOVI\tv0, 1\n\tINT\t0x80\n\tMOVI\tv0, 2\n\tSYSCALL"
	output := Optimize(input, 1)
	// v0=1 should survive because INT reads it as the syscall number
	if !strings.Contains(output, "MOVI\tv0, 1") {
		t.Errorf("write to v0=1 should be kept (read by INT as syscall number): %q", output)
	}
	// v0=2 should survive (last write, read by SYSCALL)
	if !strings.Contains(output, "MOVI\tv0, 2") {
		t.Errorf("write to v0=2 should remain: %q", output)
	}
	if !strings.Contains(output, "SYSCALL") {
		t.Errorf("SYSCALL should remain: %q", output)
	}
}

// Bug 5: constBlock missing INT — constant propagation across INT is incorrect
func TestConstPropagateInt(t *testing.T) {
	// INT clobbers v0 with the return value, so ADD v1, v0, 0 should NOT fold
	lines := []string{"\tMOVI\tv0, 60", "\tINT\t0x80", "\tADD\tv1, v0, 0"}
	result := constPropagate(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
	// Without fix: ADD would fold to MOVI v1, 60 (wrong! INT clobbered v0)
	if strings.Contains(result[2], "MOVI\tv1, 60") {
		t.Errorf("ADD should NOT be folded across INT (INT clobbers v0): %q", result[2])
	}
	// ADD should remain as-is (or at least not fold to a stale constant)
	if !strings.Contains(result[2], "ADD") && !strings.Contains(result[2], "MOV") {
		t.Errorf("ADD should remain after INT: %q", result[2])
	}
}

// #19: Optimizer preserves empty lines and comments
func TestOptimizePreservesEmptyLines(t *testing.T) {
	// Use MOVI with different registers so neither write is dead
	input := "\tMOVI\tv0, 1\n\n\tMOVI\tv1, 2\n\tSYSCALL"
	output := Optimize(input, 1)
	// The empty line should survive
	lines := strings.Split(output, "\n")
	if len(lines) != 4 {
		t.Errorf("expected 4 lines (with empty line), got %d: %q", len(lines), output)
	}
	if lines[1] != "" {
		t.Errorf("expected line 2 to be empty, got %q", lines[1])
	}
}

func TestOptimizePreservesComments(t *testing.T) {
	// Use different registers for each MOVI so neither is dead
	input := "\tMOVI\tv0, 1\t; set first\n\tMOVI\tv1, 60\t; exit\n\tSYSCALL"
	output := Optimize(input, 1)
	// Comments should be preserved
	if !strings.Contains(output, "; set first") {
		t.Errorf("expected comment '; set first' to survive: %q", output)
	}
	if !strings.Contains(output, "; exit") {
		t.Errorf("expected comment '; exit' to survive: %q", output)
	}
}

func TestOptimizePreservesHashComments(t *testing.T) {
	input := "\tMOVI\tv0, 60\t# syscall: exit\n\tSYSCALL"
	output := Optimize(input, 1)
	if !strings.Contains(output, "# syscall: exit") {
		t.Errorf("expected hash comment to survive: %q", output)
	}
}

func TestOptimizePreservesCommentOnlyLines(t *testing.T) {
	input := "\tMOVI\tv0, 1\n; this is a comment-only line\n\tMOVI\tv1, 60\n\tSYSCALL"
	output := Optimize(input, 1)
	if !strings.Contains(output, "; this is a comment-only line") {
		t.Errorf("expected comment-only line to survive: %q", output)
	}
}

func TestOptimizePreservesMixedBlanksAndComments(t *testing.T) {
	// Use distinct registers to prevent DCE from removing lines
	input := "\tMOVI\tv0, 1\n\n; comment\n\n\tMOVI\tv1, 60\n\tSYSCALL"
	output := Optimize(input, 1)
	if !strings.Contains(output, "; comment") {
		t.Errorf("expected '; comment' to survive: %q", output)
	}
	lines := strings.Split(output, "\n")
	if len(lines) != 6 {
		t.Errorf("expected 6 lines (preserving blanks), got %d: %q", len(lines), output)
	}
}

// #17: Fuzz test for optimizers
func FuzzOptimize(f *testing.F) {
	seeds := []string{
		"\tMOVI\tv0, 1\n\tMOVI\tv0, 2\n\tSYSCALL",
		"\tADD\tv1, 1, 2\n\tSUB\tv2, v1, 1",
		"\tMUL\tv1, v0, 8\n\tSTORE\tv1, [x]\n\tLOAD\tv2, [x]",
		"\tNOP\n\tNOP\n\tNOP",
		"\tMOVI\tv0, 0\n\tMOVI\tv1, 0\n\tADD\tv2, v0, v1",
		"\tMOVI\tv1, 3\n\tADD\tv1, 5\n\tSUB\tv1, 1\n\tMUL\tv1, v1, 4\n\tSYSCALL",
		"\tLEA\tv0, [data]\n\tADD\tv5, v0, v5\n\tLOAD\tv6, [v5]",
		"\tPUSH\tv0\n\tADD\tv1, v1, v2\n\tPOP\tv0",
		"\tNOT\tv1\n\tNOT\tv1",
		"\tADD\tv0, 1\n\tNEG\tv0",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_ = Optimize(input, 1)
		_ = Optimize(input, 2)
	})
}

// --- -O2: CSE (common subexpression elimination) ---

func TestCseSimple(t *testing.T) {
	// Same MUL appears twice with different dst → CSE should keep both (conservative approach)
	lines := []string{"\tMUL\tv5, v3, 8", "\tMUL\tv6, v3, 8"}
	result := cse(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	// Conservative CSE keeps both instructions to preserve data flow correctness
	if result[0] != "\tMUL\tv5, v3, 8" {
		t.Errorf("first MUL should be preserved: %q", result[0])
	}
	if result[1] != "\tMUL\tv6, v3, 8" {
		t.Errorf("second MUL should be preserved (conservative CSE): %q", result[1])
	}
}

func TestCseDifferentOps(t *testing.T) {
	// Different ops should NOT be CSE'd
	lines := []string{"\tMUL\tv5, v3, 8", "\tADD\tv6, v3, 8"}
	result := cse(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if strings.Contains(result[1], "MOV") {
		t.Errorf("different ops should NOT be CSE'd: %q", result[1])
	}
}

func TestCseKilled(t *testing.T) {
	// If dst is overwritten, the CSE entry should be invalidated
	lines := []string{"\tMUL\tv5, v3, 8", "\tMOVI\tv5, 0", "\tMUL\tv6, v3, 8"}
	result := cse(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	// After MOVI v5, 0, the CSE entry for v5 is killed
	// So the second MUL should NOT be replaced with MOV
	if strings.Contains(result[2], "MOV") {
		t.Errorf("CSE should be killed after dst overwrite: %q", result[2])
	}
}

// --- -O2: LICM (loop invariant code motion) ---

func TestLicmLEA(t *testing.T) {
	// LEA v0, [data] inside loop → hoisted before loop header
	lines := []string{
		"loop:",
		"\tLEA\tv0, [data]",
		"\tADD\tv5, v0, v5",
		"\tJMP\tloop",
	}
	result := licm(lines)
	if len(result) != 4 {
		t.Fatalf("expected 4 lines, got %d: %v", len(result), result)
	}
	// LEA should be BEFORE the label
	if !strings.Contains(result[0], "LEA") {
		t.Errorf("LEA should be hoisted before loop label: %q", result[0])
	}
	if result[1] != "loop:" {
		t.Errorf("label should follow hoisted LEA: %q", result[1])
	}
}

func TestLicmNoLoop(t *testing.T) {
	// No backward jump → no hoisting
	lines := []string{
		"\tLEA\tv0, [data]",
		"\tADD\tv5, v0, v5",
	}
	result := licm(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
}

// --- -O2: redundant load elimination ---

func TestRedundantLoadElimSimple(t *testing.T) {
	lines := []string{"\tLOAD\tv7, [v5]", "\tLOAD\tv8, [v5]"}
	result := redundantLoadElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[1], "MOV\tv8, v7") {
		t.Errorf("second LOAD should become MOV v8, v7: %q", result[1])
	}
}

func TestRedundantLoadElimNoMatch(t *testing.T) {
	// Different address → no elimination
	lines := []string{"\tLOAD\tv7, [v5]", "\tLOAD\tv8, [v6]"}
	result := redundantLoadElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if strings.Contains(result[1], "MOV") {
		t.Errorf("different addresses should not be eliminated: %q", result[1])
	}
}

func TestRedundantLoadElimStoreInvalidates(t *testing.T) {
	// STORE between two LOADs → second LOAD should stay
	lines := []string{"\tLOAD\tv7, [v5]", "\tSTORE\tv1, [v5]", "\tLOAD\tv8, [v5]"}
	result := redundantLoadElim(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(result), result)
	}
	if strings.Contains(result[2], "MOV") {
		t.Errorf("LOAD after STORE should not be forwarded: %q", result[2])
	}
}

// --- -O2: PUSH/POP elimination ---

func TestPushPopElimSimple(t *testing.T) {
	lines := []string{"\tPUSH\tv0", "\tPOP\tv0"}
	result := pushPopElim(lines)
	if len(result) != 0 {
		t.Errorf("balanced PUSH/POP should be eliminated, got %d lines: %v", len(result), result)
	}
}

func TestPushPopElimWithCode(t *testing.T) {
	lines := []string{"\tPUSH\tv0", "\tMOVI\tv1, 1", "\tPOP\tv0"}
	result := pushPopElim(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line (MOVI kept), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "MOVI") {
		t.Errorf("MOVI should survive: %q", result[0])
	}
}

func TestPushPopElimModifiedReg(t *testing.T) {
	// PUSH v0; MOVI v0, 1; POP v0 → MOVI writes a value that POP overwrites.
	// The entire triple is a dead write and should be eliminated.
	lines := []string{"\tPUSH\tv0", "\tMOVI\tv0, 1", "\tPOP\tv0"}
	result := pushPopElim(lines)
	if len(result) != 0 {
		t.Fatalf("expected 0 lines (dead write eliminated), got %d: %v", len(result), result)
	}
}

func TestPushPopElimFlagSettingMiddle(t *testing.T) {
	// PUSH v0; ADD v0, v0, v1; POP v0 → middle instr sets FLAGS.
	// Conservative: keep, let the nasm-level peephole pass deal with it.
	lines := []string{"\tPUSH\tv0", "\tADD\tv0, v0, v1", "\tPOP\tv0"}
	result := pushPopElim(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines (flag-setting middle kept), got %d: %v", len(result), result)
	}
}

func TestPushPopElimDifferentReg(t *testing.T) {
	// PUSH v0; POP v1 → different reg, no elimination
	lines := []string{"\tPUSH\tv0", "\tPOP\tv1"}
	result := pushPopElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (different reg), got %d: %v", len(result), result)
	}
}

// --- -O2: tail call optimisation ---

func TestTailCallOptSimple(t *testing.T) {
	lines := []string{"\tCALL\tcompute", "\tRET"}
	result := tailCallOpt(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "JMP") {
		t.Errorf("CALL+RET should become JMP: %q", result[0])
	}
}

func TestTailCallOptNoMatch(t *testing.T) {
	// CALL without RET → keep as-is
	lines := []string{"\tCALL\tcompute", "\tMOVI\tv0, 1"}
	result := tailCallOpt(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "CALL") {
		t.Errorf("CALL without following RET should survive: %q", result[0])
	}
}

// --- -O2 peephole: noopElim ---

func TestNoopElimMovSame(t *testing.T) {
	lines := []string{"\tmov\trax, rax", "\tmov\trbx, rcx"}
	result := noopElim(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "mov\trbx, rcx") {
		t.Errorf("non-noop mov should survive: %q", result[0])
	}
}

func TestNoopElimAddZero(t *testing.T) {
	lines := []string{"\tadd\trax, 0", "\tsub\trax, 0", "\timul\trax, 1", "\tmov\trax, rbx"}
	result := noopElim(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line (all noops removed), got %d: %v", len(result), result)
	}
}

func TestNoopElimNotNoop(t *testing.T) {
	lines := []string{"\tadd\trax, 1", "\tsub\trax, 2", "\timul\trax, 3"}
	result := noopElim(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines (non-zero operands kept), got %d: %v", len(result), result)
	}
}

// --- -O2 peephole: pushPopMov ---

func TestPushPopMovSimple(t *testing.T) {
	lines := []string{"\tpush\trax", "\tpop\trbx"}
	result := pushPopMov(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "mov\trbx, rax") {
		t.Errorf("push+pop should become mov: %q", result[0])
	}
}

func TestPushPopMovNotPair(t *testing.T) {
	lines := []string{"\tpush\trax", "\tadd\trax, 1"}
	result := pushPopMov(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (not a push+pop pair), got %d: %v", len(result), result)
	}
}

// --- -O2 peephole: xorMovElim ---

func TestXorMovElimSimple(t *testing.T) {
	lines := []string{"\txor\teax, eax", "\tmov\trax, rbx"}
	result := xorMovElim(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "mov\trax, rbx") {
		t.Errorf("xor+mov should become single mov: %q", result[0])
	}
	// Should NOT contain xor
	if strings.Contains(result[0], "xor") {
		t.Errorf("xor should be eliminated: %q", result[0])
	}
}

func TestXorMovElimDifferentReg(t *testing.T) {
	// xor rax, rax; mov rbx, rcx → different regs, keep both
	lines := []string{"\txor\teax, eax", "\tmov\trbx, rcx"}
	result := xorMovElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (different regs), got %d: %v", len(result), result)
	}
}

func TestXorMovElimNotXor(t *testing.T) {
	// add rax, rbx; mov rax, rcx → not xor, keep both
	lines := []string{"\tadd\trax, rbx", "\tmov\trax, rcx"}
	result := xorMovElim(lines)
	if len(result) != 2 {
		t.Fatalf("expected 2 lines (not xor), got %d: %v", len(result), result)
	}
}

// --- -O2 integration: full pipeline test ---

func TestOptimizeLevel2(t *testing.T) {
	// Verify -O2 pipeline runs without error on typical input
	input := "\tMOVI\tv1, 0\nloop:\n\tLEA\tv0, [data]\n\tADD\tv1, v1, 1\n\tCMP\tv1, 10\n\tJLE\tloop"
	output := Optimize(input, 2)
	// -O2 should run and produce output
	if len(output) == 0 {
		t.Errorf("expected non-empty -O2 output")
	}
	// LICM should hoist LEA before loop
	if strings.Contains(output, "loop:") {
		loopIdx := strings.Index(output, "loop:")
		leaIdx := strings.Index(output, "LEA")
		if leaIdx > loopIdx {
			t.Errorf("LEA should be hoisted before 'loop:' label by LICM")
		}
	}
}

// --- shlAddFuse ---

func TestShlAddFuseK1(t *testing.T) {
	// mov rax, rcx; shl rax, 1; add rax, rcx  →  lea rax, [rcx+rcx*2]
	lines := []string{"\tmov\trax, rcx", "\tshl\trax, 1", "\tadd\trax, rcx"}
	result := shlAddFuse(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	expected := "\tlea\trax, [rcx+rcx*2]"
	if result[0] != expected {
		t.Errorf("shlAddFuse = %q, want %q", result[0], expected)
	}
}

func TestShlAddFuseK2(t *testing.T) {
	// mov rax, rcx; shl rax, 2; add rax, rcx  →  lea rax, [rcx+rcx*4]
	lines := []string{"\tmov\trax, rcx", "\tshl\trax, 2", "\tadd\trax, rcx"}
	result := shlAddFuse(lines)
	expected := "\tlea\trax, [rcx+rcx*4]"
	if len(result) != 1 || result[0] != expected {
		t.Errorf("shlAddFuse = %q, want %q", result[0], expected)
	}
}

func TestShlAddFuseK3(t *testing.T) {
	// mov rax, rcx; shl rax, 3; add rax, rcx  →  lea rax, [rcx+rcx*8]
	lines := []string{"\tmov\trax, rcx", "\tshl\trax, 3", "\tadd\trax, rcx"}
	result := shlAddFuse(lines)
	expected := "\tlea\trax, [rcx+rcx*8]"
	if len(result) != 1 || result[0] != expected {
		t.Errorf("shlAddFuse = %q, want %q", result[0], expected)
	}
}

func TestShlAddFuseNoMatchDifferentSrc(t *testing.T) {
	// mov rax, rcx; shl rax, 2; add rax, rdx  →  different add source, no fuse
	lines := []string{"\tmov\trax, rcx", "\tshl\trax, 2", "\tadd\trax, rdx"}
	result := shlAddFuse(lines)
	if len(result) != 3 {
		t.Errorf("expected 3 lines unchanged (different add src), got %d", len(result))
	}
}

func TestShlAddFuseNoMatchDifferentDst(t *testing.T) {
	// mov rax, rcx; shl rax, 2; add rbx, rcx  →  different add dst, no fuse
	lines := []string{"\tmov\trax, rcx", "\tshl\trax, 2", "\tadd\trbx, rcx"}
	result := shlAddFuse(lines)
	if len(result) != 3 {
		t.Errorf("expected 3 lines unchanged (different add dst), got %d", len(result))
	}
}

// ── Benchmarks ─────────────────────────────────────────────────────────
// These benchmarks exercise the optimized peephole pipeline directly so
// we can measure the effect of single-scan merging and sync.Pool reuse.

var benchOut string

// makeVASInput builds a moderate-sized VAS input with enough virtual-
// register traffic to trigger all pre-expansion passes plus the
// peephole pass at the end.
func makeVASInput(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "MOVI v%d, %d\n", i%13, i+1)
		fmt.Fprintf(&b, "ADD v%d, v%d, %d\n", (i+1)%13, i%13, i%5+1)
		fmt.Fprintf(&b, "CMP v%d, %d\n", i%13, i%10+1)
	}
	return b.String()
}

// makeASMInput builds a moderate-sized nasm-style assembly output
// exercising every 1-line and 2-line peephole rule (mov reg, 0,
// cmp reg, 0, mov reg, reg, push/pop pairs, mov+add patterns,
// nop runs, not/not and inc/dec cancel pairs, etc.).
func makeASMInput(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("\tmov\trax, 0\n")
		b.WriteString("\tcmp\trbx, 0\n")
		b.WriteString("\tmov\trcx, rcx\n")
		b.WriteString("\tmov\trdx, rbx\n")
		b.WriteString("\tadd\trdx, rcx\n")
		b.WriteString("\tpush\trax\n")
		b.WriteString("\tpop\trbx\n")
		b.WriteString("\tnop\n")
		b.WriteString("\tnop\n")
		b.WriteString("\tnot\trcx\n")
		b.WriteString("\tnot\trcx\n")
	}
	return b.String()
}

func BenchmarkOptimizeO2Small(b *testing.B) {
	input := makeVASInput(20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchOut = Optimize(input, 2)
	}
}

func BenchmarkOptimizeO2Large(b *testing.B) {
	input := makeVASInput(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchOut = Optimize(input, 2)
	}
}

func BenchmarkPeepholeOnlySmall(b *testing.B) {
	input := makeASMInput(50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchOut = PeepholeOnly(input)
	}
}

func BenchmarkPeepholeOnlyLarge(b *testing.B) {
	input := makeASMInput(1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchOut = PeepholeOnly(input)
	}
}

// BenchmarkPeepholeNoMatch isolates the mightOptimize fast-path:
// a pure-directive input that contains nothing any peephole rule
// could transform.
func BenchmarkPeepholeNoMatch(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 500; i++ {
		sb.WriteString("\t; comment\n")
		sb.WriteString("\tdq\t0\n")
	}
	input := sb.String()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchOut = PeepholeOnly(input)
	}
}
