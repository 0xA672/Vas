# VAS - Virtual Assembler
[![zread](https://img.shields.io/badge/Ask_Zread-_.svg?style=for-the-badge&color=00b0aa&labelColor=000000&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTQuOTYxNTYgMS42MDAxSDIuMjQxNTZDMS44ODgxIDEuNjAwMSAxLjYwMTU2IDEuODg2NjQgMS42MDE1NiAyLjI0MDFWNC45NjAxQzEuNjAxNTYgNS4zMTM1NiAxLjg4ODEgNS42MDAxIDIuMjQxNTYgNS42MDAxSDQuOTYxNTZDNS4zMTUwMiA1LjYwMDEgNS42MDE1NiA1LjMxMzU2IDUuNjAxNTYgNC45NjAxVjIuMjQwMUM1LjYwMTU2IDEuODg2NjQgNS4zMTUwMiAxLjYwMDEgNC45NjE1NiAxLjYwMDFaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00Ljk2MTU2IDEwLjM5OTlIMi4yNDE1NkMxLjg4ODEgMTAuMzk5OSAxLjYwMTU2IDEwLjY4NjQgMS42MDE1NiAxMS4wMzk5VjEzLjc1OTlDMS42MDE1NiAxNC4xMTM0IDEuODg4MSAxNC4zOTk5IDIuMjQxNTYgMTQuMzk5OUg0Ljk2MTU2QzUuMzE1MDIgMTQuMzk5OSA1LjYwMTU2IDE0LjExMzQgNS42MDE1NiAxMy43NTk5VjExLjAzOTlDNS42MDE1NiAxMC42ODY0IDUuMzE1MDIgMTAuMzk5OSA0Ljk2MTU2IDEwLjM5OTlaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik0xMy43NTg0IDEuNjAwMUgxMS4wMzg0QzEwLjY4NSAxLjYwMDEgMTAuMzk4NCAxLjg4NjY0IDEwLjM5ODQgMi4yNDAxVjQuOTYwMUMxMC4zOTg0IDUuMzEzNTYgMTAuNjg1IDUuNjAwMSAxMS4wMzg0IDUuNjAwMUgxMy43NTg0QzE0LjExMTkgNS42MDAxIDE0LjM5ODQgNS4zMTM1NiAxNC4zOTg0IDQuOTYwMVYyLjI0MDFDMTQuMzk4NCAxLjg4NjY0IDE0LjExMTkgMS42MDAxIDEzLjc1ODQgMS42MDAxWiIgZmlsbD0iI2ZmZiIvPgo8cGF0aCBkPSJNNCAxMkwxMiA0TDQgMTJaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00IDEyTDEyIDQiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K&logoColor=ffffff)](https://zread.ai/0xA672/Vas)
[![Playground](https://img.shields.io/badge/Playground-Online-blue?style=for-the-badge&logo=github&color=181717&labelColor=24292f)](https://0xa672.github.io/Vas/)

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.asm)  ->  nasm + ld / gcc  ->  executable
```

VAS (Virtual Assembler) is a lightweight text-replacement translator. It reads pseudo-instructions that use **virtual registers** (v0-v12) and outputs standard **x86-64 NASM** assembly.

It does not perform register allocation, instruction scheduling, or linking. Its sole purpose is to turn educational/prototype pseudo-code into NASM-assemblable source.

## Quick Start

### 1. Write Pseudocode (`hello.vas`)

```asm
; hello.vas -- print "hello world" via Linux write syscall
default rel

section .data
    msg db 'Hello, World from VAS!', 10
    msglen equ $ - msg

section .text
    global _start

_start:
    MOVI    v5, 1           ; rdi = 1 (stdout)
    LEA     v4, [msg]       ; rsi = msg address
    MOVI    v3, msglen      ; rdx = message length
    MOVI    v0, 1           ; rax = 1 (sys_write)
    SYSCALL

    MOVI    v5, 0           ; rdi = exit code 0
    MOVI    v0, 60          ; rax = 60 (sys_exit)
    SYSCALL
```

### 2. Translate to NASM Assembly

```bash
vas -o hello.asm hello.vas
```

Generated `hello.asm`:

```asm
; hello.vas -- print "hello world" via Linux write syscall

        default rel

        section .data
        msg db 'Hello, World from VAS!', 10
        msglen equ $ - msg

        section .text
        global _start

_start:
        mov     rdi, 1
        lea     rsi, [msg]
        mov     rdx, msglen
        mov     rax, 1
        syscall

        mov     rdi, 0
        mov     rax, 60
        syscall
```

### 3. Build and Run

**Choose your platform:**

**Linux / WSL** (nasm + ld):
```bash
nasm -f elf64 -o hello.o hello.asm
ld -o hello hello.o
./hello
```

**Windows** (nasm + ld):
```bash
vas -target win64 hello.vas -o hello.asm
nasm -f win64 -o hello.obj hello.asm
ld -e main -o hello.exe hello.obj
hello.exe
```

> **Tip**: If your code doesn't define its own `section .text`, VAS automatically wraps it in a runnable skeleton. See [Standalone Mode](#standalone-mode) for details.

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
| v8               | `r11`             |
| v9               | `r12`             |
| v10              | `r13`             |
| v11              | `r14`             |
| v12              | `r15`             |

Virtual registers can be used in any operand position, including memory addressing (e.g. `[v0+8]`, `[v1+v2*8]`), and are automatically replaced during translation.

For example:
```asm
; VAS input
ADD v1, [v0+8], v2

; NASM output
mov rbx, [rax+8]
add rbx, r12
```

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
| `RET` | - | `ret` |

### Stack Operations

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `PUSH` | `src` | `push src` |
| `POP` | `dst` | `pop dst` |

### System Calls / Interrupts

| Pseudo-instruction | Operands | Expansion |
|--------------------|----------|-----------|
| `SYSCALL` | - | `syscall` |
| `INT` | `n` | `int n` |

### Miscellaneous

| Pseudo-instruction | Expansion |
|--------------------|-----------|
| `NOP` | `nop` |

### Passthrough (Raw x86 Instructions)

Any line not recognized as a pseudo-instruction passes through with virtual registers substituted. **You don't need to memorize which instructions are supported—you can write any x86-64 instruction using v0-v12 as registers.** This lets you use raw x86 instructions directly:

| Instruction | Example | Notes |
|-------------|---------|-------|
| `movzx` | `movzx v0, byte [v1]` | Zero-extend byte load |
| `div` | `div v6` | Unsigned divide rdx:rax by v6 |
| `shl` / `shr` | `shl v0, 8` | Shift left/right |
| `and` / `or` / `xor` | `and v0, 0xFF` | Bitwise operations |
| `test` | `test v0, v0` | Set flags without write |
| `not` / `neg` | `neg v0` | Bitwise NOT / negate |

Virtual register substitution works inside passthrough, so `div v6` becomes `div r8`.

### Directives (Passthrough without Register Substitution)

These directives pass through unchanged (no v-register substitution):
- `SECTION .text`, `SECTION .data`, `SECTION .bss`
- `GLOBAL label`, `EXTERN label`
- `DB`, `DW`, `DD`, `DQ`, `BYTE`, `WORD`, `DWORD`, `QWORD`
- `ALIGN n`, `TYPE`, `SIZE`, `LENGTH`

**GAS-to-NASM conversion**: Dot-prefixed directives (`.section`, `.global`, `.globl`, `.data`, `.text`, `.bss`) are automatically converted to NASM syntax (dot stripped, `.globl` -> `global`, `.data` -> `section .data`).

## Syntax Details

### Comments

Both `;` and `#` start inline comments. Quoted strings preserve `;` and `#` as literals.

```asm
MOVI v0, 42   ; this is a comment
ADD  v1, v0   # this is also a comment
```

### Preprocessor Directives

The VAS preprocessor runs automatically whenever the source contains
any preprocessor directive, even if no virtual registers are present.
The supported directives are:

### File Inclusion (`.include`)

```asm
.include "utils.vas"      ; Include from current directory or search path
.include <std/io>         ; Include from package cache or VAS_PATH
```

**Automatic Deduplication**: VAS automatically prevents duplicate file inclusion based on absolute paths. The same file will only be expanded once, regardless of how many times it's included. This eliminates One Definition Rule (ODR) issues without requiring manual guards.

### Single-Inclusion Guards

The `.once` directive (optional) documents that a file is designed for single inclusion:

```asm
; utils.vas
.once  ; Optional: documents that this file is designed for single inclusion

.const SYS_write = 1
.macro print_str ptr
  MOVI v0, SYS_write
  SYSCALL
.endm
```

**Note**: Since VAS already handles automatic deduplication at the file level, `.once` has no functional effect. It serves as documentation only.

For fine-grained control over code blocks, use `.once begin <name>` and `.once end [<name>]`:

```asm
; utils.vas - Block-level deduplication

.once begin constants
  .const SYS_write = 1
  .const BUFFER_SIZE = 1024
.once end constants

.once begin macros
  .macro print_str ptr
    MOVI v0, SYS_write
    SYSCALL
  .endm
.once end macros

; Later in the same file or another included file...
.once begin constants
  ; This block will be SKIPPED because "constants" was already included
  .const SHOULD_NOT_APPEAR = 999
.once end constants
```

**Features:**
- **Named Blocks**: Each block must have a unique name for identification
- **Deduplication**: Blocks with the same name are only included once (first occurrence)
- **Nesting Support**: Blocks can be nested; inner blocks maintain their own deduplication state
- **Name Validation**: Optional name in `.once end` is checked against the matching `.once begin`
- **Error Detection**: Unmatched `.once end` or missing block names produce clear error messages

**Use Cases:**
- Organize large header files into logical sections
- Conditionally include different implementations
- Prevent ODR issues for specific code blocks without affecting the entire file

### Constants and Conditional Compilation

Constants are pure text substitutions, defined with `.const`:

```asm
.const SYS_write = 1
.const BUFFER_SIZE = 1024

MOVI v0, SYS_write     ; → MOVI v0, 1
```

Defined constants are automatically available for `.ifdef` checks. **Important**: constant replacement only occurs in code regions, not inside quoted strings or comments.

Conditional compilation uses `.ifdef` / `.ifndef` / `.else` / `.endif`:

```asm
.const DEBUG = 1

.ifdef DEBUG
  MOVI v0, 1      ; Debug mode code
.else
  MOVI v0, 0      ; Release mode code
.endif
```

Only checks if a name is defined (via `.const`), does not support value comparison. Nested conditionals are supported, with proper handling of `.else` in true/false branches. The `.else` is ignored when the corresponding block is inside a skipped false branch.

### Macros (`.macro` / `.endm`)

```asm
.macro strlen ptr, len
  MOVI \len, 0
  .loop\@:
    CMP [\ptr + \len], 0
    JE .done\@
    ADD \len, \len, 1
    JMP .loop\@
  .done\@:
.endm

strlen msg, v1  ; Expands with unique labels (.loop_1, .done_1)
```

- `\param` - Parameter substitution
- `\@` - Unique label generation (auto-incrementing counter)

### Repetition (`.rept` / `.endr`)

```asm
.rept 5
  NOP
.endr
; Expands to 5 NOP instructions
```

**Nested rept blocks are supported.** Inner `.rept` blocks expand correctly:

```asm
.rept 2
  .rept 3
    NOP
  .endr
.endr
; Expands to 6 NOPs (2 × 3)
```

### Binary Data Inclusion (`.include_bytes`)

```asm
.include_bytes "data.bin"
; Converts to db directives with hex bytes
```

### Symbol Visibility in Package Includes

When including a package with angle brackets (`.include <pkg>`), VAS processes the package in a **separate context**. This means:
- Macros and constants defined in the package are **not** visible to the including file.
- The package cannot see macros/constants from the including file either.
- This isolation enforces modular boundaries – packages must be self-contained.

For sharing definitions across files, use **file includes** (`.include "file.vas"`), which process everything in the same context.

Cross-context deduplication still applies: the same package or file is never processed twice, even if included from multiple locations.

### Labels

Lines ending with `:` that are not known pseudo-instructions pass through as labels with virtual register substitution:

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

### Error Handling

Errors include source context when reading from a file:

```
error at line 3:
  MOVI v99, 42
  ^
line 3: "MOVI v99, 42": virtual register v99 out of range (valid: v0-v12)
```

- **Virtual register out of range**: reports the invalid name and valid range (v0-v12)
- **Operand count mismatch**: reports the original line and expected count
- **Input file not found**: reports the path
- **Unknown instruction**: passes through silently (only virtual register substitution applied)

## Command Line Usage

```bash
vas                           # Read from stdin, output to stdout
vas input.vas                 # Translate input.vas, output to stdout
vas -o output.asm input.vas   # Write to file
vas input.vas -o output.asm   # Same as above
vas -target win64 input.vas   # Output Windows x64 skeleton
vas -O1 input.vas             # Enable -O1 optimizations
vas -O2 input.vas             # Enable -O2 optimizations (includes -O1)
vas diff input.vas            # Show VAS source vs NASM output
vas stats input.vas           # Show instruction and register statistics
vas check input.vas           # Validate syntax (exit: 0=ok, 1=error)
vas check --strict input.vas  # Also fail on dangerous instruction patterns
vas list                      # List all instructions and syntax
vas version                   # Print version
```

Options:
- `-o <file>`         - Write output to file instead of stdout
- `-target <arch>`    - Target platform: `elf64` (default) or `win64`
- `-O1`               - Enable optimizations (constant folding, dead code elimination, peephole)
- `-O2`               - Enable -O2 optimizations (LICM, CSE, redundant load elimination, PUSH/POP elimination, tail call)
- `-v`, `--version`   - Print version and exit
- `-h`, `--help`      - Show help
- `--strict`          - In check mode, treat lint errors as failures

### Prep – View Preprocessed Output

`vas prep` resolves all preprocessor directives (includes, macros, constants, conditionals) and outputs the fully expanded source. This is useful for debugging complex include chains or macro expansions. The same preprocessing step is performed automatically by `vas build` before assembly, so you don't need to prep separately unless you want to inspect the intermediate result.

Example:
```bash
vas prep app.vas
vas prep -v app.vas   # show statistics
```

## Standalone Mode

When the assembled output does not contain a `section .text` directive, VAS automatically wraps it in a minimal standalone skeleton that can be assembled and run directly. If the input already defines its own `.text` section, the skeleton is skipped.

### ELF64 (Linux / WSL, default)

```bash
echo "MOVI v0, 42" | vas
```

Output includes: `default rel`, `section .text`, `global _start`, `_start:` entry that calls `vas_main` and then performs `exit(eax)` via syscall. User code is placed under `vas_main:`.

### Win64 (Windows)

```bash
echo "MOVI v0, 42" | vas -target win64
```

Uses `main:` as the entry point. Ends with `xor eax, eax; ret` unless the user's last instruction is already `RET`.

### Skip Standalone Mode

If the assembled output already defines a `.text` section, output is passed through as-is without wrapping.

## Optimization (-O1)

`-O1` enables:

1. **Constant Folding**: Computes literal arithmetic at compile time. `ADD v1, 1, 2` becomes `MOVI v1, 3`.
2. **Dead Code Elimination**: Removes register writes that are never read before being overwritten.
3. **Peephole Optimizations**:
   - `mov reg, 0` -> `xor reg, reg` (smaller encoding)
   - `cmp reg, 0` -> `test reg, reg` (smaller encoding)
   - Multi-nop sequences merged into one
   - `mov + add` fused into `lea`

## Optimization (-O2)

`-O2` includes all `-O1` passes plus:

1. **Common Subexpression Elimination (CSE)**: Repeats of (op, arg1, arg2) replaced with MOV from the first result.
2. **Loop Invariant Code Motion (LICM)**: LEA with label operand hoisted before loop header.
3. **Redundant Load Elimination**: LOAD from same address replaced with MOV from previous load.
4. **PUSH/POP Elimination**: Balanced push/pop pairs removed when the register is unmodified.
5. **Tail Call Optimization**: `CALL label; RET` -> `JMP label`.

## Formal Verification

VAS's optimization passes have been formally verified via exhaustive enumeration + SMT solver (Z3) for window size W=2, confirming that all hand-written optimizations are sound and no valid optimization within the window is missed. Users are welcome to run their own verification using the open-source toolchain.

## Examples

| File | Description | Instructions Used | Complexity |
|------|-------------|-------------------|------------|
| `hello.vas` | Linux write syscall | MOVI, LEA, SYSCALL | ★☆☆ |
| `calc.vas` | Sum 1..n arithmetic | MOVI, ADD, CMP, JLE, MOV, SYSCALL | ★☆☆ |
| `fib.vas` | Iterative Fibonacci | MOVI, MOV, ADD, CMP, JGE, JMP | ★★☆ |
| `fact.vas` | Recursive factorial | CALL, RET, PUSH, POP | ★★☆ |
| `sort.vas` | Bubble sort of 8 elements | LOAD, STORE, CMP, JMP, PUSH, POP | ★★☆ |
| `greet.vas` | CLI tool with args | POP, CMP, STORE, LOAD, SYSCALL | ★★☆ |
| `win-ops.vas` | Win64 arithmetic chain | ADD, MUL, SUB, RET | ★☆☆ |
| `win-edge.vas` | Win64 edge cases | PUSH, POP, STORE, LOAD, CMP, JE, RET | ★★☆ |
| `multitool.vas` | Multi-function demo | strlen, Fibonacci, prime, factorial | ★★★ |

Build and run Linux examples:
```bash
vas examples/fib.vas -o fib.asm && nasm -f elf64 fib.asm -o fib.o && ld fib.o -o fib && ./fib; echo $?
```

Build and run Windows examples:
```bash
vas -target win64 examples/win-ops.vas -o win-ops.asm
nasm -f win64 win-ops.asm -o win-ops.obj
ld -e main -o win-ops.exe win-ops.obj
win-ops.exe
```

## Installation and Build

**Prerequisites**: Go 1.21+, no third-party dependencies.

```bash
# Clone
git clone https://github.com/0xA672/Vas.git
cd vas

# Build (dev version prints "vas dev")
go build -o vas.exe .

# Build with version string
go build -ldflags "-X main.Version=v0.2.0" -o vas.exe .

# Install to $GOPATH/bin
go install
```

`vas version` and `vas -v` print the embedded version string.

## Project Structure

```
vas/
+-- main.go                  # CLI entry, argument parsing, subcommands
+-- go.mod                   # Go module
+-- vas/
|   +-- core.go              # Core translation: scan -> expand -> wrap (includes regMap)
|   +-- opt/
|       +-- opt.go           # -O1 optimizer (DCE + constant folding + peephole)
+-- test/
|   +-- assembler_test.go    # Unit tests
+-- examples/                # Example .vas files
+-- bin/                     # Build artifacts (gitignored)
+-- README.md
+-- LICENSE
```

## Distinction from a Real Assembler

VAS explicitly does not perform:
- Register allocation or instruction scheduling
- Instruction selection or optimization (beyond simple -O1)
- Linking or relocation

Generated `.asm` files must be assembled by NASM and linked by ld to produce an executable. VAS is a thin translation layer; NASM handles the rest.

VAS is designed for learning, prototyping, and small utilities. For production code or performance-critical sections, consider writing NASM directly or using a higher-level language with inline assembly.

## License

MIT - see [LICENSE](LICENSE) file.
