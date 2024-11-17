package core

import (
	"fmt"
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
	src    [4]bool
	color  int
	branch string
}

type coreState struct {
	PC           uint32
	TileX, TileY uint32
	Registers    []uint32
	Code         []string

	RecvBufHead      [][]uint32 //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]uint32
	SendBufHeadBusy  [][]bool

	routingRules []*routingRule
	triggers     []*Trigger
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
		"WAIT":             i.runWait,
		"SEND":             i.runSend,
		"RECV":				i.runRecv,
		"JMP":              i.runJmp,
		"JEQ":              i.runJeq,
		"JNE":              i.runJne,
		"CMP":              i.runCmp,
		"DONE":             func(_ []string, _ *coreState) { i.runDone() }, // Since runDone might not have parameters
		"MAC":              i.runMac,
		"MUL":              i.runMul,
		"CONFIG_ROUTING":   i.runConfigRouting,
		"TRIGGER_SEND":     i.runTriggerSend,
		"TRIGGER_TWO_SIDE": i.runTriggerTwoSide, // must be from two direction
		"TRIGGER_ONE_SIDE": i.runTriggerOneSide,
		"ADDI":             i.runIAdd,
		"IDLE":             func(_ []string, state *coreState) { i.runIdle(state) },
		"RECV_SEND":        i.runRecvSend,
		"SEND_RECV":        i.runSendRecv,
		"SLEEP":            i.runSleep,
		"MOV":				i.runMov,
		//"ADDF": 			i.runFAdd,
		//ld, st instructions
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


/**
 * @description:
 * @prototype:
 */
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

func (i instEmulator) runRecv(inst []string, state *coreState) {
    // Parse the instruction arguments
    dstReg := inst[1]    // The register to store the received value
    src := inst[2]       // The source side (e.g., NORTH, SOUTH, WEST, EAST)
    color := inst[3]     // The color of the message

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
    fmt.Printf("RECV Instruction: Received %d from %s buffer, stored in %s\n", data, src, dstReg)
}


// runMov handles the MOV instruction for both immediate values and register-to-register moves.
// Prototype for moving an immediate: MOV, DstReg, Immediate
// Prototype for register to register: MOV, DstReg, SrcReg
func (i instEmulator) runMov(inst []string, state *coreState) {
    dst := inst[1]
    src := inst[2]

    // Determine if the source is an immediate value or a register
    var value uint32
    if strings.HasPrefix(src, "$") {
        // Source is a register, so read the value from that register
        value = i.readOperand(src, state)
    } else {
        // Source is an immediate value, so parse it from string to uint32
        immediateValue, err := strconv.ParseUint(src, 10, 32)
        if err != nil {
            panic(fmt.Sprintf("invalid immediate value for MOV: %s", src))
        }
        value = uint32(immediateValue)
    }

    // Write the value into the destination register
    i.writeOperand(dst, value, state)

    fmt.Printf("MOV Instruction: Moving %v into %s\n", value, dst)

    state.PC++
}

// func (i instEmulator) runFAdd(inst []string, state *coreState) {
//     dst := inst[1]  // Destination register
//     src1 := inst[2] // Source register 1
//     src2 := inst[3] // Source register 2

//     // Read the values from the source registers
//     srcVal1 := i.readOperand(src1, state)
//     srcVal2 := i.readOperand(src2, state)

//     // Convert uint32 to float32
//     srcVal1Float := math.Float32frombits(srcVal1)
//     srcVal2Float := math.Float32frombits(srcVal2)

//     // Perform the floating-point addition
//     resultFloat := srcVal1Float + srcVal2Float

//     // Convert the result back to uint32
//     resultUint := math.Float32bits(resultFloat)

//     // Write the result to the destination register
//     i.writeOperand(dst, resultUint, state)

//     // Advance the program counter
//     state.PC++

//     // Debug log for verification
//     fmt.Printf("ADDF: %s = %f + %f = %f\n", dst, srcVal1Float, srcVal2Float, resultFloat)
// }

/**
 * @description:
 * @prototype:
 */
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
	fmt.Printf("SEND: Stored value %v in send buffer for color %d and destination index %d\n", val, colorIndex, dstIndex)
	state.PC++
}

