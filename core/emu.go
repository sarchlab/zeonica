//nolint:funlen,gocyclo,lll,unused,gosimple
package core

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// OpMode controls how instruction groups are scheduled.
type OpMode int

const (
	ExitDelay = 100
)

const (
	SyncOp OpMode = iota
	// AsyncOp executes operations asynchronously with reservation tracking.
	AsyncOp
)

type routingRule struct {
	src   cgra.Side
	dst   cgra.Side
	color string
}

// Trigger describes routing trigger metadata.
type Trigger struct {
	src    [12]bool
	color  int
	branch string
}

// ReservationState tracks pending operations and operand usage.
type ReservationState struct {
	ReservationMap  map[int]bool // to show whether each operation of a instruction group is finished.
	OpToExec        int
	RefCountRuntime map[string]int // to record the left times to be used of each source opearand. deep copied from RefCount
}

// DecrementRefCount decrements runtime operand use count and reports if still in use.
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
	}
	// something wrong, raise error
	panic("invalid operand in DecrementRefCount")
}

// SetRefCount initializes runtime operand reference counters for an instruction group.
func (r *ReservationState) SetRefCount(ig InstructionGroup, state *coreState) {
	for _, op := range ig.Operations {
		for _, opr := range op.SrcOperands.Operands {
			if state.Directions[opr.Impl] {
				key := opr.Impl + opr.Color
				r.RefCountRuntime[key]++
			}
		}
		// only port in the src is needed to be considered
	}
}

// SetReservationMap marks operations in the instruction group as pending.
func (r *ReservationState) SetReservationMap(ig InstructionGroup, state *coreState) {
	for i := 0; i < len(ig.Operations); i++ {
		r.ReservationMap[i] = true
	}
	r.OpToExec = len(ig.Operations)
	print("SetReservationMap: ", r.OpToExec, "\n")
}

type coreState struct {
	exit                 *bool // the signal is shared between cores
	requestExitTimestamp *float64
	retVal               *uint32 // the value is shared between cores
	SelectedBlock        *EntryBlock
	Directions           map[string]bool
	PCInBlock            int32
	NextPCInBlock        int32
	TileX, TileY         uint32
	Registers            []cgra.Data
	States               map[string]interface{} // This is to store core states, such as Phiconst, CmpFlags
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
	CurrentTime  float64 // current simulation time for logging
}

type instEmulator struct {
	CareFlags bool
}

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
	if *state.exit && time > *state.requestExitTimestamp {
		fmt.Println("Exit signal ( requested at", *state.requestExitTimestamp, ") received at time", time)
		return false
	}
	prevPC := state.PCInBlock
	prevCount := state.CurrReservationState.OpToExec
	progressSync := false
	if state.Mode == SyncOp {
		progressSync = i.RunInstructionGroupWithSyncOps(cinst, state, time)
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
				state.PCInBlock++
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
		if progressSync {
			if state.NextPCInBlock == -1 {
				print("PC+4 for PC=", state.PCInBlock, " X:", state.TileX, " Y:", state.TileY, "\n")
				print("Instruction at PC=", state.PCInBlock, " is ", state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[0].OpCode, "\n")
				state.PCInBlock++
			} else {
				print("PC+Jump to ", state.NextPCInBlock, " X:", state.TileX, " Y:", state.TileY, "\n")
				state.PCInBlock = state.NextPCInBlock
			}
		}
		if state.SelectedBlock != nil && state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
			state.PCInBlock = -1
			state.SelectedBlock = nil
			print("PCInBlock = -1 at (", state.TileX, ",", state.TileY, ")\n")
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
		return progressSync
	} else {
		panic("invalid mode")
	}
	return true
}

