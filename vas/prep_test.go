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
		got, _, ok := parseInclude(tt.line)
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

func TestParseIncludePathTraversal(t *testing.T) {
	tests := []struct {
		line   string
		want   string
		wantOK bool
	}{
		{`.include "../secret.vas"`, "../secret.vas", true},
		{`.include "./local.vas"`, "./local.vas", true},
	}
	for _, tt := range tests {
		got, _, ok := parseInclude(tt.line)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("parseInclude(%q) = (%q, %v), want (%q, %v)", tt.line, got, ok, tt.want, tt.wantOK)
		}
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

func TestConstUndefined(t *testing.T) {
	src := "MOVI v0, UNDEFINED_CONST\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("unexpected error for undefined constant: %v", err)
	}
	if !strings.Contains(got, "UNDEFINED_CONST") {
		t.Errorf("expected UNDEFINED_CONST to remain, got:\n%s", got)
	}
}

func TestConstRedefinition(t *testing.T) {
	src := ".const A = 1\n.const A = 2\nMOVI v0, A\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: unexpected error: %v", err)
	}
	if strings.Contains(got, ".const") {
		t.Errorf(".const lines should be stripped")
	}
	if !strings.Contains(got, "MOVI v0, 2") {
		t.Errorf("expected A=2 after redefinition, got:\n%s", got)
	}
}

func TestConstString(t *testing.T) {
	src := `.const MSG = "hello"`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, ".const") {
		t.Errorf(".const line should be stripped, got:\n%s", got)
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

func TestMacroArgMismatch(t *testing.T) {
	src := `.macro add a, b
ADD \a, \b
.endm
add v0
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Fatal("expected error for macro argument mismatch")
	}
}

func TestMacroWithIfdefInside(t *testing.T) {
	src := `.macro debug_cond cond
.ifdef DEBUG
MOVI v0, \cond
.endif
.endm
.const DEBUG = 1
debug_cond 42
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("expected expanded macro with ifdef inside, got:\n%s", got)
	}
}

func TestMacroQuotedArgs(t *testing.T) {
	src := `.macro greet msg
  db \msg, 0
.endm
greet "hello, world"
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "db \"hello, world\", 0") {
		t.Errorf("expected quoted argument preserved, got:\n%s", got)
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

func TestIfndefElse(t *testing.T) {
	src := ".ifndef DEBUG\nMOVI v0, 1\n.else\nMOVI v1, 2\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected ifndef branch, got:\n%s", got)
	}
	if strings.Contains(got, "MOVI v1, 2") {
		t.Errorf("else branch should not appear, got:\n%s", got)
	}
}

func TestIfdefNestedWithElse(t *testing.T) {
	src := ".const A = 1\n.ifdef A\n.ifdef B\nNOP\n.else\nMOVI v0, 1\n.endif\n.endif\n"
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected nested else branch, got:\n%s", got)
	}
	if strings.Contains(got, "NOP") {
		t.Errorf("NOP should not appear (B not defined), got:\n%s", got)
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
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("const not resolved, got:\n%s", got)
	}
	if !strings.Contains(got, "LEA v4, [msg]") {
		t.Errorf("macro not expanded, got:\n%s", got)
	}
	for _, directive := range []string{".macro", ".const", ".include", ".ifdef", ".endm"} {
		if strings.Contains(got, directive) {
			t.Errorf("preprocessor directive %q should be stripped, got:\n%s", directive, got)
		}
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

// ── .once begin/end tests ────────────────────────────────────────────────

func TestPreprocessOnceBlockBasic(t *testing.T) {
	src := `.once begin constants
.const SYS_write = 1
.const BUFFER_SIZE = 1024
.once end constants

MOVI v0, SYS_write
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected const to be resolved in block, got:\n%s", got)
	}
	if strings.Contains(got, ".once begin") || strings.Contains(got, ".once end") {
		t.Errorf(".once begin/end should be stripped, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockUnclosed(t *testing.T) {
	src := `.once begin unclosed
NOP
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Fatal("expected error for unclosed .once begin block")
	}
}

func TestPreprocessOnceBlockCrossFileDedup(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lib.vas"), []byte(".once begin lib_init\nNOP\n.once end lib_init\n"), 0644)

	src := `.include "lib.vas"
.once begin lib_init
MOVI v0, 42
.once end lib_init
`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("second occurrence of block across files should be skipped, got:\n%s", got)
	}
	if strings.Count(got, "NOP") != 1 {
		t.Errorf("expected lib block once, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockDedup(t *testing.T) {
	src := `.once begin utils
.const UTILS_LOADED = 1
.once end utils

; Some code here
MOVI v0, UTILS_LOADED

.once begin utils
.const SHOULD_NOT_APPEAR = 999
.once end utils

MAIN:
  RET
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected first utils block const to be resolved, got:\n%s", got)
	}
	if strings.Contains(got, "SHOULD_NOT_APPEAR") || strings.Contains(got, "999") {
		t.Errorf("second utils block should be skipped, got:\n%s", got)
	}
	if !strings.Contains(got, "MAIN:") {
		t.Errorf("expected code between blocks, got:\n%s", got)
	}
}

