package lint

import (
	"testing"
)

func TestDivWithoutPreparation(t *testing.T) {
	src := "MOVI V1, 20\nIDIV V1"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected violation for idiv without rdx prep")
	}
}

func TestDivWithCQO(t *testing.T) {
	src := "CQO\nIDIV V1"
	violations := Run(src)
	if len(violations) != 0 {
		t.Errorf("expected no violation, got %v", violations)
	}
}

func TestDivWithXorV3(t *testing.T) {
	src := "XOR V3, V3\nDIV V1"
	violations := Run(src)
	if len(violations) != 0 {
		t.Errorf("expected no violation, got %v", violations)
	}
}

func TestDivWithInterruptingInstruction(t *testing.T) {
	src := "ADD V3, V3, V3\nIDIV V1" // v3 modified, not prepared
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected violation because v3 was modified")
	}
}
