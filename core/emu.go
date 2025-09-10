package core

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

type routingRule struct {
	src   cgra.Side
	dst   cgra.Side
	color string
}

type Trigger struct {
	src    [12]bool
	color  int
	branch string
}

type coreState struct {
	SelectedBlock *EntryBlock
	Directions    map[string]bool
	PCInBlock     int32
	TileX, TileY  uint32
	Registers     []uint32
	States        map[string]interface{} // This is to store core states, such as Phiconst, CmpFlags
	// still consider using outside block to control pc
	//Code         [][]string
	Memory []uint32
	Code   Program

	RecvBufHead      [][]uint32 //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]uint32
	SendBufHeadBusy  [][]bool

	routingRules []*routingRule
	triggers     []*Trigger
}

type instEmulator struct {
	CareFlags bool
}

func (i instEmulator) RunCombinedInst(cinst CombinedInst, state *coreState) {
	LogState(state)

	instOpcodes := make([]string, len(cinst.Insts))
	for i, inst := range cinst.Insts {
		instOpcodes[i] = inst.OpCode
	}

	if i.CareFlags {
		for _, inst := range cinst.Insts {
			if !i.CheckFlags(inst, state) {
				slog.Info("CheckFlags",
					"result", false,
					"victim", inst.OpCode,
					"X", state.TileX,
					"Y", state.TileY,
					"instOpcodes", instOpcodes,
				)
				return
			}
		}
	}
	slog.Info("CheckFlags", "result", true, "X", state.TileX, "Y", state.TileY, "instOpcodes", instOpcodes)
	for _, inst := range cinst.Insts {
		i.RunInst(inst, state)
	}
	LogState(state)
}

func (i instEmulator) CheckFlags(inst Inst, state *coreState) bool {

	//PrintState(state)
	flag := true
	for _, src := range inst.SrcOperands.Operands {
		if state.Directions[src.Impl] {
			if !state.RecvBufHeadReady[i.getColorIndex(src.Color)][i.getDirecIndex(src.Impl)] {
				flag = false
				break
			}
		}
	}

	for _, dst := range inst.DstOperands.Operands {
		if state.Directions[dst.Impl] {
			if state.SendBufHeadBusy[i.getColorIndex(dst.Color)][i.getDirecIndex(dst.Impl)] {
				flag = false
				break
			}
		}
	}
	//fmt.Println("[CheckFlags] checking flags for inst", inst.OpCode, "@(", state.TileX, ",", state.TileY, "):", flag)
	return flag
}

func (i instEmulator) RunInst(inst Inst, state *coreState) {

	instName := inst.OpCode
	/*
		if strings.Contains(instName, "CMP") {
			instName = "CMP"
		}*/
	// alwaysflag := false
	// if strings.HasPrefix(instName, "@") && !alwaysflag {
	// 	instName = "SENDREC"
	// 	alwaysflag = true
	// 	i.runAlwaysSendRec(tokens, state)
	// }

	instFuncs := map[string]func(Inst, *coreState){
		"ADD": i.runAdd, // ADD, ADDI, INC, SUB, DEC
		"SUB": i.runSub,
		"LLS": i.runLLS,
		"LRS": i.runLRS,
		"MUL": i.runMul, // MULI
		"DIV": i.runDiv,
		"OR":  i.runOR,
		"XOR": i.runXOR, // XOR XORI
		"AND": i.runAND,
		//"LD":  i.runLoad,  // LDI, STI // need some adaption for more complex memory
		//"ST":  i.runStore, //able to load store imm as well
		"MOV": i.runMov,
		"JMP": i.runJmp,
		"BNE": i.runBne,
		"BEQ": i.runBeq, // BEQI
		"BLT": i.runBlt,
		"RET": i.runRet,

		"FADD": i.runFAdd, // FADDI
		"FSUB": i.runFSub,
		"FMUL": i.runFMul,
		"NOP":  i.runNOP,

		"PHI":       i.runPhi,
		"PHI_CONST": i.runPhiConst,
		"GPRED":     i.runGrantPred,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LDD": i.runLoadDirect,
		"STD": i.runStoreDirect,

		"TRIGGER": i.runTrigger,
	}

	if instFunc, ok := instFuncs[instName]; ok {
		instFunc(inst, state)
	} else {
		//panic("unknown instruction " + inst)
		panic(fmt.Sprintf("unknown instruction '%s' at PC %d", instName, state.PCInBlock))
	}
}

