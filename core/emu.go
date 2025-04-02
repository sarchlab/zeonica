package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// type routingRule struct {
// 	src      cgra.Side
// 	dst      cgra.Side
// 	srcColor string
// 	dstColor string
// }

// type Trigger struct {
// 	src    [4]bool
// 	color  int
// 	branch string
// }

type coreState struct {
	PC           uint32
	blockMode    bool
	instStalled  bool
	TileX, TileY uint32
	Registers    []uint32
	Memory       []uint32
	Code         []string

	RecvBufHead      [][]uint32 //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]uint32
	SendBufHeadBusy  [][]bool

	// routingRules []*routingRule
	// triggers []*Trigger
}

type instEmulator struct {
}

func splitInstLine(line string) []string {
	var tokens []string
	start := 0
	bracketDepth := 0
	for i, ch := range line {
		switch ch {
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case ',':
			if bracketDepth == 0 {
				// split outside brackets
				tokens = append(tokens, strings.TrimSpace(line[start:i]))
				start = i + 1
			}
		}
	}
	// Add the last token
	tokens = append(tokens, strings.TrimSpace(line[start:]))
	return tokens
}

func (i instEmulator) RunInst(inst string, state *coreState) {
	//tokens := strings.Split(inst, ",")
	tokens := splitInstLine(inst)
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	instName := tokens[0]
	if strings.Contains(instName, "CMP") {
		instName = "CMP"
	}

	instFuncs := map[string]func([]string, *coreState){
		"JNE":  i.runJne,
		"JEQ":  i.runJeq,
		"DONE": func(_ []string, _ *coreState) { i.runDone() }, // Since runDone might not have parameters

		"IDLE": func(_ []string, state *coreState) { i.runIdle(state) },
		"MOV":  i.runMov,

		//Arithmetic: MUL_CONST, MUL_CONST_ADD, MUL_SUB, DIV
		"MAC":           i.runMac,
		"MUL":           i.runMul,
		"ADDI":          i.runIAdd,
		"SUB":           i.runSub,
		"MUL_CONST":     i.runMul_Const,
		"MUL_CONST_ADD": i.runMul_Const_Add,
		"MUL_SUB":       i.runMul_Sub,
		"DIV":           i.runDiv,

		//Bitwise: LLS, LRS, OR, XOR, NOT, AND
		"LLS": i.runLLS,
		"LRS": i.runLRS,
		"OR":  i.runOR,
		"XOR": i.runXOR,
		"NOT": i.runNOT,
		"AND": i.runAND,

		//Comparison: EQ, EQ_CONST, LT, LTE, GT, GTE, most is already in parseAndCompareI
		"CMP": i.runCmp,

		//Control Flow: BRH, RET, BRH_START
		"JMP": i.runJmp,

		//ld, st, ld_const, str_const,
		"LD":  i.runLoad,
		"ST":  i.runStore, //able to load store imm as well
		"LDI": i.runLoadImm,
		"STI": i.runStoreImm,

		//Advanced Arithmetic: FADD, FADD_CONST, FINC, FSUB, FMUL, FMUL_CONST
		"FADD":       i.runFAdd,
		"FADD_CONST": i.runFAdd_Const,
		"FINC":       i.runFInc,
		"FSUB":       i.runFSub,
		"FMUL":       i.runFMul,
		"FDIV":       i.runFDiv,
		"FMUL_CONST": i.runFMul_Const,
		//Vector Operations: VEC_ADD, VEC_ADD_CONST, VEC_INC, VEC_SUB, VEC_SUB_CONST, VEC_MUL, VEC_REDUCE_ADD, VEC_REDUCE_MUL

		//Specialized Control: OPT_PAS, OPT_START, OPT_NAH, OPT_PHI, OPT_PHI_CONST, OPT_SEL
		"PAS":       i.runPAS,
		"START":     i.runStart,
		"NAH":       i.runNah,
		"PHI":       i.runPhi,
		"PHI_CONST": i.runPhi_const,
		"SEL":       i.runSel,
	}

	if instFunc, ok := instFuncs[instName]; ok {
		instFunc(tokens, state)
	} else {
		//panic("unknown instruction " + inst)
		panic(fmt.Sprintf("unknown instruction '%s' at PC %d", instName, state.PC))
	}
}

