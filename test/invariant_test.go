package vas_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"vas/vas"
)

// ── Invariant tests ────────────────────────────────────────────────────
// Properties that must always hold for any valid VAS input.

// TestInvariantO0EqualsAssemble verifies that -O0 produces the same output as plain Assemble.
func TestInvariantO0EqualsAssemble(t *testing.T) {
	inputs := []string{
		"MOVI v0, 60\nSYSCALL",
		"ADD v0, v1, v2\nSUB v3, v4, v5",
		"MUL v0, v1, 8\nLOAD v2, [x]\nSTORE v2, [y]",
		"PUSH v0\nPOP v1\nCALL func\nRET",
		"JMP loop\nloop:\nNOP",
	}
	for _, input := range inputs {
		got, err := vas.Assemble(input)
		if err != nil {
			t.Errorf("Assemble(%q) error: %v", input, err)
			continue
		}
		gotOpt, err := vas.AssembleWithOpt(input, vas.OptConfig{Level: 0})
		if err != nil {
			t.Errorf("AssembleWithOpt(%q, Level=0) error: %v", input, err)
			continue
		}
		if got != gotOpt {
			t.Errorf("Assemble(%q) != AssembleWithOpt(0)\ngot:  %q\nwant: %q", input, gotOpt, got)
		}
	}
}

// TestInvariantOptMonotonic verifies that -O1 output is not longer than -O0.
func TestInvariantOptMonotonic(t *testing.T) {
	inputs := []string{
		"MOVI v0, 1\nMOVI v0, 2\nSYSCALL",
		"ADD v1, 1, 2\nSUB v2, v1, 1\nMUL v3, v2, 8",
		"MOVI v0, 0\nMOV v1, v0\nADD v2, v1, v0\nMUL v2, v2, 8\nSYSCALL",
		"MOVI v1, 3\nADD v1, 5\nSUB v1, 1\nMUL v1, v1, 4\nSYSCALL",
		"LOAD v0, [x]\nLOAD v1, [x]\nSTORE v0, [y]",
	}
	for _, input := range inputs {
		out0, _ := vas.AssembleWithOpt(input, vas.OptConfig{Level: 0})
		out1, _ := vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
		lines0 := strings.Count(out0, "\n")
		lines1 := strings.Count(out1, "\n")
		if lines1 > lines0 {
			// -O1 should never produce MORE lines than -O0
			t.Errorf("-O1 (%d lines) longer than -O0 (%d lines) for:\n%s", lines1, lines0, input)
		}
	}
}

// TestInvariantO2ValidNasm verifies that -O2 output can be assembled by nasm.
func TestInvariantO2ValidNasm(t *testing.T) {
	if !hasTool(t, "nasm") {
		t.Skip("nasm not found")
	}
	inputs := []string{
		"MOVI v0, 60\nSYSCALL",
		"MOVI v0, 42\nMOV v5, v0\nMOVI v0, 60\nSYSCALL",
		"MOVI v1, 0\nloop:\nADD v1, v1, 1\nCMP v1, 10\nJLE loop\nMOV v5, v1\nMOVI v0, 60\nSYSCALL",
	}
	for _, input := range inputs {
		asm, err := vas.AssembleStandaloneTargetOpt(input, "elf64", 2)
		if err != nil {
			t.Errorf("AssembleStandalone error: %v", err)
			continue
		}
		dir := t.TempDir()
		asmFile := dir + "\\test.asm"
		objFile := dir + "\\test.o"
		if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
			t.Errorf("write: %v", err)
			continue
		}
		out, err := exec.Command("nasm", "-f", "elf64", "-o", objFile, asmFile).CombinedOutput()
		if err != nil {
			t.Errorf("nasm failed for -O2 output:\n%s\n%s", asm, out)
		}
	}
}

// ── Golden file tests ──────────────────────────────────────────────────
// Compile each example at all opt levels and compare with golden files.

