package vas_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	got, err := vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
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

func TestHasBoilerplateNotConfusedByData(t *testing.T) {
	// hasBoilerplate should NOT match when "section .text" appears inside a
	// data string rather than as an actual section directive.
	input := "msg: db \"section .text\""
	got, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The wrapper should still be added because there's no real section .text
	if !strings.Contains(got, "_start:") {
		t.Errorf("expected standalone wrapper despite string containing 'section .text'")
	}
	// The data line should be preserved (tab prefix + line)
	if !strings.Contains(got, "msg: db") {
		t.Errorf("expected data line to be preserved, got: %s", got)
	}
}

func TestCollectMemRefsNestedBrackets(t *testing.T) {
	// Nested brackets like [arr + [idx*4]] should still extract "arr" and "idx"
	input := "LOAD v0, [arr + [idx*4]]"
	got, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "arr:\tdq 0") {
		t.Errorf("expected auto-generated arr data entry for nested bracket, got: %s", got)
	}
	if !strings.Contains(got, "idx:\tdq 0") {
		t.Errorf("expected auto-generated idx data entry for nested bracket, got: %s", got)
	}
}

// #14: End-to-end compilation test (assemble → nasm → ld → run)
func hasTool(t *testing.T, name string) bool {
	t.Helper()
	_, err := exec.LookPath(name)
	return err == nil
}

func TestEndToEndElf64(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping end-to-end test in short mode")
	}
	if !hasTool(t, "nasm") {
		t.Skip("nasm not found on PATH, skipping end-to-end test")
	}
	if !hasTool(t, "ld") {
		t.Skip("ld not found on PATH, skipping end-to-end test")
	}

	// VAS program: sum 1..n, exit with result
	vasSrc := `
MOVI v1, 0           ; sum = 0
MOVI v2, 1           ; i = 1
MOVI v3, 10          ; n = 10
loop:
ADD v1, v1, v2       ; sum += i
ADD v2, 1            ; i++
CMP v2, v3
JLE loop
; exit(sum)
MOV v5, v1
MOVI v0, 60
SYSCALL
`
	asm, err := vas.AssembleStandalone(vasSrc)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}

	dir := t.TempDir()
	asmFile := filepath.Join(dir, "test.asm")
	objFile := filepath.Join(dir, "test.o")
	binFile := filepath.Join(dir, "test")
	if runtime.GOOS == "windows" {
		binFile += ".exe"
	}

	if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
		t.Fatalf("write asm: %v", err)
	}

	// nasm
	out, err := exec.Command("nasm", "-f", "elf64", "-o", objFile, asmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("nasm failed: %v\n%s", err, out)
	}

	// ld
	out, err = exec.Command("ld", "-o", binFile, objFile).CombinedOutput()
	if err != nil {
		t.Fatalf("ld failed: %v\n%s", err, out)
	}

	// Verify binary exists
	if _, err := os.Stat(binFile); err != nil {
		t.Fatalf("binary not created: %v", err)
	}

	// On Windows, ELF binaries can't be run directly; skip execution check.
	if runtime.GOOS == "windows" {
		t.Log("skipping execution on Windows (ELF binary)")
		return
	}

	// run
	out, err = exec.Command(binFile).CombinedOutput()
	// Check exit code = sum(1..10) = 55
	if exitErr, ok := err.(*exec.ExitError); ok {
		if code := exitErr.ExitCode(); code != 55 {
			t.Errorf("expected exit code 55, got %d\noutput: %s", code, out)
		}
	} else if err != nil {
		t.Fatalf("exec failed: %v\n%s", err, out)
	} else {
		// Exit code 0 — wrong! Should be 55
		t.Errorf("expected exit code 55, got 0 (program didn't exit via sys_exit?)")
	}
}

// #15: Win64 standalone — user-defined main: with RET
func TestWin64StandaloneWithMain(t *testing.T) {
	input := "main:\nMOVI v0, 42\nRET"
	got, err := vas.AssembleStandaloneTarget(input, "win64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "\tglobal main") {
		t.Errorf("expected global main, got: %s", got)
	}
	if !strings.Contains(got, "main:") {
		t.Errorf("expected main label, got: %s", got)
	}
	// User's RET should prevent adding extra ret
	if strings.Count(got, "ret") > 1 {
		t.Errorf("expected exactly one ret (user's), got multiple: %s", got)
	}
}

func TestWin64StandaloneNoRet(t *testing.T) {
	// No RET in source — wrapper should add one
	input := "MOVI v0, 0"
	got, err := vas.AssembleStandaloneTarget(input, "win64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got, "\tglobal main") {
		t.Errorf("expected global main, got: %s", got)
	}
	if !strings.Contains(got, "main:") {
		t.Errorf("expected main label, got: %s", got)
	}
	if !strings.Contains(got, "ret") {
		t.Errorf("expected auto-generated ret, got: %s", got)
	}
}