func (i instEmulator) getDirecIndex(side string) int {
	var srcIndex int

	switch side {
	case "NORTH":
		srcIndex = int(cgra.North)
	case "WEST":
		srcIndex = int(cgra.West)
	case "SOUTH":
		srcIndex = int(cgra.South)
	case "EAST":
		srcIndex = int(cgra.East)
	default:
		panic("invalid side")
	}

	return srcIndex
}

func (i instEmulator) RouterSrcMustBeDirection(src string) {
	arr := []string{"NORTH", "SOUTH", "WEST", "EAST"}
	res := false
	for _, s := range arr {
		if s == src {
			res = true
			break
		}
	}

	if res {
		panic("the source of a ROUTER_FORWARD instruction must be directions")
	}
}

func (i instEmulator) getColorIndex(color string) int {
	switch color {
	case "R":
		return 0
	case "Y":
		return 1
	case "B":
		return 2
	default:
		panic("Wrong Color")
	}
}

func (i instEmulator) readOperandWithIfPred(operand string, state *coreState, ifPred int) (uint32, bool) {
	val, ready := i.readFlexibleOperand(operand, state)
	if ifPred == 1 && !ready {
		// Stall
		return 0, false
	}
	if ifPred == 0 && !ready {
		// Treat as zero if not ready
		val = 0
	}
	return val, true
}

// read flexible operand "[$1] or [port, color]"
func (i instEmulator) readFlexibleOperand(operand string, state *coreState) (value uint32, ready bool) {
	operand = strings.Trim(operand, "[] ")

	if strings.HasPrefix(operand, "$") {
		// Register read
		val := i.readOperand(operand, state)
		return val, true
	} else {
		// Format: PORT, COLOR
		parts := strings.Split(operand, ",")
		if len(parts) != 2 {
			panic(fmt.Sprintf("Invalid operand format for port: %s", operand))
		}
		direction := i.getDirecIndex(strings.TrimSpace(parts[0]))
		color := i.getColorIndex(strings.TrimSpace(parts[1]))

		if !state.RecvBufHeadReady[color][direction] {
			// Data not arrived yet
			return 0, false
		}

		val := state.RecvBufHead[color][direction]
		if !state.blockMode {
			state.RecvBufHeadReady[color][direction] = false
		}
		return val, true
	}
}

// send results to reg OR port
func (i instEmulator) parseDestPort(dest string) (directionIndex int, colorIndex int, isPort bool) {
	dest = strings.TrimSpace(dest)
	if strings.HasPrefix(dest, "$") {
		return -1, -1, false // register destination
	}

	dest = strings.Trim(dest, "[] ")
	parts := strings.Split(dest, ",")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Invalid destination port format: %s", dest))
	}
	directionIndex = i.getDirecIndex(strings.TrimSpace(parts[0]))
	colorIndex = i.getColorIndex(strings.TrimSpace(parts[1]))
	return directionIndex, colorIndex, true
}

// float32 to uint32
func float2Uint(f float32) uint32 {
	return math.Float32bits(f)
}

// uint32 to float32
func uint2Float(u uint32) float32 {
	return math.Float32frombits(u)
}

func (i instEmulator) waitSrcMustBeNetRecvReg(src string) {
	if !strings.HasPrefix(src, "NET_RECV_") {
		panic("the source of a WAIT instruction must be NET_RECV registers")
	}
}

func (i instEmulator) readFlexibleOperandMov(token string, state *coreState) (uint32, bool) {
	token = strings.Trim(token, "[] ")

	// Immediate operand check
	if num, err := strconv.ParseUint(token, 10, 32); err == nil {
		return uint32(num), true // immediate is always ready
	}

	// Register operand check
	if strings.HasPrefix(token, "$") {
		val := i.readOperand(token, state)
		return val, true
	}

	// Port operand check
	parts := strings.Split(token, ",")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Invalid port operand format for MOV: %s", token))
	}
	dir := i.getDirecIndex(strings.TrimSpace(parts[0]))
	color := i.getColorIndex(strings.TrimSpace(parts[1]))

	if !state.RecvBufHeadReady[color][dir] {
		return 0, false
	}

	val := state.RecvBufHead[color][dir]
	if !state.blockMode {
		state.RecvBufHeadReady[color][dir] = false
	}

	return val, true
}

