package vas_test

import (
	"testing"

	"vas/vas"
)

func TestBasicAdd(t *testing.T) {
	input := `ADD v0, v1, v2`
	expected := "\tmov\trax, rdi\n\tadd\trax, rsi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestSub(t *testing.T) {
	input := `SUB v3, v4, v5`
	expected := "\tmov\trdx, rcx\n\tsub\trdx, r8"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestMul(t *testing.T) {
	input := `MUL v1, v2, v3`
	expected := "\tmov\trdi, rsi\n\timul\trdi, rdx"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestMul2Op(t *testing.T) {
	input := `MUL v0, v1`
	expected := "\timul\trax, rdi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestLoadReg(t *testing.T) {
	input := `LOAD v0, [v1+8]`
	expected := "\tmov\trax, [rdi+8]"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestLoadLabel(t *testing.T) {
	input := `LOAD v2, [myvar]`
	expected := "\tmov\trsi, [myvar]"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestLoadLabelOffset(t *testing.T) {
	input := `LOAD v2, [myvar+4]`
	expected := "\tmov\trsi, [myvar+4]"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestStoreReg(t *testing.T) {
	input := `STORE v0, [v1+4]`
	expected := "\tmov\t[rdi+4], rax"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestStoreLabel(t *testing.T) {
	input := `STORE v3, [result]`
	expected := "\tmov\t[result], rdx"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestMovi(t *testing.T) {
	input := `MOVI v0, 42`
	expected := "\tmov\trax, 42"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestMov(t *testing.T) {
	input := `MOV v1, v2`
	expected := "\tmov\trdi, rsi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestCmp(t *testing.T) {
	input := `CMP v0, v1`
	expected := "\tcmp\trax, rdi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestJump(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"JMP loop", "\tjmp\tloop"},
		{"JE done", "\tje\tdone"},
		{"JNE else", "\tjne\telse"},
		{"JG greater", "\tjg\tgreater"},
		{"JL lesser", "\tjl\tlesser"},
		{"JGE ge", "\tjge\tge"},
		{"JLE le", "\tjle\tle"},
	}
	for _, tt := range tests {
		got, err := vas.Assemble(tt.input)
		if err != nil {
			t.Errorf("Assemble(%q) unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("Assemble(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestCallRet(t *testing.T) {
	input := "CALL func\nRET"
	expected := "\tcall\tfunc\n\tret"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestPushPop(t *testing.T) {
	input := "PUSH v0\nPOP v1"
	expected := "\tpush\trax\n\tpop\trdi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestLabel(t *testing.T) {
	input := "loop:\nADD v0, v1, v2"
	expected := "loop:\n\tmov\trax, rdi\n\tadd\trax, rsi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestComments(t *testing.T) {
	input := "; This is a comment\nADD v0, v1, v2 ; inline comment"
	expected := "; This is a comment\n\tmov\trax, rdi\n\tadd\trax, rsi"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestHashComments(t *testing.T) {
	input := "# comment\nNOP"
	expected := "# comment\n\tnop"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestEmptyLines(t *testing.T) {
	input := "ADD v0, v1, v2\n\nSUB v0, v3"
	expected := "\tmov\trax, rdi\n\tadd\trax, rsi\n\n\tsub\trax, rdx"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func Test2OpForm(t *testing.T) {
	input := "ADD v0, v1\nSUB v2, v3\nMUL v4, v5"
	expected := "\tadd\trax, rdi\n\tsub\trsi, rdx\n\timul\trcx, r8"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestComplexMemory(t *testing.T) {
	input := `LOAD v1, [v2+v3*4+8]`
	expected := "\tmov\trdi, [rsi+rdx*4+8]"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestDirectivePassthrough(t *testing.T) {
	input := ".section .data\n.global main"
	expected := "\tsection .data\n\tglobal main"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestFullProgram(t *testing.T) {
	input := `.section .text
.global _start

_start:
    MOVI v0, 60       ; syscall number for exit
    MOVI v1, 0        ; exit code 0
    SYSCALL
`
	expected := "\tsection .text\n\tglobal _start\n\n_start:\n\tmov\trax, 60\n\tmov\trdi, 0\n\tsyscall\n"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestNop(t *testing.T) {
	input := "NOP"
	expected := "\tnop"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}

func TestSyscall(t *testing.T) {
	input := "SYSCALL"
	expected := "\tsyscall"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("got:\n%s\nwant:\n%s", got, expected)
	}
}
