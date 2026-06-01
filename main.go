package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"vas/vas"
)

func main() {
	var inputFile, outFile string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" && i+1 < len(args) {
			outFile = args[i+1]
			i++
		} else if args[i] == "-h" || args[i] == "--help" {
			fmt.Print(helpText)
			return
		} else if !strings.HasPrefix(args[i], "-") {
			inputFile = args[i]
		} else {
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			os.Exit(1)
		}
	}

	var input string

	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read file: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
	} else {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			os.Exit(1)
		}
		input = string(data)
		if input == "" {
			fmt.Fprintln(os.Stderr, "usage: vas [options] <input.asm|.vas>")
			fmt.Fprintln(os.Stderr, "       cat input.vas | vas")
			os.Exit(1)
		}
	}

	output, err := vas.Assemble(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "assembly error: %v\n", err)
		os.Exit(1)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(output)
	}
}

const helpText = `VAS — Virtual ASseMbler

Usage:
  vas [options] <input file>
  cat <input> | vas [options]

Options:
  -o <file>     Write output to file instead of stdout
  -h, --help    Show this help message

Input format: .vas or .asm files with virtual registers v0-v7.
Output: x86-64 NASM assembly.

Virtual register mapping:
  v0 → rax   v1 → rdi   v2 → rsi   v3 → rdx
  v4 → rcx   v5 → r8    v6 → r9    v7 → r10
`
