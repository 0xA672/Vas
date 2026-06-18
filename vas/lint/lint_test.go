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
	src := "ADD V3, V3, V3\nIDIV V1"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected violation because v3 was modified")
	}
}

func TestStackBalance(t *testing.T) {
	src := "PUSH v0\nPUSH v1\nPOP v0\nRET"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected stack imbalance warning")
	}
}

func TestUninitReg(t *testing.T) {
	src := "ADD v2, v1, v1\nMOVI v0, 60\nSYSCALL"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected uninitialized v1 warning")
	}
}

func TestCallerSave(t *testing.T) {
	src := "CALL func\nADD v2, v2, v2\nRET"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected caller-save v2 warning")
	}
}

func TestStoreByte(t *testing.T) {
	src := "STORE '0', [num_buf]"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected store byte warning")
	}
}

func TestCmpMemSize(t *testing.T) {
	src := "CMP byte [v0], 0"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected cmp memory size error")
	}
}

func TestInfiniteLoop(t *testing.T) {
	src := "loop:\nJMP loop"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected infinite loop warning")
	}
}

func TestInfiniteLoopWithSyscall(t *testing.T) {
	src := "loop:\nSYSCALL\nJMP loop"
	violations := Run(src)
	if len(violations) != 0 {
		t.Errorf("expected no warning for loop with syscall, got %v", violations)
	}
}

func TestNestedInfiniteLoop(t *testing.T) {
	src := "outer:\ninner:\nJMP inner\nSYSCALL\nJMP outer"
	violations := Run(src)
	if len(violations) == 0 {
		t.Error("expected infinite loop warning for inner loop")
	}
	// outer loop has a SYSCALL, so it should not be flagged
	// but inner is flagged
}