func (i instEmulator) RunInstructionGroupWithSyncOps(cinst InstructionGroup, state *coreState, time float64) bool {
	run := true
	for _, operation := range cinst.Operations {
		if (!i.CareFlags) || operation.InvalidIterations > 0 || i.CheckFlags(operation, state) {
			continue
		} else {
			run = false
			break
		}
	}
	if run {
		// Collect all results first
		allResults := make(map[Operand]cgra.Data)
		for index := range cinst.Operations {
			// Get reference to the original operation in state.SelectedBlock
			operation := &state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[index]
			// Decrement InvalidIterations before running if needed
			if operation.InvalidIterations > 0 {
				print("Invalid iteration for ", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")\n")
				operation.InvalidIterations--
				continue
			}
			results := i.RunOperation(*operation, state, time)
			// Merge results into allResults
			for operand, value := range results {
				allResults[operand] = value
			}
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		}
		// Write all results at once
		for operand, value := range allResults {
			i.writeOperand(operand, value, state)
		}
	}
	return run
}

func (i instEmulator) RunInstructionGroupWithAsyncOps(cinst InstructionGroup, state *coreState, time float64) {
	// Collect all results first
	allResults := make(map[Operand]cgra.Data)
	for index := range cinst.Operations {
		// check all the operations in the instruction group and if any is ready, then run
		if !state.CurrReservationState.ReservationMap[index] {
			continue
		}
		// Get reference to the original operation in state.SelectedBlock
		operation := &state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[index]
		if (!i.CareFlags) || operation.InvalidIterations > 0 || i.CheckFlags(*operation, state) { // can also only choose one (another pattern)
			state.CurrReservationState.ReservationMap[index] = false
			state.CurrReservationState.OpToExec--
			// Decrement InvalidIterations before running if needed
			if operation.InvalidIterations > 0 {
				print("Invalid iteration for ", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")\n")
				operation.InvalidIterations--
				continue
			}
			results := i.RunOperation(*operation, state, time)
			// Merge results into allResults
			for operand, value := range results {
				allResults[operand] = value
			}
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		} else {
			//print("CheckFlags (", state.TileX, ",", state.TileY, ")", time, ":", "NO", "\n")
		}
	}
	// Write all results at once
	for operand, value := range allResults {
		i.writeOperand(operand, value, state)
	}
}

func (i instEmulator) normalizeDirection(s string) string {
	u := strings.ToUpper(s)
	switch u {
	case "NORTH":
		return "North"
	case "SOUTH":
		return "South"
	case "EAST":
		return "East"
	case "WEST":
		return "West"
	case "NORTHEAST":
		return "NorthEast"
	case "NORTHWEST":
		return "NorthWest"
	case "SOUTHEAST":
		return "SouthEast"
	case "SOUTHWEST":
		return "SouthWest"
	case "ROUTER":
		return "Router"
	default:
		return s
	}
}

func (i instEmulator) CheckFlags(inst Operation, state *coreState) bool {
	//PrintState(state)
	flag := true
	for index, src := range inst.SrcOperands.Operands {
		if index == 1 {
			if inst.OpCode == "PHI_CONST" || inst.OpCode == "PHI_START" {
				// Track PHI_CONST per instruction to avoid cross-interference.
				var stateKey string
				if inst.OpCode == "PHI_CONST" {
					stateKey = fmt.Sprintf("PhiConst_%d", inst.ID)
				} else if inst.OpCode == "PHI_START" {
					stateKey = fmt.Sprintf("PhiStart_%d", inst.ID)
				}
				if state.States[stateKey] == nil || state.States[stateKey] == false { // first execution
					if len(inst.SrcOperands.Operands) > 1 {
						fmt.Println("ID", inst.ID, "bypass check")
						continue
					} else {
						panic("PHI_CONST or PHI_START must have two sources")
					}
				}
			}
		}
		srcImpl := i.normalizeDirection(src.Impl)
		if state.Directions[srcImpl] {
			if !state.RecvBufHeadReady[i.getColorIndex(src.Color)][i.getDirecIndex(srcImpl)] {
				flag = false
				break
			}
		}
	}

	for _, dst := range inst.DstOperands.Operands {
		dstImpl := i.normalizeDirection(dst.Impl)
		if state.Directions[dstImpl] {
			if state.SendBufHeadBusy[i.getColorIndex(dst.Color)][i.getDirecIndex(dstImpl)] {
				flag = false
				break
			}
		}
	}
	//fmt.Println("[CheckFlags] checking flags for inst", inst.OpCode, "@(", state.TileX, ",", state.TileY, "):", flag)
	fmt.Println("Check", inst.OpCode, "ID", inst.ID, "@(", state.TileX, ",", state.TileY, "):", flag)
	return flag
}