func (i instEmulator) writeFlexibleOperandMov(token string, value uint32, state *coreState) bool {
	token = strings.Trim(token, "[] ")

	// Register write
	if strings.HasPrefix(token, "$") {
		i.writeOperand(token, value, state)
		return true
	}

	// Port send
	parts := strings.Split(token, ",")
	if len(parts) != 2 {
		panic(fmt.Sprintf("Invalid port operand format for MOV destination: %s", token))
	}
	dir := i.getDirecIndex(strings.TrimSpace(parts[0]))
	color := i.getColorIndex(strings.TrimSpace(parts[1]))

	if !state.SendBufHeadBusy[color][dir] {
		state.SendBufHeadBusy[color][dir] = true
		state.SendBufHead[color][dir] = value
		return true
	}

	return false
}

// runMov handles the MOV instruction for both immediate values and register-to-register moves.
// Prototype for moving an immediate: MOV, DstReg, Immediate
// Prototype for register to register: MOV, DstReg, SrcReg
func (i instEmulator) runMov(inst []string, state *coreState) {
	if len(inst) != 3 {
		panic(fmt.Sprintf("Invalid MOV format, expected 3 tokens but got %d", len(inst)))
	}

	srcToken := strings.TrimSpace(inst[1])
	dstToken := strings.TrimSpace(inst[2])

	// Parse source
	srcRequired := false
	if strings.HasPrefix(srcToken, "!") {
		srcRequired = true
		srcToken = strings.TrimPrefix(srcToken, "!")
	}

	value, ready := i.readFlexibleOperandMov(srcToken, state)

	if srcRequired && !ready {
		state.instStalled = true
		fmt.Printf("MOV: Stalled — source %s not ready\n", srcToken)
		return
	}

	if !ready {
		value = 0 // Optional, default zero if not ready
	}

	// Parse destination
	dstRequired := false
	if strings.HasPrefix(dstToken, "!") {
		dstRequired = true
		dstToken = strings.TrimPrefix(dstToken, "!")
	}

	if !i.writeFlexibleOperandMov(dstToken, value, state) {
		if dstRequired {
			state.instStalled = true
			fmt.Printf("MOV: Stalled — destination %s busy\n", dstToken)
			return
		}
	}

	fmt.Printf("MOV: Moved value %d from %s to %s\n", value, srcToken, dstToken)
}

func (i instEmulator) parseAddress(addrStr string, state *coreState) uint32 {
	// imm addr
	if immediate, err := strconv.ParseUint(addrStr, 0, 32); err == nil {
		return uint32(immediate)
	}

	// addr in reg
	if strings.Contains(addrStr, "$") {
		parts := strings.Split(addrStr, "+")
		baseReg := strings.TrimSpace(parts[0])
		baseAddr := i.readOperand(baseReg, state)

		offset := uint32(0)
		if len(parts) > 1 {
			offsetVal, err := strconv.ParseUint(parts[1], 0, 32)
			if err != nil {
				panic("invalid offset")
			}
			offset = uint32(offsetVal)
		}
		return baseAddr + offset
	}

	panic("invalid address format")
}

func (i instEmulator) runLoad(inst []string, state *coreState) {
	dstReg := inst[1]
	addrStr := inst[2] // address（ 0x100 or $1+0x10）
	// $1 + 0x10 implies that the base address of register 1
	// (which is 0xF0) plus an offset of 0x10
	// results in the address 0x100.
	addr := i.parseAddress(addrStr, state)

	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}
	value := state.Memory[addr]
	i.writeOperand(dstReg, value, state)
	state.PC++
}

func (i instEmulator) runLoadImm(inst []string, state *coreState) {

}

func (i instEmulator) runStore(inst []string, state *coreState) {
	srcReg := inst[1]
	addrStr := inst[2]
	addr := i.parseAddress(addrStr, state)
	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}
	value := i.readOperand(srcReg, state)
	state.Memory[addr] = value
	state.PC++
}

func (i instEmulator) runStoreImm(inst []string, state *coreState) {

}