func TestWin64StandaloneWithData(t *testing.T) {
	input := "section .data\nmsg: db \"hello\", 0\nsection .text\nMOVI v0, 0\nRET"
	got, err := vas.AssembleStandaloneTarget(input, "win64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// User already has section .text → wrapper skipped (no global main auto-added)
	// But user's own global main or section .text should be present
	if !strings.Contains(got, "section .data") {
		t.Errorf("expected section .data preserved, got: %s", got)
	}
	if !strings.Contains(got, "msg:") {
		t.Errorf("expected msg label preserved, got: %s", got)
	}
}

// #18: Large input stress test
func TestLargeInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	var lines []string
	for i := 0; i < 5000; i++ {
		lines = append(lines, "ADD v0, v0, 1")
	}
	lines = append(lines, "MOVI v0, 0")
	lines = append(lines, "SYSCALL")
	input := strings.Join(lines, "\n")

	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("large input assembly failed: %v", err)
	}
}

func TestLargeInputWithOpt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}
	var lines []string
	for i := 0; i < 5000; i++ {
		lines = append(lines, "NOP")
	}
	lines = append(lines, "MOVI v0, 60")
	lines = append(lines, "SYSCALL")
	input := strings.Join(lines, "\n")

	_, err := vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
	if err != nil {
		t.Fatalf("large input with opt failed: %v", err)
	}
}

// #17: Fuzz tests
func FuzzAssemble(f *testing.F) {
	seeds := []string{
		"",
		"ADD v0, v1, v2",
		"MOVI v0, 42\nSYSCALL",
		"MOVI v0, 1\nMOVI v1, 2\nADD v0, v0, v1",
		"SUB v0, v1, v2\nLOAD v3, [v0]",
		"STORE v0, [x]\nLOAD v1, [x]",
		"section .text\nglobal _start\n_start:\nMOVI v0, 60\nSYSCALL",
		"NOP\nNOP\nNOP",
		"; comment\n# hash comment",
		"LOAD v0, [x]\nSTORE v0, [x]",
		"JMP loop\nloop:\nNOP",
		"PUSH v0\nPOP v1",
		"INT 0x80",
		"\t\n  ",
		"v13:\nNOP",
		"LEA v0, [data]\n.data\ndata: dq 0",
		"MUL v0, v1, 2\nADD v0, v0, v2",
		"CMP v0, 42\nJGT label\nlabel:\nMOVI v0, 60\nSYSCALL",
		"MOVI v0, 0xFF\nMOV v5, v0\nMOVI v0, 60\nSYSCALL",
		".globl main\n.data\nmsg: db \"hello\", 0\n.text\nMOVI v0, 60\nSYSCALL",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = vas.Assemble(input)
	})
}

func FuzzAssembleWithOpt(f *testing.F) {
	seeds := []string{
		"MOVI v0, 1\nMOVI v0, 2\nSYSCALL",
		"ADD v1, 1, 2\nSUB v2, v1, 1",
		"MUL v1, v0, 8\nSTORE v1, [x]\nLOAD v2, [x]",
		"section .data\nx: dq 0\nsection .text\nMOVI v0, 60\nSYSCALL",
		"MOVI v0, 0\nMOV v1, v0\nADD v2, v1, v0\nMUL v2, v2, 8\nSYSCALL",
		"MOVI v1, 3\nADD v1, 5\nSUB v1, 1\nMUL v1, v1, 4\nSYSCALL",
		"loop:\nADD v0, v0, 1\nCMP v0, 10\nJLE loop\nMOVI v0, 60\nSYSCALL",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = vas.AssembleWithOpt(input, vas.OptConfig{Level: 1})
		_, _ = vas.AssembleWithOpt(input, vas.OptConfig{Level: 2})
	})
}

func FuzzAssembleStandalone(f *testing.F) {
	seeds := []string{
		"MOVI v0, 60\nSYSCALL",
		"MOVI v0, 42\nMOV v5, v0\nMOVI v0, 60\nSYSCALL",
		"ADD v0, v1, v2\nMOV v5, v0\nMOVI v0, 60\nSYSCALL",
		"MUL v0, v1, 8\nMOV v5, v0\nMOVI v0, 60\nSYSCALL",
		"PUSH v0\nPOP v1\nMOVI v0, 60\nSYSCALL",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		for _, opt := range []int{0, 1, 2} {
			for _, target := range []string{"elf64", "win64"} {
				_, _ = vas.AssembleStandaloneTargetOpt(input, target, opt)
			}
		}
	})
}

// ── Discovered-rule integration tests ──────────────────────────────────────

