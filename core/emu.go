package core

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

type OpMode int

const (
	SyncOp OpMode = iota
	AsyncOp
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

type ReservationState struct {
	ReservationMap  map[int]bool // to show whether each operation of a instruction group is finished.
	OpToExec        int
	RefCountRuntime map[string]int // to record the left times to be used of each source opearand. deep copied from RefCount
}

// return bool, True means the operand is still in use, False means the operand is not in use anymore
func (r *ReservationState) DecrementRefCount(opr Operand, state *coreState) bool {
	key := opr.Impl + opr.Color
	if _, ok := r.RefCountRuntime[key]; ok {
		if r.RefCountRuntime[key] == 0 {
			panic("ref count is 0 in DecrementRefCount before decrement " + key + "@(" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ")")
		}
		r.RefCountRuntime[key]--
		if r.RefCountRuntime[key] == 0 {
			return false
		}
		return true
	} else {
		// something wrong, raise error
		panic("invalid operand in DecrementRefCount")
	}
}

func (r *ReservationState) SetRefCount(ig InstructionGroup, state *coreState) {
	for _, op := range ig.Operations {
		for _, opr := range op.SrcOperands.Operands {
			if state.Directions[opr.Impl] {
				key := opr.Impl + opr.Color
				if _, ok := r.RefCountRuntime[key]; ok {
					r.RefCountRuntime[key]++
				} else {
					r.RefCountRuntime[key] = 1
				}
			}
		}
		// only port in the src is needed to be considered
	}
}

func (r *ReservationState) SetReservationMap(ig InstructionGroup, state *coreState) {
	for i := 0; i < len(ig.Operations); i++ {
		r.ReservationMap[i] = true
	}
	r.OpToExec = len(ig.Operations)
	print("SetReservationMap: ", r.OpToExec, "\n")
}

type coreState struct {
	exit          *bool   // the signal is shared between cores
	retVal        *uint32 // the value is shared between cores
	SelectedBlock *EntryBlock
	Directions    map[string]bool
	PCInBlock     int32
	NextPCInBlock int32
	TileX, TileY  uint32
	Registers     []cgra.Data
	States        map[string]interface{} // This is to store core states, such as Phiconst, CmpFlags
	// still consider using outside block to control pc
	//Code         [][]string
	Memory               []uint32
	Code                 Program
	CurrReservationState ReservationState

	Mode OpMode

	RecvBufHead      [][]cgra.Data //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]cgra.Data
	SendBufHeadBusy  [][]bool
	AddrBuf          uint32 // buffer for the address of the memory
	IsToWriteMemory  bool

	routingRules []*routingRule
	triggers     []*Trigger
}

type instEmulator struct {
	CareFlags bool
}

/*
func (i instEmulator) RunInstructionGroup(cinst InstructionGroup, state *coreState) {
	LogState(state)

	instOpcodes := make([]string, len(cinst.Operations))
	for i, inst := range cinst.Operations {
		instOpcodes[i] = inst.OpCode
	}

	// check and run all the operations in the instruction group

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
	for _, inst := range cinst.Operations {
		i.RunInst(inst, state)
	}
	LogState(state)
}
*/

// set up the necessary state for the instruction group
func (i instEmulator) SetUpInstructionGroup(index int32, state *coreState) {
	iGroup := state.SelectedBlock.InstructionGroups[index]

	state.CurrReservationState = ReservationState{
		ReservationMap:  make(map[int]bool),
		OpToExec:        0,
		RefCountRuntime: make(map[string]int),
	}
	state.CurrReservationState.SetReservationMap(iGroup, state)
	state.CurrReservationState.SetRefCount(iGroup, state)
}

func (i instEmulator) RunInstructionGroup(cinst InstructionGroup, state *coreState, time float64) bool {
	// check the Return signal
	if *state.exit {
		return false
	}
	prevPC := state.PCInBlock
	prevCount := state.CurrReservationState.OpToExec
	progress_sync := false
	if state.Mode == SyncOp {
		progress_sync = i.RunInstructionGroupWithSyncOps(cinst, state, time)
	} else if state.Mode == AsyncOp {
		i.RunInstructionGroupWithAsyncOps(cinst, state, time)
	} else {
		panic("invalid mode")
	}

	nowCount := state.CurrReservationState.OpToExec

	// find the nextPC
	if state.Mode == AsyncOp {
		if state.CurrReservationState.OpToExec == 0 { // this instruction group is finished
			if state.NextPCInBlock == -1 { // nobody elect PC other than +4
				state.PCInBlock += 1
			} else { //  there is a jump
				state.PCInBlock = state.NextPCInBlock
				// not set block, in case of index out of range
			}

			if state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
				state.PCInBlock = -1
				state.SelectedBlock = nil
				slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
			} else {
				// set up for the new instruction group
				i.SetUpInstructionGroup(state.PCInBlock, state)
			}
			state.NextPCInBlock = -1
		} // else, this group is not finished, PC stays the same
	} else if state.Mode == SyncOp {
		if progress_sync {
			if state.NextPCInBlock == -1 {
				print("PC+4 X:", state.TileX, " Y:", state.TileY, "\n")
				state.PCInBlock++
			} else {
				print("PC+Jump to ", state.NextPCInBlock, " X:", state.TileX, " Y:", state.TileY, "\n")
				state.PCInBlock = state.NextPCInBlock
			}
		}
		if state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
			state.PCInBlock = -1
			state.SelectedBlock = nil
			print("PCInBlock = -1\n")
			slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
		}
		state.NextPCInBlock = -1
	} else {
		panic("invalid mode")
	}

	nowPC := state.PCInBlock

	if state.Mode == AsyncOp {
		if prevPC == nowPC && prevCount == nowCount {
			//print("Kernel (", state.TileX, ",", state.TileY, ") want to sleep, ", prevPC, " = ", nowPC, " ", prevCount, " = ", nowCount, " ", time, "\n")
			return false
		}
	} else if state.Mode == SyncOp {
		return progress_sync
	} else {
		panic("invalid mode")
	}
	return true
}

