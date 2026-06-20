//go:build js && wasm

// Package main provides the WebAssembly bridge for the VAS assembler.
// It registers global JavaScript functions that can be called from the
// playground or any web application.
package main

import (
	"fmt"
	"syscall/js"

	"vas/vas"
)

// jsReturn wraps a Go result/error into a value suitable for JavaScript.
// On error it throws a JS Error, otherwise returns the string result.
func jsReturn(result string, err error) any {
	if err != nil {
		js.Global().Get("Error").New(err.Error()).Invoke()
		return nil
	}
	return result
}

// main registers the assembly functions with the JavaScript runtime and
// blocks forever so the WASM module stays alive.
func main() {
	// vasAssemble assembles VAS source code into NASM assembly.
	// Signature: (input: string) => output: string
	js.Global().Set("vasAssemble", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return jsReturn("", fmt.Errorf("vasAssemble: expected 1 string argument"))
		}
		return jsReturn(vas.Assemble(args[0].String()))
	}))

	// vasAssembleWithOpt assembles VAS source with an optional optimisation level.
	// Signature: (input: string, optLevel?: number) => output: string
	js.Global().Set("vasAssembleWithOpt", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return jsReturn("", fmt.Errorf("vasAssembleWithOpt: expected at least 1 string argument"))
		}
		input := args[0].String()
		optLevel := 0
		if len(args) >= 2 && args[1].Type() == js.TypeNumber {
			optLevel = args[1].Int()
		}
		return jsReturn(vas.AssembleWithOpt(input, vas.OptConfig{Level: optLevel}))
	}))

	// vasAssembleStandalone assembles VAS source with a target platform and
	// optional optimisation level, wrapping it in a runnable skeleton if needed.
	// Signature: (input: string, target?: string, optLevel?: number) => output: string
	js.Global().Set("vasAssembleStandalone", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return jsReturn("", fmt.Errorf("vasAssembleStandalone: expected at least 1 string argument"))
		}
		input := args[0].String()
		target := "elf64"
		optLevel := 0
		if len(args) >= 2 && args[1].Type() == js.TypeString {
			target = args[1].String()
		}
		if len(args) >= 3 && args[2].Type() == js.TypeNumber {
			optLevel = args[2].Int()
		}
		return jsReturn(vas.AssembleStandaloneTargetOpt(input, target, optLevel))
	}))

	// Notify the host that the WASM module is ready.
	js.Global().Call("vasReady")

	// Keep the WASM module alive.
	select {}
}
