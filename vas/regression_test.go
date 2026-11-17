package vas

import (
	"strings"
	"testing"
)

func TestMapRegTokenBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v0_loop:", "v0_loop:"},
		{"JMP v0_loop", "JMP v0_loop"},
		{"[v0_table]", "[v0_table]"},
		{"av0", "av0"},
		{"v13_loop:", "v13_loop:"},
		{"v12x", "v12x"},
		{"db `v0`, 0", "db `v0`, 0"},
		{"MOVI v0, 1", "MOVI rax, 1"},
	}
	for _, tc := range cases {
		got, err := mapReg(tc.in)
		if err != nil {
			t.Errorf("mapReg(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("mapReg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPreprocessErrorInsideFalseConditional(t *testing.T) {
	got, err := Preprocess(".ifdef DEBUG\n.error \"should not fire\"\n.endif\nNOP\n", "/tmp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "NOP") {
		t.Errorf("expected NOP, got %q", got)
	}
}

func TestPreprocessMacroGeneratedDirective(t *testing.T) {
	src := `.macro m
.ifdef DEBUG
MOVI v0, 42
.endif
.endm
.const DEBUG = 1
m
`
	got, err := Preprocess(src, "/tmp")
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 42") {
		t.Errorf("macro-generated .ifdef was not evaluated:\n%s", got)
	}
	if strings.Contains(got, ".ifdef") || strings.Contains(got, ".endif") {
		t.Errorf("preprocessor directives leaked into output:\n%s", got)
	}
}

func TestPreprocessConstDoesNotTouchSingleQuotes(t *testing.T) {
	got, err := Preprocess(".const A = 1\ndb 'A', 0\n", "/tmp")
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "db 'A', 0") {
		t.Errorf("constant was substituted inside single-quoted string: %q", got)
	}
}

func TestPreprocessNestedMacroExpansion(t *testing.T) {
	src := `.macro inner x
MOVI v0, \x
.endm
.macro outer y
inner \y
.endm
outer 7
`
	got, err := Preprocess(src, "/tmp")
	if err != nil {
		t.Fatalf("Preprocess: %v", err)
	}
	if !strings.Contains(got, "MOVI v0, 7") {
		t.Errorf("nested macro invocation was not expanded:\n%s", got)
	}
}
