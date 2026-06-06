// Package vas provides the VAS virtual assembler core.
package vas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prepContext tracks state during .include resolution.
type prepContext struct {
	// dir is the directory of the file currently being processed.
	dir string
	// included tracks files already processed (for .once / duplicate guard).
	included map[string]bool
	// pkgDir is the vpk package cache directory (default ~/.vpk/pkg/).
	pkgDir string
	// vasPath is the additional search path from VAS_PATH env var.
	vasPath []string
}

// Preprocess resolves .include directives and returns the flattened source.
// baseDir is the logical source directory (for relative .include resolution).
func Preprocess(src, baseDir string) (string, error) {
	ctx := &prepContext{
		dir:      baseDir,
		included: map[string]bool{},
		pkgDir:   pkgCacheDir(),
		vasPath:  searchPath(),
	}
	return ctx.resolve(src, baseDir)
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

// resolve processes a single file's content, inlining .include directives.
func (ctx *prepContext) resolve(input, baseDir string) (string, error) {
	lines := strings.Split(input, "\n")
	var out strings.Builder
	savedDir := ctx.dir
	ctx.dir = baseDir
	defer func() { ctx.dir = savedDir }()

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// .include "path"
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
			continue // skip; dedup handled by ctx.included
		}

		out.WriteString(line)
		out.WriteString("\n")
	}

	return out.String(), nil
}

// parseInclude checks if a line is a .include directive and extracts the path.
func parseInclude(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, ".include")
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)

	// "quoted path"
	if len(rest) >= 2 && rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end >= 0 {
			return rest[1 : end+1], true
		}
	}

	// <pkg> or <pkg/file>
	if len(rest) >= 2 && rest[0] == '<' {
		end := strings.IndexByte(rest[1:], '>')
		if end >= 0 {
			return rest[1 : end+1], true
		}
	}

	return "", false
}

// loadInclude loads a file by path, with search order:
//  1. Relative to the including file's directory
//  2. VPK package cache (<name> → ~/.vpk/pkg/<name>/<name>.vas)
//  3. VAS_PATH directories
func (ctx *prepContext) loadInclude(path string) (string, error) {
	if ctx.included[path] {
		return "", nil
	}

	var content string
	var resolvedDir string

	// 1. Relative to current directory
	if !filepath.IsAbs(path) {
		candidate := filepath.Join(ctx.dir, path)
		if data, err := os.ReadFile(candidate); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(candidate)
			goto found
		}
	}

	// 2. Absolute path
	if filepath.IsAbs(path) {
		if data, err := os.ReadFile(path); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(path)
			goto found
		}
	}

	// 3. VPK cache (<name> → ~/.vpk/pkg/<name>/<name>.vas)
	if ctx.pkgDir != "" {
		candidate := filepath.Join(ctx.pkgDir, path, path+".vas")
		if data, err := os.ReadFile(candidate); err == nil {
			content = string(data)
			resolvedDir = filepath.Dir(candidate)
			goto found
		}
		// Also try without the file part if path contains a slash
		if strings.Contains(path, "/") || strings.Contains(path, "\\") {
			candidate = filepath.Join(ctx.pkgDir, path)
			if data, err := os.ReadFile(candidate); err == nil {
				content = string(data)
				resolvedDir = filepath.Dir(candidate)
				goto found
			}
		}
	}

	// 4. VAS_PATH
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