func (i instEmulator) findPCPlus4(state *coreState) {
	if state.SelectedBlock == nil {
		return // Just for test make the unit test to run normally
	}
	//fmt.Println("PC+4 ", state.PCInBlock)
	state.PCInBlock++
	if state.PCInBlock >= int32(len(state.SelectedBlock.CombinedInsts)) {
		state.PCInBlock = -1
		state.SelectedBlock = nil
		slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
	}
}

func (i instEmulator) getDirecIndex(side string) int {
	var srcIndex int

	switch side {
	case "North":
		srcIndex = int(cgra.North)
	case "West":
		srcIndex = int(cgra.West)
	case "South":
		srcIndex = int(cgra.South)
	case "East":
		srcIndex = int(cgra.East)
	case "NorthEast":
		srcIndex = int(cgra.NorthEast)
	case "NorthWest":
		srcIndex = int(cgra.NorthWest)
	case "SouthEast":
		srcIndex = int(cgra.SouthEast)
	case "SouthWest":
		srcIndex = int(cgra.SouthWest)
	case "Router":
		srcIndex = int(cgra.Router)
	default:
		panic("invalid side")
	}

	return srcIndex
}

func (i instEmulator) RouterSrcMustBeDirection(src string) {
	arr := []string{"NORTH", "SOUTH", "WEST", "EAST", "NORTHEAST", "NORTHWEST", "SOUTHEAST", "SOUTHWEST", "ROUTER"}
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

// float32 to uint32
func float2Uint(f float32) uint32 {
	return math.Float32bits(f)
}

// uint32 to float32
func uint2Float(u uint32) float32 {
	return math.Float32frombits(u)
}

/**
 * @description:
 * @prototype:
 */
/*
func (i instEmulator) runWait(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	colorIndex := i.getColorIndex(inst[3])

	i.waitSrcMustBeNetRecvReg(src)

	direction := src[9:]
	srcIndex := i.getDirecIndex(direction)

	if !state.RecvBufHeadReady[colorIndex][srcIndex] {
		return
	}

	state.RecvBufHeadReady[colorIndex][srcIndex] = false
	i.writeOperand(dst, state.RecvBufHead[colorIndex][srcIndex], state)
	state.PC++
}

func (i instEmulator) waitSrcMustBeNetRecvReg(src string) {
	if !strings.HasPrefix(src, "NET_RECV_") {
		panic("the source of a WAIT instruction must be NET_RECV registers")
	}
}

func (i instEmulator) runRecv(inst Inst, state *coreState) {
	// Parse the instruction arguments
	dstReg := inst[1] // The register to store the received value
	src := inst[2]    // The source side (e.g., NORTH, SOUTH, WEST, EAST)
	color := inst[3]  // The color of the message

	// Determine direction and color indices
	srcIndex := i.getDirecIndex(src)
	colorIndex := i.getColorIndex(color)

	// Check if the data is ready to be received from the buffer
	if !state.RecvBufHeadReady[colorIndex][srcIndex] {
		// If the data is not ready, just return and keep the PC as is.
		// This effectively stalls until the data is available.
		return
	}

	// Retrieve the data from the buffer and mark it as no longer ready
	data := state.RecvBufHead[colorIndex][srcIndex]
	state.RecvBufHeadReady[colorIndex][srcIndex] = false

	// Write the received value to the destination register
	i.writeOperand(dstReg, data, state)

	// Advance the program counter to the next instruction
	state.PC++

	// Debug log to indicate the RECV operation
	//fmt.Printf("RECV Instruction: Received %d from %s buffer, stored in %s\n", data, src, dstReg)
}*/

/**
 * @description:
 * @prototype:
 */

/*
func (i instEmulator) runSend(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	colorIndex := i.getColorIndex(inst[3])

	i.sendDstMustBeNetSendReg(dst)

	direction := dst[9:]
	dstIndex := i.getDirecIndex(direction)

	if state.SendBufHeadBusy[colorIndex][dstIndex] {
		return
	}

	state.SendBufHeadBusy[colorIndex][dstIndex] = true
	val := i.readOperand(src, state)
	state.SendBufHead[colorIndex][dstIndex] = val
	//fmt.Printf("SEND: Stored value %v in send buffer for color %d and destination index %d\n", val, colorIndex, dstIndex)
	state.PC++
}*/

// runMov handles the MOV instruction for both immediate values and register-to-register moves.
// Prototype for moving an immediate: MOV, DstReg, Immediate
// Prototype for register to register: MOV, DstReg, SrcReg
func (i instEmulator) runMov(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]

	// Write the value into the destination register
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, i.readOperand(src, state), state)
	}
	i.findPCPlus4(state)
}