func (i instEmulator) RunOperation(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	state.CurrentTime = time

	// Note: InvalidIterations is now handled in RunInstructionGroupWithSyncOps
	// before calling RunOperation, so we don't need to check it here

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

	instFuncs := map[string]func(Operation, *coreState) map[Operand]cgra.Data{
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

		// do not distinguish between data_mov and control mov
		"DATA_MOV": i.runMov,
		"CTRL_MOV": i.runMov,
		"CONSTANT": i.runMov,

		"GEP": i.runGep,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LOAD":  i.runLoadDirect,
		"STORE": i.runStoreDirect,
		"LDD":   i.runLoadDirect,  // backward compatibility
		"STD":   i.runStoreDirect, // backward compatibility

		"LD":  i.runLoadDRAM,
		"LDW": i.runLoadWaitDRAM,

		"ST":  i.runStoreDRAM,
		"STW": i.runStoreWaitDRAM,

		"TRIGGER": i.runTrigger,

		"NOT": i.runNot,
	}

	retFuncs := map[string]func(Operation, *coreState, float64) map[Operand]cgra.Data{
		"RETURN_VALUE": i.runRetImm,
		"RETURN_VOID":  i.runRetDelay,
		"RET":          i.runRetImm, // backward compatibility
	}

	if instFunc, ok := instFuncs[instName]; ok {
		return instFunc(inst, state)
	} else if retFunc, ok := retFuncs[instName]; ok {
		return retFunc(inst, state, time)
	} else {
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
	} else {
		normalizedImpl := i.normalizeDirection(operand.Impl)
		if state.Directions[normalizedImpl] {
			//fmt.Println("operand.Impl", operand.Impl)
			// must first check it is ready
			color, direction := i.getColorIndex(operand.Color), i.getDirecIndex(normalizedImpl)
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
			operand.Impl = strings.TrimPrefix(operand.Impl, "#")
			num, err := strconv.Atoi(operand.Impl)
			if err == nil {
				value = cgra.NewScalar(uint32(num))
			} else {
				if immediate, err := strconv.ParseUint(operand.Impl, 0, 32); err == nil {
					value = cgra.NewScalar(uint32(immediate))
				} else {
					panic(fmt.Sprintf("Invalid operand %v in readOperand; at PC %d, (%d, %d)", operand, state.PCInBlock, state.TileX, state.TileY))
				}
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
	} else {
		normalizedImpl := i.normalizeDirection(operand.Impl)
		if state.Directions[normalizedImpl] {
			if state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedImpl)] {
				//fmt.Printf("sendbufhead busy\n")
				return
			}
			state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedImpl)] = true
			state.SendBufHead[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedImpl)] = value
		} else {
			panic(fmt.Sprintf("Invalid operand %v in writeOperand; expected register", operand))
		}
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
	switch strings.ToUpper(color) {
	case "R", "RED":
		return 0
	case "Y", "YELLOW":
		return 1
	case "B", "BLUE":
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
func (i instEmulator) runMov(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	oprStruct := i.readOperand(src, state)
	opr := oprStruct.First()
	finalPred := oprStruct.Pred
	// Collect results for destination operands
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(opr, finalPred)
	}

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runGep(inst Operation, state *coreState) map[Operand]cgra.Data {
	results := make(map[Operand]cgra.Data)
	if len(inst.SrcOperands.Operands) == 0 {
		return results
	}

	src1 := inst.SrcOperands.Operands[0]
	src1Struct := i.readOperand(src1, state)
	src1Val := src1Struct.First()
	finalPred := src1Struct.Pred
	dstVal := src1Val

	if len(inst.SrcOperands.Operands) > 1 {
		src2 := inst.SrcOperands.Operands[1]
		src2Struct := i.readOperand(src2, state)
		src2Val := src2Struct.First()
		dstVal = src1Val + src2Val
		finalPred = src1Struct.Pred && src2Struct.Pred

		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	}

	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))

	return results
}

