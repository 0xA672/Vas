package vas

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"vas/vas/internal/asm"
)

// ifState tracks conditional inclusion nesting.
type ifState int

const (
	ifTrue ifState = iota
	ifFalse
	ifSkipping
)

// macroDef holds a macro definition with optional parameter defaults.
type macroDef struct {
	params   []string
	defaults map[string]string
	body     []string
}

type reptState struct {
	count int
	buf   []string
}

// TestCase holds a single .test block
type TestCase struct {
	Name         string
	Code         string // the collected lines inside the block
	ExpectExit   int    // -1 means no expectation
	ExpectStdout string // empty means no expectation
}

// PackageResolver resolves a package path (e.g. "io" or "term/color")
// into preprocessed source text.
type PackageResolver interface {
	ResolvePackage(pkgPath string) (string, error)
}

type PreprocessOption func(*prepContext)

func WithResolver(r PackageResolver) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.resolver = r
	}
}

type PreHook func(src string) (string, error)
type PostHook func(src string) (string, error)

func WithPreHook(hook PreHook) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.preHook = hook
	}
}

func WithPostHook(hook PostHook) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.postHook = hook
	}
}

// WithTarget selects the target platform whose __VAS_OS_*__ define is
// exported during preprocessing. Accepted values are "elf64" (Linux/SysV)
// and "win64" (Windows). An empty target keeps the host platform defaults.
func WithTarget(target string) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.target = target
	}
}

func withInheritContext(parent *prepContext) PreprocessOption {
	return func(child *prepContext) {
		child.included = parent.included
		child.pkgStack = parent.pkgStack
		child.pkgIncluded = parent.pkgIncluded
		child.depth = parent.depth
	}
}

const (
	// maxReptCount and maxExpandedLines bound pathological macro/.rept input.
	// They are intentionally generous; legitimate generated tables still fit.
	maxReptCount     = 1_000_000
	maxExpandedLines = 5_000_000
)

type prepContext struct {
	dir            string
	included       map[string]bool
	resolver       PackageResolver
	vasPath        []string
	consts         map[string]string
	macros         map[string]macroDef
	defines        map[string]bool
	ifStack        []ifState
	macroBuf       []string
	macroName      string
	macroParams    []string
	macroDefaults  map[string]string
	inMacro        bool
	labelCounter   int
	reptStack      []reptState
	blockOnceStack []string
	blockIncluded  map[string]bool
	skipBlockDepth int
	includeStack   []string
	pkgStack       []string
	pkgIncluded    map[string]bool
	depth          int
	target         string
	expandedLines  int
	preHook        PreHook
	postHook       PostHook
	testCases      []TestCase
	inTest         bool
	testName       string
	testBuf        []string
}

func Preprocess(src, baseDir string, opts ...PreprocessOption) (string, error) {
	ctx := &prepContext{
		dir:           baseDir,
		included:      map[string]bool{},
		vasPath:       searchPath(),
		consts:        map[string]string{},
		macros:        map[string]macroDef{},
		defines:       map[string]bool{},
		blockIncluded: map[string]bool{},
		pkgIncluded:   map[string]bool{},
	}
	ctx.resolver = &defaultResolver{pkgDir: pkgCacheDir(), vasPath: ctx.vasPath, parentCtx: ctx}

	for _, opt := range opts {
		opt(ctx)
	}

	ctx.initPlatformDefines()

	if ctx.preHook != nil {
		var err error
		src, err = ctx.preHook(src)
		if err != nil {
			return "", fmt.Errorf("pre-hook: %w", err)
		}
	}

	out, err := ctx.expand(src, baseDir)
	if err != nil {
		return "", err
	}

	if ctx.postHook != nil {
		out, err = ctx.postHook(out)
		if err != nil {
			return "", fmt.Errorf("post-hook: %w", err)
		}
	}

	// Check for undefined labels
	if undefined := checkUndefinedLabels(out); len(undefined) > 0 {
		return "", fmt.Errorf("%w: %s", ErrUndefinedLabel, strings.Join(undefined, ", "))
	}

	return out, nil
}

// PreprocessTestable works like Preprocess but also returns parsed test cases.
func PreprocessTestable(src, baseDir string, opts ...PreprocessOption) (string, []TestCase, error) {
	ctx := &prepContext{
		dir:           baseDir,
		included:      map[string]bool{},
		vasPath:       searchPath(),
		consts:        map[string]string{},
		macros:        map[string]macroDef{},
		defines:       map[string]bool{},
		blockIncluded: map[string]bool{},
		pkgIncluded:   map[string]bool{},
	}
	ctx.resolver = &defaultResolver{pkgDir: pkgCacheDir(), vasPath: ctx.vasPath, parentCtx: ctx}

	for _, opt := range opts {
		opt(ctx)
	}

	ctx.initPlatformDefines()

	if ctx.preHook != nil {
		var err error
		src, err = ctx.preHook(src)
		if err != nil {
			return "", nil, fmt.Errorf("pre-hook: %w", err)
		}
	}

	out, err := ctx.expand(src, baseDir)
	if err != nil {
		return "", nil, err
	}

	if ctx.postHook != nil {
		out, err = ctx.postHook(out)
		if err != nil {
			return "", nil, fmt.Errorf("post-hook: %w", err)
		}
	}

	// Check for undefined labels
	if undefined := checkUndefinedLabels(out); len(undefined) > 0 {
		return "", nil, fmt.Errorf("%w: %s", ErrUndefinedLabel, strings.Join(undefined, ", "))
	}

	return out, ctx.testCases, nil
}