func (i instEmulator) runNOP(inst Inst, state *coreState) {
	i.findPCPlus4(state)
}

/*
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
*/
func (i instEmulator) runLoadDirect(inst Inst, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addr := i.readOperand(src1, state)

	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}
	value := state.Memory[addr]

	slog.Warn("Memory",
		"Behavior", "LoadDirect",
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, value, state)
	}
	i.findPCPlus4(state)
}

func (i instEmulator) runStoreDirect(inst Inst, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addr := i.readOperand(src1, state)
	src2 := inst.SrcOperands.Operands[1]
	value := i.readOperand(src2, state)
	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}
	slog.Warn("Memory",
		"Behavior", "StoreDirect",
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	state.Memory[addr] = value
	i.findPCPlus4(state)
}

func (i instEmulator) runTrigger(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	i.readOperand(src, state)
	// just consume a operand and do nothing
	i.findPCPlus4(state)
}

/*
func (i instEmulator) sendDstMustBeNetSendReg(dst string) {
	if !strings.HasPrefix(dst, "NET_SEND_") {
		panic("the destination of a SEND instruction must be NET_SEND_ registers")
	}
}*/

/**
 * @description:
 * @prototype:
 */

func (i instEmulator) readOperand(operand Operand, state *coreState) (value uint32) {
	// if strings.HasPrefix(operand, "$") {
	// 	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
	// 	if err != nil {
	// 		panic("invalid register index")
	// 	}
	// 	value = state.Registers[registerIndex]
	// }
	if strings.HasPrefix(operand.Impl, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in readOperand: %v", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in readOperand", registerIndex))
		}

		value = state.Registers[registerIndex]
		//fmt.Println("[readOperand] read ", value, "from register", registerIndex, ":", value, "@(", state.TileX, ",", state.TileY, ")")
	} else if state.Directions[operand.Impl] {
		//fmt.Println("operand.Impl", operand.Impl)
		// must first check it is ready
		color, direction := i.getColorIndex(operand.Color), i.getDirecIndex(operand.Impl)
		value = state.RecvBufHead[color][direction]
		// set the ready flag to false
		state.RecvBufHeadReady[color][direction] = false
		//mt.Println("state.RecvBufHead[", i.getColorIndex(operand.Color), "][", i.getDirecIndex(operand.Impl), "]:", value)
		//fmt.Println("[ReadOperand] read", value, "from port", operand.Impl, ":", value, "@(", state.TileX, ",", state.TileY, ")")
	} else {
		// try to convert into int
		num, err := strconv.Atoi(operand.Impl)
		if err == nil {
			value = uint32(num)
		} else {
			if immediate, err := strconv.ParseUint(operand.Impl, 0, 32); err == nil {
				value = uint32(immediate)
			} else {
				panic(fmt.Sprintf("Invalid operand %v in readOperand; expected register", operand))
			}
		}
	}
	return value
}

func (i instEmulator) writeOperand(operand Operand, value uint32, state *coreState) {
	// if strings.HasPrefix(operand, "$") {
	// 	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
	// 	if err != nil {
	// 		panic("invalid register index")
	// 	}

	// 	state.Registers[registerIndex] = value
	// }
	if strings.HasPrefix(operand.Impl, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in writeOperand: %v", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in writeOperand", registerIndex))
		}

		state.Registers[registerIndex] = value
		//fmt.Printf("Updated register $%d to value %d at PC %d\n", registerIndex, value, state.PC)
	} else if state.Directions[operand.Impl] {
		if state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(operand.Impl)] {
			//fmt.Printf("sendbufhead busy\n")
			return
		}
		state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(operand.Impl)] = true
		state.SendBufHead[i.getColorIndex(operand.Color)][i.getDirecIndex(operand.Impl)] = value
	} else {
		panic(fmt.Sprintf("Invalid operand %v in writeOperand; expected register", operand))
	}
}

