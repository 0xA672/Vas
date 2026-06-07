//go:build js && wasm

package main

import (
	"syscall/js"

	"vas/vas"
)

func main() {
	c := make(chan struct{})

	js.Global().Set("vasAssemble", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return map[string]any{"error": "expected 1 string argument"}
		}
		input := args[0].String()
		result, err := vas.Assemble(input)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"output": result}
	}))

	js.Global().Set("vasAssembleWithOpt", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return map[string]any{"error": "expected at least 1 string argument"}
		}
		input := args[0].String()
		optLevel := 0
		if len(args) >= 2 && args[1].Type() == js.TypeNumber {
			optLevel = args[1].Int()
		}
		result, err := vas.AssembleWithOpt(input, optLevel)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"output": result}
	}))

	js.Global().Set("vasAssembleStandalone", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 || args[0].Type() != js.TypeString {
			return map[string]any{"error": "expected at least 1 string argument"}
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
		result, err := vas.AssembleStandaloneTargetOpt(input, target, optLevel)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"output": result}
	}))

	<-c
}