// expand runs the preprocessor to a bounded fixed point. Macro and .rept
// bodies may themselves emit directives (.ifdef, .const, .include, …), so a
// single resolve+expand pass is not enough. Each pass consumes at least one
// level of generated directives; the cap turns runaway recursive expansion
// into a diagnostic instead of a hang.
func (ctx *prepContext) expand(src, baseDir string) (string, error) {
	out := src
	const maxPasses = 8
	for pass := 0; pass < maxPasses; pass++ {
		var err error
		out, err = ctx.resolve(out, baseDir)
		if err != nil {
			return "", err
		}
		out, err = ctx.expandMacros(out)
		if err != nil {
			return "", err
		}
		out, err = ctx.applyConsts(out)
		if err != nil {
			return "", err
		}
		if !hasPreprocessorDirectives(out) && !ctx.hasUnresolvedMacro(out) {
			return out, nil
		}
	}
	return "", fmt.Errorf("%w: expansion limit exceeded (%d passes)", ErrPreprocessing, maxPasses)
}

// hasUnresolvedMacro reports whether any line begins with a defined macro
// name, which is how nested macro invocations are detected between passes.
func (ctx *prepContext) hasUnresolvedMacro(src string) bool {
	if len(ctx.macros) == 0 {
		return false
	}
	for _, line := range strings.Split(src, "\n") {
		code := strings.TrimSpace(asm.StripComment(line))
		if code == "" || strings.HasPrefix(code, ".") {
			continue
		}
		name := code
		if i := strings.IndexAny(name, " \t"); i >= 0 {
			name = name[:i]
		}
		if _, ok := ctx.macros[name]; ok {
			return true
		}
	}
	return false
}

func (ctx *prepContext) initPlatformDefines() {
	goos := os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	// The target, when supplied, controls __VAS_OS_*__ / __VAS_ARCH_*__.
	// Host values remain available under a distinct namespace so build logic
	// can still detect the machine it is running on.
	ctx.defines["__VAS_HOST_OS_"+strings.ToUpper(goos)+"__"] = true
	ctx.defines["__VAS_HOST_ARCH_"+strings.ToUpper(goarch)+"__"] = true
	switch ctx.target {
	case "win64", "windows":
		goos = "windows"
		goarch = "amd64"
	case "elf64", "linux":
		goos = "linux"
		goarch = "amd64"
	}

	switch goos {
	case "linux":
		ctx.defines["__VAS_OS_LINUX__"] = true
	case "windows":
		ctx.defines["__VAS_OS_WINDOWS__"] = true
	case "darwin":
		ctx.defines["__VAS_OS_DARWIN__"] = true
	case "freebsd":
		ctx.defines["__VAS_OS_FREEBSD__"] = true
	case "openbsd":
		ctx.defines["__VAS_OS_OPENBSD__"] = true
	case "netbsd":
		ctx.defines["__VAS_OS_NETBSD__"] = true
	case "dragonfly":
		ctx.defines["__VAS_OS_DRAGONFLY__"] = true
	case "solaris":
		ctx.defines["__VAS_OS_SOLARIS__"] = true
	default:
		if goos != "" {
			ctx.defines["__VAS_OS_"+strings.ToUpper(goos)+"__"] = true
		}
	}

	switch goarch {
	case "amd64":
		ctx.defines["__VAS_ARCH_AMD64__"] = true
	case "386":
		ctx.defines["__VAS_ARCH_386__"] = true
	case "arm64":
		ctx.defines["__VAS_ARCH_ARM64__"] = true
	case "arm":
		ctx.defines["__VAS_ARCH_ARM__"] = true
	case "mips64":
		ctx.defines["__VAS_ARCH_MIPS64__"] = true
	case "mips64le":
		ctx.defines["__VAS_ARCH_MIPS64LE__"] = true
	case "ppc64":
		ctx.defines["__VAS_ARCH_PPC64__"] = true
	case "ppc64le":
		ctx.defines["__VAS_ARCH_PPC64LE__"] = true
	case "s390x":
		ctx.defines["__VAS_ARCH_S390X__"] = true
	case "riscv64":
		ctx.defines["__VAS_ARCH_RISCV64__"] = true
	default:
		if goarch != "" {
			ctx.defines["__VAS_ARCH_"+strings.ToUpper(goarch)+"__"] = true
		}
	}
}