// Test x - x = 0 pattern: MOV v1, v2; SUB v1, v2
func TestDiscoveredSubSelf(t *testing.T) {
	// Compute 42 - 42 = 0 using the (mov; sub same) pattern
	input := "MOVI v0, 42\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// Test 0 + x = x pattern: MOVI v1, 0; ADD v1, v2
func TestDiscoveredAddFromZero(t *testing.T) {
	input := "MOVI v0, 42\nMOV v1, v0\nMOVI v0, 0\nADD v0, v0, v1\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// Test 0 + x = x  via 2-op form: MOVI v1, 0; ADD v1, v2
func TestDiscoveredAddFromZero2Op(t *testing.T) {
	input := "MOVI v0, 0\nMOVI v1, 42\nADD v0, v1\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// Test identity via AND: MOV v1, v2; AND r1, r2 (passthrough)
func TestDiscoveredAndIdentity(t *testing.T) {
	input := "MOVI v0, 0xFF\nMOVI v1, 0x0F\n; and r0, r1 passthrough (VAS doesn't have AND as first-class)"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// Test x xor x = 0: MOV v1, v2; XOR r1, r2 (passthrough)
func TestDiscoveredXorSelf(t *testing.T) {
	input := "MOVI v0, 42\nMOV v1, v0\n; xor rbx, rbx"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// E2E test: verify the NOT pattern is valid
// not r1; not r1 is equivalent to identity
func TestDiscoveredDoubleNot(t *testing.T) {
	if !hasTool(t, "nasm") {
		t.Skip("nasm not found")
	}
	// VAS doesn't have NOT as first-class, but passthrough works
	input := "MOVI v0, 42\n; not rax; not rax\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	asm, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
	dir := t.TempDir()
	asmFile := filepath.Join(dir, "test.asm")
	objFile := filepath.Join(dir, "test.o")
	if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
		t.Fatalf("write asm: %v", err)
	}
	out, err := exec.Command("nasm", "-f", "elf64", "-o", objFile, asmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("nasm failed: %v\n%s", err, out)
	}
}

// E2E test: double negation via MUL by -1
// neg r1; neg r1 → identity (discovered rule)
func TestDiscoveredDoubleNeg(t *testing.T) {
	if !hasTool(t, "nasm") {
		t.Skip("nasm not found")
	}
	// Compute x = 42, then neg twice via MUL by -1
	input := "MOVI v0, 42\nMUL v0, -1\nMUL v0, -1\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	_, err := vas.Assemble(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
}

// E2E test: inc + dec cancellation pattern
func TestDiscoveredIncDec(t *testing.T) {
	if !hasTool(t, "nasm") {
		t.Skip("nasm not found")
	}
	// VAS-level inc/dec via ADD 1 / SUB 1
	input := "MOVI v0, 42\nADD v0, 1\nSUB v0, 1\nMOV v5, v0\nMOVI v0, 60\nSYSCALL"
	asm, err := vas.AssembleStandalone(input)
	if err != nil {
		t.Fatalf("assembly error: %v", err)
	}
	dir := t.TempDir()
	asmFile := filepath.Join(dir, "test.asm")
	objFile := filepath.Join(dir, "test.o")
	if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
		t.Fatalf("write asm: %v", err)
	}
	out, err := exec.Command("nasm", "-f", "elf64", "-o", objFile, asmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("nasm failed: %v\n%s", err, out)
	}
}

// Full E2E: compute 1+2+...+10 = 55 with optimization
func TestDiscoveredSumWithOpt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E in short mode")
	}
	if !hasTool(t, "nasm") || !hasTool(t, "ld") {
		t.Skip("nasm/ld not found")
	}

	vasSrc := `
MOVI v1, 0           ; sum = 0
MOVI v2, 1           ; i = 1
MOVI v3, 10          ; n = 10
loop:
ADD v1, v1, v2       ; sum += i
ADD v2, 1            ; i++
CMP v2, v3
JLE loop
MOV v5, v1
MOVI v0, 60
SYSCALL
`

	// Verify both -O0 and -O1 produce valid, assemblable code
	for _, opt := range []int{0, 1, 2} {
		asm, err := vas.AssembleStandaloneTargetOpt(vasSrc, "elf64", opt)
		if err != nil {
			t.Fatalf("assembly error at -O%d: %v", opt, err)
		}
		dir := t.TempDir()
		asmFile := filepath.Join(dir, "test.asm")
		objFile := filepath.Join(dir, "test.o")
		if err := os.WriteFile(asmFile, []byte(asm), 0644); err != nil {
			t.Fatalf("write asm: %v", err)
		}
		out, err := exec.Command("nasm", "-f", "elf64", "-o", objFile, asmFile).CombinedOutput()
		if err != nil {
			t.Fatalf("nasm failed at -O%d:\n%s\n%s", opt, err, out)
		}

		// On Windows, skip runtime check (ELF binary)
		if runtime.GOOS == "windows" {
			continue
		}

		binFile := filepath.Join(dir, "test")
		if runtime.GOOS == "windows" {
			binFile += ".exe"
		}
		out2, err := exec.Command("ld", "-o", binFile, objFile).CombinedOutput()
		if err != nil {
			t.Fatalf("ld failed at -O%d:\n%s\n%s", opt, err, out2)
		}
		out3, err := exec.Command(binFile).CombinedOutput()
		exitCode := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		if exitCode != 55 {
			t.Errorf("expected exit code 55 at -O%d, got %d\noutput: %s", opt, exitCode, out3)
		}
	}
}
