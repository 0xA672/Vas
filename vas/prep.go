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
)

// ifState tracks conditional inclusion nesting.
type ifState int

const (
	ifTrue ifState = iota
	ifFalse
	ifSkipping // nested block inside a false branch, unconditionally skipped
)

type macroDef struct {
	params []string
	body   []string
}

type reptState struct {
	count int
	buf   []string
}

// PackageResolver resolves a package path (e.g. "io" or "term/color")
// into preprocessed source text. Implementations may read from the file system,
// an in‑memory cache, or a network resource.
type PackageResolver interface {
	ResolvePackage(pkgPath string) (string, error)
}

// PreprocessOption configures a Preprocess call.
type PreprocessOption func(*prepContext)

// WithResolver replaces the default package resolver.
func WithResolver(r PackageResolver) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.resolver = r
	}
}

// PreHook is called once before preprocessing starts.
type PreHook func(src string) (string, error)

// PostHook is called once after preprocessing finishes.
type PostHook func(src string) (string, error)

// WithPreHook sets a hook that transforms the input source before processing.
func WithPreHook(hook PreHook) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.preHook = hook
	}
}

// WithPostHook sets a hook that transforms the output source after processing.
func WithPostHook(hook PostHook) PreprocessOption {
	return func(ctx *prepContext) {
		ctx.postHook = hook
	}
}

// withInheritContext is an internal option that allows the default resolver
// to inherit the parent preprocessing state (included files, package stack,
// recursion depth) when recursively processing included packages.
func withInheritContext(parent *prepContext) PreprocessOption {
	return func(child *prepContext) {
		child.included = parent.included
		child.pkgStack = parent.pkgStack
		child.pkgIncluded = parent.pkgIncluded
		child.depth = parent.depth
	}
}

// prepContext tracks state during preprocessing.
type prepContext struct {
	dir          string
	included     map[string]bool // file‑level deduplication (absolute paths)
	resolver     PackageResolver
	vasPath      []string
	consts       map[string]string   // .const NAME = value
	macros       map[string]macroDef // .macro definitions
	defines      map[string]bool     // defined names (for .ifdef)
	ifStack      []ifState
	macroBuf     []string // lines collected for current macro definition
	macroName    string
	macroParams  []string
	inMacro      bool
	labelCounter int // for unique labels (\@)
	// .rept stack for nested support
	reptStack []reptState
	// Block-level deduplication for .once begin/end
	blockOnceStack []string        // stack of active block names
	blockIncluded  map[string]bool // completed block names
	skipBlockDepth int

	includeStack []string // tracks current include chain for cycle detection

	// Package inclusion tracking (shared across nested contexts)
	pkgStack    []string
	pkgIncluded map[string]bool

	// Global recursion depth (file + package includes).
	// Incremented before entering a new include, decremented when leaving.
	depth int

	preHook  PreHook
	postHook PostHook
}

// Preprocess resolves all preprocessor directives and returns flattened source.
func Preprocess(src, baseDir string, opts ...PreprocessOption) (string, error) {
	ctx := &prepContext{
		dir:           baseDir,
		included:      map[string]bool{},
		resolver:      nil, // set later, may be overridden by WithResolver
		vasPath:       searchPath(),
		consts:        map[string]string{},
		macros:        map[string]macroDef{},
		defines:       map[string]bool{},
		blockIncluded: map[string]bool{},
		pkgIncluded:   map[string]bool{},
		depth:         0,
	}
	ctx.resolver = &defaultResolver{pkgDir: pkgCacheDir(), vasPath: ctx.vasPath, parentCtx: ctx}

	for _, opt := range opts {
		opt(ctx)
	}

	// Automatically define platform symbols based on the target environment.
	ctx.initPlatformDefines()

	if ctx.preHook != nil {
		var err error
		src, err = ctx.preHook(src)
		if err != nil {
			return "", fmt.Errorf("pre-hook: %w", err)
		}
	}

	// Pass 1: resolve include, collect macros/consts, handle ifdef
	out, err := ctx.resolve(src, baseDir)
	if err != nil {
		return "", err
	}
	// Pass 2: expand macro calls
	out, err = ctx.expandMacros(out)
	if err != nil {
		return "", err
	}
	// Pass 3: apply const replacement
	out, err = ctx.applyConsts(out)
	if err != nil {
		return "", err
	}

	if ctx.postHook != nil {
		out, err = ctx.postHook(out)
		if err != nil {
			return "", fmt.Errorf("post-hook: %w", err)
		}
	}
	return out, nil
}