func (i instEmulator) sendDstMustBeNetSendReg(dst string) {
	if !strings.HasPrefix(dst, "NET_SEND_") {
		panic("the destination of a SEND instruction must be NET_SEND registers")
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
	// if strings.HasPrefix(operand, "$") {
	// 	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
	// 	if err != nil {
	// 		panic("invalid register index")
	// 	}

	// 	value = state.Registers[registerIndex]
	// }
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
	// if strings.HasPrefix(operand, "$") {
	// 	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
	// 	if err != nil {
	// 		panic("invalid register index")
	// 	}

	// 	state.Registers[registerIndex] = value
	// }
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
		fmt.Printf("Updated register $%d to value %d at PC %d\n", registerIndex, value, state.PC)
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
 * @prototype: MAC, DstReg, SrcReg1, SrcReg2
 */
func (i instEmulator) runMac(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal += srcVal1 * srcVal2
	i.writeOperand(dst, dstVal, state)

	fmt.Printf("Mac Instruction, Data are %v and %v, Res is %v\n", srcVal1, srcVal2, dstVal)
	fmt.Printf("MAC: %s += %s * %s => Result: %v\n", dst, src1, src2, dstVal)
	state.PC++
}

/**
 * @description:
 * Get data from
 * @prototype: MUL, DstReg, SrcReg1, SrcReg2
 */
 func (i instEmulator) runMul(inst []string, state *coreState) {
	dst := inst[1]
	src1 := inst[2]
	src2 := inst[3]

	srcVal1 := i.readOperand(src1, state)
	srcVal2 := i.readOperand(src2, state)
	dstVal := i.readOperand(dst, state)
	dstVal = srcVal1 * srcVal2
	i.writeOperand(dst, dstVal, state)

	fmt.Printf("Mul Instruction, Data are %v and %v, Res is %v\n", srcVal1, srcVal2, dstVal)

	state.PC++
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
}

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
		i.Jump(codeBlock, state)
		return
	}
	//fmt.Print("Untriggered\n")
	state.PC++
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
		i.Jump(codeBlock, state)
		//fmt.Print("Triggered\n")
		return
	}
	//fmt.Print("Untriggered\n")
	state.PC++
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
	fmt.Printf("IADD: Adding %v (src1) + %v (src2) = %v\n", src1Val, src2Val, dstVal)
	i.writeOperand(dst, dstVal, state)
	state.PC++
}

// Waste One time click
func (i instEmulator) runIdle(state *coreState) {
	state.PC++
}

// RECV_SEND Dst, DstReg, Src
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
		return
	}

	val := state.RecvBufHead[srcColorIndex][srcIndex]
	state.RecvBufHeadReady[srcColorIndex][srcIndex] = false

	i.writeOperand(dstReg, val, state)

	if state.SendBufHeadBusy[dstColorIndex][dstIndex] {
		return
	}

	state.SendBufHeadBusy[dstColorIndex][dstIndex] = true
	state.SendBufHead[dstColorIndex][dstIndex] = val
	state.PC++
}

// Sleep
// It will go through all the triggers in the codes and to find the first fulfilled one
// and jump to the branch
func (i instEmulator) runSleep(inst []string, state *coreState) {
	for _, t := range state.triggers {
		flag := true
		color := t.color
		branch := t.branch
		for i := 0; i < 4; i++ {
			if t.src[i] && !(state.RecvBufHeadReady[color][i]) {
				flag = false
			}
		}
		if flag {
			//fmt.Printf("[%d][%d]Sleep: Triggered: %s\n", state.TileX, state.TileY, t.branch)
			i.Jump(branch, state)
			return
		}
	}
	//fmt.Printf("[%d][%d]Sleep: Untriggered. PC%d\n", state.TileX, state.TileY, state.PC)
	// When sleep, register all registers.
	//No PC++. We want this part is a cycle until one trigger is fulfilled.
}

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
}
