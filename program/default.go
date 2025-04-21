package program

// Define the behavior of the ADD instruction in Zeonica Unified ISA.
func instADD(src1 int32, src2 int32) int32 {
	return src1 + src2
}

func instMOV(src1 int32) int32 {
	return src1
}

func instMAC(src1 int32, src2 int32, src3 int32) int32 {
	return src1*src2 + src3
}

func instSUB(src1 int32, src2 int32) int32 {
	return src1 - src2
}

func instMUL(src1 int32, src2 int32) int32 {
	return src1 * src2
}

func instDIV(src1 int32, src2 int32) int32 {
	return src1 / src2
}

func instMOD(src1 int32, src2 int32) int32 {
	return src1 % src2
}

func instAND(src1 int32, src2 int32) int32 {
	return src1 & src2
}

func instOR(src1 int32, src2 int32) int32 {
	return src1 | src2
}

func instXOR(src1 int32, src2 int32) int32 {
	return src1 ^ src2
}

func instNOT(src1 int32) int32 {
	return ^src1
}

func instSHL(src1 int32, src2 int32) int32 {
	return src1 << src2
}

func instSHR(src1 int32, src2 int32) int32 {
	return src1 >> src2
}

func instPHI(src1 int32, src2 int32, cond bool) int32 {
	if cond == true {
		return src1
	} else if cond == false {
		return src2
	}
	return 0
}

func instSEL(cond int32, src1 int32, src2 int32) int32 {
	if cond != 0 {
		return src1
	}
	return src2
}

func instEQ(src1 int32, src2 int32) int32 {
	if src1 == src2 {
		return 1
	}
	return 0
}

func instNE(src1 int32, src2 int32) int32 {
	if src1 != src2 {
		return 1
	}
	return 0
}

func instLT(src1 int32, src2 int32) int32 {
	if src1 < src2 {
		return 1
	}
	return 0
}