/**
 * @description:
 * @prototype:F32_CMP_[], Cmp_Res, Cmp_Src, imme
 */

/*
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
}*/

func (i instEmulator) parseAndCompareI(inst Inst, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state)
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst.SrcOperands.Operands[1].Impl, 10, 32)
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
	i.findPCPlus4(state)
}

func (i instEmulator) parseAndCompareF32(inst Inst, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state)
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst.SrcOperands.Operands[1].Impl, 10, 32)
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
	i.findPCPlus4(state)
}

/**
 * @description:
 * @prototype:
 */

/**
 * @description:
 * @prototype:
 */

/*
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
}*/

/**
 * @description:
 * Get data from
 * @prototype: MAC, DstReg, SrcReg1, SrcReg2
 */

/*
func (i instEmulator) runMac(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal += srcVal1 * srcVal2
	i.writeOperand(dst, dstVal, state)

	// fmt.Printf("Mac Instruction, Data are %v and %v, Res is %v\n", srcVal1, srcVal2, dstVal)
	// fmt.Printf("MAC: %s += %s * %s => Result: %v\n", dst, src1, src2, dstVal)
	state.PC++
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
	state.PC++
}*/

/**
 * @description:
 * Multiply function
 * @prototype: MUL, DstReg, SrcReg1, SrcReg2
 */

/**
 * @description: Add two numbers together. The input could be register or immediate number.
 * @prototype: ADD, DstReg, SrcReg1, SrcReg2(Imme)
 */
func (i instEmulator) runAdd(inst Inst, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	// 转换为有符号整数进行加法运算
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed + src2Signed
	dstVal := uint32(dstValSigned)

	//fmt.Printf("IADD: Adding %d (src1) + %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, dstVal, state)
	}
	i.findPCPlus4(state)
}

func (i instEmulator) runSub(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	// 转换为有符号整数进行减法运算
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed - src2Signed
	dstVal := uint32(dstValSigned)

	fmt.Printf("ISUB: Subtracting %d (src1) - %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	i.writeOperand(dst, dstVal, state)
	i.findPCPlus4(state)
}

func (i instEmulator) runMul(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)

	// 转换为有符号整数进行乘法运算
	src1Signed := int32(srcVal1)
	src2Signed := int32(srcVal2)
	dstValSigned := src1Signed * src2Signed
	dstVal := uint32(dstValSigned)

	i.writeOperand(dst, dstVal, state)
	i.findPCPlus4(state)
}

func (i instEmulator) runDiv(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)

	// 转换为有符号整数进行除法运算
	src1Signed := int32(srcVal1)
	src2Signed := int32(srcVal2)

	// 避免除零
	if src2Signed == 0 {
		panic("Division by zero at " + strconv.Itoa(int(state.PCInBlock)) + "@(" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ")")
	}

	dstValSigned := src1Signed / src2Signed
	dstVal := uint32(dstValSigned)

	i.writeOperand(dst, dstVal, state)

	fmt.Printf("DIV Instruction, Data are %d and %d, Res is %d\n", src1Signed, src2Signed, dstValSigned)
	i.findPCPlus4(state)
}

func (i instEmulator) runLLS(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	shiftStrVal := i.readOperand(shiftStr, state)

	result := srcVal << shiftStrVal
	i.writeOperand(dst, result, state)
	//fmt.Printf("LLS: %s = %s << %d => Result: %d\n", dst, src, shift, result)
	i.findPCPlus4(state)
}

func (i instEmulator) runLRS(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	shiftStrVal := i.readOperand(shiftStr, state)

	result := srcVal >> shiftStrVal
	i.writeOperand(dst, result, state)

	//fmt.Printf("LRS: %s = %s >> %d => Result: %d\n", dst, src, shift, result)
	i.findPCPlus4(state)
}

