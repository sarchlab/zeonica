package core

import (
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

type coreState struct {
	PC               uint32
	TileX, TileY     uint32
	Registers        []uint32
	Code             []string
	RecvBufHead      [][]uint32 //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]uint32
	SendBufHeadBusy  [][]bool
}

type instEmulator struct {
}

func (i instEmulator) RunInst(inst string, state *coreState) {
	tokens := strings.Split(inst, ",")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	instName := tokens[0]
	if strings.Contains(instName, "CMP") {
		instName = "CMP"
	}
	// alwaysflag := false
	// if strings.HasPrefix(instName, "@") && !alwaysflag {
	// 	instName = "SENDREC"
	// 	alwaysflag = true
	// 	i.runAlwaysSendRec(tokens, state)
	// }

	instFuncs := map[string]func([]string, *coreState){
		"WAIT": i.runWait,
		"SEND": i.runSend,
		"JMP":  i.runJmp,
		"CMP":  i.runCmp,
		"JEQ":  i.runJeq,
		"DONE": func(_ []string, _ *coreState) { i.runDone() }, // Since runDone might not have parameters
		"MAC":  i.runMac,
	}

	if instFunc, ok := instFuncs[instName]; ok {
		instFunc(tokens, state)
	} else {
		panic("unknown instruction " + inst)
	}
}

func (i instEmulator) getIndex(side string) int {
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

func (i instEmulator) runWait(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	colorIndex := i.getColorIndex(inst[3])

	i.waitSrcMustBeNetRecvReg(src)

	direction := src[9:]
	srcIndex := i.getIndex(direction)

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

func (i instEmulator) runSend(inst []string, state *coreState) {
	dst := inst[1]
	src := inst[2]
	colorIndex := i.getColorIndex(inst[3])

	i.sendDstMustBeNetSendReg(dst)

	direction := dst[9:]
	dstIndex := i.getIndex(direction)

	if state.SendBufHeadBusy[colorIndex][dstIndex] {
		return
	}

	state.SendBufHeadBusy[colorIndex][dstIndex] = true
	val := i.readOperand(src, state)
	state.SendBufHead[colorIndex][dstIndex] = val
	state.PC++
}

func (i instEmulator) sendDstMustBeNetSendReg(dst string) {
	if !strings.HasPrefix(dst, "NET_SEND_") {
		panic("the destination of a SEND instruction must be NET_SEND registers")
	}
}

func (i instEmulator) runJmp(inst []string, state *coreState) {
	dst := inst[1]

	for i := 0; i < len(state.Code); i++ {
		line := strings.Trim(state.Code[i], " \t\n")
		if strings.HasPrefix(line, dst) && strings.HasSuffix(line, ":") {
			state.PC = uint32(i)
			return
		}
	}
}

func (i instEmulator) readOperand(operand string, state *coreState) (value uint32) {
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic("invalid register index")
		}

		value = state.Registers[registerIndex]
	}

	return
}

func (i instEmulator) writeOperand(operand string, value uint32, state *coreState) {
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic("invalid register index")
		}

		state.Registers[registerIndex] = value
	}
}

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

func (i instEmulator) runMac(inst []string, state *coreState) {
	src1 := inst[2]
	src2 := inst[3]
	dst := inst[1]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal += srcVal1 * srcVal2
	i.writeOperand(dst, dstVal, state)

	state.PC++
}

func (i instEmulator) runDone() {
	// Do nothing.
}
