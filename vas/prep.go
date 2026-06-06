// Package vas provides the VAS virtual assembler core.
package vas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// macroDef stores a single macro definition.
type macroDef struct {
	params []string
	body   []string
}

// ifState tracks conditional inclusion nesting.
type ifState int

const (
	ifActive   ifState = iota // block is active (condition true, no else seen)
	ifSkipping                // block is being skipped (condition false)
	ifDone                    // block was active but .else already seen
)

// prepContext tracks state during preprocessing.
type prepContext struct {
	dir      string
	included map[string]bool
	pkgDir   string
	vasPath  []string

	consts  map[string]string // .const NAME = value
	macros  map[string]macroDef
	defines map[string]bool // defined names (for .ifdef)

	ifStack []ifState

	macroBuf     []string // lines being collected for current macro
	macroName    string   // name of macro being defined
	macroParams  []string
	inMacro      bool
	labelCounter int // for unique labels (\\@)
}

// Preprocess resolves all preprocessor directives and returns flattened source.
func Preprocess(src, baseDir string) (string, error) {
	ctx := &prepContext{
		dir:      baseDir,
		included: map[string]bool{},
		pkgDir:   pkgCacheDir(),
		vasPath:  searchPath(),
		consts:   map[string]string{},
		macros:   map[string]macroDef{},
		defines:  map[string]bool{},
	}
	// Pass 1: resolve include, collect macros/consts, handle ifdef
	out, err := ctx.resolve(src, baseDir)
	if err != nil {
		return "", err
	}
	// Pass 2: expand macro calls
	out = ctx.expandMacros(out)
	// Pass 3: apply const replacement
	out = ctx.applyConsts(out)
	return out, nil
}

func pkgCacheDir() string {
	if d := os.Getenv("VPK_CACHE"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".vpk", "pkg")
}

func searchPath() []string {
	if p := os.Getenv("VAS_PATH"); p != "" {
		return filepath.SplitList(p)
	}
	return nil
}

// ── Pass 1: resolve directives ───────────────────────────────────────────

func (ctx *prepContext) resolve(input, baseDir string) (string, error) {
	input = strings.TrimRight(input, "\n")
	lines := strings.Split(input, "\n")
	var out strings.Builder
	savedDir := ctx.dir
	ctx.dir = baseDir
	defer func() { ctx.dir = savedDir }()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if we're in a macro definition
		if ctx.inMacro {
			if trimmed == ".endm" {
				ctx.macros[ctx.macroName] = macroDef{
					params: ctx.macroParams,
					body:   ctx.macroBuf,
				}
				ctx.inMacro = false
				ctx.macroName = ""
				ctx.macroParams = nil
				ctx.macroBuf = nil
				continue
			}
			ctx.macroBuf = append(ctx.macroBuf, line)
			continue
		}

		// .ifdef / .ifndef
		if strings.HasPrefix(trimmed, ".ifdef ") || strings.HasPrefix(trimmed, ".ifndef ") {
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, ".ifdef "), ".ifndef "))
			isIfdef := strings.HasPrefix(trimmed, ".ifdef ")
			defined := ctx.defines[name]
			active := (isIfdef && defined) || (!isIfdef && !defined)

			if len(ctx.ifStack) > 0 && ctx.ifStack[len(ctx.ifStack)-1] != ifActive {
				active = false // nested inside an inactive block
			}

			if active {
				ctx.ifStack = append(ctx.ifStack, ifActive)
			} else {
				ctx.ifStack = append(ctx.ifStack, ifSkipping)
			}
			continue
		}

		// .else
		if trimmed == ".else" {
			if len(ctx.ifStack) == 0 {
				return "", fmt.Errorf(".else without .ifdef")
			}
			top := ctx.ifStack[len(ctx.ifStack)-1]
			if top == ifDone {
				return "", fmt.Errorf(".else after .else")
			}
			if top == ifSkipping {
				ctx.ifStack[len(ctx.ifStack)-1] = ifActive
			} else { // ifActive
				ctx.ifStack[len(ctx.ifStack)-1] = ifDone
			}
			continue
		}

		// .endif
		if trimmed == ".endif" {
			if len(ctx.ifStack) == 0 {
				return "", fmt.Errorf(".endif without .ifdef")
			}
			ctx.ifStack = ctx.ifStack[:len(ctx.ifStack)-1]
			continue
		}

		// Skip lines inside inactive conditional blocks
		if len(ctx.ifStack) > 0 && ctx.ifStack[len(ctx.ifStack)-1] != ifActive {
			continue
		}

		// .macro name args...
		if strings.HasPrefix(trimmed, ".macro ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ".macro "))
			parts := strings.Fields(rest)
			if len(parts) == 0 {
				return "", fmt.Errorf(".macro requires a name")
			}
			ctx.inMacro = true
			ctx.macroName = parts[0]
			ctx.macroParams = make([]string, len(parts[1:]))
			for i, p := range parts[1:] {
				ctx.macroParams[i] = strings.Trim(p, ",")
			}
			ctx.macroBuf = nil
			continue
		}

		// .endm outside macro
		if trimmed == ".endm" {
			return "", fmt.Errorf(".endm without .macro")
		}

		// .const NAME = value
		if strings.HasPrefix(trimmed, ".const ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ".const "))
			eqIdx := strings.Index(rest, "=")
			if eqIdx < 0 {
				return "", fmt.Errorf(".const requires '='")
			}
			name := strings.TrimSpace(rest[:eqIdx])
			value := strings.TrimSpace(rest[eqIdx+1:])
			if name == "" {
				return "", fmt.Errorf(".const requires a name")
			}
			ctx.defines[name] = true
			ctx.consts[name] = value
			continue
		}

		// .include "path" or .include <pkg>
		if incPath, ok := parseInclude(trimmed); ok {
			content, err := ctx.loadInclude(incPath)
			if err != nil {
				return "", fmt.Errorf("include error: %v", err)
			}
			out.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				out.WriteString("\n")
			}
			continue
		}

		// .once
		if trimmed == ".once" {
			continue
		}

		// .include_bytes "path"
		if strings.HasPrefix(trimmed, ".include_bytes ") {
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, ".include_bytes "))
			path := ""
			if len(rest) >= 2 && rest[0] == '"' {
				end := strings.IndexByte(rest[1:], '"')
				if end >= 0 {
					path = rest[1 : end+1]
				}
			}
			if path == "" {
				return "", fmt.Errorf(".include_bytes requires a quoted path")
			}
			resolvedPath := path
			if !filepath.IsAbs(path) {
				resolvedPath = filepath.Join(ctx.dir, path)
			}
			data, err := os.ReadFile(resolvedPath)
			if err != nil {
				return "", fmt.Errorf(".include_bytes: %v", err)
			}
			hex := bytesToDB(data)
			out.WriteString(hex)
			out.WriteString("\n")
			continue
		}

		out.WriteString(line)
		out.WriteString("\n")
	}

	if ctx.inMacro {
		return "", fmt.Errorf("unclosed .macro for %s (missing .endm)", ctx.macroName)
	}
	if len(ctx.ifStack) > 0 {
		return "", fmt.Errorf("unclosed .ifdef (missing .endif)")
	}

	return out.String(), nil
}

