package vas_test

import (
	"strings"
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

func TestInt(t *testing.T) {
	got, err := vas.Assemble("INT 0x80")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tint\t0x80"
	if got != expected {
		t.Errorf("Assemble(INT 0x80) = %q, want %q", got, expected)
	}
}

func TestIntError(t *testing.T) {
	_, err := vas.Assemble("INT")
	if err == nil {
		t.Error("expected error for INT with no args, got nil")
	}
}

func TestCall(t *testing.T) {
	got, err := vas.Assemble("CALL myfunc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tcall\tmyfunc"
	if got != expected {
		t.Errorf("Assemble(CALL myfunc) = %q, want %q", got, expected)
	}
}

func TestRet(t *testing.T) {
	got, err := vas.Assemble("RET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tret"
	if got != expected {
		t.Errorf("Assemble(RET) = %q, want %q", got, expected)
	}
}

func Test3OpAddCommutative(t *testing.T) {
	// ADD dst, src1, dst where dst==src2: commutative, so just "add dst, src1"
	got, err := vas.Assemble("ADD v1, v0, v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tadd\trbx, rax"
	if got != expected {
		t.Errorf("Assemble(ADD v1, v0, v1) = %q, want %q", got, expected)
	}
}

func Test3OpSubNonCommutative(t *testing.T) {
	// SUB dst, src1, dst where dst==src2: non-commutative, save via r10
	got, err := vas.Assemble("SUB v1, v0, v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tmov\tr10, rbx\n\tmov\trbx, rax\n\tsub\trbx, r10"
	if got != expected {
		t.Errorf("Assemble(SUB v1, v0, v1) = %q, want %q", got, expected)
	}
}

func TestJumpError(t *testing.T) {
	_, err := vas.Assemble("JMP")
	if err == nil {
		t.Error("expected error for JMP with no args, got nil")
	}
}

func TestLoadError(t *testing.T) {
	_, err := vas.Assemble("LOAD v0")
	if err == nil {
		t.Error("expected error for LOAD with 1 arg, got nil")
	}
	_, err2 := vas.Assemble("LOAD v0, v1, v2")
	if err2 == nil {
		t.Error("expected error for LOAD with 3 args, got nil")
	}
}

func TestStoreError(t *testing.T) {
	_, err := vas.Assemble("STORE v0")
	if err == nil {
		t.Error("expected error for STORE with 1 arg, got nil")
	}
}

func TestCmpError(t *testing.T) {
	_, err := vas.Assemble("CMP v0")
	if err == nil {
		t.Error("expected error for CMP with 1 arg, got nil")
	}
}

func TestPushError(t *testing.T) {
	_, err := vas.Assemble("PUSH")
	if err == nil {
		t.Error("expected error for PUSH with no args, got nil")
	}
}

func TestPopError(t *testing.T) {
	_, err := vas.Assemble("POP")
	if err == nil {
		t.Error("expected error for POP with no args, got nil")
	}
}

func TestLeaError(t *testing.T) {
	_, err := vas.Assemble("LEA v0")
	if err == nil {
		t.Error("expected error for LEA with 1 arg, got nil")
	}
}

func TestGloblDirective(t *testing.T) {
	got, err := vas.Assemble(".globl main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tglobal main"
	if got != expected {
		t.Errorf("Assemble(.globl main) = %q, want %q", got, expected)
	}
}

func TestDataDirective(t *testing.T) {
	got, err := vas.Assemble(".data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tsection .data"
	if got != expected {
		t.Errorf("Assemble(.data) = %q, want %q", got, expected)
	}
}

func TestBssDirective(t *testing.T) {
	got, err := vas.Assemble(".bss")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tsection .bss"
	if got != expected {
		t.Errorf("Assemble(.bss) = %q, want %q", got, expected)
	}
}

func TestTextDirective(t *testing.T) {
	got, err := vas.Assemble(".text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tsection .text"
	if got != expected {
		t.Errorf("Assemble(.text) = %q, want %q", got, expected)
	}
}

func TestMoviHex(t *testing.T) {
	got, err := vas.Assemble("MOVI v0, 0xFF")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tmov\trax, 0xFF"
	if got != expected {
		t.Errorf("Assemble(MOVI v0, 0xFF) = %q, want %q", got, expected)
	}
}

func TestMoviNegative(t *testing.T) {
	got, err := vas.Assemble("MOVI v0, -1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "\tmov\trax, -1"
	if got != expected {
		t.Errorf("Assemble(MOVI v0, -1) = %q, want %q", got, expected)
	}
}

func TestEmptyInput(t *testing.T) {
	got, err := vas.Assemble("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("Assemble('') = %q, want ''", got)
	}
}

func TestWhitespaceInput(t *testing.T) {
	got, err := vas.Assemble("  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "  " {
		t.Errorf("Assemble('  ') = %q, want '  '", got)
	}
}

func TestLabelAndInstruction(t *testing.T) {
	input := "start:\nMOVI v0, 42"
	got, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "start:\n\tmov\trax, 42"
	if got != expected {
		t.Errorf("Assemble = %q, want %q", got, expected)
	}
}

func TestAssembleWithOptLevel1(t *testing.T) {
	// Dead MOVI should be eliminated at -O1
	input := "MOVI v0, 1\nMOVI v0, 60\nSYSCALL"
	got, err := vas.AssembleWithOpt(input, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "mov\trax, 1") {
		t.Errorf("dead MOVI v0,1 should be eliminated at -O1, got: %s", got)
	}
	if !strings.Contains(got, "mov\trax, 60") {
		t.Errorf("expected mov rax, 60 to remain, got: %s", got)
	}
	if !strings.Contains(got, "syscall") {
		t.Errorf("expected syscall to remain, got: %s", got)
	}
}

func TestAssembleStandaloneElf(t *testing.T) {
	input := "MOVI v0, 60\nSYSCALL"
	got, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "\tglobal _start") {
		t.Errorf("expected global _start in standalone ELF64 output")
	}
	if !strings.Contains(got, "_start:") {
		t.Errorf("expected _start label in standalone ELF64 output")
	}
	if !strings.Contains(got, "call\tvas_main") {
		t.Errorf("expected call vas_main in standalone ELF64 output")
	}
	if !strings.Contains(got, "vas_main:") {
		t.Errorf("expected vas_main label in standalone ELF64 output")
	}
	if !strings.Contains(got, "mov\teax, 60") {
		t.Errorf("expected exit syscall in standalone ELF64 output")
	}
}

func TestAssembleStandaloneWin64(t *testing.T) {
	input := "MOVI v0, 0\nRET"
	got, err := vas.AssembleStandaloneTarget(input, "win64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "\tglobal main") {
		t.Errorf("expected global main in standalone Win64 output")
	}
	if !strings.Contains(got, "main:") {
		t.Errorf("expected main label in standalone Win64 output")
	}
	if !strings.Contains(got, "ret") {
		t.Errorf("expected ret in standalone Win64 output")
	}
}

func TestAssembleStandaloneWithMemRefs(t *testing.T) {
	input := "LOAD v0, [result]\nSTORE v1, [buf+8]"
	got, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "result:\tdq 0") {
		t.Errorf("expected auto-generated result data entry, got: %s", got)
	}
	if !strings.Contains(got, "buf:\tdq 0") {
		t.Errorf("expected auto-generated buf data entry, got: %s", got)
	}
}
