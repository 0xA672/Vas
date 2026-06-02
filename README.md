# VAS — Virtual Assembler

```
.vas pseudocode  ->  VAS  ->  x86-64 NASM assembly (.s / .asm)  ->  nasm + ld / gcc  ->  executable
```

**VAS** (Virtual Assembler) 是一个轻量级文本替换翻译工具。它读取使用**虚拟寄存器** (v0–v7) 的伪指令，输出标准的 **x86-64 NASM** 汇编。

它不进行寄存器分配、指令调度或链接——其唯一目的是将教学/原型伪代码转化为 NASM 可汇编的源代码。

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

虚拟寄存器可在任何操作数位置使用，包括内存寻址（如 `[v0+8]`），翻译时自动替换。

---

## Supported Pseudo-Instructions

### Arithmetic Instructions

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `ADD` | `dst, src1, src2` | `mov dst, src1` then `add dst, src2` | 三操作数加法 |
| `ADD` | `dst, src` | `add dst, src` | 二操作数加法 |
| `SUB` | `dst, src1, src2` | `mov dst, src1` then `sub dst, src2` | 三操作数减法 |
| `SUB` | `dst, src` | `sub dst, src` | 二操作数减法 |
| `MUL` | `dst, src1, src2` | `mov dst, src1` then `imul dst, src2` | 三操作数乘法 |
| `MUL` | `dst, src` | `imul dst, src` | 二操作数乘法 |

### Memory Access

| Pseudo-instruction | Operands | Expansion | Notes |
|--------------------|----------|-----------|-------|
| `MOVI` | `dst, imm` | `mov dst, imm` | 加载立即数 |
| `MOV` | `dst, src` | `mov dst, src` | 寄存器间传送 |
| `LOAD` | `dst, [addr]` | `mov dst, [addr]` | 从内存加载 |
| `STORE` | `src, [addr]` | `mov [addr], src` | 存入内存 |
| `LEA` | `dst, [addr]` | `lea dst, [addr]` | 取有效地址 |

地址表达式（如 `[v1]`、`[v0+8]`、`[label]`）透传，仅替换虚拟寄存器。

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
vas                         # 从 stdin 读取，输出到 stdout
vas input.vas               # 翻译 input.vas，输出到 stdout
vas -o output.s input.vas   # 写入文件（-o 可在输入前后）
vas input.vas -o output.s   # 同上
vas -target win64 input.vas # 输出 Windows x64 骨架取代默认 ELF64
vas -O1 input.vas           # 启用优化（死代码消除 + 常量折叠）
vas -h / --help             # 显示帮助
```

- **管道输入**: `echo "MOVI v0, 42" | vas`
- 输入文件不存在时报错退出
- 无输入且 stdin 为空时打印帮助信息并退出

---

## Standalone Mode (独立模式)

当输入**不包含**任何 `section` / `global` / `extern` 样板代码时（即纯虚拟寄存器伪代码），VAS 自动包裹输出为可独立汇编运行的最小骨架。

### ELF64（Linux / WSL，默认）

```bash
echo "MOVI v0, 42" | vas
```

输出：

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

骨架包含：
- `default rel` + `section .text` + `global _start` / `_start:` → `call vas_main` → syscall exit
- 用户代码以 `vas_main:` 包裹，末尾 `ret` 返回后自动执行 `exit(eax)` syscall
- 自动添加 `.data` 段（含用户数据定义则跳过）

### Win64（Windows）

```bash
echo "MOVI v0, 42" | vas -target win64
```

骨架使用 `main:` 入口点，末尾 `xor eax, eax; ret` 退出（除非用户最后一条指令已是 `RET`）。

### 跳过独立模式

如果输入已包含 `section`、`global` 或 `extern`，则按原样输出，不添加任何骨架（`-target` 仍有效用于寄存器映射风格）。

---

## 优化 (-O1)

`-O1` 启用两级优化：

1. **死代码消除 (DCE)**：删除对后续无影响的寄存器赋值（例如 `MOV v1, v0` 后 v1 被覆盖前从未使用）
2. **常量折叠**：字面量运算在编译期计算（如 `MOVI v1, 3; ADD v0, v1, 7` → `mov rax, 10`）

DCE 正确保留有副作用的指令（PUSH、POP、CALL、STORE、LOAD、SYSCALL、INT、RET 等）。

---

## Syntax Details

### 注释
`;` 和 `#` 均可开始行内注释（不会与字符串字面量内的分隔符混淆）：

