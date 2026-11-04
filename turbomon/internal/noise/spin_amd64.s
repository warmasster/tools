#include "textflag.h"

// func spin(dur, thr1, thr2, thr3 uint64) (stolen, maxGap, n1, n2, n3 uint64)
//
// Lee el contador del procesador (RDTSC) sin parar durante «dur» ciclos y
// apunta cada hueco mayor que thr1: el tiempo que algo (una interrupción, un
// driver, otro hilo) le ha quitado el núcleo. No llama a nada, así que Go no
// puede interrumpirlo por su cuenta.
TEXT ·spin(SB), NOSPLIT, $0-72
	MOVQ dur+0(FP), R8
	MOVQ thr1+8(FP), R9
	MOVQ thr2+16(FP), R10
	MOVQ thr3+24(FP), BX
	MOVQ $0, n3+64(FP)
	XORQ R12, R12 // tiempo robado
	XORQ R13, R13 // hueco más largo
	XORQ SI, SI   // huecos > thr1
	XORQ DI, DI   // huecos > thr2
	RDTSC
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, R11 // anterior
	ADDQ AX, R8  // fin
loop:
	RDTSC
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, CX
	SUBQ R11, CX
	MOVQ AX, R11
	CMPQ CX, R9
	JBE  next
	ADDQ CX, R12
	INCQ SI
	CMPQ CX, R13
	JBE  big
	MOVQ CX, R13
big:
	CMPQ CX, R10
	JBE  next
	INCQ DI
	CMPQ CX, BX
	JBE  next
	INCQ n3+64(FP)
next:
	CMPQ AX, R8
	JB   loop
	MOVQ R12, stolen+32(FP)
	MOVQ R13, maxGap+40(FP)
	MOVQ SI, n1+48(FP)
	MOVQ DI, n2+56(FP)
	RET

// func tsc() uint64
TEXT ·tsc(SB), NOSPLIT, $0-8
	RDTSC
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, ret+0(FP)
	RET
