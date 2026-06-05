# VAS - Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.asm)  ->  nasm + ld / gcc  ->  executable
```

VAS (Virtual Assembler) is a lightweight text-replacement translator. It reads pseudo-instructions that use **virtual registers** (v0-v12) and outputs standard **x86-64 NASM** assembly.

It does not perform register allocation, instruction scheduling, or linking. Its sole purpose is to turn educational/prototype pseudo-code into NASM-assemblable source.

---

## Quick Start

### 1. Write Pseudocode (`hello.vas`)

```asm
; hello.vas - print "hello world" via Linux write syscall
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
vas -o hello.asm hello.vas
```

Generated `hello.asm`:

```asm
default rel

        section .text
        global _start
_start:
        call    vas_main
        mov     edi, eax
        mov     eax, 60
        syscall
vas_main:
        mov     rax, 1
        mov     rdi, 1
        lea     rsi, [msg]
        mov     rdx, 12
        syscall
        mov     rax, 60
        mov     rdi, 0
        syscall

        section .data
msg: db "hello world", 10
```

### 3. Build and Run

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
| v8               | `r11`             |
| v9               | `r12`             |
| v10              | `r13`             |
| v11              | `r14`             |
| v12              | `r15`             |

Virtual registers can be used in any operand position, including memory addressing (e.g. `[v0+8]`, `[v1+v2*8]`), and are automatically replaced during translation.

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

Any line not recognized as a pseudo-instruction passes through with virtual registers substituted. This lets you use raw x86 instructions directly:

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

---

## Command Line Usage

```bash
vas                         # Read from stdin, output to stdout
vas input.vas               # Translate input.vas, output to stdout
vas -o output.asm input.vas # Write to file
vas input.vas -o output.asm # Same as above
vas -target win64 input.vas # Output Windows x64 skeleton
vas -O1 input.vas           # Enable optimizations
vas diff input.vas          # Show VAS source vs NASM output
vas stats input.vas         # Show instruction and register statistics
vas check input.vas         # Validate syntax (exit: 0=ok, 1=error)
vas list                    # List all instructions and syntax
vas version                 # Print version
```

Options:
- `-o <file>`         - Write output to file instead of stdout
- `-target <arch>`    - Target platform: `elf64` (default) or `win64`
- `-O1`               - Enable optimizations (constant folding, dead code elimination, peephole)
- `-v`, `--version`   - Print version and exit
- `-h`, `--help`      - Show help

---

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

---

## Optimization (-O1)

`-O1` enables:

1. **Constant Folding**: Computes literal arithmetic at compile time. `ADD v1, 1, 2` becomes `MOVI v1, 3`.
2. **Dead Code Elimination**: Removes register writes that are never read before being overwritten.
3. **Peephole Optimizations**:
   - `mov reg, 0` -> `xor reg, reg` (smaller encoding)
   - `cmp reg, 0` -> `test reg, reg` (smaller encoding)
   - Multi-nop sequences merged into one
   - `mov + add` fused into `lea`

---

## Optimization (-O2)

`-O2` includes all `-O1` passes plus:

1. **Common Subexpression Elimination (CSE)**: Repeats of (op, arg1, arg2) replaced with MOV from the first result.
2. **Loop Invariant Code Motion (LICM)**: LEA with label operand hoisted before loop header.
3. **Redundant Load Elimination**: LOAD from same address replaced with MOV from previous load.
4. **PUSH/POP Elimination**: Balanced push/pop pairs removed when the register is unmodified.
5. **Tail Call Optimization**: `CALL label; RET` -> `JMP label`.

## Formal Verification

VAS's optimization passes have been formally verified using **ExaPO**, an exhaustive enumeration + SMT solver (Z3) based verifier. For window size W=2, ExaPO enumerated **47,931 candidate instruction sequences** from a 20-instruction pool, verified each against the original using Z3's BitVec 64 theory, and confirmed:

- **All 17 hand-written optimizations in VAS are sound** and no valid optimization within the W=2 window is missed.
- **2 instruction-saving rules** were independently rediscovered (`MOV + ADD → LEA`), matching VAS's existing `leaFuse` pass.
- **459 length-preserving equivalences** (register reorderings, commutative variants) were exhaustively enumerated and verified.

This provides **window-level completeness**: for any straight-line code of ≤2 instructions, VAS's optimizer does not miss any valid optimization.

The verification toolchain (ExaPO + x7a7) is currently closed-source and will be open-sourced after paper publication.

---
## Syntax Details

### Comments

Both `;` and `#` start inline comments. Quoted strings preserve `;` and `#` as literals.

```asm
MOVI v0, 42   ; this is a comment
ADD  v1, v0   # this is also a comment
```

### Labels

Lines ending with `:` that are not known pseudo-instructions pass through as labels with virtual register substitution:

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

---

## Error Handling

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

---

## Examples

| File | Description | Instructions Used |
|------|-------------|-------------------|
| `hello.vas` | Linux write syscall | MOVI, LEA, SYSCALL |
| `calc.vas` | Sum 1..n arithmetic | MOVI, ADD, CMP, JLE, MOV, SYSCALL |
| `fib.vas` | Iterative Fibonacci | MOVI, MOV, ADD, CMP, JGE, JMP |
| `fact.vas` | Recursive factorial | CALL, RET, PUSH, POP |
| `sort.vas` | Bubble sort of 8 elements | LOAD, STORE, CMP, JMP, PUSH, POP |
| `greet.vas` | CLI tool with args | POP, CMP, STORE, LOAD, SYSCALL |
| `win-ops.vas` | Win64 arithmetic chain | ADD, MUL, SUB, RET |
| `win-edge.vas` | Win64 edge cases | PUSH, POP, STORE, LOAD, CMP, JE, RET |
| `multitool.vas` | Multi-function demo | strlen, Fibonacci, prime, factorial |

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

---

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

---

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

---

## Distinction from a Real Assembler

VAS explicitly does not perform:
- Register allocation or instruction scheduling
- Instruction selection or optimization (beyond simple -O1)
- Linking or relocation

Generated `.asm` files must be assembled by NASM and linked by ld to produce an executable. VAS is a thin translation layer; NASM handles the rest.

---

## License

MIT - see [LICENSE](LICENSE) file.
