//go:build js && wasm

package main_test

import (
	"strings"
	"syscall/js"
	"testing"
)

// TestWasmAssemble checks that the vasAssemble function is registered and
// correctly translates a minimal VAS program.
func TestWasmAssemble(t *testing.T) {
	fn := js.Global().Get("vasAssemble")
	if fn.IsUndefined() {
		t.Fatal("vasAssemble is not registered")
	}

	input := "MOVI v0, 60\nSYSCALL"
	result := fn.Invoke(input)
	if result.Get("error").Truthy() {
		t.Fatalf("unexpected error: %s", result.Get("error").String())
	}

	output := result.Get("output").String()
	if !strings.Contains(output, "mov\trax, 60") {
		t.Errorf("expected 'mov\trax, 60' in output, got:\n%s", output)
	}
}

// TestWasmAssembleWithOpt verifies that vasAssembleWithOpt accepts an
// optimisation level and produces valid output.
func TestWasmAssembleWithOpt(t *testing.T) {
	fn := js.Global().Get("vasAssembleWithOpt")
	if fn.IsUndefined() {
		t.Fatal("vasAssembleWithOpt is not registered")
	}

	input := "MOVI v0, 60\nSYSCALL"
	result := fn.Invoke(input, 1)
	if result.Get("error").Truthy() {
		t.Fatalf("unexpected error: %s", result.Get("error").String())
	}

	output := result.Get("output").String()
	if !strings.Contains(output, "mov\trax, 60") {
		t.Errorf("output missing expected instruction:\n%s", output)
	}
}

// TestWasmAssembleStandalone verifies that vasAssembleStandalone produces
// a runnable skeleton when given a target platform.
func TestWasmAssembleStandalone(t *testing.T) {
	fn := js.Global().Get("vasAssembleStandalone")
	if fn.IsUndefined() {
		t.Fatal("vasAssembleStandalone is not registered")
	}

	input := "MOVI v0, 60\nSYSCALL"
	result := fn.Invoke(input, "elf64", 0)
	if result.Get("error").Truthy() {
		t.Fatalf("unexpected error: %s", result.Get("error").String())
	}

	output := result.Get("output").String()
	// Standalone wrapper should include a .text section and the user code.
	if !strings.Contains(output, "section .text") {
		t.Errorf("expected 'section .text' in standalone output, got:\n%s", output)
	}
	if !strings.Contains(output, "mov\trax, 60") {
		t.Errorf("expected 'mov\trax, 60' in standalone output, got:\n%s", output)
	}
}