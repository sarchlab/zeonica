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
