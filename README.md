# VAS — Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.s)  ->  nasm + ld / gcc  ->  executable
```

**VAS** (Virtual ASseMbler) is a lightweight, text‑replacement translation tool that reads pseudo‑instructions with **virtual registers** (v0–v7) and emits standard **x86-64 NASM** assembly.  
It is not a compiler backend — it performs **no register allocation, no optimization, and no linking**. Its sole purpose is to turn teaching or prototyping pseudocode into NASM‑compatible assembly.

---

## Quick Start

### 1. Minimal Example (`hello.vas`)

```asm
MOVI v0, 10
MOVI v1, 20
ADD  v2, v0, v1
STORE v2, [result]
```

### 2. Translate to NASM Assembly

```bash
vas -o hello.s hello.vas
```

Generated `hello.s`:

```asm
mov     rax, 10
mov     rdi, 20
mov     rsi, rax
add     rsi, rdi
mov     [result], rsi
```

### 3. Build an Executable

**Windows** (using nasm and gcc):
```bash
nasm -f win64 -o hello.obj hello.s
gcc -nostartfiles -o hello.exe hello.obj
hello.exe
```

**Linux** (using nasm and ld):
```bash
nasm -f elf64 -o hello.o hello.s
ld -o hello hello.o
./hello
```

---

## Virtual Register Mapping

The eight virtual registers `v0`–`v7` are mapped to physical x86-64 registers as follows. No explicit declaration is required — they can be used immediately.

| Virtual Register | Physical Register |
|------------------|-------------------|
| v0               | `rax`             |
| v1               | `rdi`             |
| v2               | `rsi`             |
| v3               | `rdx`             |
| v4               | `rcx`             |
| v5               | `r8`              |
| v6               | `r9`              |
| v7               | `r10`             |

Virtual registers may appear in any operand position, including memory addressing (e.g., `[v0+8]`). They are replaced during translation.

---

## Supported Pseudo-Instructions

### Arithmetic Instructions

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `ADD`              | `dst, src1, src2` | `mov dst, src1` then `add dst, src2` | Three‑operand addition |
| `ADD`              | `dst, src`        | `add dst, src`                       | Two‑operand addition |
| `SUB`              | `dst, src1, src2` | `mov dst, src1` then `sub dst, src2` | Three‑operand subtraction |
| `SUB`              | `dst, src`        | `sub dst, src`                       | Two‑operand subtraction |
| `MUL`              | `dst, src1, src2` | `mov dst, src1` then `imul dst, src2`| Three‑operand multiplication |
| `MUL`              | `dst, src`        | `imul dst, src`                      | Two‑operand multiplication |

### Memory Access

| Pseudo-instruction | Operands         | Expansion            | Notes                  |
|--------------------|------------------|----------------------|------------------------|
| `MOVI`             | `dst, imm`       | `mov dst, imm`       | Load immediate         |
| `MOV`              | `dst, src`       | `mov dst, src`       | Register‑to‑register   |
| `LOAD`             | `dst, [addr]`    | `mov dst, [addr]`    | Load from memory       |
| `STORE`            | `src, [addr]`    | `mov [addr], src`    | Store to memory        |

Address expressions (e.g., `[v1]`, `[rax*4]`, `[v0+8]`, `[label]`) are passed through, with virtual registers replaced.

### Control Flow

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `CMP`              | `a, b`   | `cmp a, b`|
| `JMP`              | `label`  | `jmp label`|
| `JE`               | `label`  | `je label`|
| `JNE`              | `label`  | `jne label`|
| `JG`               | `label`  | `jg label`|
| `JL`               | `label`  | `jl label`|
| `JGE`              | `label`  | `jge label`|
| `JLE`              | `label`  | `jle label`|
| `CALL`             | `label`  | `call label`|
| `RET`              | —        | `ret`|

### Stack Operations

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `PUSH`             | `src`    | `push src`|
| `POP`              | `dst`    | `pop dst` |

### System Calls / Interrupts

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `SYSCALL`          | —        | `syscall` |
| `INT`              | `n`      | `int n`   |

### Miscellaneous

| Pseudo-instruction | Expansion |
|--------------------|-----------|
| `NOP`              | `nop`     |

---

## Command Line Usage

```bash
vas                         # Read from stdin, write to stdout
vas input.vas               # Translate input.vas, output to stdout
vas -o output.s input.vas   # Write to file (-o may precede or follow the input file)
vas input.vas -o output.s   # Equally valid
vas -h                      # Display help
```

- **Piped input**: `echo "MOVI v0, 42" | vas`
- An error is raised if the input file does not exist.
- If there is no input and stdin is empty, usage information is printed and the program exits.

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
section .data:
result: dq 0

section .text:
global _start:
_start:
```

### Passthrough
Any line that is not a known pseudo‑instruction (data definitions, section directives, alignment, etc.) is emitted verbatim with only virtual registers replaced.  
Recognized data/section keywords (e.g., `SECTION`, `GLOBAL`, `EXTERN`, `DQ`, `DB`) are token‑checked; malformed constructs still trigger errors.

---

## Error Handling

- A **known instruction with an incorrect number of operands** causes an error, displaying the line number and original line, then exit.
- A **missing input file** causes an immediate error and exit.
- **Unknown instructions** are passed through without error (only virtual registers are substituted).

---

## Installation and Build

**Prerequisites**: Go 1.21 or later, no third‑party dependencies.

```bash
# Clone the repository
git clone https://github.com/your/vas.git
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
├── vas/
│   └── core.go     # Core translation logic, register substitution, tokenisation
├── hello.vas       # Example input
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