```asm
MOVI v0, 42   ; 这是一条注释
ADD  v1, v0   # 这也是注释
```

### 标签和定义
以冒号结尾且非已知指令的行透传输出，仅替换虚拟寄存器：

```asm
section .data
result: dq 0

section .text
global _start
_start:
```

### 透传 (Passthrough)
任何非已知伪指令的行（数据定义、段指令、对齐等）原样输出，仅替换虚拟寄存器。
可识别数据/段关键字：`SECTION`、`GLOBAL`、`EXTERN`、`DQ`、`DB`、`resb`、`equ` 等。

**GAS → NASM 点前缀剥离**：`.section` → `section`，`.global` → `global`，`.text` → `text`，`.globl` → `global`：

```asm
; 输入 (GAS 风格):
.section .data
msg: .asciz "hello"

.section .text
.globl _start

; 输出 (NASM 风格):
section .data
msg: db "hello", 0

section .text
global _start
```

---

## Error Handling

- **已知指令操作数数量不匹配**：报错显示行号和原文，退出
- **输入文件不存在**：立即报错退出
- **未知指令**：透传（仅替换虚拟寄存器），不报错

---

## Examples

项目自带多个实战示例，涵盖各种功能场景：

| 文件 | 功能 | 测试指令 |
|------|------|----------|
| `hello.vas` | Linux write syscall 打印 | MOVI, LEA, SYSCALL |
| `calc.vas` | 算术运算链 | ADD, SUB, MUL |
| `fib.vas` | 迭代斐波那契 F(20) | CMP, JLE, JMP, LOOP |
| `fact.vas` | 递归阶乘 fact(5) | CALL, RET, PUSH, POP |
| `sort.vas` | 冒泡排序 8 元素 | LOAD, STORE, 嵌套循环, LEA |
| `greet.vas` | Linux syscall 字符串输出 | .data 段, SYSCALL |
| `win-ret42.vas` | Win64 返回 42 | MOVI, RET |
| `win-ops.vas` | Win64 运算流水线 | ADD, MUL, SUB, Win64 |
| `win-edge.vas` | Win64 边界测试 | PUSH/POP/STORE/LOAD/CMP/JE, .data |

在 Linux/WSL 上运行 ELF 示例：
```bash
vas fib.vas -o fib.s && nasm -f elf64 fib.s -o fib.o && ld fib.o -o fib && ./fib; echo $?
```

在 Windows 上运行 Win64 示例：
```bash
vas -target win64 win-ops.vas -o win-ops.asm
nasm -f win64 win-ops.asm -o win-ops.obj
ld -e main -o win-ops.exe win-ops.obj
win-ops.exe
```

---

## Installation and Build

**Prerequisites**: Go 1.21+, 无第三方依赖。

```bash
# 克隆
git clone https://github.com/0xA672/Vas.git
cd vas

# 构建
go build -o bin\vas.exe main.go

# 或安装到 $GOPATH/bin
go install
```

---

## Project Structure

```
vas/
├── main.go                  # CLI 入口，参数解析
├── go.mod                   # Go module
├── vas/
│   ├── core.go              # 核心翻译逻辑：扫描 → 展开 → 包装
│   └── arch/
│       └── reg.go           # 寄存器映射表
│   └── opt/
│       └── opt.go           # -O1 优化器（DCE + 常量折叠）
├── test/
│   └── assembler_test.go    # 单元测试（26 项）
├── bin/                     # 构建产物（gitignored）
├── hello.vas                # 入门示例
├── calc.vas                 # 算术示例
├── fib.vas                  # 斐波那契示例
├── fact.vas                 # 递归阶乘示例
├── sort.vas                 # 冒泡排序示例
├── greet.vas                # Linux syscall 示例
├── win-ret42.vas            # Win64 最小示例
├── win-ops.vas              # Win64 运算示例
├── win-edge.vas             # Win64 边界测试
├── README.md
└── LICENSE
```

---

## Distinction from a Real Assembler

VAS **明确不执行**以下任务，不应与 GCC、LLVM 或真实汇编器比较：

- ❌ 无寄存器分配 / 指令调度
- ❌ 无指令选择或优化（除简单的 -O1 外）
- ❌ 无链接或重定位
- ❌ 生成的 `.s` / `.asm` 文件**必须**由 NASM 汇编 + ld 链接才能运行

它只是一个薄翻译层，让你能用更友好的伪指令编写原型，其余工作交给 NASM。

---

## License

MIT — 参见 [LICENSE](LICENSE) 文件。
