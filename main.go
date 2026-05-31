package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"vas/vas"
)

func main() {
	outFile := flag.String("o", "", "output file (default: stdout)")
	flag.Parse()

	var input string

	if args := flag.Args(); len(args) > 0 {
		data, err := os.ReadFile(args[0])
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

	if *outFile != "" {
		if err := os.WriteFile(*outFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write file: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(output)
	}
}
