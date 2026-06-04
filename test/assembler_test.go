package vas_test

import (
	"testing"
	"vas/vas"
)

func TestBasicAdd(t *testing.T) {
	input := `ADD v0, v1, v2`
	expected := "\tmov\trax, rbx\n\tadd\trax, rcx"
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
	expected := "\tmov\trdx, rsi\n\tsub\trdx, rdi"
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
	expected := "\tmov\trbx, rcx\n\timul\trbx, rdx"
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
	expected := "\timul\trax, rbx"
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
	expected := "\tmov\trax, [rbx+8]"
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
	expected := "\tmov\trcx, [myvar]"
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
	expected := "\tmov\trcx, [myvar+4]"
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
	expected := "\tmov\t[rbx+4], rax"
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
	expected := "\tmov\trbx, rcx"
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
	expected := "\tcmp\trax, rbx"
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
	expected := "\tpush\trax\n\tpop\trbx"
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
	expected := "loop:\n\tmov\trax, rbx\n\tadd\trax, rcx"
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
	expected := "; This is a comment\n\tmov\trax, rbx\n\tadd\trax, rcx"
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
	expected := "\tmov\trax, rbx\n\tadd\trax, rcx\n\n\tsub\trax, rdx"
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
	expected := "\tadd\trax, rbx\n\tsub\trcx, rdx\n\timul\trsi, rdi"
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
	expected := "\tmov\trbx, [rcx+rdx*4+8]"
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
	expected := "\tsection .text\n\tglobal _start\n\n_start:\n\tmov\trax, 60\n\tmov\trbx, 0\n\tsyscall\n"
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

// Out-of-bounds virtual register checks

func TestRegOutOfBoundsAdd(t *testing.T) {
	_, err := vas.Assemble("ADD v0, v1, v13")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsSub(t *testing.T) {
	_, err := vas.Assemble("SUB v13, v0, v1")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsLoad(t *testing.T) {
	_, err := vas.Assemble("LOAD v13, [msg]")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsStore(t *testing.T) {
	_, err := vas.Assemble("STORE v13, [msg]")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsMovi(t *testing.T) {
	_, err := vas.Assemble("MOVI v13, 42")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsMov(t *testing.T) {
	_, err := vas.Assemble("MOV v13, v0")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsCmp(t *testing.T) {
	_, err := vas.Assemble("CMP v0, v13")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsPush(t *testing.T) {
	_, err := vas.Assemble("PUSH v13")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsPop(t *testing.T) {
	_, err := vas.Assemble("POP v13")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsLea(t *testing.T) {
	_, err := vas.Assemble("LEA v13, [v0+1]")
	if err == nil {
		t.Fatal("expected error for v13, got nil")
	}
}

func TestRegOutOfBoundsV13(t *testing.T) {
 _, err := vas.Assemble("MOVI v13, 1")
 if err == nil {
  t.Fatal("expected error for v13, got nil")
 }
}

func TestRegOutOfBoundsV14(t *testing.T) {
 _, err := vas.Assemble("MOVI v14, 1")
 if err == nil {
  t.Fatal("expected error for v14, got nil")
 }
}

func TestRegOutOfBoundsLabel(t *testing.T) {
	_, err := vas.Assemble("v13:\n    NOP")
	if err == nil {
		t.Fatal("expected error for v13 label, got nil")
	}
}

func TestRegInBounds(t *testing.T) {
	// v0-v12 should all work
	for _, r := range []string{"v0", "v1", "v2", "v3", "v4", "v5", "v6", "v7", "v8", "v9", "v10", "v11", "v12"} {
		input := "MOVI " + r + ", 1"
		_, err := vas.Assemble(input)
		if err != nil {
			t.Errorf("Assemble(%q) unexpected error: %v", input, err)
		}
	}
}
