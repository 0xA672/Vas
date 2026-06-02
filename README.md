# VAS — Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.s)  ->  nasm + ld / gcc  ->  executable
```

**VAS** (Virtual Assembler) is a lightweight, text-replacement translation tool that reads pseudo-instructions with **virtual registers** (v0-v7) and emits standard **x86-64 NASM** assembly.
It is not a compiler backend — it performs **no register allocation, no optimization, and no linking**. Its sole purpose is to turn teaching or prototyping pseudocode into NASM-compatible assembly.

---

## Quick Start

### 1. Minimal Example (`hello.vas`)

```asm
default rel

section .data
msg: db "hello world", 10

section .text
global _start
_start:
    MOVI v0, 1       ; rax = 1 (write syscall)
    MOVI v5, 1       ; rdi = 1 (stdout fd)
    LEA  v4, [msg]   ; rsi = address of msg
    MOVI v3, 12      ; rdx = length
    SYSCALL
    MOVI v0, 60      ; rax = 60 (exit syscall)
    MOVI v5, 0       ; rdi = 0 (exit code)
    SYSCALL
```

### 2. Translate to NASM Assembly

```bash
vas -o hello.s hello.vas
```

Generated `hello.s`:

```asm
default rel

	section .data
	msg: db "hello world", 10

	section .text
	global _start
_start:
	mov	rax, 1
	mov	rdi, 1
	lea	rsi, [msg]
	mov	rdx, 12
	syscall
	mov	rax, 60
	mov	rdi, 0
	syscall
```

### 3. Build an Executable

**Linux** (using nasm and ld):
```bash
nasm -f elf64 -o hello.o hello.s
ld -o hello hello.o
./hello
```

For Windows, write a `.vas` file with a `main:` entry point instead of `_start:`, then:
```bash
vas -target win64 hello-win.vas -o hello.asm
nasm -f win64 -o hello.obj hello.asm
ld -e main -o hello.exe hello.obj
hello.exe
```

---

## Virtual Register Mapping

The eight virtual registers `v0`-`v7` are mapped to physical x86-64 registers as follows. No explicit declaration is required — they can be used immediately.

| Virtual Register | Physical Register |
|------------------|-------------------|
| v0               | `rax`             |
| v1               | `rbx`             |
| v2               | `rcx`             |
| v3               | `rdx`             |
| v4               | `rsi`             |
| v5               | `rdi`             |
| v6               | `r8`              |
| v7               | `r9`              |

Virtual registers may appear in any operand position, including memory addressing (e.g., `[v0+8]`). They are replaced during translation.

---

## Supported Pseudo-Instructions

### Arithmetic Instructions

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `ADD` | `dst, src1, src2` | `mov dst, src1` then `add dst, src2` | Three-operand addition |
| `ADD` | `dst, src` | `add dst, src` | Two-operand addition |
| `SUB` | `dst, src1, src2` | `mov dst, src1` then `sub dst, src2` | Three-operand subtraction |
| `SUB` | `dst, src` | `sub dst, src` | Two-operand subtraction |
| `MUL` | `dst, src1, src2` | `mov dst, src1` then `imul dst, src2` | Three-operand multiplication |
| `MUL` | `dst, src` | `imul dst, src` | Two-operand multiplication |

### Memory Access

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `MOVI` | `dst, imm` | `mov dst, imm` | Load immediate |
| `MOV` | `dst, src` | `mov dst, src` | Register-to-register |
| `LOAD` | `dst, [addr]` | `mov dst, [addr]` | Load from memory |
| `STORE` | `src, [addr]` | `mov [addr], src` | Store to memory |
| `LEA` | `dst, [addr]` | `lea dst, [addr]` | Load effective address |

Address expressions (e.g., `[v1]`, `[rax*4]`, `[v0+8]`, `[label]`) are passed through, with virtual registers replaced.

### Control Flow

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `CMP` | `a, b` | `cmp a, b` |
| `JMP` | `label` | `jmp label` |
| `JE` | `label` | `je label` |
| `JNE` | `label` | `jne label` |
| `JG` | `label` | `jg label` |
| `JL` | `label` | `jl label` |
| `JGE` | `label` | `jge label` |
| `JLE` | `label` | `jle label` |
| `CALL` | `label` | `call label` |
| `RET` | — | `ret` |

### Stack Operations

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `PUSH` | `src` | `push src` |
| `POP` | `dst` | `pop dst` |

### System Calls / Interrupts

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `SYSCALL` | — | `syscall` |
| `INT` | `n` | `int n` |

### Miscellaneous

| Pseudo-instruction | Expansion |
|--------------------|-----------|
| `NOP` | `nop` |

---

## Command Line Usage

```bash
vas                         # Read from stdin, write to stdout
vas input.vas               # Translate input.vas, output to stdout
vas -o output.s input.vas   # Write to file (-o may precede or follow the input file)
vas input.vas -o output.s   # Equally valid
vas -target win64 input.vas # Output Windows x64 (COFF/NASM) skeleton instead of default ELF64
vas -h                      # Display help
```

- **Piped input**: `echo "MOVI v0, 42" | vas`
- An error is raised if the input file does not exist.
- If there is no input and stdin is empty, usage information is printed and the program exits.

---

## Standalone Mode

When the input does **not** contain any section/global/extern boilerplate (i.e. it is pure pseudocode with virtual registers), `vas` automatically wraps the output in a minimal skeleton so you can assemble and run it immediately.

By default a **Linux ELF64** skeleton is emitted. Use `-target win64` for a **Windows x64** skeleton instead.

```bash
echo "MOVI v0, 42" | vas          # default: ELF64
echo "MOVI v0, 42" | vas -target win64   # Windows COFF
```

Default (ELF64) output:

```asm
default rel

	section .text
	global _start