func (i instEmulator) sendDstMustBeNetSendReg(dst string) {
	if !strings.HasPrefix(dst, "NET_SEND_") {
		panic("the destination of a SEND instruction must be NET_SEND_ registers")
	}
}

/**
 * @description:
 * @prototype:
 */
func (i instEmulator) runJmp(inst []string, state *coreState) {
	dst := inst[1]
	i.Jump(dst, state)
}

func (i instEmulator) Jump(dst string, state *coreState) {
	for i := 0; i < len(state.Code); i++ {
		line := strings.Trim(state.Code[i], " \t\n")
		if strings.HasPrefix(line, dst) && strings.HasSuffix(line, ":") {
			state.PC = uint32(i) + 1
			return
		}
	}
}

func (i instEmulator) readOperand(operand string, state *coreState) (value uint32) {
	operand = strings.TrimSpace(operand)
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in readOperand: %s", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in readOperand", registerIndex))
		}

		value = state.Registers[registerIndex]
	} else {
		panic(fmt.Sprintf("Invalid operand %s in readOperand; expected register", operand))
	}

	return
}

func (i instEmulator) writeOperand(operand string, value uint32, state *coreState) {
	operand = strings.TrimSpace(operand)
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in writeOperand: %s", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in writeOperand", registerIndex))
		}

		state.Registers[registerIndex] = value
		//fmt.Printf("Updated register $%d to value %d at PC %d\n", registerIndex, value, state.PC)
	} else {
		panic(fmt.Sprintf("Invalid operand %s in writeOperand; expected register", operand))
	}
}

/**
 * @description:
 * @prototype:F32_CMP_[], Cmp_Res, Cmp_Src, imme
 */
func (i instEmulator) runCmp(inst []string, state *coreState) {
	Itype := inst[0]
	//Float or Integer
	switch {
	case strings.Contains(Itype, "I"):
		i.parseAndCompareI(inst, state)
	case strings.Contains(Itype, "F32"):
		i.parseAndCompareF32(inst, state)
	default:
		panic("invalid cmp")
	}
}

func (i instEmulator) parseAndCompareI(inst []string, state *coreState) {
	instruction := inst[0]
	dst := inst[1]
	src := inst[2]

	srcVal := i.readOperand(src, state)
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst[3], 10, 32)
	if err != nil {
		panic("invalid compare number")
	}

	immeI32 := int32(uint32(imme))
	srcValI := int32(srcVal)

	conditionFuncs := map[string]func(int32, int32) bool{
		"EQ": func(a, b int32) bool { return a == b },
		"NE": func(a, b int32) bool { return a != b },
		"LE": func(a, b int32) bool { return a <= b },
		"LT": func(a, b int32) bool { return a < b },
		"GT": func(a, b int32) bool { return a > b },
		"GE": func(a, b int32) bool { return a >= b },
	}

	for key, function := range conditionFuncs {
		if strings.Contains(instruction, key) && function(srcValI, immeI32) {
			dstVal = 1
		}
	}
	i.writeOperand(dst, dstVal, state)
	state.PC++
}

func (i instEmulator) parseAndCompareF32(inst []string, state *coreState) {
	instruction := inst[0]
	dst := inst[1]
	src := inst[2]

	srcVal := i.readOperand(src, state)
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst[3], 10, 32)
	if err != nil {
		panic("invalid compare number")
	}

	conditionFuncsF := map[string]func(float32, float32) bool{
		"EQ": func(a, b float32) bool { return a == b },
		"NE": func(a, b float32) bool { return a != b },
		"LT": func(a, b float32) bool { return a < b },
		"LE": func(a, b float32) bool { return a <= b },
		"GT": func(a, b float32) bool { return a > b },
		"GE": func(a, b float32) bool { return a >= b },
	}

	immeF32 := math.Float32frombits(uint32(imme))
	srcValF := math.Float32frombits(srcVal)

	for key, function := range conditionFuncsF {
		if strings.Contains(instruction, key) && function(srcValF, immeF32) {
			dstVal = 1
		}
	}
	i.writeOperand(dst, dstVal, state)
	state.PC++
}

/**
 * @description:
 * @prototype:
 */
