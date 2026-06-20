//go:build js && wasm

// Package main provides the WebAssembly bridge for the VAS assembler.
// Three global JavaScript functions are registered:
//
//	vasAssemble(input) -> {output, error}
//	vasAssembleWithOpt(input, optLevel) -> {output, error}
//	vasAssembleStandalone(input, target, optLevel) -> {output, error}
//
// Each returns a plain JS object. On success, output is non-empty and error
// is the empty string. On failure, output is empty and error contains the
// message. This avoids exception-throwing across the JS<->Go bridge, which
// is significantly faster and friendlier to host code.
package main

import (
	"syscall/js"

	"vas/vas"
)

// newResult builds the {output, error} JS object that all registered
// functions return. It takes a Go string for the output and the error
// message (empty if none).
func newResult(output, errMsg string) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("output", output)
	obj.Set("error", errMsg)
	return obj
}

// assembleFn is a helper wrapping a Go assembly function with the JS bridge
// argument shape. It delegates to the provided closure after validating
// that the first argument is a string.
func assembleFn(args []js.Value, fn func(string) (string, error)) js.Value {
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return newResult("", "expected a string argument")
	}
	out, err := fn(args[0].String())
	if err != nil {
		return newResult("", err.Error())
	}
	return newResult(out, "")
}

func main() {
	// vasAssemble: plain VAS -> NASM assembly.
	js.Global().Set("vasAssemble", js.FuncOf(func(_ js.Value, args []js.Value) any {
		return assembleFn(args, func(input string) (string, error) {
			return vas.Assemble(input)
		})
	}))

	// vasAssembleWithOpt: same as above with an integer optimisation level.
	js.Global().Set("vasAssembleWithOpt", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return newResult("", "expected at least 1 string argument")
		}
		optLevel := 0
		if len(args) >= 2 && args[1].Type() == js.TypeNumber {
			optLevel = args[1].Int()
		}
		out, err := vas.AssembleWithOpt(args[0].String(), vas.OptConfig{Level: optLevel})
		if err != nil {
			return newResult("", err.Error())
		}
		return newResult(out, "")
	}))

	// vasAssembleStandalone: VAS -> NASM with a platform skeleton wrapper.
	js.Global().Set("vasAssembleStandalone", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return newResult("", "expected at least 1 string argument")
		}
		target := "elf64"
		optLevel := 0
		if len(args) >= 2 && args[1].Type() == js.TypeString {
			target = args[1].String()
		}
		if len(args) >= 3 && args[2].Type() == js.TypeNumber {
			optLevel = args[2].Int()
		}
		out, err := vas.AssembleStandaloneTargetOpt(args[0].String(), target, optLevel)
		if err != nil {
			return newResult("", err.Error())
		}
		return newResult(out, "")
	}))

	// Notify the host JS that the WASM module is initialised and ready.
	if ready := js.Global().Get("vasReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}

	// Keep the WASM module alive so that the registered JS functions remain
	// callable. Without this, the Go runtime would exit immediately after
	// main returns.
	select {}
}