_start:
	mov	rax, 42

	xor	edi, edi
	mov	eax, 60
	syscall

	section .data
	result:	dq 0
```

The ELF64 skeleton includes:
- `default rel`, `section .text`, `global _start`, `_start:` header
- An automatic `exit(0)` syscall (unless the last instruction is already `SYSCALL`)
- A `.data` section with zero-initialised `dq` entries for any memory references found in `[...]`

The win64 skeleton includes:
- `default rel`, `section .text`, `global main`, `main:` header
- An automatic `exit(0)` via `xor eax, eax; ret` (unless the last instruction is already `RET`)

If your input already contains `section`, `global`, or `extern` directives, the standalone wrapping is skipped and the output is emitted verbatim regardless of `-target`.

---

## Syntax Details

### Comments
Both `;` and `#` start inline comments (never confused with delimiters inside string literals):

```asm
MOVI v0, 42   ; This is a comment
ADD  v1, v0   # This is also a comment
```

### Labels and Definitions
Lines ending with a colon that are not recognized instructions are passed through unchanged, except for virtual register substitution. Example:

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

### Passthrough
Any line that is not a known pseudo-instruction (data definitions, section directives, alignment, etc.) is emitted verbatim with only virtual registers replaced.
Recognized data/section keywords (e.g., `SECTION`, `GLOBAL`, `EXTERN`, `DQ`, `DB`, `resb`, `equ`) are token-checked; malformed constructs still trigger errors.

**GAS → NASM dot prefix stripping**: Directive keywords prefixed with `.` (e.g., `.section`, `.global`, `.text`, `.data`) have the leading dot stripped automatically, so GAS-style source files translate smoothly to NASM output:

```asm
; Input (GAS style):
.section .data
msg: .asciz "hello"

.section .text
.global _start

; Output (NASM style):
section .data
msg: db "hello", 0

section .text
global _start
```

---

## Error Handling

- A **known instruction with an incorrect number of operands** causes an error, displaying the line number and original line, then exit.
- A **missing input file** causes an immediate error and exit.
- **Unknown instructions** are passed through without error (only virtual registers are substituted).

---

## Installation and Build

**Prerequisites**: Go 1.21 or later, no third-party dependencies.

```bash
# Clone the repository
git clone https://github.com/0xA672/Vas.git
cd vas

# Build
go build -o vas

# Or install to $GOPATH/bin
go install
```

---

## Project Structure

```
vas/
├── main.go         # CLI entry point, argument parsing
├── go.mod          # Go module definition
├── vas/
│   └── core.go     # Core translation logic, register substitution, tokenisation
├── bin/            # Build output directory (gitignored)
├── test/
│   └── assembler_test.go  # Unit tests
├── examples/       # Additional .vas example files
├── syntaxes/       # VSCode syntax highlighting extension (vas.tmLanguage)
├── hello.vas       # Hello world (syscall) example
├── .gitignore
└── README.md
```

---

## Distinction from a Real Assembler

VAS **explicitly does not** perform the following tasks, and should not be compared to tools such as GCC or LLVM:

- No register allocation / scheduling
- No instruction selection or optimization of any kind
- No linking or relocation
- The generated `.s` file **must** be assembled by NASM and linked with `ld` (or `gcc`) to produce an executable

It is a thin translation layer that allows you to write prototypes with friendlier syntax, leaving the rest to NASM.

---

## License

MIT — see the [LICENSE](LICENSE) file.