func (i instEmulator) runJeq(inst []string, state *coreState) {
	src := inst[2]
	imme, err := strconv.ParseUint(inst[3], 10, 32)

	if err != nil {
		panic("invalid compare number")
	}

	srcVal := i.readOperand(src, state)

	if srcVal == uint32(imme) {
		i.runJmp(inst, state)
	} else {
		state.PC++
	}
}

/**
 * @description:
 * @prototype:
 */
func (i instEmulator) runJne(inst []string, state *coreState) {
	src := inst[2]
	imme, err := strconv.ParseUint(inst[3], 10, 32)

	if err != nil {
		panic("invalid compare number")
	}

	srcVal := i.readOperand(src, state)

	if srcVal != uint32(imme) {
		i.runJmp(inst, state)
	} else {
		state.PC++
	}
}

/**
 * @description:
 * Get data from
 * @prototype: MAC, ifpred, Dst, Src1, Src2, Src3
 * src and dst can be either port and color or register
 */
func (i instEmulator) runMac(inst []string, state *coreState) {
	if len(inst) != 5 {
		panic(fmt.Sprintf("Invalid MAC format, got %d tokens, expected 5", len(inst)))
	}
	// Parse source operands
	srcVals := make([]uint32, 3)
	for idx := 0; idx < 3; idx++ {
		token := strings.TrimSpace(inst[idx+1])
		required := false
		if strings.HasPrefix(token, "!") {
			required = true
			token = strings.TrimPrefix(token, "!")
		}

		val, ready := i.readFlexibleOperand(token, state)
		if required && !ready {
			state.instStalled = true
			fmt.Printf("MAC: Stalled — required operand %s not ready\n", token)
			return
		}
		if !ready {
			val = 0
		}
		srcVals[idx] = val
	}

	// Compute MAC
	result := srcVals[0]*srcVals[1] + srcVals[2]

	// Destination
	dst := strings.TrimSpace(inst[4])
	dirIdx, colorIdx, isPort := i.parseDestPort(dst)
	if isPort {
		if !state.SendBufHeadBusy[colorIdx][dirIdx] {
			state.SendBufHeadBusy[colorIdx][dirIdx] = true
			state.SendBufHead[colorIdx][dirIdx] = result
			fmt.Printf("MAC: Sent result %v to [%s]\n", result, dst)
		} else {
			state.instStalled = true
			fmt.Println("MAC: Send buffer busy")
			return
		}
	} else {
		i.writeOperand(dst, result, state)
		fmt.Printf("MAC: Wrote result %v to register %s\n", result, dst)
	}
}

func (i instEmulator) runMul_Sub(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal -= srcVal1 * srcVal2
	i.writeOperand(dst, dstVal, state)

	//fmt.Printf("MUL_SUB: %s -= %s * %s => Result: %v\n", dst, src1, src2, dstVal)
	if !state.blockMode {
		state.PC++
	}
}

/**
 * @description:
 * Multiply function
 * @prototype: MUL, DstReg, SrcReg1, SrcReg2
 */
func (i instEmulator) runMul(inst []string, state *coreState) {
	ifPred, err := strconv.Atoi(inst[1]) // ifPred should be either 1 or 0
	if err != nil {
		panic(fmt.Sprintf("Invalid IF_PRED value in MAC: %s", inst[1]))
	}
	dst := inst[2]
	src1 := inst[3]
	src2 := inst[4]

	val1, ready1 := i.readOperandWithIfPred(src1, state, ifPred)
	val2, ready2 := i.readOperandWithIfPred(src2, state, ifPred)
	if !ready1 || !ready2 {
		state.instStalled = true
		fmt.Println("MUL: Stalled due to missing operands")
		return
	}

	result := val1 * val2

	// Send to reg or port
	dirIdx, colorIdx, isPort := i.parseDestPort(dst)
	if isPort {
		if !state.SendBufHeadBusy[colorIdx][dirIdx] {
			state.SendBufHeadBusy[colorIdx][dirIdx] = true
			state.SendBufHead[colorIdx][dirIdx] = result
			fmt.Printf("MUL result sent %v to [%d,%d]\n", result, dirIdx, colorIdx)
		} else {
			state.instStalled = true
			return
		}
	} else {
		i.writeOperand(dst, result, state)
		fmt.Printf("MUL result %v written to %s\n", result, dst)
	}

	if !state.blockMode {
		state.PC++
	}
}

