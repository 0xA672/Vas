// standalone_test.go – integration tests for the standalone wrapper
package vas_test

import (
	"strings"
	"testing"

	"vas/vas"
)

// TestStandaloneAutoRet verifies that the ELF64 standalone wrapper
// automatically inserts a `ret` when the user code does not end with
// an explicit termination instruction.
func TestStandaloneAutoRet(t *testing.T) {
	// Code that does not end with ret → ret must be appended.
	src := "MOVI v0, 42"
	out, err := vas.AssembleStandalone(src)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\tret\n") {
		t.Errorf("expected ret to be added, output:\n%s", out)
	}

	// Code that already ends with ret → no duplicate ret.
	srcWithRet := "MOVI v0, 42\n\tret"
	out, err = vas.AssembleStandalone(srcWithRet)
	if err != nil {
		t.Fatal(err)
	}
	// The wrapper itself should not produce an extra ret in vas_main,
	// so the only ret comes from the user.
	if strings.Count(out, "\tret\n") != 1 {
		t.Errorf("expected exactly one ret (user's), got %d, output:\n%s",
			strings.Count(out, "\tret\n"), out)
	}

	// Code that ends with a syscall → no ret inserted.
	srcWithSyscall := "MOVI v0, 60\nSYSCALL"
	out, err = vas.AssembleStandalone(srcWithSyscall)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\tret\n") {
		t.Errorf("unexpected ret when code ends with syscall, output:\n%s", out)
	}
}

// TestStandaloneAutoRetEdgeCases covers boundary conditions where the
// last effective instruction is not a ret, including empty input and
// trailing comments.
func TestStandaloneAutoRetEdgeCases(t *testing.T) {
	// Empty code → ret must be added.
	out, err := vas.AssembleStandalone("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\tret\n") {
		t.Errorf("expected ret for empty code, output:\n%s", out)
	}

	// Code with a comment and blank lines; last instruction is not ret.
	srcWithComments := "; comment\nMOVI v0, 1\n\n"
	out, err = vas.AssembleStandalone(srcWithComments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\tret\n") {
		t.Errorf("expected ret after code with trailing comments/blanks, output:\n%s", out)
	}
}
