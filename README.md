# VAS — Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.s / .asm)  ->  nasm + ld / gcc  ->  executable
```

**VAS** (Virtual Assembler) is a lightweight text-replacement translator. It reads pseudo-instructions that use **virtual registers** (v0–v7) and outputs standard **x86-64 NASM** assembly.

It does not perform register allocation, instruction scheduling, or linking — its sole purpose is to turn educational/prototype pseudo-code into NASM-assemblable source.

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

	section .data
msg: db "hello world", 10
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

Virtual registers can be used in any operand position, including memory addressing (e.g. `[v0+8]`), and are automatically replaced during translation.

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
| `MOV` | `dst, src` | `mov dst, src` | Register-to-register move |
| `LOAD` | `dst, [addr]` | `mov dst, [addr]` | Load from memory |
| `STORE` | `src, [addr]` | `mov [addr], src` | Store to memory |
| `LEA` | `dst, [addr]` | `lea dst, [addr]` | Load effective address |

Address expressions (e.g. `[v1]`, `[v0+8]`, `[label]`) pass through with only virtual register substitution.

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
vas                         # Read from stdin, output to stdout
vas input.vas               # Translate input.vas, output to stdout
vas -o output.s input.vas   # Write to file (-o can appear before or after input)
vas input.vas -o output.s   # Same as above
vas -target win64 input.vas # Output Windows x64 skeleton instead of default ELF64
vas -O1 input.vas           # Enable optimization (dead code elimination + constant folding)
vas diff input.vas          # Show VAS source vs NASM output side-by-side
vas stats input.vas         # Show instruction category counts and register usage
vas check input.vas         # Validate VAS syntax; exits 0 on success, 1 on error
vas list                    # List all supported instructions and syntax
vas version                 # Print version string (e.g. "vas v0.2.0")
vas -v / --version          # Same as above
vas -h / --help             # Show help
```

- **Pipeline input**: `echo "MOVI v0, 42" | vas`
- Exits with error if input file does not exist
- If no input file is given and stdin is empty, prints help and exits

---

## Standalone Mode

When the assembled output **does not** contain a `section .text` directive, VAS automatically wraps the output in a minimal standalone skeleton that can be assembled and run directly. If the input already defines its own `.text` section, the standalone skeleton is skipped.

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
```

The skeleton includes:
- `default rel` + `section .text` + `global _start` / `_start:` → `call vas_main` → syscall exit
- User code is placed under `vas_main:`; to return cleanly the user code should end with `RET`
- After `call vas_main` returns, the exit syscall `exit(eax)` executes automatically

### Win64 (Windows)

```bash
echo "MOVI v0, 42" | vas -target win64
```

The skeleton uses `main:` as the entry point, ending with `xor eax, eax; ret` (unless the user's last instruction is already `RET`).

### Skip Standalone Mode

If the assembled output already contains a `section .text` directive (i.e. the input defines its own text section), the output is passed through as-is without any skeleton wrapping.

---

## Optimization (-O1)

`-O1` enables two levels of optimization:

1. **Dead Code Elimination (DCE)**: Removes register assignments that have no effect on subsequent output (e.g. `MOV v1, v0` where v1 is never used before being overwritten)
2. **Constant Folding**: Computes literal arithmetic at compile time (e.g. `MOVI v1, 3; ADD v0, v1, 7` → `mov rax, 10`)

DCE correctly preserves instructions with side effects (PUSH, POP, CALL, STORE, LOAD, SYSCALL, INT, RET, etc.).

---

## Syntax Details

### Comments

Both `;` and `#` can start inline comments (they won't be confused with delimiters inside string literals):

```asm
MOVI v0, 42   ; this is a comment
ADD  v1, v0   # this is also a comment
```

### Labels and Directives

Lines ending with a colon that are not known pseudo-instructions pass through with only virtual register substitution:

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

### Passthrough

Any line that is not a known pseudo-instruction (data definitions, section directives, alignment, etc.) is output as-is, with only virtual register substitution. Recognized data/section keywords include: `section`, `global`, `extern`, `dq`, `db`, `resb`, `equ`, etc.

**GAS → NASM dot-prefix stripping**: the default case in `processInstruction` automatically strips the leading `.` from GAS-style directives and performs a few common conversions:

| GAS input | NASM output |
|-----------|-------------|
| `.section .data` | `section .data` |
| `.global _start` | `global _start` |
| `.globl _start` | `global _start` |
| `.data` | `section .data` |
| `.text` | `section .text` |
| `.bss` | `section .bss` |