/**
 * @description: Add two numbers together. The input could be register or immediate number.
 * @prototype: ADD, DstReg, SrcReg1, SrcReg2(Imme)
 */
func (i instEmulator) runIAdd(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]
	src1Val := i.readOperand(src1, state)
	var src2Val uint32
	src2flag := false

	if strings.HasPrefix(src2, "$") {
		src2flag = true
	}

	if src2flag {
		src2Val = i.readOperand(src2, state)
	} else {
		num, err := strconv.ParseUint(src2, 10, 32)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		src2Val = uint32(num)
	}
	dstVal := src1Val + src2Val
	//fmt.Printf("IADD: Adding %v (src1) + %v (src2) = %v\n", src1Val, src2Val, dstVal)
	i.writeOperand(dst, dstVal, state)
	state.PC++
}

func (i instEmulator) runSub(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal = srcVal1 - srcVal2
	i.writeOperand(dst, dstVal, state)

	//fmt.Printf("SUB Instruction, Data are %v and %v, Res is %v\n", srcVal1, srcVal2, dstVal)

	state.PC++
}

func (i instEmulator) runMul_Const(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	immeStr := inst[3]

	srcVal := i.readOperand(src, state)
	imme, err := strconv.ParseUint(immeStr, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid immediate value for MUL_CONST: %s", immeStr))
	}
	immeVal := uint32(imme)

	result := srcVal * immeVal
	i.writeOperand(dst, result, state)

	//fmt.Printf("MUL_CONST: %s = %s * %d => Result: %d\n", dst, src, immeVal, result)
	state.PC++
}

func (i instEmulator) runMul_Const_Add(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	immeStr := inst[3]

	srcVal := i.readOperand(src, state)
	imme, _ := strconv.ParseUint(immeStr, 10, 32)
	immeVal := uint32(imme)

	// dst = dst + (src * immediate)
	dstVal := i.readOperand(dst, state)
	dstVal += srcVal * immeVal
	i.writeOperand(dst, dstVal, state)

	//fmt.Printf("MUL_CONST_ADD: %s += %s * %d => Result: %d\n", dst, src, immeVal, dstVal)
	state.PC++
}

func (i instEmulator) runDiv(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal = srcVal1 / srcVal2
	i.writeOperand(dst, dstVal, state)

	//fmt.Printf("DIV Instruction, Data are %v and %v, Res is %v\n", srcVal1, srcVal2, dstVal)

	state.PC++
}

func (i instEmulator) runFAdd(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float + src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FADD: %s = %s(%f) + %s(%f) => %f (0x%08x)\n",
	// 	dst, src1, src1Float, src2, src2Float, resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFSub(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float - src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FSUB: %s = %s(%f) - %s(%f) => %f (0x%08x)\n",
	// 	dst, src1, src1Float, src2, src2Float, resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFMul(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float * src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FMUL: %s = %s(%f) * %s(%f) => %f (0x%08x)\n",
	// 	dst, src1, src1Float, src2, src2Float, resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFDiv(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float / src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FDIV: %s = %s(%f) / %s(%f) => %f (0x%08x)\n",
	// 	dst, src1, src1Float, src2, src2Float, resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFAdd_Const(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	immeStr := inst[3]

	srcVal := i.readOperand(src, state)
	srcFloat := uint2Float(srcVal)
	imme, err := strconv.ParseFloat(immeStr, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid immediate value for FADD_CONST: %s", immeStr))
	}
	immeVal := float32(imme)

	resultFloat := srcFloat + immeVal
	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FADD_CONST: %s = %s(%f) + %f => %f (0x%08x)\n",
	// 	dst, src, uint2Float(srcVal), imme, resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFInc(inst []string, state *coreState) {
	dst := inst[1]
	//src := inst[2]

	dstVal := i.readOperand(dst, state)
	resultFloat := uint2Float(dstVal) + 1.0
	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FINC: %s = %s(%f) + 1.0 => %f (0x%08x)\n",
	// 	dst, dst, uint2Float(dstVal), resultFloat, resultUint,
	// )
	state.PC++
}