func (i instEmulator) runOR(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 | srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("OR: %s = %s | %s => Result: %d\n", dst, src1, src2, result)
	i.findPCPlus4(state)
}

func (i instEmulator) runXOR(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 ^ srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("XOR: %s = %s ^ %s => Result: %d\n", dst, src1, src2, result)
	i.findPCPlus4(state)
}

func (i instEmulator) runAND(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	result := srcVal1 & srcVal2
	i.writeOperand(dst, result, state)

	//fmt.Printf("AND: %s = %s & %s => Result: %d\n", dst, src1, src2, result)
	i.findPCPlus4(state)
}

func (i instEmulator) Jump(dst uint32, state *coreState) {
	state.PCInBlock = int32(dst)
}

func (i instEmulator) runJmp(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcVal := i.readOperand(src, state)
	i.Jump(srcVal, state)
}

func (i instEmulator) runBeq(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	immeVal := i.readOperand(imme, state)

	if srcVal == immeVal {
		i.Jump(srcVal, state)
	} else {
		i.findPCPlus4(state)
	}
}

func (i instEmulator) runBne(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	immeVal := i.readOperand(imme, state)

	if srcVal != immeVal {
		i.Jump(srcVal, state)
	} else {
		i.findPCPlus4(state)
	}
}

func (i instEmulator) runBlt(inst Inst, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	immeVal := i.readOperand(imme, state)

	if srcVal < immeVal {
		i.Jump(srcVal, state)
	} else {
		i.findPCPlus4(state)
	}
}

func (i instEmulator) runRet(inst Inst, state *coreState) {
	// not exist
	return
}

/*
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
*/

func (i instEmulator) runFAdd(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float + src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	i.findPCPlus4(state)
}

func (i instEmulator) runFSub(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float - src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	i.findPCPlus4(state)
}

func (i instEmulator) runFMul(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Uint := i.readOperand(src1, state)
	src2Uint := i.readOperand(src2, state)

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float * src2Float

	resultUint := float2Uint(resultFloat)
	i.writeOperand(dst, resultUint, state)

	i.findPCPlus4(state)
}

func (i instEmulator) runCmpExport(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	if src1Val == src2Val {
		i.writeOperand(dst, 1, state)
	} else {
		i.writeOperand(dst, 0, state)
	}
	i.findPCPlus4(state)
}

func (i instEmulator) runLTExport(inst Inst, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	if src1Val < src2Val {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, 1, state)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, 0, state)
		}
	}

	i.findPCPlus4(state)
}

func (i instEmulator) runPhi(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src3 := inst.SrcOperands.Operands[2]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)
	src3Val := i.readOperand(src3, state)

	var result uint32
	if src3Val == 0 {
		result = src1Val
	} else {
		result = src2Val
	}

	i.writeOperand(dst, result, state)
	i.findPCPlus4(state)
}

func (i instEmulator) runPhiConst(inst Inst, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	var result uint32
	if state.States["Phiconst"] == false {
		result = src1Val
		state.States["Phiconst"] = true
	} else {
		result = src2Val
	}
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, result, state)
	}
	i.findPCPlus4(state)
}

func (i instEmulator) runGrantPred(inst Inst, state *coreState) {
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]
	pred := inst.SrcOperands.Operands[1]

	srcVal := i.readOperand(src, state)
	predVal := i.readOperand(pred, state)

	if predVal == 1 {
		i.writeOperand(dst, srcVal, state)
	}

	i.findPCPlus4(state)
}

/*
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

func (i instEmulator) runNOT(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]

	srcVal := i.readOperand(src, state)
	result := ^srcVal
	i.writeOperand(dst, result, state)

	//fmt.Printf("NOT: %s = ~%s => Result: %d\n", dst, src, result)
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

func (i instEmulator) runConfigRouting(inst []string, state *coreState) {
	src := inst[2]
	dst := inst[1]
	color := inst[3]

	rule := &routingRule{
		src:   cgra.Side(i.getDirecIndex(src)),
		dst:   cgra.Side(i.getDirecIndex(dst)),
		color: color,
	}

	i.addRoutingRule(rule, state)
	state.PC++
}

func (i instEmulator) addRoutingRule(rule *routingRule, state *coreState) {
	for _, r := range state.routingRules {
		if r.src == rule.src && r.color == rule.color {
			r.dst = rule.dst
			return
		}
	}

	state.routingRules = append(state.routingRules, rule)
}
*/