Other dot-prefixed directives (e.g. `.asciz`, `.type`, `.size`) are stripped of the leading dot but otherwise pass through unchanged — they may not be valid NASM and should be written in NASM syntax directly.

---

## Error Handling

- **Known instruction operand count mismatch**: reports the original line text, then exits
- **Virtual register out of range** (only v0–v7 are defined): reports the invalid register name, then exits
- **Input file does not exist**: immediately reports error and exits
- **Unknown instruction**: passed through (only virtual registers are substituted), no error reported

---

## Examples

The project includes several practical examples covering a range of features:

| File | Description | Instructions Used |
|------|-------------|-------------------|
| `hello.vas` | Linux write syscall to print | MOVI, LEA, SYSCALL |
| `calc.vas` | Arithmetic operation chain | MOVI, ADD, CMP, JLE, MOV, SYSCALL |
| `demo.vas` | Combined example: syscall, branches, memory | MOVI, ADD, STORE, CMP, JG, JMP, SYSCALL |
| `fib.vas` | Iterative Fibonacci F(20) | MOVI, MOV, ADD, CMP, JGE, JMP |
| `fact.vas` | Recursive factorial fact(5) | CALL, RET, PUSH, POP |
| `sort.vas` | Bubble sort of 8 elements | MOVI, LEA, LOAD, STORE, CMP, JGE, JLE, JE, JMP, PUSH, POP, RET |
| `greet.vas` | Linux CLI tool with cmd-line args | MOVI, MOV, ADD, LOAD, STORE, CMP, JE, JNE, JMP, POP, SYSCALL |
| `win-ops.vas` | Win64 operation pipeline | MOVI, ADD, MUL, SUB, RET |
| `win-edge.vas` | Win64 edge-case tests | MOVI, PUSH, POP, STORE, LOAD, CMP, JE, RET |

Run ELF examples on Linux/WSL:
```bash
vas examples/fib.vas -o fib.s && nasm -f elf64 fib.s -o fib.o && ld fib.o -o fib && ./fib; echo $?
```

Run Win64 examples on Windows:
```bash
vas -target win64 examples/win-ops.vas -o win-ops.asm
nasm -f win64 win-ops.asm -o win-ops.obj
ld -e main -o win-ops.exe win-ops.obj
win-ops.exe
```

---

## Installation and Build

**Prerequisites**: Go 1.21+, no third-party dependencies.

```bash
# Clone
git clone https://github.com/0xA672/Vas.git
cd vas

# Build (dev version prints "vas dev")
go build -o bin\vas.exe .

# Build with a version string embedded via ldflags
go build -ldflags "-X main.Version=v0.2.0" -o bin\vas.exe .

# Or install to $GOPATH/bin
go install
```

`vas version` and `vas -v` print the embedded version string.

---

## Project Structure

```
vas/
├── main.go                  # CLI entry, argument parsing
├── go.mod                   # Go module
├── vas/
│   ├── core.go              # Core translation: scan → expand → wrap (includes regMap)
│   └── opt/
│       └── opt.go           # -O1 optimizer (DCE + constant folding)
├── test/
│   └── assembler_test.go    # Unit tests (39 cases)
├── examples/                # Example .vas files
│   ├── hello.vas            # Getting-started example
│   ├── calc.vas             # Arithmetic example
│   ├── demo.vas             # Combined demonstration
│   ├── fib.vas              # Fibonacci example
│   ├── fact.vas             # Recursive factorial
│   ├── sort.vas             # Bubble sort
│   ├── greet.vas            # Linux syscall example
│   ├── win-ops.vas          # Win64 operation example
│   └── win-edge.vas         # Win64 edge-case test
├── bin/                     # Build artifacts (gitignored)
├── README.md
└── LICENSE
```

---

## Distinction from a Real Assembler

VAS **explicitly does not** perform the following tasks and should not be compared to GCC, LLVM, or a full assembler:

- No register allocation / instruction scheduling
- No instruction selection or optimization (beyond simple -O1)
- No linking or relocation
- Generated `.s` / `.asm` files **must** be assembled by NASM and linked by ld to produce an executable

It is simply a thin translation layer that lets you write prototypes with friendlier pseudo-instructions; NASM handles the rest.

---

## License

MIT — see [LICENSE](LICENSE) file.
