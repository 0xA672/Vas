# VAS — Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.s / .asm)  ->  nasm + ld / gcc  ->  executable
```

**VAS** (Virtual Assembler) is a lightweight text‑substitution translation tool. It reads pseudo‑instructions that use **virtual registers** (v0–v7) and outputs standard **x86-64 NASM** assembly.

It performs no register allocation, instruction scheduling, or linking—its sole purpose is to convert teaching/prototype pseudocode into NASM‑assemblable source code.

---

## Quick Start

### 1. Write Pseudocode (`hello.vas`)

```asm
; hello.vas — print "hello world" via Linux write syscall
MOVI v0, 1        ; rax = 1 (write syscall number)
MOVI v5, 1        ; rdi = 1 (stdout fd)
LEA  v4, [msg]    ; rsi = address of msg
MOVI v3, 12       ; rdx = length
SYSCALL
MOVI v0, 60       ; rax = 60 (exit syscall)
MOVI v5, 0        ; rdi = 0 (exit code)
SYSCALL

.data
msg: db "hello world", 10
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
	call	vas_main
	mov	edi, eax
	mov	eax, 60
	syscall
vas_main:
	mov	rax, 1
	mov	rdi, 1
	lea	rsi, [msg]
	mov	rdx, 12
	syscall
	mov	rax, 60
	mov	rdi, 0
	syscall
	ret
```

### 3. Build and Run

**Linux / WSL** (nasm + ld):
```bash
nasm -f elf64 -o hello.o hello.s
ld -o hello hello.o
./hello
```

**Windows** (nasm + link.exe or ld):
```bash
vas -target win64 hello.vas -o hello.asm
nasm -f win64 -o hello.obj hello.asm
ld -e main -o hello.exe hello.obj
hello.exe
```

---

## Virtual Register Mapping

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

Virtual registers can be used in any operand position, including memory addressing (e.g., `[v0+8]`), and are automatically replaced during translation.

---

## Supported Pseudo-Instructions

### Arithmetic Instructions

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `ADD` | `dst, src1, src2` | `mov dst, src1` then `add dst, src2` | Three‑operand addition |
| `ADD` | `dst, src` | `add dst, src` | Two‑operand addition |
| `SUB` | `dst, src1, src2` | `mov dst, src1` then `sub dst, src2` | Three‑operand subtraction |
| `SUB` | `dst, src` | `sub dst, src` | Two‑operand subtraction |
| `MUL` | `dst, src1, src2` | `mov dst, src1` then `imul dst, src2` | Three‑operand multiplication |
| `MUL` | `dst, src` | `imul dst, src` | Two‑operand multiplication |

### Memory Access

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `MOVI` | `dst, imm` | `mov dst, imm` | Load immediate |
| `MOV` | `dst, src` | `mov dst, src` | Register‑to‑register move |
| `LOAD` | `dst, [addr]` | `mov dst, [addr]` | Load from memory |
| `STORE` | `src, [addr]` | `mov [addr], src` | Store to memory |
| `LEA` | `dst, [addr]` | `lea dst, [addr]` | Load effective address |

Address expressions (such as `[v1]`, `[v0+8]`, `[label]`) are passed through, with only virtual registers replaced.

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
vas                         # read from stdin, output to stdout
vas input.vas               # translate input.vas, output to stdout
vas -o output.s input.vas   # write to file (-o can be before or after input)
vas input.vas -o output.s   # same as above
vas -target win64 input.vas # output Windows x64 skeleton instead of default ELF64
vas -O1 input.vas           # enable optimizations (dead code elimination + constant folding)
vas -h / --help             # show help
```

- **Pipe input**: `echo "MOVI v0, 42" | vas`
- Exits with an error if the input file does not exist
- When there is no input and stdin is empty, prints help and exits

---

## Standalone Mode

When the input does **not** contain any `section` / `global` / `extern` boilerplate (i.e., pure virtual‑register pseudocode), VAS automatically wraps the output in a minimal skeleton that can be assembled and run independently.

### ELF64 (Linux / WSL, default)

```bash
echo "MOVI v0, 42" | vas
```

Output:

```asm
default rel

	section .text
	global _start
_start:
	call	vas_main
	mov	edi, eax
	mov	eax, 60
	syscall
vas_main:
	mov	rax, 42
	ret

	section .data
	result:	dq 0
```

The skeleton contains:
- `default rel` + `section .text` + `global _start` / `_start:` → `call vas_main` → syscall exit
- User code is wrapped in `vas_main:`, and after returning via `ret`, an `exit(eax)` syscall is automatically executed
- A `.data` section is automatically added (skipped if user data definitions already exist)

### Win64 (Windows)

```bash
echo "MOVI v0, 42" | vas -target win64
```

The skeleton uses a `main:` entry point and exits via `xor eax, eax; ret` (unless the user’s last instruction is already `RET`).

### Skipping Standalone Mode

If the input already contains `section`, `global`, or `extern`, the output is passed through as‑is, with no skeleton added (`-target` still applies for register mapping style).