func (i instEmulator) RunInstructionGroupWithSyncOps(cinst InstructionGroup, state *coreState, time float64) bool {
	run := true
	for _, operation := range cinst.Operations {
		if (!i.CareFlags) || i.CheckFlags(operation, state) {
			continue
		} else {
			run = false
			break
		}
	}
	if run {
		for _, operation := range cinst.Operations {
			i.RunOperation(operation, state, time)
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		}
	}
	return run
}

func (i instEmulator) RunInstructionGroupWithAsyncOps(cinst InstructionGroup, state *coreState, time float64) {
	for index, operation := range cinst.Operations {
		// check all the operations in the instruction group and if any is ready, then run
		if !state.CurrReservationState.ReservationMap[index] {
			continue
		}
		if (!i.CareFlags) || i.CheckFlags(operation, state) { // can also only choose one (another pattern)
			state.CurrReservationState.ReservationMap[index] = false
			state.CurrReservationState.OpToExec--
			i.RunOperation(operation, state, time)
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		} else {
			//print("CheckFlags (", state.TileX, ",", state.TileY, ")", time, ":", "NO", "\n")
		}
	}
}

func (i instEmulator) CheckFlags(inst Operation, state *coreState) bool {
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
	fmt.Println("Check", inst.OpCode, "@(", state.TileX, ",", state.TileY, "):", flag)
	return flag
}

func (i instEmulator) RunOperation(inst Operation, state *coreState, time float64) {

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

	Trace("Inst", "Time", time, "OpCode", inst.OpCode, "X", state.TileX, "Y", state.TileY)

	instFuncs := map[string]func(Operation, *coreState){
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

		"PHI":             i.runPhi,
		"PHI_CONST":       i.runPhiConst,
		"GRANT_PREDICATE": i.runGrantPred,
		"GRANT_ONCE":      i.runGrantOnce,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LDD": i.runLoadDirect,
		"STD": i.runStoreDirect,

		"LD":  i.runLoadDRAM,
		"LDW": i.runLoadWaitDRAM,

		"ST":  i.runStoreDRAM,
		"STW": i.runStoreWaitDRAM,

		"TRIGGER": i.runTrigger,

		"NOT": i.runNot,
	}

	if instFunc, ok := instFuncs[instName]; ok {
		instFunc(inst, state)
	} else {
		//panic("unknown instruction " + inst)
		panic(fmt.Sprintf("unknown instruction '%s' at PC %d", instName, state.PCInBlock))
	}
}

