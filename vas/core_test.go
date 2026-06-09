package vas

import (
	"strings"
	"testing"
)

func TestMapRegRespectsLabels(t *testing.T) {
	input := "v0:"
	got, err := mapReg(input)
	if err != nil {
		t.Fatalf("mapReg error: %v", err)
	}
	if got != "v0:" {
		t.Errorf("label should not be replaced, got %q", got)
	}
}

func TestMapRegSkipsQuotedStrings(t *testing.T) {
	// Single-quoted
	input1 := `db 'v0 should stay as v0', 0`
	got1, err := mapReg(input1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got1, "rax") {
		t.Errorf("v0 inside single quotes was replaced: %s", got1)
	}

	// Double-quoted
	input2 := `db "v12 is here"`
	got2, err := mapReg(input2)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got2, "r15") {
		t.Errorf("v12 inside double quotes was replaced: %s", got2)
	}
}

func TestMacroParamVRegNameFails(t *testing.T) {
	_, _, err := parseMacro(".macro test v0, v1")
	if err == nil {
		t.Fatal("expected error for using v0 as macro parameter name")
	}
	if !strings.Contains(err.Error(), "reserved virtual register") {
		t.Errorf("unexpected error: %v", err)
	}
}