func (i instEmulator) runNOP(inst Operation, state *coreState) map[Operand]cgra.Data {
	// do nothing
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", false)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runNot(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := srcPred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runLoadDirect(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()

	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}
	value := state.Memory[addr]
	slog.Warn("Memory",
		"Behavior", "LoadDirect",
		"ID", inst.ID,
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	finalPred := addrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	//Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runLoadDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := addrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(addr, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	state.AddrBuf = addr
	state.IsToWriteMemory = false // not for write memory
	return results
}

func (i instEmulator) runLoadWaitDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a LOAD_WAIT_DRAM instruction must be Router")
	}
	valueStruct := i.readOperand(src, state)
	value := valueStruct.First()
	finalPred := valueStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runStoreDirect(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	valueStruct := i.readOperand(src1, state)
	value := valueStruct.First()
	src2 := inst.SrcOperands.Operands[1]
	addrStruct := i.readOperand(src2, state)
	addr := addrStruct.First()
	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds, addr: " + strconv.Itoa(int(addr)) + ", len(state.Memory): " + strconv.Itoa(len(state.Memory)))
	}
	slog.Warn("Memory",
		"Behavior", "StoreDirect",
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	state.Memory[addr] = value
	finalPred := addrStruct.Pred && valueStruct.Pred
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runStoreDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := addrStruct.Pred && valueStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	state.AddrBuf = addr
	state.IsToWriteMemory = true // for write memory
	return results
}

func (i instEmulator) runStoreWaitDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a STORE_WAIT_DRAM instruction must be Router")
	}
	srcStruct := i.readOperand(src, state) // do nothing, only get the write done
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runTrigger(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	// just consume a operand and do nothing
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	// elect no next PC
	return make(map[Operand]cgra.Data)
}

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

func (i instEmulator) runAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Struct.Pred && src2Struct.Pred
	//fmt.Printf("IADD: Adding %d (src1) + %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runSub(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runMul(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runMulAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	// MUL_ADD: dst = src0 * src1 + src2 (systolic MAC: psum += activation * weight)
	src0 := inst.SrcOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[1]
	src2 := inst.SrcOperands.Operands[2]

	s0 := i.readOperand(src0, state)
	s1 := i.readOperand(src1, state)
	s2 := i.readOperand(src2, state)

	s0Val := int32(s0.First())
	s1Val := int32(s1.First())
	s2Val := int32(s2.First())
	dstValSigned := s0Val*s1Val + s2Val
	dstVal := uint32(dstValSigned)
	finalPred := s0.Pred && s1.Pred && s2.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runDiv(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// fmt.Printf("DIV Instruction, Data are %d and %d, Res is %d\n", src1Signed, src2Signed, dstValSigned)
	// elect no next PC
	return results
}