// initPlatformDefines populates ctx.defines with symbols that reflect the
// compilation target (GOOS and GOARCH). It prefers the GOOS/GOARCH environment
// variables for cross-compilation, falling back to runtime.GOOS/GOARCH.
// Unknown but non-empty platforms are handled with a dynamically generated
// macro, providing forward compatibility.
func (ctx *prepContext) initPlatformDefines() {
	goos := os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	// Operating system
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

	// Architecture
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

// resolve is the main directive resolution pass.
func (ctx *prepContext) resolve(src, dir string) (string, error) {
	if ctx.depth > 100 {
		return "", fmt.Errorf("preprocessing recursion limit exceeded (check for circular or very deep includes)")
	}
	ctx.dir = dir
	var out strings.Builder
	lines := strings.Split(src, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle macro definition collection
		if ctx.inMacro {
			if strings.HasPrefix(trimmed, ".endm") {
				ctx.macros[ctx.macroName] = macroDef{
					params: ctx.macroParams,
					body:   ctx.macroBuf,
				}
				ctx.inMacro = false
				ctx.macroBuf = nil
				continue
			}
			ctx.macroBuf = append(ctx.macroBuf, line)
			continue
		}

		// Handle rept collection (nested via stack, no recursive resolve)
		if len(ctx.reptStack) > 0 {
			if strings.HasPrefix(trimmed, ".rept") {
				countStr := strings.TrimSpace(strings.TrimPrefix(trimmed, ".rept"))
				count, err := strconv.Atoi(countStr)
				if err != nil {
					return "", fmt.Errorf("invalid .rept count: %s", countStr)
				}
				ctx.reptStack = append(ctx.reptStack, reptState{count: count, buf: []string{}})
				continue
			}
			if strings.HasPrefix(trimmed, ".endr") {
				top := ctx.reptStack[len(ctx.reptStack)-1]
				ctx.reptStack = ctx.reptStack[:len(ctx.reptStack)-1]

				// Expand: repeat the collected lines top.count times
				var expanded []string
				for i := 0; i < top.count; i++ {
					expanded = append(expanded, top.buf...)
				}

				if len(ctx.reptStack) > 0 {
					// Nested: append expanded lines to parent buffer
					parent := &ctx.reptStack[len(ctx.reptStack)-1]
					parent.buf = append(parent.buf, expanded...)
				} else {
					// Top-level: write directly to output
					for _, l := range expanded {
						out.WriteString(l)
						out.WriteByte('\n')
					}
				}
				continue
			}
			// Inside a rept: collect raw line into top buffer
			ctx.reptStack[len(ctx.reptStack)-1].buf = append(
				ctx.reptStack[len(ctx.reptStack)-1].buf, line)
			continue
		}

		// Handle skipping for .once blocks (second+ encounter)
		if ctx.skipBlockDepth > 0 {
			if strings.HasPrefix(trimmed, ".once begin") {
				ctx.skipBlockDepth++
			} else if strings.HasPrefix(trimmed, ".once end") {
				ctx.skipBlockDepth--
			}
			continue
		}

		// Handle conditional inclusion skipping (ifdef / ifndef false or skipping branches)
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

		// Directives processing (active branches)
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
			ctx.consts[name] = value
			ctx.defines[name] = true
			continue
		}

		if strings.HasPrefix(trimmed, ".macro") {
			name, params, err := parseMacro(line)
			if err != nil {
				return "", err
			}
			ctx.macroName = name
			ctx.macroParams = params
			ctx.macroBuf = []string{}
			ctx.inMacro = true
			continue
		}

		if strings.HasPrefix(trimmed, ".endm") {
			return "", fmt.Errorf("orphan .endm")
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
				return "", fmt.Errorf("orphan .else")
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
				return "", fmt.Errorf("orphan .endif")
			}
			ctx.ifStack = ctx.ifStack[:len(ctx.ifStack)-1]
			continue
		}

		if strings.HasPrefix(trimmed, ".once begin") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ".once begin"))
			if name == "" {
				return "", fmt.Errorf(".once begin requires a block name")
			}
			if ctx.blockIncluded[name] {
				ctx.skipBlockDepth = 1
			} else {
				ctx.blockOnceStack = append(ctx.blockOnceStack, name)
			}
			continue
		}

		if strings.HasPrefix(trimmed, ".once end") {
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, ".once end"))
			if name == "" {
				return "", fmt.Errorf(".once end requires a block name")
			}
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
			continue
		}

		if trimmed == ".once" {
			continue
		}

		if strings.HasPrefix(trimmed, ".rept") {
			countStr := strings.TrimSpace(strings.TrimPrefix(trimmed, ".rept"))
			count, err := strconv.Atoi(countStr)
			if err != nil {
				return "", fmt.Errorf("invalid .rept count: %s", countStr)
			}
			ctx.reptStack = append(ctx.reptStack, reptState{count: count, buf: []string{}})
			continue
		}

		// Normal line
		out.WriteString(line)
		out.WriteByte('\n')
	}

	if ctx.inMacro {
		return "", fmt.Errorf("unclosed macro: %s", ctx.macroName)
	}
	if len(ctx.ifStack) > 0 {
		return "", fmt.Errorf("unclosed ifdef")
	}
	if len(ctx.blockOnceStack) > 0 {
		return "", fmt.Errorf("unclosed .once begin block: %s", ctx.blockOnceStack[len(ctx.blockOnceStack)-1])
	}
	if len(ctx.reptStack) > 0 {
		return "", fmt.Errorf("unclosed .rept")
	}

	return out.String(), nil
}