func (i instEmulator) readOperand(operand Operand, state *coreState) (value cgra.Data) {
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
		if state.Mode == SyncOp {
			state.RecvBufHeadReady[color][direction] = false
		} else {
			if !state.CurrReservationState.DecrementRefCount(operand, state) {
				state.RecvBufHeadReady[color][direction] = false // no longer used, closed
				//fmt.Println("Reduce {", operand.Impl, "} to zero")
			} else {
				//fmt.Println("Reduce {", operand.Impl, "} to ", state.CurrReservationState.RefCountRuntime[operand.Impl], "@(", state.TileX, ",", state.TileY, ")")
			}
		}
		//fmt.Println("state.RecvBufHead[", i.getColorIndex(operand.Color), "][", i.getDirecIndex(operand.Impl), "]:", value)
		//fmt.Println("[ReadOperand] read", value, "from port", operand.Impl, ":", value, "@(", state.TileX, ",", state.TileY, ")")
	} else {
		// try to convert into int
		num, err := strconv.Atoi(operand.Impl)
		if err == nil {
			value = cgra.NewScalar(uint32(num))
		} else {
			if immediate, err := strconv.ParseUint(operand.Impl, 0, 32); err == nil {
				value = cgra.NewScalar(uint32(immediate))
			} else {
				panic(fmt.Sprintf("Invalid operand %v in readOperand; expected register", operand))
			}
		}
	}
	return value
}

func (i instEmulator) writeOperand(operand Operand, value cgra.Data, state *coreState) {
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

/*
func (i instEmulator) findPCPlus4(state *coreState) {
	if state.SelectedBlock == nil {
		return // Just for test make the unit test to run normally
	}
	//fmt.Println("PC+4 ", state.PCInBlock)
	if state.Mode == SyncOp {
		state.PCInBlock++
		if state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
			state.PCInBlock = -1
			state.SelectedBlock = nil
			slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
		}
	} else if state.Mode == AsyncOp {
		if state.CurrReservationState.OpToExec == 0 {
		} else {
			state.PCInBlock = state.NextPCInBlock
		}
	} else {
		panic("invalid mode")
	}
}
*/

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
		panic("Wrong Color: [" + color + "]")
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

// runMov handles the MOV instruction for both immediate values and register-to-register moves.
// Prototype for moving an immediate: MOV, DstReg, Immediate
// Prototype for register to register: MOV, DstReg, SrcReg
func (i instEmulator) runMov(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	oprStruct := i.readOperand(src, state)
	opr := oprStruct.First()

	// Write the value into the destination register
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(opr, oprStruct.Pred), state)
	}
}

func (i instEmulator) runNOP(inst Operation, state *coreState) {
	// do nothing
}

func (i instEmulator) runNot(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	srcPred := srcStruct.Pred

	result := uint32(0)
	if srcVal == 0 {
		result = 1
	} else {
		result = 0
	}
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, srcPred), state)
	}
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
func (i instEmulator) runLoadDirect(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()

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
		i.writeOperand(dst, cgra.NewScalarWithPred(value, addrStruct.Pred), state)
	}
	// elect no next PC
}

func (i instEmulator) runLoadDRAM(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	dst := inst.DstOperands.Operands[0]
	if dst.Impl != "Router" {
		panic("the destination of a LOAD_DRAM instruction must be Router")
	}

	slog.Warn("DRAM",
		"Behavior", "LoadDRAM",
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(addr, addrStruct.Pred), state)
	}
	state.AddrBuf = addr
	state.IsToWriteMemory = false // not for write memory
}

