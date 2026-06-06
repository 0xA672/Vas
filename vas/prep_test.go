package vas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInclude(t *testing.T) {
	tests := []struct {
		line    string
		want    string
		wantOK  bool
	}{
		{`.include "foo.vas"`, "foo.vas", true},
		{`.include "std/io.vas"`, "std/io.vas", true},
		{`.include <io>`, "io", true},
		{`.include <pkg/sub.vas>`, "pkg/sub.vas", true},
		{`    .include "lib.vas"`, "lib.vas", true},
		{`MOVI v0, 1`, "", false},
		{`.include`, "", false},
		{`.inc`, "", false},
		{`; .include "comment"`, "", false},
	}
	for _, tt := range tests {
		got, ok := parseInclude(tt.line)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("parseInclude(%q) = (%q, %v), want (%q, %v)", tt.line, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestPreprocessBasic(t *testing.T) {
	// Create a temp directory with include files
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.vas")
	os.WriteFile(lib, []byte("; lib\n.once\nADD v0, v0, v1\nRET\n"), 0644)

	src := `.include "lib.vas"
MAIN:
  CALL lib_func
  RET`

	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "; lib") {
		t.Errorf("expected inlined lib content, got:\n%s", got)
	}
	if !strings.Contains(got, "MAIN:") {
		t.Errorf("expected MAIN label preserved, got:\n%s", got)
	}
	// .once should be stripped
	if strings.Contains(got, ".once") {
		t.Errorf(".once should be stripped, got:\n%s", got)
	}
	// .include should be stripped
	if strings.Contains(got, ".include") {
		t.Errorf(".include should be stripped, got:\n%s", got)
	}
}

func TestPreprocessMultipleInclude(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.vas"), []byte("; a\n.once\nADD v0, v0, v1\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.vas"), []byte("; b\n.once\nSUB v0, v0, v1\n"), 0644)

	src := `.include "a.vas"
.include "b.vas"
MAIN:
  RET`

	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "; a") {
		t.Errorf("expected a.vas content, got:\n%s", got)
	}
	if !strings.Contains(got, "; b") {
		t.Errorf("expected b.vas content, got:\n%s", got)
	}
	if !strings.Contains(got, "MAIN:") {
		t.Errorf("expected MAIN, got:\n%s", got)
	}
}

func TestPreprocessDedup(t *testing.T) {
	// Including the same file twice should only include it once
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lib.vas"), []byte("; lib\n.once\nNOP\n"), 0644)

	src := `.include "lib.vas"
.include "lib.vas"
MAIN:
  RET`

	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	// "; lib" should appear only once
	if strings.Count(got, "; lib") != 1 {
		t.Errorf("expected lib content once, got %d occurrences:\n%s", strings.Count(got, "; lib"), got)
	}
}

func TestPreprocessRecursive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.vas"), []byte(`.include "b.vas"
; a
`), 0644)
	os.WriteFile(filepath.Join(dir, "b.vas"), []byte(`.include "c.vas"
; b
`), 0644)
	os.WriteFile(filepath.Join(dir, "c.vas"), []byte("; c\n.once\n"), 0644)

	src := `.include "a.vas"
MAIN:
  RET`

	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	for _, want := range []string{"; c", "; b", "; a", "MAIN:"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output:\n%s", want, got)
		}
	}
}

func TestPreprocessNotFound(t *testing.T) {
	_, err := Preprocess(`.include "nonexistent.vas"`, "/tmp")
	if err == nil {
		t.Fatal("expected error for missing include file")
	}
}

func TestPreprocessOnce(t *testing.T) {
	// .once without include — should just be stripped
	got, err := Preprocess(".once\nNOP\n", "/tmp")
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, ".once") {
		t.Errorf(".once should be stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "NOP") {
		t.Errorf("NOP should remain, got:\n%s", got)
	}
}

func TestPreprocessNoInclude(t *testing.T) {
	// Input without any .include should pass through unchanged
	src := "MOVI v0, 42\nRET\n"
	got, err := Preprocess(src, "/tmp")
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if got != src+"\n" {
		t.Errorf("expected unchanged:\n%q\ngot:\n%q", src, got)
	}
}