func parseInclude(line string) (path string, isPkg bool, ok bool) {
	trimmed := strings.TrimSpace(line)
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
	rest, found := strings.CutPrefix(strings.TrimSpace(line), ".include_bytes")
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
	parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ".const")), "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid .const syntax: %s", line)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func parseMacro(line string) (string, []string, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ".macro"))
	var name string
	var paramStr string
	if idx := strings.IndexAny(rest, " \t"); idx >= 0 {
		name = rest[:idx]
		paramStr = strings.TrimSpace(rest[idx+1:])
	} else {
		name = rest
	}
	if name == "" {
		return "", nil, fmt.Errorf("invalid .macro syntax: missing name")
	}
	var params []string
	if paramStr != "" {
		params = splitArgs(paramStr)
	}
	return name, params, nil
}

// splitArgs splits a string by commas, respecting quoted substrings.
// Both opening and closing quotes are preserved in the returned arguments.
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
			if c == ',' {
				args = append(args, strings.TrimSpace(current.String()))
				current.Reset()
			} else if c == '"' || c == '\'' {
				inQuote = true
				quoteChar = c
				current.WriteByte(c) // write the opening quote
			} else {
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
		delete(ctx.pkgIncluded, pkgKey) // allow retry
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
				return Preprocess(string(data), filepath.Dir(cand), withInheritContext(r.parentCtx))
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
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, ".") || strings.HasPrefix(trimmed, ";") {
			outLines = append(outLines, line)
			continue
		}
		parts := strings.Fields(trimmed)
		if macro, ok := ctx.macros[parts[0]]; ok {
			var args []string
			if len(parts) > 1 {
				argStr := strings.Join(parts[1:], " ")
				args = splitArgs(argStr)
			}
			if len(args) != len(macro.params) {
				return "", fmt.Errorf("macro argument mismatch for %s: expected %d, got %d", parts[0], len(macro.params), len(args))
			}
			ctx.labelCounter++

			type paramPair struct {
				name string
				idx  int
			}
			ordered := make([]paramPair, len(macro.params))
			for i, p := range macro.params {
				ordered[i] = paramPair{p, i}
			}
			sort.Slice(ordered, func(i, j int) bool {
				return len(ordered[i].name) > len(ordered[j].name)
			})

			for _, mline := range macro.body {
				expanded := mline
				for _, pp := range ordered {
					expanded = strings.ReplaceAll(expanded, `\`+pp.name, args[pp.idx])
				}
				expanded = strings.ReplaceAll(expanded, `\@`, fmt.Sprintf("_%d", ctx.labelCounter))
				outLines = append(outLines, expanded)
			}
		} else {
			outLines = append(outLines, line)
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
		var outLine strings.Builder
		inString := false
		inComment := false
		codeStart := 0
		for i := 0; i < len(line); i++ {
			c := line[i]
			if inComment {
				outLine.WriteByte(c)
				continue
			}
			if inString {
				if c == '"' {
					inString = false
					codeStart = i + 1
				}
				outLine.WriteByte(c)
				continue
			}
			if c == '"' {
				code := line[codeStart:i]
				outLine.WriteString(re.ReplaceAllStringFunc(code, replace))
				outLine.WriteByte(c)
				inString = true
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
		if !inComment && !inString {
			code := line[codeStart:]
			outLine.WriteString(re.ReplaceAllStringFunc(code, replace))
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