func (i instEmulator) runSHL(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal << shiftStrVal
	finalPred := srcStruct.Pred && shiftStrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("LLS: %s = %s << %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
	return results
}

func (i instEmulator) runLRS(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal >> shiftStrVal
	finalPred := srcStruct.Pred && shiftStrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("LRS: %s = %s >> %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
	return results
}

func (i instEmulator) runOR(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 | srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("OR: %s = %s | %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) runXOR(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 ^ srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("XOR: %s = %s ^ %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) runAND(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 & srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("AND: %s = %s & %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) Jump(dst uint32, state *coreState) {
	state.NextPCInBlock = int32(dst)
}

func (i instEmulator) runJmp(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	i.Jump(srcVal, state)
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBeq(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal == immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBne(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal != immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBlt(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal < immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runRetImm(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	var finalPred bool
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		srcStruct := i.readOperand(src, state)
		srcVal := srcStruct.First()
		srcPred := srcStruct.Pred
		finalPred = srcPred
		if srcPred {
			slog.Info("Control: Cond",
				"X", state.TileX,
				"Y", state.TileY,
				"SrcVal", srcVal,
			)
			*state.retVal = srcVal
			*state.exit = true
			*state.requestExitTimestamp = time
			fmt.Println("++++++++++++ RETURN executed", srcVal, "T=", time)
		} else {
			fmt.Println("++++++++++++ RETURN bypassed")
		}
	} else {
		panic("RETURN_VALUE requires a source operand")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runRetDelay(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	var finalPred bool
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		srcStruct := i.readOperand(src, state)
		srcVal := srcStruct.First()
		srcPred := srcStruct.Pred
		finalPred = srcPred
		if srcPred {
			slog.Info("Control: Cond",
				"X", state.TileX,
				"Y", state.TileY,
				"SrcVal", srcVal,
			)
			*state.retVal = 0
			*state.exit = true
			*state.requestExitTimestamp = time + ExitDelay
			fmt.Println("++++++++++++ RETURN executed", srcVal, "T=", time)
		} else {
			fmt.Println("++++++++++++ RETURN bypassed")
		}
	} else {
		panic("RETURN_VOID requires a source operand")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runFAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runFSub(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runFMul(inst Operation, state *coreState) map[Operand]cgra.Data {
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
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runCmpExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	resultVal := 0

	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if src1Val.First() == src2Val.First() && src1Val.Pred == src2Val.Pred {
		finalPred = src1Val.Pred
		resultVal = 1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
		fmt.Println(">>>>>>>>>>>>>>> ICMP_EQ: ", src1Val.First(), src2Val.First(), "Yes")
	} else {
		finalPred = src1Val.Pred
		resultVal = 0
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
		fmt.Println(">>>>>>>>>>>>>>> ICMP_EQ: ", src1Val.First(), src2Val.First(), "No")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val.First(), src1Val.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val.First(), src2Val.Pred), "Result", fmt.Sprintf("%d(%t)", resultVal, finalPred))
	return results
}

func (i instEmulator) runSgtExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred

	//o
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	results := make(map[Operand]cgra.Data)
	if src1Signed > src2Signed {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runLTExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred
	results := make(map[Operand]cgra.Data)
	if src1Val < src2Val {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runGTExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred
	results := make(map[Operand]cgra.Data)
	if src1Val > src2Val {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runSGEExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	resultVal := 0

	results := make(map[Operand]cgra.Data)
	if src1Signed >= src2Signed {
		resultVal = 1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, resultPred)
		}
	} else {
		resultVal = 0
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, resultPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Result", fmt.Sprintf("%d(%t)", resultVal, resultPred))
	return results
}

func (i instEmulator) runPhi(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if src1Struct.Pred && !src2Struct.Pred {
		finalPred = src1Struct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src1Struct.First(), finalPred)
		}
	} else if !src1Struct.Pred && src2Struct.Pred {
		finalPred = src2Struct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src2Struct.First(), finalPred)
		}
	} else if !src1Struct.Pred && !src2Struct.Pred {
		finalPred = false
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	} else {
		panic("Phi operation: both sources have the same predicate")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runPhiConst(inst Operation, state *coreState) map[Operand]cgra.Data { // Possibly wrong
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	// Track PHI_CONST per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("PhiConst_%d", inst.ID)
	var result uint32
	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if state.States[stateKey] == nil || state.States[stateKey] == false {
		result = src1Val
		finalPred = src1Pred
		state.States[stateKey] = true
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
	} else {
		result = src2Val
		finalPred = src2Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runPhiStart(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src1Val := src1Struct.First()
	src1Pred := src1Struct.Pred

	// Track PHI_START per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("PhiStart_%d", inst.ID)
	var result uint32
	var finalPred bool
	results := make(map[Operand]cgra.Data)

	if state.States[stateKey] == nil || state.States[stateKey] == false { // first execution
		if !src1Pred {
			panic("Predicate of first time PHI_START must be true at (" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ") instruction " + strconv.Itoa(int(inst.ID)))
		}
		result = src1Val
		finalPred = src1Pred
		state.States[stateKey] = true
		fmt.Println("set state.States[", stateKey, "] to true")
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src0(FirstTime)", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Result", fmt.Sprintf("%d(%t)", result, finalPred))
	} else {
		src2Struct := i.readOperand(src2, state) // only in normal path will consume src2
		src2Val := src2Struct.First()
		src2Pred := src2Struct.Pred
		if src1Pred && src2Pred {
			panic("Only one of the predicates of PHI_START can be true at (" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ") instruction " + strconv.Itoa(int(inst.ID)))
		}
		if src1Pred {
			result = src1Val
			finalPred = src1Pred
		} else { // src2Pred is true or both are false(arbitrary)
			result = src2Val
			finalPred = src2Pred
		}
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src0", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src1", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Result", fmt.Sprintf("%d(%t)", result, finalPred))
	}

	// elect no next PC
	return results
}

func (i instEmulator) runGrantPred(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	pred := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	predStruct := i.readOperand(pred, state)
	srcVal := srcStruct.First()
	predVal := predStruct.First()
	srcPred := srcStruct.Pred
	predPred := predStruct.Pred

	resultPred := false

	if predVal == 0 {
		resultPred = false
	} else {
		resultPred = true
	}

	//fmt.Printf("GRANTPRED: srcVal = %d, predVal = %t at (%d, %d)\n", srcVal, predVal, state.TileX, state.TileY)
	finalPred := resultPred && predPred && srcPred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(srcVal, finalPred)
	}

	fmt.Println("<<<<<<<<<<<<<< GRANTPRED: ", srcVal, predVal, finalPred)

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "SrcOperand", fmt.Sprintf("%d(%t)", srcVal, srcStruct.Pred), "PredOperand", fmt.Sprintf("%d(%t)", predVal, predStruct.Pred), "Pred", finalPred, "Result", fmt.Sprintf("%d(%t)", srcVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runGrantOnce(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]

	srcStruct := i.readOperand(src, state)
	// Track GRANT_ONCE per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("GrantOnce_%d", inst.ID)
	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if state.States[stateKey] == nil || state.States[stateKey] == false {
		state.States[stateKey] = true
		finalPred = srcStruct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(srcStruct.First(), finalPred)
		}
	} else {
		finalPred = false
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(srcStruct.First(), finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runSel(inst Operation, state *coreState) map[Operand]cgra.Data {
	sel := inst.SrcOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[1]
	src2 := inst.SrcOperands.Operands[2]

	selStruct := i.readOperand(sel, state)
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	selVal := selStruct.First()
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	selPred := selStruct.Pred
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	resultPred := selPred && src1Pred && src2Pred

	if selVal != 0 && selVal != 1 {
		panic("Sel must be 0 or 1")
	}

	results := make(map[Operand]cgra.Data)
	if selVal == 0 { // if sel is 0, select src2
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src2Val, resultPred)
		}
	} else { // if sel is 1, select src1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src1Val, resultPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Sel", fmt.Sprintf("%d(%t)", selVal, selPred), "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Sel", fmt.Sprintf("%d(%t)", selVal, selPred))
	return results
}
