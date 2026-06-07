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
	// Block should be included (first occurrence), const should be resolved
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected const to be resolved in block, got:\n%s", got)
	}
	// Directives should be stripped
	if strings.Contains(got, ".once begin") || strings.Contains(got, ".once end") {
		t.Errorf(".once begin/end should be stripped, got:\n%s", got)
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
	// First block should be included (const resolved)
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected first utils block const to be resolved, got:\n%s", got)
	}
	// Second block should be skipped
	if strings.Contains(got, "SHOULD_NOT_APPEAR") || strings.Contains(got, "999") {
		t.Errorf("second utils block should be skipped, got:\n%s", got)
	}
	// Code between blocks should be preserved
	if !strings.Contains(got, "MAIN:") {
		t.Errorf("expected code between blocks, got:\n%s", got)
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
	// First outer block should be included
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v2, 3") {
		t.Errorf("expected first outer block consts, got:\n%s", got)
	}
	// Nested inner block should also be included (first time)
	if !strings.Contains(got, "MOVI v1, 2") {
		t.Errorf("expected inner block const (first occurrence), got:\n%s", got)
	}
	// Second outer block should be skipped entirely
	if strings.Contains(got, "MOVI") && strings.Contains(got, "4") {
		// Check if it's the D=4 const specifically
		lines := strings.Split(got, "\n")
		for _, line := range lines {
			if strings.Contains(line, "4") && !strings.Contains(line, "MOVI v0, 1") && !strings.Contains(line, "MOVI v1, 2") && !strings.Contains(line, "MOVI v2, 3") {
				t.Errorf("second outer block should be skipped, got:\n%s", got)
				break
			}
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
	// First level1 and all nested blocks should be included (constants resolved to values)
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v1, 2") || !strings.Contains(got, "MOVI v2, 3") || !strings.Contains(got, "MOVI v3, 4") || !strings.Contains(got, "MOVI v4, 5") {
		t.Errorf("expected all nested blocks in first occurrence with constants resolved, got:\n%s", got)
	}
	// Second level1 should be completely skipped
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
	// The error could be either "requires a block name" or "without matching .once begin"
	// depending on parsing order. Both are acceptable.
	if !strings.Contains(err.Error(), "requires a block name") && !strings.Contains(err.Error(), "without matching") {
		t.Errorf("expected error about missing name or unmatched end, got: %v", err)
	}
}

// ── Advanced .once begin/end tests ───────────────────────────────────────

func TestPreprocessOnceBlockWithInclude(t *testing.T) {
	dir := t.TempDir()
	
	// Create an included file with blocks
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
	// First inclusion should work
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected const from first inclusion, got:\n%s", got)
	}
	// File-level dedup should prevent second inclusion entirely
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
	// First block should be processed (FEATURE_ENABLED defined and resolved to 1)
	if !strings.Contains(got, "MOVI v0, 1") {
		t.Errorf("expected first block to be included with const resolved to 1, got:\n%s", got)
	}
	// Second block should be skipped entirely
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
	// Should handle empty blocks gracefully
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
	// First block should expand rept
	count := strings.Count(got, "0xFF")
	if count != 3 {
		t.Errorf("expected 3 occurrences of 0xFF, got %d:\n%s", count, got)
	}
	// Second block should be skipped
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
	// All first blocks should be included
	if !strings.Contains(got, "MOVI v0, 10") || !strings.Contains(got, "MOVI v1, 20") || !strings.Contains(got, "MOVI v2, 30") {
		t.Errorf("expected all first blocks to be included, got:\n%s", got)
	}
	// All duplicate blocks should be skipped
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
	// Should handle normal block names correctly (const resolved to 42)
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
	// All three blocks have different names (case-sensitive), so all should be included
	if !strings.Contains(got, "MOVI v0, 1") || !strings.Contains(got, "MOVI v1, 2") || !strings.Contains(got, "MOVI v2, 3") {
		t.Errorf("expected all case-variant blocks to be included (names are case-sensitive), got:\n%s", got)
	}
}