---

## Optimization (-O1)

`-O1` enables two levels of optimization:

1. **Dead code elimination (DCE)**: removes register assignments that have no effect on subsequent code (e.g., after `MOV v1, v0`, `v1` is never used before being overwritten)
2. **Constant folding**: literal arithmetic is computed at compile time (e.g., `MOVI v1, 3; ADD v0, v1, 7` → `mov rax, 10`)

DCE correctly preserves instructions with side effects (PUSH, POP, CALL, STORE, LOAD, SYSCALL, INT, RET, etc.).

---

## Syntax Details

### Comments
Both `;` and `#` can start inline comments (they will not be confused with delimiters inside string literals):

```asm
MOVI v0, 42   ; this is a comment
ADD  v1, v0   # this is also a comment
```

### Labels and Definitions
Lines that end with a colon and are not recognized instructions are passed through, with only virtual registers replaced:

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

### Passthrough
Any line that is not a known pseudo‑instruction (data definitions, section directives, alignment, etc.) is output as‑is, with only virtual registers replaced.  
Recognized data/section keywords: `SECTION`, `GLOBAL`, `EXTERN`, `DQ`, `DB`, `resb`, `equ`, etc.

**GAS → NASM dot‑prefix stripping**: `.section` → `section`, `.global` → `global`, `.text` → `text`, `.globl` → `global`:

```asm
; Input (GAS style):
.section .data
msg: .asciz "hello"

.section .text
.globl _start

; Output (NASM style):
section .data
msg: db "hello", 0

section .text
global _start
```

---

## Error Handling

- Known instruction with operand count mismatch: error with line number and source text, then exit
- Input file does not exist: error and exit immediately
- Unknown instruction: passed through (only virtual registers replaced), no error

---

## Examples

The project includes several practical examples covering various functional scenarios:

| File | Feature | Test Instructions |
|------|---------|-------------------|
| `hello.vas` | Linux write syscall print | MOVI, LEA, SYSCALL |
| `calc.vas` | Arithmetic operation chain | ADD, SUB, MUL |
| `fib.vas` | Iterative Fibonacci F(20) | CMP, JLE, JMP, LOOP |
| `fact.vas` | Recursive factorial fact(5) | CALL, RET, PUSH, POP |
| `sort.vas` | Bubble sort 8 elements | LOAD, STORE, nested loops, LEA |
| `greet.vas` | Linux syscall string output | .data section, SYSCALL |
| `win-ret42.vas` | Win64 return 42 | MOVI, RET |
| `win-ops.vas` | Win64 operation pipeline | ADD, MUL, SUB, Win64 |
| `win-edge.vas` | Win64 edge‑case test | PUSH/POP/STORE/LOAD/CMP/JE, .data |

Run ELF examples on Linux/WSL:
```bash
vas fib.vas -o fib.s && nasm -f elf64 fib.s -o fib.o && ld fib.o -o fib && ./fib; echo $?
```

Run Win64 examples on Windows:
```bash
vas -target win64 win-ops.vas -o win-ops.asm
nasm -f win64 win-ops.asm -o win-ops.obj
ld -e main -o win-ops.exe win-ops.obj
win-ops.exe
```

---

## Installation and Build

**Prerequisites**: Go 1.21+, no third‑party dependencies.

```bash
# Clone
git clone https://github.com/0xA672/Vas.git
cd vas

# Build
go build -o bin\vas.exe main.go

# Or install to $GOPATH/bin
go install
```

---

## Project Structure

```
vas/
├── main.go                  # CLI entry point, argument parsing
├── go.mod                   # Go module
├── vas/
│   ├── core.go              # Core translation logic: scan → expand → wrap
│   └── arch/
│       └── reg.go           # Register mapping table
│   └── opt/
│       └── opt.go           # -O1 optimizer (DCE + constant folding)
├── test/
│   └── assembler_test.go    # Unit tests (26 items)
├── bin/                     # Build artifacts (gitignored)
├── hello.vas                # Getting started example
├── calc.vas                 # Arithmetic example
├── fib.vas                  # Fibonacci example
├── fact.vas                 # Recursive factorial example
├── sort.vas                 # Bubble sort example
├── greet.vas                # Linux syscall example
├── win-ret42.vas            # Win64 minimal example
├── win-ops.vas              # Win64 arithmetic example
├── win-edge.vas             # Win64 edge‑case test
├── README.md
└── LICENSE
```

---

## Distinction from a Real Assembler

VAS explicitly does **not** perform the following tasks and should not be compared to GCC, LLVM, or real assemblers:

- No register allocation / instruction scheduling
- No instruction selection or optimization (except for simple -O1)
- No linking or relocation
- The generated `.s` / `.asm` file **must** be assembled by NASM and linked by ld to run

It is merely a thin translation layer that lets you write prototypes with friendlier pseudo‑instructions, leaving the rest to NASM.

---

## License

MIT — see the [LICENSE](LICENSE) file.