func (i instEmulator) runLoadWaitDRAM(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a LOAD_WAIT_DRAM instruction must be Router")
	}
	valueStruct := i.readOperand(src, state)
	value := valueStruct.First()
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(value, valueStruct.Pred), state)
	}
}

func (i instEmulator) runStoreDirect(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	src2 := inst.SrcOperands.Operands[1]
	valueStruct := i.readOperand(src2, state)
	value := valueStruct.First()
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
	// elect no next PC
}

func (i instEmulator) runStoreDRAM(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	src2 := inst.SrcOperands.Operands[1]
	valueStruct := i.readOperand(src2, state)
	value := valueStruct.First()
	dst := inst.DstOperands.Operands[0]
	if dst.Impl != "Router" {
		panic("the destination of a STORE_DRAM instruction must be Router")
	}

	slog.Warn("DRAM",
		"Behavior", "StoreDRAM",
		"Addr", addr,
		"Value", value,
		"X", state.TileX,
		"Y", state.TileY,
	)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(value, addrStruct.Pred && valueStruct.Pred), state)
	}
	state.AddrBuf = addr
	state.IsToWriteMemory = true // for write memory
}

func (i instEmulator) runStoreWaitDRAM(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a STORE_WAIT_DRAM instruction must be Router")
	}
	i.readOperand(src, state) // do nothing, only get the write done
}

func (i instEmulator) runTrigger(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	i.readOperand(src, state)
	// just consume a operand and do nothing
	// elect no next PC
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

func (i instEmulator) parseAndCompareI(inst Operation, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state).First()
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
	i.writeOperand(dst, cgra.NewScalar(dstVal), state)
	// elect no next PC
}

func (i instEmulator) parseAndCompareF32(inst Operation, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state).First()
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
	i.writeOperand(dst, cgra.NewScalar(dstVal), state)
	// elect no next PC
}

func (i instEmulator) runAdd(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed + src2Signed
	dstVal := uint32(dstValSigned)

	//fmt.Printf("IADD: Adding %d (src1) + %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dstVal, src1Struct.Pred && src2Struct.Pred), state)
	}
	// elect no next PC
}

func (i instEmulator) runSub(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed - src2Signed
	dstVal := uint32(dstValSigned)

	fmt.Printf("ISUB: Subtracting %d (src1) - %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dstVal, src1Struct.Pred && src2Struct.Pred), state)
	}
	// elect no next PC
}

func (i instEmulator) runMul(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	// convert to signed integer for multiplication
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed * src2Signed
	dstVal := uint32(dstValSigned)

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dstVal, src1Struct.Pred && src2Struct.Pred), state)
	}
	// elect no next PC
}

func (i instEmulator) runDiv(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	// convert to signed integer for division
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	// avoid division by zero
	if src2Signed == 0 {
		panic("Division by zero at " + strconv.Itoa(int(state.PCInBlock)) + "@(" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ")")
	}

	dstValSigned := src1Signed / src2Signed
	dstVal := uint32(dstValSigned)

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dstVal, src1Struct.Pred && src2Struct.Pred), state)
	}

	// fmt.Printf("DIV Instruction, Data are %d and %d, Res is %d\n", src1Signed, src2Signed, dstValSigned)
	// elect no next PC
}

func (i instEmulator) runLLS(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal << shiftStrVal
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, srcStruct.Pred && shiftStrStruct.Pred), state)
	}
	//fmt.Printf("LLS: %s = %s << %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
}

func (i instEmulator) runLRS(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal >> shiftStrVal
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, srcStruct.Pred && shiftStrStruct.Pred), state)
	}

	//fmt.Printf("LRS: %s = %s >> %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
}

func (i instEmulator) runOR(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 | srcVal2
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, src1Struct.Pred && src2Struct.Pred), state)
	}

	//fmt.Printf("OR: %s = %s | %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
}

