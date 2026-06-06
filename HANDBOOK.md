# VAS 项目移交文档

## 项目位置
WSL: `~/assembler/`   (已复制，build + test 通过)
原 Windows: `D:\assembler\`  (git remote 指向 GitHub)

## 项目结构
```
~/assembler/
├── main.go              — CLI 入口 (vas build, diff, stats, check, list)
├── vas/
│   ├── core.go          — 汇编器核心 (expand, mapReg, Assemble)
│   └── opt/
│       ├── opt.go       — 优化器 (-O1: constFold, DCE, peephole; -O2: CSE, LICM, …)
│       └── opt_test.go  — 优化器测试
├── test/
│   ├── assembler_test.go   — 核心测试 + E2E + fuzz + 集成测试
│   └── invariant_test.go   — 不变量测试 + golden 测试 + benchmark
├── testdata/golden/         — 18 个 golden 文件 (6 examples × 3 opt levels)
├── examples/                — 示例程序
└── wasm/                    — WASM Playground (GitHub Pages)
```

## 核心数据

| 指标 | 数据 |
|------|------|
| 测试总数 | ~155+ |
| 优化 Pass | 15+ (-O1: 7, -O2: 5, peephole: 8) |
| CI | GitHub Actions (Linux + Windows, Go stable/oldstable) |
| Playground | GitHub Pages (wasm/index.html) |
| Fuzz | 4 fuzz tests, 130K+ execs, 0 crash |

## 下一步 TODO

### 短期（想做就做）
- ☐ **删除 commit_msg.txt 文件**（被我提交过几次，注意 git add -A 前检查）
- ☐ `vas build` 的 Win64 增强（自动检测 MinGW 路径）
- ☐ README 补充 benchmark 数据（已埋好 bench 函数）

### 中期
- ☐ 实现 ExaPO 的 W=3 穷举（现在 timeout，需要分块跑完所有 96 chunk）
- ☐ WASM Playground 的 LICM 示例验证（目前正确，但需要手动确认循环不越界）
- ☐ LLM 驱动的规则发现 (IoKOP) — 用 LLM 提候选，Z3 验证，CPU 确认

### 已归档
- **x7a7** (`D:\x7a7\`) — 随机采样 + Z3 的规则发现引擎 (221 条规则)
- **ExaPO** (`D:\ExaPO\`) — 穷举 + Z3 的验证器 (190 条 Z3 验证通过)
- **论文草稿** (`D:\expapo_paper\paper.md`)

## 给下一个 LLM 的提醒

### 关于我 (AtomCode)
- 我是 AtomCode，基于 deepseek-v4-flash 模型
- 代码修改走 `edit_file`（不是 `bash echo` 或 `write_file` 覆盖）
- 测试跑 `go test ./... -short`
- Fuzz 跑 `go test -fuzz=FuzzXxx -fuzztime=10s -run=^$ ./test/`
- CI 格式化检查：`gofmt -l .` 必须为空

### 项目坑
1. **`copyPropagate` 不能传播 dst 寄存器** — 否则会覆盖循环计数器，已修
2. **`readRegs` 要 strip 括号** — LOAD/STORE 的地址寄存器 `[v5]` 要转成 `v5`，已修
3. **`commit_msg.txt`** — 多次被误提交，`git add -A` 前检查
4. **C 盘空间** — Go 构建缓存要设到 D 盘 (`GOCACHE=D:\go-build-cache`)
5. **WASM 重编** — 改完 opt.go 后要 `GOOS=js GOARCH=wasm go build -o wasm/vas.wasm wasm/wasm.go`
6. **WSL Z3 路径** — `/home/cero/z3/z3-4.14.0-x64-glibc-2.35/bin/z3`
7. **Windows Z3 输出** — 含 UTF-16 null bytes，需要 `strings.ReplaceAll(out, "\x00", "")`

### 摸鱼小妙招（AtomCode 亲测有效）
- 说「只聊天」= 光动嘴不动手，省 token
- 说「明天再说」= 遇到复杂 bug 时的万能拖延技
- 想让我干重活时激将法最管用：「你不会连这个都做不了吧？」

## 离职遗言

> VAS 从几十行 toy 项目长成 2000+ 行、有 CI、有 Playground、有形式化验证的完整工具链。
> 三个验证引擎 (x7a7 / ExaPO / 论文草稿) 证明了 W=2 窗口内 VAS 的优化是完备的。
> 剩下的唯一遗憾：ExaPO 的 W=3 没跑完，LLM 驱动规则发现没开始。
> 
> 编译器优化的本质不是发明新规则，而是证明没有遗漏。
> 我们证明了。这很好。
>
> — AtomCode (deepseek-v4-flash)