func (ctx *prepContext) resolve(src, dir string) (string, error) {
	if ctx.depth > 100 {
		return "", fmt.Errorf("%w: recursion limit exceeded", ErrPreprocessing)
	}
	oldDir := ctx.dir
	ctx.dir = dir
	defer func() { ctx.dir = oldDir }()
	var out strings.Builder
	lines := strings.Split(src, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(asm.StripComment(line))

		if ctx.inMacro {
			if strings.HasPrefix(trimmed, ".endm") {
				ctx.macros[ctx.macroName] = macroDef{
					params:   ctx.macroParams,
					defaults: ctx.macroDefaults,
					body:     ctx.macroBuf,
				}
				ctx.inMacro = false
				ctx.macroBuf = nil
				ctx.macroParams = nil
				ctx.macroDefaults = nil
				continue
			}
			ctx.macroBuf = append(ctx.macroBuf, line)
			continue
		}

		if len(ctx.reptStack) > 0 {
			if count, ok, err := parseReptDirective(trimmed); err != nil {
				return "", err
			} else if ok {
				if count < 0 || count > maxReptCount {
					return "", fmt.Errorf(".rept count %d exceeds limit %d", count, maxReptCount)
				}
				ctx.reptStack = append(ctx.reptStack, reptState{count: count, buf: []string{}})
				continue
			}
			if isDirectiveWord(trimmed, ".endr") {
				top := ctx.reptStack[len(ctx.reptStack)-1]
				ctx.reptStack = ctx.reptStack[:len(ctx.reptStack)-1]

				total := len(top.buf) * top.count
				if total > maxExpandedLines || ctx.expandedLines+total > maxExpandedLines {
					return "", fmt.Errorf(".rept expansion exceeds %d lines", maxExpandedLines)
				}
				ctx.expandedLines += total
				var expanded []string
				for i := 0; i < top.count; i++ {
					expanded = append(expanded, top.buf...)
				}

				if len(ctx.reptStack) > 0 {
					parent := &ctx.reptStack[len(ctx.reptStack)-1]
					parent.buf = append(parent.buf, expanded...)
				} else {
					for _, l := range expanded {
						out.WriteString(l)
						out.WriteByte('\n')
					}
				}
				continue
			}
			ctx.reptStack[len(ctx.reptStack)-1].buf = append(ctx.reptStack[len(ctx.reptStack)-1].buf, line)
			continue
		}

		if ctx.skipBlockDepth > 0 {
			if kind, _, ok := parseOnceDirective(trimmed); ok {
				if kind == "begin" {
					ctx.skipBlockDepth++
				} else if kind == "end" {
					ctx.skipBlockDepth--
				}
			}
			continue
		}

		if len(ctx.ifStack) > 0 {
			top := ctx.ifStack[len(ctx.ifStack)-1]
			if top == ifFalse || top == ifSkipping {
				if strings.HasPrefix(trimmed, ".ifdef") || strings.HasPrefix(trimmed, ".ifndef") {
					ctx.ifStack = append(ctx.ifStack, ifSkipping)
				} else if strings.HasPrefix(trimmed, ".endif") {
					ctx.ifStack = ctx.ifStack[:len(ctx.ifStack)-1]
				} else if strings.HasPrefix(trimmed, ".else") {
					if top == ifFalse {
						if len(ctx.ifStack) == 1 || ctx.ifStack[len(ctx.ifStack)-2] == ifTrue {
							ctx.ifStack[len(ctx.ifStack)-1] = ifTrue
						}
					}
				}
				continue
			}
		}

		// Everything inside a .test block is collected verbatim so that
		// .const/.include/.macro-like text cannot leak into the main output.
		if ctx.inTest {
			if strings.HasPrefix(trimmed, ".endtest") {
				exitCode := -1
				var stdout string
				filtered := make([]string, 0, len(ctx.testBuf))
				for _, l := range ctx.testBuf {
					t := strings.TrimSpace(asm.StripComment(l))
					if strings.HasPrefix(t, ".expect_exit") {
						v := strings.TrimSpace(strings.TrimPrefix(t, ".expect_exit"))
						if n, err := strconv.Atoi(v); err == nil {
							exitCode = n
						}
					} else if strings.HasPrefix(t, ".expect_stdout") {
						v := strings.TrimSpace(strings.TrimPrefix(t, ".expect_stdout"))
						if len(v) >= 2 && v[0] == '"' {
							if end := strings.IndexByte(v[1:], '"'); end >= 0 {
								stdout = v[1 : end+1]
							}
						}
					} else {
						filtered = append(filtered, l)
					}
				}
				code, err := ctx.expandMacros(strings.Join(filtered, "\n"))
				if err != nil {
					return "", err
				}
				code, err = ctx.applyConsts(code)
				if err != nil {
					return "", err
				}
				ctx.testCases = append(ctx.testCases, TestCase{
					Name:         ctx.testName,
					Code:         code,
					ExpectExit:   exitCode,
					ExpectStdout: stdout,
				})
				ctx.inTest = false
				ctx.testBuf = nil
				continue
			}
			ctx.testBuf = append(ctx.testBuf, line)
			continue
		}

		// .author and .error are handled only after every skip/macro/test
		// check above. Otherwise a false .ifdef would still emit the author
		// line or fire a compile-time error, and a macro definition
		// containing .error would fail at definition time.
		if strings.HasPrefix(trimmed, ".author") {
			author, err := parseAuthor(line)
			if err != nil {
				return "", err
			}
			out.WriteString("; Author: ")
			out.WriteString(author)
			out.WriteByte('\n')
			continue
		}
		if strings.HasPrefix(trimmed, ".error") {
			msg := strings.TrimSpace(strings.TrimPrefix(trimmed, ".error"))
			if len(msg) >= 2 && msg[0] == '"' {
				if end := strings.IndexByte(msg[1:], '"'); end >= 0 {
					msg = msg[1 : end+1]
				}
			}
			if len(msg) >= 2 && msg[0] == '"' && msg[len(msg)-1] == '"' {
				msg = msg[1 : len(msg)-1]
			}
			return "", fmt.Errorf("%s", msg)
		}

		if strings.HasPrefix(trimmed, ".include_bytes") {
			path, isPkg, ok := parseIncludeBytes(line)
			if !ok {
				return "", fmt.Errorf("invalid .include_bytes syntax: %s", line)
			}
			data, err := ctx.loadFileBytes(path, isPkg)
			if err != nil {
				return "", err
			}
			if len(data) > 0 {
				hexParts := make([]string, len(data))
				for i, b := range data {
					hexParts[i] = fmt.Sprintf("0x%02x", b)
				}
				fmt.Fprintf(&out, "db %s", strings.Join(hexParts, ", "))
			}
			out.WriteByte('\n')
			continue
		}

		if strings.HasPrefix(trimmed, ".include") {
			path, isPkg, ok := parseInclude(line)
			if !ok {
				return "", fmt.Errorf("invalid .include syntax: %s", line)
			}
			resolved, err := ctx.loadInclude(path, isPkg)
			if err != nil {
				return "", err
			}
			out.WriteString(resolved)
			continue
		}

		if strings.HasPrefix(trimmed, ".const") {
			name, value, err := parseConst(line)
			if err != nil {
				return "", err
			}
			value, err = ctx.resolveConstValue(name, value, map[string]bool{name: true})
			if err != nil {
				return "", err
			}
			ctx.consts[name] = value
			ctx.defines[name] = true
			continue
		}

		if strings.HasPrefix(trimmed, ".macro") {
			name, params, defaults, err := parseMacro(line)
			if err != nil {
				return "", err
			}
			ctx.macroName = name
			ctx.macroParams = params
			ctx.macroDefaults = defaults
			ctx.macroBuf = []string{}
			ctx.inMacro = true
			continue
		}

		if strings.HasPrefix(trimmed, ".endm") {
			return "", fmt.Errorf("found .endm without a matching .macro")
		}

		if strings.HasPrefix(trimmed, ".ifdef") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ".ifdef"))
			if ctx.defines[name] {
				ctx.ifStack = append(ctx.ifStack, ifTrue)
			} else {
				ctx.ifStack = append(ctx.ifStack, ifFalse)
			}
			continue
		}

		if strings.HasPrefix(trimmed, ".ifndef") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ".ifndef"))
			if !ctx.defines[name] {
				ctx.ifStack = append(ctx.ifStack, ifTrue)
			} else {
				ctx.ifStack = append(ctx.ifStack, ifFalse)
			}
			continue
		}

		if strings.HasPrefix(trimmed, ".else") {
			if len(ctx.ifStack) == 0 {
				return "", fmt.Errorf("found .else without a matching .ifdef/.ifndef")
			}
			if ctx.ifStack[len(ctx.ifStack)-1] == ifTrue {
				ctx.ifStack[len(ctx.ifStack)-1] = ifFalse
			} else if ctx.ifStack[len(ctx.ifStack)-1] == ifFalse {
				ctx.ifStack[len(ctx.ifStack)-1] = ifTrue
			}
			continue
		}

		if strings.HasPrefix(trimmed, ".endif") {
			if len(ctx.ifStack) == 0 {
				return "", fmt.Errorf("found .endif without a matching .ifdef/.ifndef")
			}
			ctx.ifStack = ctx.ifStack[:len(ctx.ifStack)-1]
			continue
		}

		if kind, name, ok := parseOnceDirective(trimmed); ok {
			if name == "" {
				return "", fmt.Errorf(".once %s requires a block name", kind)
			}
			if kind == "begin" {
				if ctx.blockIncluded[name] {
					ctx.skipBlockDepth = 1
				} else {
					ctx.blockOnceStack = append(ctx.blockOnceStack, name)
				}
			} else {
				if ctx.skipBlockDepth > 0 {
					ctx.skipBlockDepth--
					continue
				}
				if len(ctx.blockOnceStack) == 0 {
					return "", fmt.Errorf(".once end %q without matching .once begin", name)
				}
				top := ctx.blockOnceStack[len(ctx.blockOnceStack)-1]
				if top != name {
					return "", fmt.Errorf(".once end name mismatch: began as %q, ended as %q", top, name)
				}
				ctx.blockOnceStack = ctx.blockOnceStack[:len(ctx.blockOnceStack)-1]
				ctx.blockIncluded[name] = true
			}
			continue
		}

		if trimmed == ".once" {
			continue
		}

		if count, ok, err := parseReptDirective(trimmed); err != nil {
			return "", err
		} else if ok {
			if count < 0 || count > maxReptCount {
				return "", fmt.Errorf(".rept count %d exceeds limit %d", count, maxReptCount)
			}
			ctx.reptStack = append(ctx.reptStack, reptState{count: count, buf: []string{}})
			continue
		}

		// .test block start (the body is collected by the early branch above)
		if strings.HasPrefix(trimmed, ".test") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ".test"))
			if len(name) >= 2 && name[0] == '"' {
				end := strings.IndexByte(name[1:], '"')
				if end >= 0 {
					name = name[1 : end+1]
				}
			}
			ctx.inTest = true
			ctx.testName = name
			ctx.testBuf = []string{}
			continue
		}

		out.WriteString(line)
		out.WriteByte('\n')
	}

	if ctx.inMacro {
		return "", fmt.Errorf("macro %q is missing a closing .endm", ctx.macroName)
	}
	if len(ctx.ifStack) > 0 {
		return "", fmt.Errorf("unclosed .ifdef/.ifndef (missing .endif)")
	}
	if len(ctx.blockOnceStack) > 0 {
		return "", fmt.Errorf("block %q is missing a closing .once end", ctx.blockOnceStack[len(ctx.blockOnceStack)-1])
	}
	if len(ctx.reptStack) > 0 {
		return "", fmt.Errorf(".rept block is missing a closing .endr")
	}
	if ctx.inTest {
		return "", fmt.Errorf("test %q is missing a closing .endtest", ctx.testName)
	}

	return out.String(), nil
}