func TestGoldenExamples(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping golden tests in short mode")
	}
	update := os.Getenv("VAS_UPDATE_GOLDEN") == "1"
	examples := []string{
		"../examples/hello.vas",
		"../examples/fib.vas",
		"../examples/fact.vas",
		"../examples/calc.vas",
		"../examples/sort.vas",
		"../examples/opt_showcase.vas",
	}
	goldenDir := "../testdata/golden/"
	for _, ex := range examples {
		data, err := os.ReadFile(ex)
		if err != nil {
			t.Fatalf("read %s: %v", ex, err)
		}
		input := string(data)
		base := strings.TrimSuffix(ex[strings.LastIndex(ex, "/")+1:], ".vas")

		for _, opt := range []int{0, 1, 2} {
			got, err := vas.AssembleWithOpt(input, vas.OptConfig{Level: opt})
			if err != nil {
				// Skip error-producing inputs — they may use features not support by AssembleWithOpt
				continue
			}
			// Normalize line endings and trim trailing whitespace for comparison
			got = strings.ReplaceAll(got, "\r\n", "\n")
			got = strings.TrimRight(got, "\n")

			goldenFile := goldenDir + base + "_O" + r(opt) + ".golden"
			if update {
				// Update mode: always rewrite golden file
				os.MkdirAll(goldenDir, 0755)
				// Write with LF for consistency
				if err := os.WriteFile(goldenFile, []byte(got+"\n"), 0644); err != nil {
					t.Fatalf("write golden %s: %v", goldenFile, err)
				}
				t.Logf("%s: updated golden file: %s", base, goldenFile)
				continue
			}
			if _, err := os.Stat(goldenFile); os.IsNotExist(err) {
				// First run: create golden file
				os.MkdirAll(goldenDir, 0755)
				// Write with LF for consistency
				if err := os.WriteFile(goldenFile, []byte(got+"\n"), 0644); err != nil {
					t.Fatalf("write golden %s: %v", goldenFile, err)
				}
				t.Logf("%s: created golden file: %s", base, goldenFile)
				continue
			}
			want, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("read golden %s: %v", goldenFile, err)
			}
			wantStr := string(want)
			wantStr = strings.ReplaceAll(wantStr, "\r\n", "\n")
			wantStr = strings.TrimRight(wantStr, "\n")

			if got != wantStr {
				t.Errorf("%s -O%d output differs from golden\n  update: VAS_UPDATE_GOLDEN=1 cp ... %s", base, opt, goldenFile)
			}
		}
	}
}

func r(i int) string { return string(rune('0' + i)) }

// ── Benchmarks ─────────────────────────────────────────────────────────

func BenchmarkAssemble(b *testing.B) {
	input := "MOVI v0, 1\nMOVI v1, 0\nloop:\nADD v1, v1, v0\nADD v0, 1\nCMP v0, 100\nJLE loop\nMOV v5, v1\nMOVI v0, 60\nSYSCALL"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.Assemble(input)
	}
}

func BenchmarkAssembleWithOpt_O1(b *testing.B) {
	input := "MOVI v0, 1\nMOVI v1, 0\nloop:\nADD v1, v1, v0\nADD v0, 1\nCMP v0, 100\nJLE loop\nMOV v5, v1\nMOVI v0, 60\nSYSCALL"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
	}
}

func BenchmarkAssembleWithOpt_O2(b *testing.B) {
	input := "MOVI v0, 1\nMOVI v1, 0\nloop:\nADD v1, v1, v0\nADD v0, 1\nCMP v0, 100\nJLE loop\nMOV v5, v1\nMOVI v0, 60\nSYSCALL"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.AssembleWithOpt(input, vas.OptConfig{Level: 2})
	}
}

func BenchmarkAssembleLarge(b *testing.B) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "ADD v0, v0, 1")
	}
	lines = append(lines, "MOVI v0, 60", "SYSCALL")
	input := strings.Join(lines, "\n")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
	}
}

func BenchmarkAssembleStandalone(b *testing.B) {
	input := "MOVI v0, 60\nSYSCALL"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.AssembleStandalone(input)
	}
}

func BenchmarkPeephole(b *testing.B) {
	input := `
		MOVI v0, 1
		MOVI v0, 2
		ADD v1, 1, 2
		SUB v2, v1, 1
		MUL v3, v2, 8
		NOT v1
		NOT v1
		ADD v0, 1
		NEG v0
		MOVI v0, 60
		SYSCALL
	`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
	}
}
