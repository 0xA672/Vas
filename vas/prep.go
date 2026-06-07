package vas

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ifState tracks conditional inclusion nesting.
type ifState int

const (
	ifNone ifState = iota
	ifTrue
	ifFalse
)

type macroDef struct {
	params []string
	body   []string
}

// prepContext tracks state during preprocessing.
type prepContext struct {
	dir          string
	included     map[string]bool // file-level deduplication (absolute paths)
	pkgDir       string
	vasPath      []string
	consts       map[string]string   // .const NAME = value
	macros       map[string]macroDef // .macro definitions
	defines      map[string]bool     // defined names (for .ifdef)
	ifStack      []ifState
	macroBuf     []string // lines collected for current macro definition
	macroName    string   // name of macro being defined
	macroParams  []string
	inMacro      bool
	labelCounter int // for unique labels (\@)
	inRept       bool
	reptCount    int
	reptBuf      []string
	// Block-level deduplication for .once begin/end
	blockOnceStack []string        // stack of active block names (.once begin <name>)
	blockIncluded  map[string]bool // set of completed block names that were marked with .once
	skipBlockDepth int             // depth of nested blocks being skipped (for .once end matching)
	includeStack   []string        // tracks current include chain for cycle detection
}

// Preprocess resolves all preprocessor directives and returns flattened source.
func Preprocess(src, baseDir string) (string, error) {
	ctx := &prepContext{
		dir:           baseDir,
		included:      map[string]bool{},
		pkgDir:        pkgCacheDir(),
		vasPath:       searchPath(),
		consts:        map[string]string{},
		macros:        map[string]macroDef{},
		defines:       map[string]bool{},
		blockIncluded: map[string]bool{},
	}
	// Pass 1: resolve include, collect macros/consts, handle ifdef
	out, err := ctx.resolve(src, baseDir, 0)
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
	return out, nil
}

func (ctx *prepContext) resolve(src, dir string, depth int) (string, error) {
	if depth > 100 {
		return "", fmt.Errorf("preprocessing recursion limit exceeded")
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

		// Handle rept collection
		if ctx.inRept {
			if strings.HasPrefix(trimmed, ".endr") {
				for i := 0; i < ctx.reptCount; i++ {
					for _, rline := range ctx.reptBuf {
						out.WriteString(rline)
						out.WriteByte('\n')
					}
				}
				ctx.inRept = false
				ctx.reptBuf = nil
				continue
			}
			ctx.reptBuf = append(ctx.reptBuf, line)
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

		// Handle conditional inclusion skipping (ifdef / ifndef false branch)
		if len(ctx.ifStack) > 0 && ctx.ifStack[len(ctx.ifStack)-1] == ifFalse {
			if strings.HasPrefix(trimmed, ".ifdef") || strings.HasPrefix(trimmed, ".ifndef") {
				ctx.ifStack = append(ctx.ifStack, ifFalse)
			} else if strings.HasPrefix(trimmed, ".endif") {
				ctx.ifStack = ctx.ifStack[:len(ctx.ifStack)-1]
			} else if strings.HasPrefix(trimmed, ".else") {
				ctx.ifStack[len(ctx.ifStack)-1] = ifTrue
			}
			continue
		}

		// Directives processing
		if strings.HasPrefix(trimmed, ".include_bytes") {
			path, ok := parseIncludeBytes(line)
			if !ok {
				return "", fmt.Errorf("invalid .include_bytes syntax: %s", line)
			}
			data, err := ctx.loadFileBytes(path)
			if err != nil {
				return "", err
			}
			if len(data) > 0 {
				hexParts := make([]string, len(data))
				for i, b := range data {
					hexParts[i] = fmt.Sprintf("0x%02x", b)
				}
				out.WriteString("db " + strings.Join(hexParts, ", "))
			}
			out.WriteByte('\n')
			continue
		}

		if strings.HasPrefix(trimmed, ".include") {
			path, isPkg, ok := parseInclude(line)
			if !ok {
				return "", fmt.Errorf("invalid .include syntax: %s", line)
			}
			resolved, err := ctx.loadInclude(path, isPkg, depth)
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
			} else {
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
			ctx.reptCount = count
			ctx.reptBuf = []string{}
			ctx.inRept = true
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

func parseIncludeBytes(line string) (string, bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(line), ".include_bytes")
	if !found {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], true
		}
	}
	return "", false
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
		params = strings.Split(paramStr, ",")
		for i := range params {
			params[i] = strings.TrimSpace(params[i])
		}
	}
	return name, params, nil
}