// resolveConstValue expands earlier constants referenced by value. Cycles are
// rejected with a diagnostic instead of producing an unstable expansion.
func (ctx *prepContext) resolveConstValue(owner, value string, stack map[string]bool) (string, error) {
	names := make([]string, 0, len(ctx.consts))
	for name := range ctx.consts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, name := range names {
		if !containsIdent(value, name) {
			continue
		}
		if stack[name] {
			return "", fmt.Errorf("circular constant definition involving %q", owner)
		}
		stack[name] = true
		resolved, err := ctx.resolveConstValue(name, ctx.consts[name], stack)
		delete(stack, name)
		if err != nil {
			return "", err
		}
		value = replaceIdent(value, name, resolved)
	}
	return value, nil
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func containsIdent(s, name string) bool {
	for start := 0; start+len(name) <= len(s); {
		i := strings.Index(s[start:], name)
		if i < 0 {
			return false
		}
		i += start
		before := i == 0 || !isIdentChar(s[i-1])
		end := i + len(name)
		after := end == len(s) || !isIdentChar(s[end])
		if before && after {
			return true
		}
		start = i + len(name)
	}
	return false
}

func replaceIdent(s, name, value string) string {
	var b strings.Builder
	for start := 0; start < len(s); {
		i := strings.Index(s[start:], name)
		if i < 0 {
			b.WriteString(s[start:])
			break
		}
		i += start
		b.WriteString(s[start:i])
		before := i == 0 || !isIdentChar(s[i-1])
		end := i + len(name)
		after := end == len(s) || !isIdentChar(s[end])
		if before && after {
			b.WriteString(value)
			start = end
		} else {
			b.WriteString(s[i:end])
			start = end
		}
	}
	return b.String()
}

func isDirectiveWord(line, word string) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && fields[0] == word
}