// bytesToDB converts binary data to VAS db directives.
func bytesToDB(data []byte) string {
	var b strings.Builder
	b.WriteString("; .include_bytes\n")
	const cols = 16
	for i := 0; i < len(data); i += cols {
		end := i + cols
		if end > len(data) {
			end = len(data)
		}
		b.WriteString("\tdb ")
		for j := i; j < end; j++ {
			if j > i {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "0x%02x", data[j])
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ── Pass 2: macro expansion ───────────────────────────────────────────────

func (ctx *prepContext) expandMacros(input string) string {
	lines := strings.Split(input, "\n")
	var out strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Check if this line is a macro call
		firstWord := strings.Fields(trimmed)
		if len(firstWord) > 0 {
			if def, ok := ctx.macros[firstWord[0]]; ok {
				// Extract arguments (rest of line)
				args := strings.Fields(strings.TrimSpace(strings.TrimPrefix(trimmed, firstWord[0])))
				for i := range args {
					args[i] = strings.Trim(args[i], ",")
				}
				if len(args) < len(def.params) {
					// Pad missing args with empty
					for len(args) < len(def.params) {
						args = append(args, "")
					}
				}
				// Expand body
				ctx.labelCounter++
				for _, bodyLine := range def.body {
					expanded := bodyLine
					for i, p := range def.params {
						if i < len(args) {
							expanded = strings.ReplaceAll(expanded, "\\"+p, args[i])
						}
					}
					expanded = strings.ReplaceAll(expanded, "\\@", fmt.Sprintf("_%d", ctx.labelCounter))
					out.WriteString(expanded)
					out.WriteString("\n")
				}
				continue
			}
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

// ── Pass 3: const replacement ─────────────────────────────────────────────

func (ctx *prepContext) applyConsts(input string) string {
	if len(ctx.consts) == 0 {
		return input
	}
	// Replace longest names first to avoid partial substitution
	type kv struct{ k, v string }
	var sorted []kv
	for k, v := range ctx.consts {
		sorted = append(sorted, kv{k, v})
	}
	// Sort by length descending (longest match first)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if len(sorted[j].k) > len(sorted[i].k) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	result := input
	for _, kv := range sorted {
		result = strings.ReplaceAll(result, kv.k, kv.v)
	}
	return result
}

// ── Include helpers ──────────────────────────────────────────────────────

func parseInclude(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), ".include")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], true
		}
	}
	if len(rest) >= 2 && rest[0] == '<' {
		end := strings.IndexByte(rest[1:], '>')
		if end >= 0 {
			return rest[1 : end+1], true
		}
	}
	return "", false
}

func (ctx *prepContext) loadInclude(path string) (string, error) {
	if ctx.included[path] {
		return "", nil
	}
	var content string
	var resolvedDir string

	if !filepath.IsAbs(path) {
		candidate := filepath.Join(ctx.dir, path)
		if data, err := os.ReadFile(candidate); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(candidate)
			goto found
		}
	}
	if filepath.IsAbs(path) {
		if data, err := os.ReadFile(path); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(path)
			goto found
		}
	}
	if ctx.pkgDir != "" {
		candidate := filepath.Join(ctx.pkgDir, path, path+".vas")
		if data, err := os.ReadFile(candidate); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(candidate)
			goto found
		}
		if strings.Contains(path, "/") || strings.Contains(path, "\\") {
			candidate = filepath.Join(ctx.pkgDir, path)
			if data, err := os.ReadFile(candidate); err == nil {
				content = string(data)
				resolvedDir = filepath.Dir(candidate)
				goto found
			}
		}
	}
	for _, searchDir := range ctx.vasPath {
		candidate := filepath.Join(searchDir, path)
		if data, err := os.ReadFile(candidate); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(candidate)
			goto found
		}
	}
	return "", fmt.Errorf("%q not found in search path", path)

found:
	ctx.included[path] = true
	return ctx.resolve(content, resolvedDir)
}
