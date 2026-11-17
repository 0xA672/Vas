package vas_test

import (
	"strings"
	"testing"

	"vas/vas"
)

func TestStandaloneELFWithUserMain(t *testing.T) {
	out, err := vas.AssembleStandalone("main:\nMOVI v0, 0\nRET")
	if err != nil {
		t.Fatalf("AssembleStandalone: %v", err)
	}
	if !strings.Contains(out, "_start:") {
		t.Errorf("ELF64 output with user main has no _start:\n%s", out)
	}
	if !strings.Contains(out, "call\tmain") {
		t.Errorf("_start does not call user main:\n%s", out)
	}
}

func TestCollectMemRefsIgnoresQuotedBrackets(t *testing.T) {
	out, err := vas.AssembleStandalone(`db "hello [world]", 0` + "\n" + `LOAD v0, [real]`)
	if err != nil {
		t.Fatalf("AssembleStandalone: %v", err)
	}
	if strings.Contains(out, "world:") {
		t.Errorf("quoted bracket produced a bogus data label:\n%s", out)
	}
	if !strings.Contains(out, "real:") {
		t.Errorf("real memory reference was not allocated:\n%s", out)
	}
}