func (i instEmulator) runXOR(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 ^ srcVal2
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, src1Struct.Pred && src2Struct.Pred), state)
	}

	//fmt.Printf("XOR: %s = %s ^ %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
}

func (i instEmulator) runAND(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 & srcVal2
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalar(result), state)
	}

	//fmt.Printf("AND: %s = %s & %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
}

func (i instEmulator) Jump(dst uint32, state *coreState) {
	state.NextPCInBlock = int32(dst)
}

func (i instEmulator) runJmp(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	i.Jump(srcVal, state)
}

func (i instEmulator) runBeq(inst Operation, state *coreState) {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()

	if srcVal == immeVal {
		i.Jump(srcVal, state)
	} else {
		// elect no next PC
	}
}

func (i instEmulator) runBne(inst Operation, state *coreState) {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()

	if srcVal != immeVal {
		i.Jump(srcVal, state)
	} else {
		// elect no next PC
	}
}

func (i instEmulator) runBlt(inst Operation, state *coreState) {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()

	if srcVal < immeVal {
		i.Jump(srcVal, state)
	} else {
		// elect no next PC
	}
}

func (i instEmulator) runRet(inst Operation, state *coreState) {
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		srcStruct := i.readOperand(src, state)
		srcVal := srcStruct.First()
		srcPred := srcStruct.Pred
		if srcPred {
			*state.retVal = srcVal
			*state.exit = true
		}
	} else {
		*state.exit = true
		*state.retVal = 0
	}
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

func (i instEmulator) runFAdd(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float + src2Float

	resultUint := float2Uint(resultFloat)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(resultUint, src1Pred && src2Pred), state)
	}

	// elect no next PC
}

func (i instEmulator) runFSub(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float - src2Float

	resultUint := float2Uint(resultFloat)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(resultUint, src1Pred && src2Pred), state)
	}

	// elect no next PC
}

func (i instEmulator) runFMul(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float * src2Float

	resultUint := float2Uint(resultFloat)
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(resultUint, src1Pred && src2Pred), state)
	}

	// elect no next PC
}

func (i instEmulator) runCmpExport(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	if src1Val.First() == src2Val.First() && src1Val.Pred == src2Val.Pred {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(1, src1Val.Pred), state)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(0, src1Val.Pred), state)
		}
	}
	// elect no next PC
}

func (i instEmulator) runLTExport(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	if src1Val < src2Val {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(1, resultPred), state)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(0, resultPred), state)
		}
	}

	// elect no next PC
}

func (i instEmulator) runPhi(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	if src1Struct.Pred && !src2Struct.Pred {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(src1Struct.First(), src1Struct.Pred), state)
		}
	} else if !src1Struct.Pred && src2Struct.Pred {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(src2Struct.First(), src2Struct.Pred), state)
		}
	} else {
		panic("Phi operation: both sources have the same predicate")
	}

	// elect no next PC
}

func (i instEmulator) runPhiConst(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	var result uint32
	if state.States["Phiconst"] == false {
		result = src1Val
		state.States["Phiconst"] = true
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(result, src1Pred), state)
		}
	} else {
		result = src2Val
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(result, src2Pred), state)
		}
	}
	// elect no next PC
}

func (i instEmulator) runGrantPred(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	pred := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	predStruct := i.readOperand(pred, state)
	srcVal := srcStruct.First()
	predVal := predStruct.First()

	resultPred := false

	if predVal == 0 {
		resultPred = false
	} else {
		resultPred = true
	}

	//fmt.Printf("GRANTPRED: srcVal = %d, predVal = %t at (%d, %d)\n", srcVal, predVal, state.TileX, state.TileY)

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(srcVal, resultPred), state)
	}

	// elect no next PC
}

func (i instEmulator) runGrantOnce(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]

	srcStruct := i.readOperand(src, state)
	if state.States["GrantOnce"] == false {
		state.States["GrantOnce"] = true
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(srcStruct.First(), true), state)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			i.writeOperand(dst, cgra.NewScalarWithPred(srcStruct.First(), false), state)
		}
	}
}
