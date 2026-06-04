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
	// The 3-op case returns a single element with embedded \n
	if len(result) != 1 {
		t.Fatalf("expected 1 line (with embedded newline), got %d: %v", len(result), result)
	}
	if !strings.Contains(result[0], "MOV\tv1, v0") && !strings.Contains(result[0], "mov\tv1, v0") {
		t.Errorf("strengthReduce[0] = %q, want MOV v1, v0", result[0])
	}
	if !strings.Contains(result[0], "shl\tv1, 4") {
		t.Errorf("strengthReduce[0] = %q, want shl v1, 4", result[0])
	}
}

func TestStrengthReduceNonPowerOf2(t *testing.T) {
	lines := []string{"\tMUL\tv1, 3"}
	result := strengthReduce(lines)
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d: %v", len(result), result)
	}
	if result[0] != lines[0] {
		t.Errorf("non-power-of-2 MUL should be unchanged, got %q", result[0])
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
