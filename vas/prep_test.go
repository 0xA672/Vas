// prep_test.go

package vas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseInclude(t *testing.T) {
	tests := []struct {
		line   string
		want   string
		wantOK bool
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

func testPrep(t *testing.T, src string) (string, error) {
	return Preprocess(src, "/tmp")
}

// ── .include tests ────────────────────────────────────────────────────────

func TestPreprocessBasic(t *testing.T) {
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
		t.Errorf("expected inlined lib, got:\n%s", got)
	}
	if strings.Contains(got, ".once") {
		t.Errorf(".once should be stripped, got:\n%s", got)
	}
	if strings.Contains(got, ".include") {
		t.Errorf(".include should be stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "MAIN:") {
		t.Errorf("expected MAIN, got:\n%s", got)
	}
}

func TestPreprocessDedup(t *testing.T) {
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
	if strings.Count(got, "; lib") != 1 {
		t.Errorf("expected lib once, got %d:\n%s", strings.Count(got, "; lib"), got)
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
		t.Fatal("expected error for missing include")
	}
}

func TestPreprocessOnce(t *testing.T) {
	got, err := testPrep(t, ".once\nNOP\n")
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
	src := "MOVI v0, 42\nRET\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if got != src+"\n" {
		t.Errorf("expected:\n%q\ngot:\n%q", src+"\n", got)
	}
}

// ── .const tests ──────────────────────────────────────────────────────────

func TestConst(t *testing.T) {
	src := ".const SYS_write = 1\nMOVI v0, SYS_write\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, ".const") {
		t.Errorf(".const line should be stripped, got:\n%s", got)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("SYS_write should be replaced with 1, got:\n%s", got)
	}
}

func TestConstMultiple(t *testing.T) {
	src := ".const A = 10\n.const B = 20\nMOVI v0, A\nMOVI v1, B\nADD v0, v0, v1\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 10") {
		t.Errorf("expected A=10, got:\n%s", got)
	}
	if !strings.Contains(got, "MOVI v1, 20") {
		t.Errorf("expected B=20, got:\n%s", got)
	}
}

// ── .macro tests ──────────────────────────────────────────────────────────

func TestMacroBasic(t *testing.T) {
	src := `.macro strlen ptr, len
  MOVI \len, 0
  ADD \len, \len, \ptr
.endm
strlen v0, v1
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, ".macro") {
		t.Errorf("macro def should be stripped")
	}
	if !strings.Contains(got, "MOVI v1, 0") {
		t.Errorf("expected expanded MOVI, got:\n%s", got)
	}
	if !strings.Contains(got, "ADD v1, v1, v0") {
		t.Errorf("expected expanded ADD, got:\n%s", got)
	}
}

func TestMacroUniqueLabels(t *testing.T) {
	src := `.macro myloop
.loop\@:
  NOP
.endm
myloop
myloop
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	// Should have two different labels
	if !strings.Contains(got, ".loop_1:") || !strings.Contains(got, ".loop_2:") {
		t.Errorf("expected unique labels .loop_1 and .loop_2, got:\n%s", got)
	}
}

func TestMacroMissingEndm(t *testing.T) {
	_, err := testPrep(t, ".macro foo\nNOP\n")
	if err == nil {
		t.Fatal("expected error for unclosed macro")
	}
}

func TestMacroOrphanEndm(t *testing.T) {
	_, err := testPrep(t, ".endm\n")
	if err == nil {
		t.Fatal("expected error for orphan .endm")
	}
}

// ── .ifdef / .else / .endif tests ────────────────────────────────────────

func TestIfdef(t *testing.T) {
	src := ".const DEBUG = 1\n.ifdef DEBUG\nMOVI v0, 42\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("expected MOVI inside ifdef, got:\n%s", got)
	}
}

func TestIfndef(t *testing.T) {
	src := ".ifndef DEBUG\nMOVI v0, 42\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("expected MOVI inside ifndef, got:\n%s", got)
	}
}

func TestIfdefSkip(t *testing.T) {
	src := ".ifdef UNDEFINED\nMOVI v0, 42\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("MOVI should be excluded when ifdef false, got:\n%s", got)
	}
}

func TestIfdefElse(t *testing.T) {
	src := ".ifdef DEBUG\nMOVI v0, 1\n.else\nMOVI v1, 2\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v1, 2") {
		t.Errorf("expected else branch, got:\n%s", got)
	}
	if strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("if branch should not appear, got:\n%s", got)
	}
}

func TestIfdefNested(t *testing.T) {
	src := ".ifdef A\n.ifdef B\nNOP\n.endif\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, "NOP") {
		t.Errorf("NOP should not appear (neither A nor B defined), got:\n%s", got)
	}
}

func TestIfdefOrphanEndif(t *testing.T) {
	_, err := testPrep(t, ".endif\n")
	if err == nil {
		t.Fatal("expected error for orphan .endif")
	}
}

func TestIfdefUnclosed(t *testing.T) {
	_, err := testPrep(t, ".ifdef X\nNOP\n")
	if err == nil {
		t.Fatal("expected error for unclosed ifdef")
	}
}

// ── .include_bytes tests ─────────────────────────────────────────────────

func TestIncludeBytes(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "data.bin")
	os.WriteFile(bin, []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F}, 0644)
	src := `.include_bytes "` + bin + `"`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "db") {
		t.Errorf("expected db directive, got:\n%s", got)
	}
	if !strings.Contains(got, "0x48") || !strings.Contains(got, "0x6f") {
		t.Errorf("expected hex bytes, got:\n%s", got)
	}
}

func TestIncludeBytesNotFound(t *testing.T) {
	_, err := testPrep(t, `.include_bytes "no.bin"`)
	if err == nil {
		t.Fatal("expected error for missing include_bytes")
	}
}

// ── Combined test ─────────────────────────────────────────────────────────

func TestPreprocessCombined(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "utils.vas"), []byte(`.const DEBUG = 1
.const SYS_WRITE = 1
.macro print_str ptr
  MOVI v0, SYS_WRITE
  MOVI v5, 1
  LEA v4, [\ptr]
  SYSCALL
.endm
`), 0644)

	src := `.include "utils.vas"
.ifdef DEBUG
print_str msg
.endif

.data
msg: db "hello", 0
`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	// .const should be resolved (SYS_WRITE → 1)
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("const not resolved, got:\n%s", got)
	}
	// .macro should be expanded
	if !strings.Contains(got, "LEA v4, [msg]") {
		t.Errorf("macro not expanded, got:\n%s", got)
	}
	// No preprocessor directives should remain
	if strings.Contains(got, ".macro") || strings.Contains(got, ".const") || strings.Contains(got, ".include") || strings.Contains(got, ".ifdef") || strings.Contains(got, ".endm") {
		t.Errorf("preprocessor directives should be stripped, got:\n%s", got)
	}
}

func TestPreprocessComboIfdefTrue(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "defs.vas"), []byte(".const DEBUG = 1\n"), 0644)
	src := `.include "defs.vas"
.ifdef DEBUG
NOP
.endif
`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "NOP") {
		t.Errorf("expected NOP (DEBUG defined via .const), got:\n%s", got)
	}
}
