package core

import "github.com/sarchlab/zeonica/cgra"

type opcodeFunc func(Operation, *coreState) map[Operand]cgra.Data

type returnOpcodeFunc func(Operation, *coreState, float64) map[Operand]cgra.Data

func (i instEmulator) scalarOpcodeFuncs() map[string]opcodeFunc {
	return map[string]opcodeFunc{
		"ADD":       i.runAdd, // ADD, ADDI, INC, SUB, DEC
		"SUB":       i.runSub,
		"SHL":       i.runSHL,
		"LRS":       i.runLRS,
		"MUL":       i.runMul,    // MULI
		"MUL_ADD":   i.runMulAdd, // dst = src0 * src1 + src2
		"DIV":       i.runDiv,
		"OR":        i.runOR,
		"XOR":       i.runXOR, // XOR XORI
		"AND":       i.runAND,
		"MOV":       i.runMov,
		"JMP":       i.runJmp,
		"BNE":       i.runBne,
		"BEQ":       i.runBeq, // BEQI
		"BLT":       i.runBlt,
		"PHI_CONST": i.runPhiConst, // backward compatibility
		"SEXT":      i.runMov,      // identity operation by now
		"ZEXT":      i.runMov,      // identity operation by now

		"FADD": i.runFAdd, // FADDI
		"FSUB": i.runFSub,
		"FMUL": i.runFMul,
		"NOP":  i.runNOP,

		"PHI":             i.runPhi,
		"PHI_START":       i.runPhiStart,
		"GRANT_PREDICATE": i.runGrantPred,
		"GRANT_ONCE":      i.runGrantOnce,
		"SEL":             i.runSel,

		// comparisons
		"ICMP_EQ":  i.runCmpExport,
		"ICMP_SLT": i.runLTExport,
		"ICMP_SGT": i.runGTExport,
		"ICMP_SGE": i.runSGEExport,
		"ICMP_ULT": i.runULTExport,

		// do not distinguish between data_mov and control mov
		"DATA_MOV": i.runMov,
		"CTRL_MOV": i.runMov,
		"CONSTANT": i.runMov,

		"GEP": i.runGep,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LOAD":  i.runLoad,
		"STORE": i.runStore,
		"LDD":   i.runLoadDirect,  // backward compatibility
		"STD":   i.runStoreDirect, // backward compatibility

		"LD":  i.runLoadDRAM,
		"LDW": i.runLoadWaitDRAM,

		"ST":  i.runStoreDRAM,
		"STW": i.runStoreWaitDRAM,

		"TRIGGER": i.runTrigger,

		"NOT": i.runNot,
	}
}

func (i instEmulator) vectorOpcodeFuncs() map[string]opcodeFunc {
	return map[string]opcodeFunc{
		"VBROADCAST":        i.runVBroadcast,
		"VADD":              i.runVAdd,
		"VMUL":              i.runVMul,
		"VECTOR.REDUCE.ADD": i.runVectorReduceAdd,
		"VEXTRACT":          i.runVExtract,
		"VLOAD_CONTIG":      i.runVLoadContig,
		"VSTORE_CONTIG":     i.runVStoreContig,
	}
}

func (i instEmulator) returnOpcodeFuncs() map[string]returnOpcodeFunc {
	return map[string]returnOpcodeFunc{
		"RETURN_VALUE": i.runRetImm,
		"RETURN_VOID":  i.runRetDelay,
		"RET":          i.runRetImm, // backward compatibility
	}
}
