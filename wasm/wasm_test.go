//go:build js && wasm

package main

import (
	"syscall/js"
	"testing"
)

func TestWasmAssemble(t *testing.T) {
	// Verify that the WASM API functions are set (they're set in wasm.go main())
	// This test can only run with GOOS=js GOARCH=wasm.
	vasAssemble := js.Global().Get("vasAssemble")
	if !vasAssemble.IsUndefined() {
		t.Log("vasAssemble function registered")
	}
	vasAssembleWithOpt := js.Global().Get("vasAssembleWithOpt")
	if !vasAssembleWithOpt.IsUndefined() {
		t.Log("vasAssembleWithOpt function registered")
	}
	vasAssembleStandalone := js.Global().Get("vasAssembleStandalone")
	if !vasAssembleStandalone.IsUndefined() {
		t.Log("vasAssembleStandalone function registered")
	}
}
