package lint

import (
	"strings"
	"testing"
)

func TestUnsignedDivRejectsCQO(t *testing.T) {
	vs := Run("CQO\nDIV v1")
	found := false
	for _, v := range vs {
		if v.Severity == "error" && strings.Contains(v.Message, "DIV") {
			found = true
		}
	}
	if !found {
		t.Error("CQO must not count as preparation for unsigned DIV")
	}
}

func TestUninitReadBeforeLaterWrite(t *testing.T) {
	vs := Run("ADD v1, v1, v1\nMOVI v1, 0")
	found := false
	for _, v := range vs {
		if strings.Contains(v.Message, "uninitialized") && strings.Contains(v.Message, "v1") {
			found = true
		}
	}
	if !found {
		t.Error("expected uninitialized v1 warning before its later write")
	}
}

func TestCmpSizeDoesNotMatchSymbolSubstring(t *testing.T) {
	vs := Run("CMP v0, byte_count")
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("symbol byte_count was mistaken for a size prefix: %v", v)
		}
	}
}
