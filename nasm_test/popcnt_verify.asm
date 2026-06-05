; Verify: software popcount ≡ hardware POPCNT
; Build: nasm -f win64 -o test.obj test.asm && ld -e main -o test.exe test.obj

default rel

section .text
global main
main:
    sub rsp, 40

; Test each value with both software and hardware
%macro test_popcnt 1
    mov rax, %1

    ; Software popcount (Kernighan's)
    mov rcx, rax
    xor ebx, ebx
.bit_loop_%1:
    test rcx, rcx
    jz .done_%1
    mov rdx, rcx
    dec rdx
    and rcx, rdx
    inc ebx
    jmp .bit_loop_%1
.done_%1:
    mov r8, rbx          ; expected in r8

    ; Hardware POPCNT
    popcnt r9, rax       ; actual in r9

    cmp r8, r9
    jne fail
%endmacro

    ; Test values
    test_popcnt 0
    test_popcnt 1
    test_popcnt 0xFFFFFFFFFFFFFFFF
    test_popcnt 0x7FFFFFFFFFFFFFFF
    test_popcnt 0x8000000000000000
    test_popcnt 0x5555555555555555
    test_popcnt 0xAAAAAAAAAAAAAAAA
    test_popcnt 0x0F0F0F0F0F0F0F0F
    test_popcnt 42
    test_popcnt 0x0102030405060708

    xor eax, eax
    add rsp, 40
    ret

fail:
    mov eax, 1
    add rsp, 40
    ret