func (ctx *prepContext) loadInclude(path string, isPkg bool, depth int) (string, error) {
	if isPkg {
		return ctx.loadPackageInclude(path, depth)
	}
	return ctx.loadFileInclude(path, depth)
}

func (ctx *prepContext) loadFileInclude(path string, depth int) (string, error) {
	if !filepath.IsAbs(path) {
		cand := filepath.Join(ctx.dir, path)
		if data, err := os.ReadFile(cand); err == nil {
			return ctx.includeFile(cand, data, depth)
		}
	}
	if filepath.IsAbs(path) {
		if data, err := os.ReadFile(path); err == nil {
			return ctx.includeFile(path, data, depth)
		}
	}
	for _, dir := range ctx.vasPath {
		cand := filepath.Join(dir, path)
		if data, err := os.ReadFile(cand); err == nil {
			return ctx.includeFile(cand, data, depth)
		}
	}
	return "", fmt.Errorf("%q not found in search path", path)
}

func (ctx *prepContext) loadFileBytes(path string) ([]byte, error) {
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

func (ctx *prepContext) loadPackageInclude(pkgPath string, depth int) (string, error) {
	parts := strings.SplitN(pkgPath, "/", 2)
	pkgName := parts[0]
	modPath := ""
	if len(parts) > 1 {
		modPath = parts[1]
	} else {
		modPath = pkgName
	}
	searchDirs := []string{}
	if ctx.pkgDir != "" {
		searchDirs = append(searchDirs, ctx.pkgDir)
	}
	searchDirs = append(searchDirs, ctx.vasPath...)
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
				return ctx.includeFile(cand, data, depth)
			}
		}
	}
	return "", fmt.Errorf("package %q not found – run `vpk install %s` to install it", pkgPath, pkgName)
}

func (ctx *prepContext) includeFile(filePath string, data []byte, depth int) (string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		abs = filePath
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		abs = real
	}

	// Cycle detection: check if this file is already on the current include stack
	for _, p := range ctx.includeStack {
		if p == abs {
			return "", fmt.Errorf("circular include detected: %s", abs)
		}
	}

	// Already fully included (and not currently on stack) → skip
	if ctx.included[abs] {
		return "", nil
	}

	// Push onto stack and mark as included
	ctx.includeStack = append(ctx.includeStack, abs)
	ctx.included[abs] = true

	resolved, err := ctx.resolve(string(data), filepath.Dir(filePath), depth+1)

	// Pop stack
	ctx.includeStack = ctx.includeStack[:len(ctx.includeStack)-1]

	if err != nil {
		// Rollback on failure to allow future attempts
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
				args = strings.Split(argStr, ",")
				for i := range args {
					args[i] = strings.TrimSpace(args[i])
				}
			}
			if len(args) != len(macro.params) {
				return "", fmt.Errorf("macro argument mismatch for %s: expected %d, got %d", parts[0], len(macro.params), len(args))
			}
			ctx.labelCounter++
			for _, mline := range macro.body {
				expanded := mline
				for pi, param := range macro.params {
					expanded = strings.ReplaceAll(expanded, `\`+param, args[pi])
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
	for name, value := range ctx.consts {
		src = replaceWord(src, name, value)
	}
	lines := strings.Split(src, "\n")
	for _, line := range lines {
		tokens := strings.Fields(line)
		for _, token := range tokens {
			if isUndefConst(token) {
				if _, ok := ctx.consts[token]; !ok {
					return "", fmt.Errorf("undefined constant: %s", token)
				}
			}
		}
	}
	return src, nil
}

func isInstructionOrDirective(s string) bool {
	switch s {
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
	upper := strings.ToUpper(s)
	if isInstructionOrDirective(upper) {
		return false
	}
	if strings.HasPrefix(s, "v") && len(s) <= 3 {
		if _, err := strconv.Atoi(s[1:]); err == nil {
			return false
		}
	}
	hasLetter := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			return false
		}
		if (r >= 'A' && r <= 'Z') || r == '_' {
			hasLetter = true
		}
	}
	return hasLetter
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
		out.WriteString(strings.Join(tokens, " "))
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String()
}

func isPathSafe(root, candidate string) bool {
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	return strings.HasPrefix(abs, absRoot+string(os.PathSeparator)) || abs == absRoot
}

func pkgCacheDir() string  { return "" }
func searchPath() []string { return nil }