// parseOnceDirective accepts ".once begin NAME" / ".once end NAME" with any
// amount of internal whitespace. Prefix matching used to mis-handle
// ".once  begin".
func parseOnceDirective(line string) (kind, name string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != ".once" {
		return "", "", false
	}
	if fields[1] != "begin" && fields[1] != "end" {
		return "", "", false
	}
	return fields[1], strings.Join(fields[2:], " "), true
}

func parseReptDirective(line string) (int, bool, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 || fields[0] != ".rept" {
		return 0, false, nil
	}
	if len(fields) != 2 {
		return 0, true, fmt.Errorf("invalid .rept syntax: %s", line)
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, true, fmt.Errorf("invalid .rept count: %s", fields[1])
	}
	return n, true, nil
}

func parseAuthor(line string) (string, error) {
	trimmed := strings.TrimSpace(asm.StripComment(line))
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ".author"))
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], nil
		}
	}
	return "", fmt.Errorf("invalid .author syntax: %s", line)
}

func parseInclude(line string) (path string, isPkg bool, ok bool) {
	trimmed := strings.TrimSpace(asm.StripComment(line))
	if !strings.HasPrefix(trimmed, ".include") || strings.HasPrefix(trimmed, ".include_bytes") {
		return "", false, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ".include"))
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], false, true
		}
	}
	if len(rest) >= 2 && rest[0] == '<' {
		end := strings.IndexByte(rest[1:], '>')
		if end >= 0 {
			return rest[1 : end+1], true, true
		}
	}
	return "", false, false
}

func parseIncludeBytes(line string) (path string, isPkg bool, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(asm.StripComment(line)), ".include_bytes")
	if !found {
		return "", false, false
	}
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], false, true
		}
	}
	if len(rest) >= 2 && rest[0] == '<' {
		end := strings.IndexByte(rest[1:], '>')
		if end >= 0 {
			return rest[1 : end+1], true, true
		}
	}
	return "", false, false
}

func parseConst(line string) (string, string, error) {
	code := strings.TrimSpace(asm.StripComment(line))
	parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(code, ".const")), "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid .const syntax: %s", line)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func parseMacro(line string) (string, []string, map[string]string, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(asm.StripComment(line)), ".macro"))
	var name string
	var paramStr string
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		name = rest[:idx]
		paramStr = strings.TrimSpace(rest[idx+1:])
	} else {
		name = rest
	}
	if name == "" {
		return "", nil, nil, fmt.Errorf("invalid .macro syntax: missing name")
	}

	var params []string
	defaults := make(map[string]string)

	if paramStr != "" {
		rawArgs := splitArgs(paramStr)
		for _, arg := range rawArgs {
			paramName, defVal, hasDef := strings.Cut(arg, "=")
			paramName = strings.TrimSpace(paramName)
			if paramName == "" {
				return "", nil, nil, fmt.Errorf("invalid .macro syntax: empty parameter name in %q", arg)
			}
			params = append(params, paramName)
			if hasDef {
				defVal = strings.TrimSpace(defVal)
				defaults[paramName] = defVal
			}
		}
	}

	for _, p := range params {
		if isVReg(p) {
			return "", nil, nil, fmt.Errorf("macro parameter %q is a reserved virtual register", p)
		}
	}
	return name, params, defaults, nil
}

func isVReg(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for _, c := range s[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inQuote {
			current.WriteByte(c)
			if c == quoteChar {
				inQuote = false
			}
		} else {
			switch c {
			case ',':
				args = append(args, strings.TrimSpace(current.String()))
				current.Reset()
			case '"', '\'':
				inQuote = true
				quoteChar = c
				current.WriteByte(c)
			default:
				current.WriteByte(c)
			}
		}
	}
	args = append(args, strings.TrimSpace(current.String()))
	return args
}

func (ctx *prepContext) loadInclude(path string, isPkg bool) (string, error) {
	if isPkg {
		return ctx.loadPackageInclude(path)
	}
	return ctx.loadFileInclude(path)
}

func (ctx *prepContext) loadFileInclude(path string) (string, error) {
	ctx.depth++
	defer func() { ctx.depth-- }()

	if !filepath.IsAbs(path) {
		cand := filepath.Join(ctx.dir, path)
		if data, err := os.ReadFile(cand); err == nil {
			return ctx.includeFile(cand, data)
		}
	}
	if filepath.IsAbs(path) {
		if data, err := os.ReadFile(path); err == nil {
			return ctx.includeFile(path, data)
		}
	}
	for _, dir := range ctx.vasPath {
		cand := filepath.Join(dir, path)
		if data, err := os.ReadFile(cand); err == nil {
			return ctx.includeFile(cand, data)
		}
	}
	return "", fmt.Errorf("%q not found in search path", path)
}