func TestIncludeBytesEmpty(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "empty.bin")
	os.WriteFile(bin, []byte{}, 0644)
	src := `.include_bytes "` + bin + `"`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, "db") {
		t.Errorf("empty file should not produce db directive, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockNested(t *testing.T) {
	src := `.once begin outer
.const A = 1

.once begin inner
.const B = 2
.once end inner

.const C = 3
.once end outer

MOVI v0, A
MOVI v1, B
MOVI v2, C

.once begin outer
.const D = 4
.once end outer
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v2, 3") {
		t.Errorf("expected first outer block consts, got:\n%s", got)
	}
	if !strings.Contains(got, "MOVI v1, 2") {
		t.Errorf("expected inner block const (first occurrence), got:\n%s", got)
	}
	if strings.Contains(got, "D") || strings.Contains(got, "4") {
		lines := strings.Split(got, "\n")
		found := false
		for _, l := range lines {
			if strings.Contains(l, "MOVI") && strings.Contains(l, "4") {
				found = true
				break
			}
		}
		if found {
			t.Errorf("second outer block should be skipped, got:\n%s", got)
		}
	}
}

func TestPreprocessOnceBlockDeepNesting(t *testing.T) {
	src := `.once begin level1
.const A = 1

.once begin level2
.const B = 2

.once begin level3
.const C = 3
.once end level3

.const D = 4
.once end level2

.const E = 5
.once end level1

MOVI v0, A
MOVI v1, B
MOVI v2, C
MOVI v3, D
MOVI v4, E

; Second occurrence - all nested blocks should be skipped
.once begin level1
.const SHOULD_NOT_APPEAR = 999
.once end level1
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v1, 2") ||
		!strings.Contains(got, "MOVI v2, 3") || !strings.Contains(got, "MOVI v3, 4") ||
		!strings.Contains(got, "MOVI v4, 5") {
		t.Errorf("expected all nested blocks in first occurrence with constants resolved, got:\n%s", got)
	}
	if strings.Contains(got, "SHOULD_NOT_APPEAR") || strings.Contains(got, "999") {
		t.Errorf("second level1 block should be skipped, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockMismatch(t *testing.T) {
	src := `.once begin foo
NOP
.once end bar
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Error("expected error for mismatched block names")
	}
	if !strings.Contains(err.Error(), "name mismatch") {
		t.Errorf("expected 'name mismatch' error, got: %v", err)
	}
}

func TestPreprocessOnceBlockUnmatchedEnd(t *testing.T) {
	src := `.once end orphan
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Error("expected error for unmatched .once end")
	}
	if !strings.Contains(err.Error(), "without matching") {
		t.Errorf("expected 'without matching' error, got: %v", err)
	}
}

func TestPreprocessOnceBlockNoName(t *testing.T) {
	src := `.once begin
NOP
.once end
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Error("expected error for .once begin without name")
	}
	if !strings.Contains(err.Error(), "requires a block name") && !strings.Contains(err.Error(), "without matching") {
		t.Errorf("expected error about missing name or unmatched end, got: %v", err)
	}
}

func TestPreprocessOnceBlockWithInclude(t *testing.T) {
	dir := t.TempDir()
	libContent := `.once begin lib_consts
.const LIB_VERSION = 1
.once end lib_consts

.once begin lib_macros
.macro lib_func
  NOP
.endm
.once end lib_macros
`
	os.WriteFile(filepath.Join(dir, "lib.vas"), []byte(libContent), 0644)

	src := `.include "lib.vas"
MOVI v0, LIB_VERSION

; Try to include again - blocks should be skipped
.include "lib.vas"
`
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected const from first inclusion, got:\n%s", got)
	}
	if strings.Count(got, "LIB_VERSION") > 1 {
		t.Errorf("file should only be included once, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockMixedWithIfdef(t *testing.T) {
	src := `.const ENABLE_FEATURE = 1

.once begin feature_block
.ifdef ENABLE_FEATURE
.const FEATURE_ENABLED = 1
.else
.const FEATURE_DISABLED = 1
.endif
.once end feature_block

MOVI v0, FEATURE_ENABLED

; Second occurrence should be skipped even if ifdef condition changes
.const ENABLE_FEATURE = 0
.once begin feature_block
.ifdef ENABLE_FEATURE
.const SHOULD_NOT_APPEAR = 999
.endif
.once end feature_block
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected first block to be included with const resolved to 1, got:\n%s", got)
	}
	if strings.Contains(got, "SHOULD_NOT_APPEAR") || strings.Contains(got, "999") {
		t.Errorf("second block should be skipped regardless of ifdef, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockEmptyBlock(t *testing.T) {
	src := `.once begin empty_block
.once end empty_block

.once begin empty_block
.once end empty_block
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if strings.Contains(got, ".once") {
		t.Errorf(".once directives should be stripped, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockWithRept(t *testing.T) {
	src := `.once begin rept_block
.rept 3
DB 0xFF
.endr
.once end rept_block

.once begin rept_block
.rept 3
DB 0x00
.once end rept_block
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	count := strings.Count(got, "0xFF")
	if count != 3 {
		t.Errorf("expected 3 occurrences of 0xFF, got %d:\n%s", count, got)
	}
	if strings.Contains(got, "0x00") {
		t.Errorf("second rept block should be skipped, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockMultipleBlocksSameFile(t *testing.T) {
	src := `.once begin block_a
.const A_VAL = 10
.once end block_a

.once begin block_b
.const B_VAL = 20
.once end block_b

.once begin block_c
.const C_VAL = 30
.once end block_c

; Duplicate all three blocks
.once begin block_a
.const A_DUP = 100
.once end block_a

.once begin block_b
.const B_DUP = 200
.once end block_b

.once begin block_c
.const C_DUP = 300
.once end block_c

; Use constants
MOVI v0, A_VAL
MOVI v1, B_VAL
MOVI v2, C_VAL
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 10") || !strings.Contains(got, "MOVI v1, 20") || !strings.Contains(got, "MOVI v2, 30") {
		t.Errorf("expected all first blocks to be included, got:\n%s", got)
	}
	if strings.Contains(got, "A_DUP") || strings.Contains(got, "B_DUP") || strings.Contains(got, "C_DUP") {
		t.Errorf("duplicate blocks should be skipped, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockEndWithoutBeginInNested(t *testing.T) {
	src := `.once begin outer
.once end inner
`
	_, err := testPrep(t, src)
	if err == nil {
		t.Error("expected error for .once end with wrong name in nested context")
	}
	if !strings.Contains(err.Error(), "name mismatch") {
		t.Errorf("expected 'name mismatch' error, got: %v", err)
	}
}

func TestPreprocessOnceBlockWhitespaceHandling(t *testing.T) {
	src := `.once begin spaced_name
.const VALUE = 42
.once end spaced_name

MOVI v0, VALUE
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("expected block content with const resolved to 42, got:\n%s", got)
	}
}

func TestPreprocessOnceBlockCaseSensitive(t *testing.T) {
	src := `.once begin MyBlock
.const A = 1
.once end MyBlock

MOVI v0, A

.once begin myblock
.const B = 2
.once end myblock

MOVI v1, B

.once begin MYBLOCK
.const C = 3
.once end MYBLOCK

MOVI v2, C
`
	got, err := testPrep(t, src)
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v1, 2") || !strings.Contains(got, "MOVI v2, 3") {
		t.Errorf("expected all case-variant blocks to be included (names are case-sensitive), got:\n%s", got)
	}
}

func TestIncludeFileRollbackOnError(t *testing.T) {
	dir := t.TempDir()

	brokenPath := filepath.Join(dir, "broken.vas")
	os.WriteFile(brokenPath, []byte(".ifdef X\nNOP\n"), 0644)

	src := `.include "broken.vas"
MOVI v0, 1
`
	_, err := Preprocess(src, dir)
	if err == nil {
		t.Fatal("expected error due to unclosed ifdef")
	}

	os.WriteFile(brokenPath, []byte("NOP\nNOP\n"), 0644)
	got, err := Preprocess(src, dir)
	if err != nil {
		t.Fatalf("Preprocess after fix: %v", err)
	}
	if strings.Count(got, "NOP") != 2 {
		t.Errorf("expected two NOPs from fixed file, got:\n%s", got)
	}
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected MOVI from main source, got:\n%s", got)
	}
}

func TestCircularIncludeDetection(t *testing.T) {
	dir := t.TempDir()

	aPath := filepath.Join(dir, "a.vas")
	os.WriteFile(aPath, []byte(`.include "b.vas"
MOVI v0, 1
`), 0644)

	bPath := filepath.Join(dir, "b.vas")
	os.WriteFile(bPath, []byte(`.include "a.vas"
MOVI v1, 2
`), 0644)

	src := `.include "a.vas"`
	_, err := Preprocess(src, dir)
	if err == nil {
		t.Fatal("expected circular include error, got nil")
	}
	if !strings.Contains(err.Error(), "circular include") {
		t.Errorf("expected 'circular include' error, got: %v", err)
	}
}

func TestCircularIncludeErrorMessage(t *testing.T) {
	dir := t.TempDir()

	aPath := filepath.Join(dir, "a.vas")
	os.WriteFile(aPath, []byte(`.include "b.vas"
MOVI v0, 1
`), 0644)

	bPath := filepath.Join(dir, "b.vas")
	os.WriteFile(bPath, []byte(`.include "a.vas"
MOVI v1, 2
`), 0644)

	src := `.include "a.vas"`
	_, err := Preprocess(src, dir)
	if err == nil {
		t.Fatal("expected circular include error, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "circular include detected:") {
		t.Errorf("expected 'circular include detected' message, got: %v", err)
	}
	if !strings.Contains(errMsg, "a.vas") || !strings.Contains(errMsg, "b.vas") {
		t.Errorf("expected both files in error message, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "cycle back to here") {
		t.Errorf("expected cycle indicator, got: %s", errMsg)
	}
}

// ── Package resolution & environment tests ─────────────────────────────────

func TestPkgCacheDirEnv(t *testing.T) {
	os.Setenv("VAS_PKG_CACHE", "/custom/vas/cache")
	defer os.Unsetenv("VAS_PKG_CACHE")
	if got := pkgCacheDir(); got != "/custom/vas/cache" {
		t.Errorf("expected /custom/vas/cache, got %s", got)
	}
}

func TestPkgCacheDirDefault(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".vas", "pkg")
	if got := pkgCacheDir(); got != want {
		t.Errorf("expected %s, got %s", want, got)
	}
}

func TestLoadPackageIncludeWithResolver(t *testing.T) {
	resolver := &mockResolver{src: "NOP\n"}

	src := `.include <test/mock>
RET
`
	got, err := Preprocess(src, "/tmp", WithResolver(resolver))
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "NOP") {
		t.Errorf("expected NOP from package, got:\n%s", got)
	}
	if !strings.Contains(got, "RET") {
		t.Errorf("expected RET from main source, got:\n%s", got)
	}
	if strings.Contains(got, ".include") {
		t.Errorf(".include directive should be stripped, got:\n%s", got)
	}
}

type mockResolver struct{ src string }

func (m *mockResolver) ResolvePackage(pkgPath string) (string, error) {
	return m.src, nil
}

func TestPackageNotFoundHint(t *testing.T) {
	_, err := Preprocess(`.include <nonexistent>`, "/tmp")
	if err == nil {
		t.Fatal("expected error")
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "vpk") {
		t.Errorf("error message should not contain 'vpk', got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "install it with your package manager") {
		t.Errorf("expected generic install hint, got: %s", errMsg)
	}
}

func TestPackageNotFoundWithPmEnv(t *testing.T) {
	os.Setenv("VAS_PM", "mypm")
	defer os.Unsetenv("VAS_PM")
	_, err := Preprocess(`.include <nonexistent>`, "/tmp")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "`mypm install") {
		t.Errorf("expected hint with 'mypm', got: %s", err.Error())
	}
}

// ── Platform define tests ─────────────────────────────────────────────────

func TestInitPlatformDefinesDefault(t *testing.T) {
	ctx := &prepContext{defines: map[string]bool{}}
	ctx.initPlatformDefines()
	foundOS, foundArch := false, false
	for k := range ctx.defines {
		if strings.HasPrefix(k, "__VAS_OS_") {
			foundOS = true
		}
		if strings.HasPrefix(k, "__VAS_ARCH_") {
			foundArch = true
		}
	}
	if !foundOS || !foundArch {
		t.Errorf("expected platform defines, got %v", ctx.defines)
	}
}

func TestInitPlatformDefinesCrossCompile(t *testing.T) {
	os.Setenv("GOOS", "plan9")
	os.Setenv("GOARCH", "mips")
	defer func() {
		os.Unsetenv("GOOS")
		os.Unsetenv("GOARCH")
	}()
	ctx := &prepContext{defines: map[string]bool{}}
	ctx.initPlatformDefines()
	if !ctx.defines["__VAS_OS_PLAN9__"] {
		t.Error("expected __VAS_OS_PLAN9__")
	}
	if !ctx.defines["__VAS_ARCH_MIPS__"] {
		t.Error("expected __VAS_ARCH_MIPS__")
	}
}

func TestInitPlatformDefinesEmptyEnv(t *testing.T) {
	os.Setenv("GOOS", "")
	os.Setenv("GOARCH", "")
	defer func() {
		os.Unsetenv("GOOS")
		os.Unsetenv("GOARCH")
	}()
	ctx := &prepContext{defines: map[string]bool{}}
	ctx.initPlatformDefines()
	if len(ctx.defines) < 2 {
		t.Errorf("expected at least OS+ARCH defines, got %d", len(ctx.defines))
	}
}