// func (i instEmulator) runRoutingRules(state *coreState) (madeProgress bool) {
// 	for _, rule := range state.routingRules {
// 		srcIndex := int(rule.src)
// 		dstIndex := int(rule.dst)
// 		colorIndex := i.getColorIndex(rule.color)

// 		if !state.RecvBufHeadReady[colorIndex][srcIndex] {
// 			continue
// 		}

// 		if state.SendBufHeadBusy[colorIndex][dstIndex] {
// 			continue
// 		}

// 		state.RecvBufHeadReady[colorIndex][srcIndex] = false
// 		state.SendBufHeadBusy[colorIndex][dstIndex] = true
// 		state.SendBufHead[colorIndex][dstIndex] =
// 			state.RecvBufHead[colorIndex][srcIndex]
// 		madeProgress = true

// 		fmt.Printf("Tile[%d][%d], %s->%s, %s\n",
// 			state.TileX, state.TileY,
// 			rule.src.Name(), rule.dst.Name(), rule.color)
// 	}

// 	return madeProgress
// }

/**
 * @description: If data is sent to the src side of the current tile, the instruction will receive it,
 *				save it to the register and send the old data to the dst side of the current tile,
 *				with no time consumed. (We need some dummy tail!!!)
 * @prototype: Trigger_Send, dst, reg, src, color
 */

/*
func (i instEmulator) runTriggerSend(inst []string, state *coreState) {
	src := inst[1]
	reg := inst[2]
	dst := inst[3]
	color := inst[4]

	srcIndex := i.getDirecIndex(src)
	dstIndex := i.getDirecIndex(dst)
	colorIndex := i.getColorIndex(color)

	if state.RecvBufHeadReady[colorIndex][srcIndex] &&
		state.SendBufHeadBusy[colorIndex][dstIndex] {
		dataRecv := state.RecvBufHead[colorIndex][srcIndex]
		dataSend := i.readOperand(reg, state)

		i.writeOperand(reg, dataRecv, state)

		state.RecvBufHeadReady[colorIndex][srcIndex] = false
		state.SendBufHeadBusy[colorIndex][dstIndex] = true
		state.SendBufHead[colorIndex][dstIndex] = dataSend
	}
	state.PC++
}*/

/**
 * @description: When the data from two sides are available, trigger the code block.
 * @prototype: Trigger_Two_Side, $Code_Block$, Src1, Src2
 */

func (i instEmulator) runTriggerTwoSide(inst []string, state *coreState) {
	codeBlock := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	parts1 := strings.Split(src1, "_")
	parts2 := strings.Split(src2, "_")

	src1Index := i.getDirecIndex(parts1[0])
	src2Index := i.getDirecIndex(parts2[0])
	color1Index := i.getColorIndex(parts1[1])
	color2Index := i.getColorIndex(parts2[1])

	// Store the trigger into state trigger list whether triggered or not.
	trigger := &Trigger{
		color:  color1Index,
		branch: codeBlock,
	}
	trigger.src[src1Index] = true
	trigger.src[src2Index] = true

	i.addTrigger(trigger, state)

	if state.RecvBufHeadReady[color1Index][src1Index] &&
		state.RecvBufHeadReady[color2Index][src2Index] {
		//fmt.Print("Triggered\n")
		//i.Jump(codeBlock, state)
		return
	}
	//fmt.Print("Untriggered\n")
	i.findPCPlus4(state)
}

/**
 * @description: When the data from the side is available, trigger the code block.
 * @prototype: Trigger_One_Side, $Code_Block$, Src
 */