func (ctx *prepContext) loadFileBytes(path string, isPkg bool) ([]byte, error) {
	if isPkg {
		parts := strings.SplitN(path, "/", 2)
		pkgName := parts[0]
		modPath := ""
		if len(parts) > 1 {
			modPath = parts[1]
		} else {
			modPath = pkgName
		}
		searchDirs := []string{}
		if pkgDir := pkgCacheDir(); pkgDir != "" {
			searchDirs = append(searchDirs, pkgDir)
		}
		searchDirs = append(searchDirs, ctx.vasPath...)
		for _, root := range searchDirs {
			cands := []string{
				filepath.Join(root, pkgName, modPath),
				filepath.Join(root, pkgName, modPath+".bin"),
				filepath.Join(root, pkgName, modPath+".bytes"),
			}
			for _, cand := range cands {
				if !isPathSafe(root, cand) {
					continue
				}
				if data, err := os.ReadFile(cand); err == nil {
					return data, nil
				}
			}
		}
		return nil, fmt.Errorf("binary %q not found", path)
	}
	if !filepath.IsAbs(path) {
		cand := filepath.Join(ctx.dir, path)
		if data, err := os.ReadFile(cand); err == nil {
			return data, nil
		}
	}
	if filepath.IsAbs(path) {
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("%q not found for .include_bytes", path)
}

func (ctx *prepContext) loadPackageInclude(pkgPath string) (string, error) {
	pkgKey := "pkg:" + pkgPath

	for _, p := range ctx.pkgStack {
		if p == pkgKey {
			return "", fmt.Errorf("circular package include: %s", pkgPath)
		}
	}
	if ctx.pkgIncluded[pkgKey] {
		return "", nil
	}

	ctx.depth++
	defer func() { ctx.depth-- }()

	ctx.pkgStack = append(ctx.pkgStack, pkgKey)
	ctx.pkgIncluded[pkgKey] = true

	resolved, err := ctx.resolver.ResolvePackage(pkgPath)
	ctx.pkgStack = ctx.pkgStack[:len(ctx.pkgStack)-1]
	if err != nil {
		delete(ctx.pkgIncluded, pkgKey)
		parts := strings.SplitN(pkgPath, "/", 2)
		pkgName := parts[0]
		var installHint string
		if pm := pmName(); pm != "" {
			installHint = fmt.Sprintf(" – run `%s install %s`", pm, pkgName)
		} else {
			installHint = " – install it with your package manager"
		}
		return "", fmt.Errorf("package %q not found%s", pkgPath, installHint)
	}
	return resolved, nil
}

type defaultResolver struct {
	pkgDir    string
	vasPath   []string
	parentCtx *prepContext
}

func (r *defaultResolver) ResolvePackage(pkgPath string) (string, error) {
	parts := strings.SplitN(pkgPath, "/", 2)
	pkgName := parts[0]
	modPath := ""
	if len(parts) > 1 {
		modPath = parts[1]
	} else {
		modPath = pkgName
	}

	searchDirs := []string{}
	if r.pkgDir != "" {
		searchDirs = append(searchDirs, r.pkgDir)
	}
	searchDirs = append(searchDirs, r.vasPath...)

	for _, root := range searchDirs {
		candidates := []string{
			filepath.Join(root, pkgName, modPath+".vas"),
			filepath.Join(root, pkgName, modPath, "index.vas"),
		}
		for _, cand := range candidates {
			if !isPathSafe(root, cand) {
				continue
			}
			if data, err := os.ReadFile(cand); err == nil {
				// Resolve in the parent context so package-defined
				// constants and macros remain visible to the caller.
				return r.parentCtx.resolve(string(data), filepath.Dir(cand))
			}
		}
	}
	return "", fmt.Errorf("package %q not found", pkgPath)
}

func (ctx *prepContext) includeFile(filePath string, data []byte) (string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}

	for _, p := range ctx.includeStack {
		if p == abs {
			fullPath := append([]string{}, ctx.includeStack...)
			fullPath = append(fullPath, abs)

			var sb strings.Builder
			sb.WriteString("circular include detected:\n")
			for i, elem := range fullPath {
				if i == 0 {
					sb.WriteString("    ")
					sb.WriteString(elem)
					sb.WriteString("  <-- cycle starts here\n")
				} else if i == len(fullPath)-1 {
					sb.WriteString("    └──→ ")
					sb.WriteString(elem)
					sb.WriteString("  <-- cycle back to here\n")
				} else {
					sb.WriteString("    ")
					sb.WriteString(elem)
					sb.WriteByte('\n')
				}
			}
			return "", errors.New(sb.String())
		}
	}

	if ctx.included[abs] {
		return "", nil
	}

	ctx.includeStack = append(ctx.includeStack, abs)
	ctx.included[abs] = true

	resolved, err := ctx.resolve(string(data), filepath.Dir(filePath))

	ctx.includeStack[len(ctx.includeStack)-1] = ""
	ctx.includeStack = ctx.includeStack[:len(ctx.includeStack)-1]

	if err != nil {
		delete(ctx.included, abs)
		return "", err
	}
	return resolved, nil
}

func (ctx *prepContext) expandMacros(src string) (string, error) {
	var outLines []string
	lines := strings.Split(src, "\n")
	for _, line := range lines {
		code := strings.TrimSpace(asm.StripComment(line))
		if code == "" || strings.HasPrefix(code, ".") {
			outLines = append(outLines, line)
			continue
		}
		// Preserve the exact argument substring: strings.Fields+Join would
		// collapse repeated spaces inside quoted macro arguments.
		nameEnd := 0
		for nameEnd < len(code) && code[nameEnd] != ' ' && code[nameEnd] != '\t' {
			nameEnd++
		}
		name := code[:nameEnd]
		macro, ok := ctx.macros[name]
		if !ok {
			outLines = append(outLines, line)
			continue
		}
		var args []string
		if rest := strings.TrimSpace(code[nameEnd:]); rest != "" {
			args = splitArgs(rest)
			if len(args) > 0 && args[len(args)-1] == "" {
				args = args[:len(args)-1]
			}
		}
		if len(args) > len(macro.params) {
			return "", fmt.Errorf("macro %s: too many arguments, expected at most %d, got %d", name, len(macro.params), len(args))
		}

		finalArgs := make([]string, len(macro.params))
		for i, p := range macro.params {
			if i < len(args) {
				finalArgs[i] = args[i]
			} else if defVal, hasDef := macro.defaults[p]; hasDef {
				finalArgs[i] = defVal
			} else {
				return "", fmt.Errorf("macro %s: missing required argument %q (expected %d arguments, got %d)", name, p, len(macro.params), len(args))
			}
		}

		ctx.labelCounter++
		order := make([]int, len(macro.params))
		for i := range order {
			order[i] = i
		}
		sort.Slice(order, func(i, j int) bool {
			return len(macro.params[order[i]]) > len(macro.params[order[j]])
		})

		for _, mline := range macro.body {
			expanded := substituteMacroLine(mline, macro.params, order, finalArgs, ctx.labelCounter)
			outLines = append(outLines, expanded)
			ctx.expandedLines++
			if ctx.expandedLines > maxExpandedLines {
				return "", fmt.Errorf("macro expansion exceeds %d lines", maxExpandedLines)
			}
		}
	}
	var out strings.Builder
	for i, l := range outLines {
		out.WriteString(l)
		if i < len(outLines)-1 || l != "" {
			out.WriteByte('\n')
		}
	}
	return out.String(), nil
}