func (i instEmulator) runFMul_Const(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	immeStr := inst[3]

	srcVal := i.readOperand(src, state)
	srcFloat := uint2Float(srcVal)
	imme, err := strconv.ParseFloat(immeStr, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid immediate value for FADD_CONST: %s", immeStr))
	}
	immeVal := float32(imme)

	resultFloat := srcFloat * immeVal
	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	// fmt.Printf(
	// 	"FADD_CONST: %s = %s(%f) * %f => %f (0x%08x)\n",
	// 	dst, src, uint2Float(srcVal), imme, resultFloat, resultUint,
	// )
	state.PC++
}

// Bitwise
func (i instEmulator) runLLS(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	shiftStr := inst[3]

	srcVal := i.readOperand(src, state)
	shift, err := strconv.ParseUint(shiftStr, 10, 5) // limitation
	if err != nil {
		panic(fmt.Sprintf("invalid shift value for LLS: %s", shiftStr))
	}

	result := srcVal << shift
	i.writeOperand(dst, result, state)

	//fmt.Printf("LLS: %s = %s << %d => Result: %d\n", dst, src, shift, result)
	state.PC++
}

func (i instEmulator) runLRS(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	shiftStr := inst[3]

	srcVal := i.readOperand(src, state)
	shift, err := strconv.ParseUint(shiftStr, 10, 5)
	if err != nil {
		panic(fmt.Sprintf("invalid shift value for LRS: %s", shiftStr))
	}

	result := srcVal >> shift
	i.writeOperand(dst, result, state)

	//fmt.Printf("LRS: %s = %s >> %d => Result: %d\n", dst, src, shift, result)
	state.PC++
}

func (i instEmulator) runOR(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 | srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("OR: %s = %s | %s => Result: %d\n", dst, src1, src2, result)
	state.PC++
}

func (i instEmulator) runXOR(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 ^ srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("XOR: %s = %s ^ %s => Result: %d\n", dst, src1, src2, result)
	state.PC++
}

func (i instEmulator) runNOT(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]

	srcVal := i.readOperand(src, state)
	result := ^srcVal
	i.writeOperand(dst, result, state)

	//fmt.Printf("NOT: %s = ~%s => Result: %d\n", dst, src, result)
	state.PC++
}

func (i instEmulator) runAND(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 & srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("AND: %s = %s & %s => Result: %d\n", dst, src1, src2, result)
	state.PC++
}

// Specialized Control
func (i instEmulator) runPAS(inst []string, state *coreState) {

}

func (i instEmulator) runStart(inst []string, state *coreState) {

}

func (i instEmulator) runNah(inst []string, state *coreState) {
	//do nothing, skip
	fmt.Printf("NAH: No action taken, PC advanced to %d\n", state.PC)
	state.PC++
}

func (i instEmulator) runPhi(inst []string, state *coreState) {

}

func (i instEmulator) runPhi_const(inst []string, state *coreState) {

}

func (i instEmulator) runSel(inst []string, state *coreState) {
	// SEL, DstReg, CondReg, TrueReg, FalseReg

}

func (i instEmulator) runDone() {
	// Do nothing.
}

// Waste One time click
func (i instEmulator) runIdle(state *coreState) {
	state.PC++
}

// Sleep
// It will go through all the triggers in the codes and to find the first fulfilled one
// and jump to the branch
// func (i instEmulator) runSleep(inst []string, state *coreState) {
// 	for _, t := range state.triggers {
// 		flag := true
// 		color := t.color
// 		branch := t.branch
// 		for i := 0; i < 4; i++ {
// 			if t.src[i] && !(state.RecvBufHeadReady[color][i]) {
// 				flag = false
// 			}
// 		}
// 		if flag {
// 			//fmt.Printf("[%d][%d]Sleep: Triggered: %s\n", state.TileX, state.TileY, t.branch)
// 			i.Jump(branch, state)
// 			return
// 		}
// 	}
// 	//fmt.Printf("[%d][%d]Sleep: Untriggered. PC%d\n", state.TileX, state.TileY, state.PC)
// 	// When sleep, register all registers.
// 	//No PC++. We want this part is a cycle until one trigger is fulfilled.
// }

func (i instEmulator) doRecv(inst []string, state *coreState) {

}

func (i instEmulator) doSend(inst []string, state *coreState) {

}