func (i instEmulator) runTriggerOneSide(inst []string, state *coreState) {
	codeBlock := inst[1]
	src := inst[2]

	parts := strings.Split(src, "_")

	srcIndex := i.getDirecIndex(parts[0])
	colorIndex := i.getColorIndex(parts[1])

	trigger := &Trigger{
		color:  colorIndex,
		branch: codeBlock,
	}
	trigger.src[srcIndex] = true
	if state.RecvBufHeadReady[colorIndex][srcIndex] {
		//i.Jump(codeBlock, state)
		//fmt.Print("Triggered\n")
		return
	}
	//fmt.Print("Untriggered\n")
	i.findPCPlus4(state)
}

// Add new trigger or modify existing trigger.
func (i instEmulator) addTrigger(trigger *Trigger, state *coreState) {
	for _, t := range state.triggers {
		if t.src[0] == trigger.src[0] &&
			t.src[1] == trigger.src[1] &&
			t.src[2] == trigger.src[2] &&
			t.src[3] == trigger.src[3] &&
			t.color == trigger.color {
			t.branch = trigger.branch
			return
		}
	}

	state.triggers = append(state.triggers, trigger)
}

// Waste One time click
func (i instEmulator) runIdle(state *coreState) {
	i.findPCPlus4(state)
}

// RECV_SEND Dst, DstReg, Src
/*
func (i instEmulator) runRecvSend(inst []string, state *coreState) {
	dst := inst[1]
	dstReg := inst[2]
	src := inst[3]
	srcParts := strings.Split(src, "_")
	dstParts := strings.Split(dst, "_")

	srcIndex := i.getDirecIndex(srcParts[0])
	dstIndex := i.getDirecIndex(dstParts[0])
	srcColorIndex := i.getColorIndex(srcParts[1])
	dstColorIndex := i.getColorIndex(dstParts[1])
	if !state.RecvBufHeadReady[srcColorIndex][srcIndex] {
		//fmt.Printf("recvbufhead not ready\n")
		return
	}

	val := state.RecvBufHead[srcColorIndex][srcIndex]
	state.RecvBufHeadReady[srcColorIndex][srcIndex] = false

	i.writeOperand(dstReg, val, state)
	if state.SendBufHeadBusy[dstColorIndex][dstIndex] {
		//fmt.Printf("sendbufhead busy\n")
		return
	}
	state.SendBufHeadBusy[dstColorIndex][dstIndex] = true
	state.SendBufHead[dstColorIndex][dstIndex] = val
	state.PC++
}*/

// Sleep
// It will go through all the triggers in the codes and to find the first fulfilled one
// and jump to the branch
func (i instEmulator) runSleep(inst []string, state *coreState) {
	for _, t := range state.triggers {
		flag := true
		color := t.color
		//branch := t.branch
		for i := 0; i < 4; i++ {
			if t.src[i] && !(state.RecvBufHeadReady[color][i]) {
				flag = false
			}
		}
		if flag {
			//fmt.Printf("[%d][%d]Sleep: Triggered: %s\n", state.TileX, state.TileY, t.branch)
			//i.Jump(branch, state)
			return
		}
	}
	//fmt.Printf("[%d][%d]Sleep: Untriggered. PC%d\n", state.TileX, state.TileY, state.PC)
	// When sleep, register all registers.
	//No PC++. We want this part is a cycle until one trigger is fulfilled.
}

/*
func (i instEmulator) runSendRecv(inst []string, state *coreState) {
	dst := inst[1]
	dstReg := inst[2]
	src := inst[3]

	srcParts := strings.Split(src, "_")
	dstParts := strings.Split(dst, "_")

	srcIndex := i.getDirecIndex(srcParts[0])
	dstIndex := i.getDirecIndex(dstParts[0])
	srcColorIndex := i.getColorIndex(srcParts[1])
	dstColorIndex := i.getColorIndex(dstParts[1])

	if !state.RecvBufHeadReady[srcColorIndex][srcIndex] {
		return
	}

	if state.SendBufHeadBusy[dstColorIndex][dstIndex] {
		return
	}
	sendVal := i.readOperand(dstReg, state)

	state.SendBufHeadBusy[dstColorIndex][dstIndex] = true
	state.SendBufHead[dstColorIndex][dstIndex] = sendVal

	val := state.RecvBufHead[srcColorIndex][srcIndex]
	state.RecvBufHeadReady[srcColorIndex][srcIndex] = false

	i.writeOperand(dstReg, val, state)
	state.PC++
}*/