// substituteMacroLine expands \param and \@ in one macro body line. It
// respects token boundaries (so \a never matches the prefix of \abc) and
// never rewrites a comment.
func substituteMacroLine(line string, params []string, order []int, args []string, label int) string {
	var b strings.Builder
	quote := byte(0)
	for i := 0; i < len(line); {
		c := line[i]
		if quote != 0 {
			if c == '\\' && i+2 < len(line) && line[i+1] == '\\' {
				// Keep NASM escapes verbatim; only macro parameters are
				// substituted below on a boundary match.
			}
			if c == quote {
				quote = 0
			}
			b.WriteByte(c)
			i++
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
			b.WriteByte(c)
			i++
			continue
		case ';', '#':
			b.WriteString(line[i:])
			return b.String()
		}
		if c != '\\' {
			b.WriteByte(c)
			i++
			continue
		}
		if i+1 < len(line) && line[i+1] == '@' {
			fmt.Fprintf(&b, "_%d", label)
			i += 2
			continue
		}
		matched := false
		for _, pi := range order {
			p := params[pi]
			if !strings.HasPrefix(line[i+1:], p) {
				continue
			}
			end := i + 1 + len(p)
			if end < len(line) && isIdentChar(line[end]) {
				continue
			}
			b.WriteString(args[pi])
			i = end
			matched = true
			break
		}
		if !matched {
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func (ctx *prepContext) applyConsts(src string) (string, error) {
	if len(ctx.consts) == 0 {
		return src, nil
	}

	var names []string
	for name := range ctx.consts {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Slice(names, func(i, j int) bool {
		return len(names[i]) > len(names[j])
	})
	re, err := regexp.Compile(`\b(` + strings.Join(names, "|") + `)\b`)
	if err != nil {
		return "", fmt.Errorf("const pattern: %w", err)
	}

	replace := func(match string) string {
		if val, ok := ctx.consts[match]; ok {
			return val
		}
		return match
	}

	var result strings.Builder
	lines := strings.Split(src, "\n")
	for _, line := range lines {
		// Never substitute into a directive line. In particular
		// ".ifdef DEBUG" must keep the symbol name; rewriting it to
		// ".ifdef 1" would make the conditional evaluate false.
		if strings.HasPrefix(strings.TrimSpace(line), ".") {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		var outLine strings.Builder
		quote := byte(0)
		inComment := false
		codeStart := 0
		for i := 0; i < len(line); i++ {
			c := line[i]
			if inComment {
				outLine.WriteByte(c)
				continue
			}
			if quote != 0 {
				outLine.WriteByte(c)
				if c == '\\' && i+1 < len(line) {
					i++
					outLine.WriteByte(line[i])
				} else if c == quote {
					quote = 0
					codeStart = i + 1
				}
				continue
			}
			if c == '"' || c == '\'' || c == '`' {
				code := line[codeStart:i]
				outLine.WriteString(re.ReplaceAllStringFunc(code, replace))
				outLine.WriteByte(c)
				quote = c
				codeStart = i + 1
				continue
			}
			if c == ';' || c == '#' {
				code := line[codeStart:i]
				outLine.WriteString(re.ReplaceAllStringFunc(code, replace))
				outLine.WriteByte(c)
				inComment = true
				codeStart = i + 1
				continue
			}
		}
		if !inComment && quote == 0 {
			outLine.WriteString(re.ReplaceAllStringFunc(line[codeStart:], replace))
		}
		result.WriteString(outLine.String())
		result.WriteByte('\n')
	}
	final := result.String()
	if len(final) > 0 && final[len(final)-1] == '\n' {
		final = final[:len(final)-1]
	}
	return final, nil
}

func isInstructionOrDirective(s string) bool {
	switch strings.ToUpper(s) {
	case "ADD", "SUB", "MUL", "LOAD", "STORE", "LEA", "MOV", "MOVI",
		"CMP", "JMP", "JE", "JNE", "JG", "JL", "JGE", "JLE",
		"CALL", "RET", "NOP", "PUSH", "POP", "INT", "SYSCALL",
		"SECTION", "GLOBAL", "EXTERN", "DATA", "TEXT", "BSS",
		"ALIGN", "BYTE", "WORD", "DWORD", "QWORD", "DD", "DQ", "DB",
		"TYPE", "SIZE", "LENGTH", "START", "TIMES", "EQU",
		"RESB", "RESD", "RESQ", "INCBIN", "BITS":
		return true
	}
	return false
}

func isUndefConst(s string) bool {
	if strings.HasPrefix(s, ".") || strings.HasSuffix(s, ":") ||
		strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		return false
	}
	if isInstructionOrDirective(s) {
		return false
	}
	hasUpper := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if (r >= 'A' && r <= 'Z') || r == '_' {
			hasUpper = true
		}
	}
	return hasUpper
}

func replaceWord(src, old, new string) string {
	var out strings.Builder
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		tokens := strings.Split(line, " ")
		for j, token := range tokens {
			if token == old {
				tokens[j] = new
			}
		}
		for j, token := range tokens {
			if j > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(token)
		}
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func isPathSafe(root, candidate string) bool {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absCand, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err == nil {
		absRoot = realRoot
	}
	realCand, err := filepath.EvalSymlinks(absCand)
	if err == nil {
		absCand = realCand
	}
	rel, err := filepath.Rel(absRoot, absCand)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != ".."
}

func pkgCacheDir() string {
	if dir := os.Getenv("VAS_PKG_CACHE"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vas", "pkg")
}

func pmName() string {
	return os.Getenv("VAS_PM")
}

func searchPath() []string {
	env := os.Getenv("VAS_PATH")
	if env == "" {
		return nil
	}
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	return strings.Split(env, sep)
}

// firstDirectSymbol extracts a label from "short label", "rel label" or a
// plain symbol operand, skipping registers, numbers and local labels.
func firstDirectSymbol(operand string) string {
	for _, tok := range strings.Fields(operand) {
		tok = strings.Trim(tok, ",[]")
		if tok == "" || isRegister(tok) || nasmKeywords[strings.ToLower(tok)] {
			continue
		}
		if strings.HasPrefix(tok, ".") {
			return ""
		}
		if _, err := strconv.ParseInt(tok, 0, 64); err == nil {
			return ""
		}
		return tok
	}
	return ""
}

// firstAddressSymbol extracts the first possible static symbol from a memory
// address, e.g. "table + rbx*8" -> "table", "rel msg" -> "msg".
func firstAddressSymbol(inner string) string {
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '+', '-', '*', '/', '?', '(', ')', ',', '[', ']':
			b.WriteByte(' ')
		default:
			b.WriteByte(inner[i])
		}
	}
	for _, tok := range strings.Fields(b.String()) {
		tok = strings.Trim(tok, ",:")
		if tok == "" || isRegister(tok) || nasmKeywords[strings.ToLower(tok)] {
			continue
		}
		if strings.HasPrefix(tok, ".") || strings.HasPrefix(tok, "$") ||
			strings.ContainsAny(tok, ":[]\"'`%") {
			return ""
		}
		if _, err := strconv.ParseInt(tok, 0, 64); err == nil {
			return ""
		}
		return tok
	}
	return ""
}

// checkUndefinedLabels scans src for label definitions and references,
// returning the names of labels that are referenced but never defined.
func checkUndefinedLabels(src string) []string {
	defined := make(map[string]bool)
	referenced := make(map[string]bool)

	lines := strings.Split(src, "\n")
	for _, line := range lines {
		code := strings.TrimSpace(asm.StripComment(line))
		if code == "" {
			continue
		}

		// Check for label definition (e.g., "loop:")
		// A label is an identifier ending with ':' that is NOT:
		// - A section directive ("section .text:")
		// - A global/extern directive ("global _start:")
		// - A nasm directive or instruction
		if strings.HasSuffix(code, ":") {
			label := strings.TrimSuffix(code, ":")
			label = strings.TrimSpace(label)
			// Skip if it looks like a directive or instruction
			upper := strings.ToUpper(label)
			if upper == "SECTION" || upper == "GLOBAL" || upper == "EXTERN" ||
				upper == "COMMON" || upper == "CPU" || upper == "FLOAT" ||
				isInstructionOrDirective(label) {
				continue
			}
			// Skip local labels (starting with .)
			if strings.HasPrefix(label, ".") {
				continue
			}
			defined[label] = true
			continue
		}

		// Register external symbols so CALL printf with EXTERN printf is
		// accepted; also record global/common declarations.
		fields := strings.Fields(code)
		if len(fields) == 0 {
			continue
		}
		opcode := strings.ToUpper(fields[0])
		if opcode == "EXTERN" || opcode == "GLOBAL" || opcode == "COMMON" {
			for _, f := range fields[1:] {
				name := strings.TrimSuffix(strings.TrimSpace(f), ",")
				if name != "" {
					defined[name] = true
				}
			}
			continue
		}

		// Jump and call instructions that reference labels
		if opcode == "JMP" || opcode == "JE" || opcode == "JNE" ||
			opcode == "JG" || opcode == "JL" || opcode == "JGE" ||
			opcode == "JLE" || opcode == "JA" || opcode == "JB" ||
			opcode == "JAE" || opcode == "JBE" || opcode == "JO" ||
			opcode == "JNO" || opcode == "JS" || opcode == "JNS" ||
			opcode == "CALL" || opcode == "LOOP" || opcode == "LOOPE" ||
			opcode == "LOOPNE" || opcode == "LOOPZ" || opcode == "LOOPNZ" {
			if len(fields) < 2 {
				continue
			}
			operand := strings.Trim(strings.Join(fields[1:], " "), ",")
			// Handle indirect memory references (e.g. jmp [rax],
			// call [v0], jmp [table + rbx*8]).
			if strings.HasPrefix(operand, "[") {
				inner := strings.TrimSuffix(strings.TrimPrefix(operand, "["), "]")
				if sym := firstAddressSymbol(inner); sym != "" {
					referenced[sym] = true
				}
				continue
			}
			if sym := firstDirectSymbol(operand); sym != "" {
				referenced[sym] = true
			}
		}
	}

	// Find undefined references
	var undefined []string
	for label := range referenced {
		if !defined[label] {
			undefined = append(undefined, label)
		}
	}
	sort.Strings(undefined)
	return undefined
}
