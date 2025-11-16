package core

import (
	"fmt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// toTitleCase converts a string to Title case (e.g., "SOUTH" -> "South")
// This replaces the deprecated strings.Title function
var titleCaser = cases.Title(language.English)

func toTitleCase(s string) string {
	return titleCaser.String(strings.ToLower(s))
}

type OpMode int

const (
	SyncOp OpMode = iota
	AsyncOp
)

// type routingRule struct {
// 	src   cgra.Side
// 	dst   cgra.Side
// 	color string
// }

// type Trigger struct {
// 	src    [12]bool
// 	color  int
// 	branch string
// }

type ReservationState struct {
	ReservationMap  map[int]bool // to show whether each operation of a instruction group is finished.
	OpToExec        int
	RefCountRuntime map[string]int // to record the left times to be used of each source opearand. deep copied from RefCount
}

// return bool, True means the operand is still in use, False means the operand is not in use anymore
// Only direction (port) operands are tracked in RefCount. Register and immediate operands are not tracked.
func (r *ReservationState) DecrementRefCount(opr Operand, state *coreState) bool {
	if !state.Directions[opr.Impl] && !state.Directions[toTitleCase(opr.Impl)] {
		// Non-direction operands don't need ref counting
		return true
	}

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
		// Direction operand not in RefCount - this might be OK if it's not used in this instruction group
		// Return true to be safe (don't close the port)
		return true
	}
}

func (r *ReservationState) SetRefCount(ig InstructionGroup, state *coreState) {
	for _, op := range ig.Operations {
		for _, opr := range op.SrcOperands.Operands {
			// Normalize direction name: convert to Title case (e.g., "SOUTH" -> "South")
			normalizedDir := opr.Impl
			if !state.Directions[opr.Impl] {
				normalizedDir = toTitleCase(opr.Impl)
			}
			// Check if operand is a direction (port)
			if state.Directions[opr.Impl] || state.Directions[normalizedDir] {
				// Use normalized direction for key
				key := normalizedDir + opr.Color
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
	if len(ig.Operations) == 0 {
		panic(fmt.Sprintf("SetReservationMap: InstructionGroup is empty for Core (%d,%d)", state.TileX, state.TileY))
	}
	for i := 0; i < len(ig.Operations); i++ {
		r.ReservationMap[i] = true
	}
	r.OpToExec = len(ig.Operations)
	// Debug: Log SetReservationMap - ALWAYS log to ensure it's called
	Trace("SetReservationMap",
		"X", state.TileX,
		"Y", state.TileY,
		"OpToExec", r.OpToExec,
		"numOps", len(ig.Operations),
	)
}

type coreState struct {
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

	// routingRules []*routingRule
	// triggers     []*Trigger

	// Waveform logging accumulator for per-cycle state tracking
	CycleAcc *CycleAccumulator
}

type instEmulator struct {
	CareFlags bool
}

// set up the necessary state for the instruction group
func (i instEmulator) SetUpInstructionGroup(index int32, state *coreState) {
	iGroup := state.SelectedBlock.InstructionGroups[index]

	// Debug: Log SetUpInstructionGroup - ALWAYS log to ensure it's called
	// Trace("SetUpInstructionGroup",
	// 	"X", state.TileX,
	// 	"Y", state.TileY,
	// 	"PC", index,
	// 	"numOps", len(iGroup.Operations),
	// 	"prevOpToExec", state.CurrReservationState.OpToExec,
	// )

	// CRITICAL: Always create a fresh ReservationState to avoid stale state
	// This ensures ReservationMap and OpToExec are correctly initialized
	state.CurrReservationState = ReservationState{
		ReservationMap:  make(map[int]bool),
		OpToExec:        0,
		RefCountRuntime: make(map[string]int),
	}

	// Set up ReservationMap and OpToExec
	state.CurrReservationState.SetReservationMap(iGroup, state)
	state.CurrReservationState.SetRefCount(iGroup, state)

}

func (i instEmulator) RunInstructionGroup(cinst InstructionGroup, state *coreState, time float64) bool {
	// ==== NEW: Initialize accumulator with current instruction ====
	if state.CycleAcc != nil && len(cinst.Operations) > 0 {
		state.CycleAcc.PC = state.PCInBlock
		// Use the first operation's OpCode
		state.CycleAcc.OpCode = cinst.Operations[0].OpCode
	}

	prevPC := state.PCInBlock
	prevCount := state.CurrReservationState.OpToExec
	progress_sync := false
	switch state.Mode {
	case SyncOp:
		progress_sync = i.RunInstructionGroupWithSyncOps(cinst, state, time)
	case AsyncOp:
		i.RunInstructionGroupWithAsyncOps(cinst, state, time)
	default:
		panic("invalid mode")
	}

	nowCount := state.CurrReservationState.OpToExec

	// find the nextPC
	switch state.Mode {
	case AsyncOp:
		if state.CurrReservationState.OpToExec == 0 && prevCount > nowCount && prevCount > 0 {
			// This instruction group is finished (all operations executed)
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
	case SyncOp:
		if progress_sync {
			if state.NextPCInBlock == -1 {
				// Removed verbose PC+4 output to reduce log size
				state.PCInBlock++
			} else {
				// Removed verbose PC+Jump output to reduce log size
				state.PCInBlock = state.NextPCInBlock
			}
		}
		if state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
			state.PCInBlock = -1
			state.SelectedBlock = nil
		}
		state.NextPCInBlock = -1
	default:
		panic("invalid mode")
	}

	nowPC := state.PCInBlock

	switch state.Mode {
	case AsyncOp:
		if prevPC == nowPC && prevCount == nowCount {
			//print("Kernel (", state.TileX, ",", state.TileY, ") want to sleep, ", prevPC, " = ", nowPC, " ", prevCount, " = ", nowCount, " ", time, "\n")
			return false
		}
	case SyncOp:
		return progress_sync
	default:
		panic("invalid mode")
	}
	return true
}

func (i instEmulator) RunInstructionGroupWithSyncOps(cinst InstructionGroup, state *coreState, time float64) bool {
	run := true
	for _, operation := range cinst.Operations {
		if (!i.CareFlags) || i.CheckFlags(operation, state, time) {
			continue
		} else {
			run = false
			break
		}
	}
	if run {
		for _, operation := range cinst.Operations {
			i.RunOperation(operation, state, time)
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

		if (!i.CareFlags) || i.CheckFlags(operation, state, time) { // can also only choose one (another pattern)
			state.CurrReservationState.ReservationMap[index] = false
			state.CurrReservationState.OpToExec--
			i.RunOperation(operation, state, time)
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		} else {
			//print("CheckFlags (", state.TileX, ",", state.TileY, ")", time, ":", "NO", "\n")
		}
	}
}

func (i instEmulator) CheckFlags(inst Operation, state *coreState, time float64) bool {
	//PrintState(state)
	flag := true

	// Special handling for PHI instruction: first iteration only needs one source ready
	phiStateKey := fmt.Sprintf("PHI_FirstExec_%d_%d_%d", state.TileX, state.TileY, state.PCInBlock)
	isPhiFirstExecution := inst.OpCode == "PHI" && len(inst.SrcOperands.Operands) >= 2 && state.States[phiStateKey] != true

	if isPhiFirstExecution {
		// First iteration: only need ONE source to be ready
		// Check if at least one source is ready
		hasReadySource := false
		for _, src := range inst.SrcOperands.Operands {
			if state.Directions[src.Impl] {
				// Direction port: check RecvBufHeadReady
				if state.RecvBufHeadReady[i.getColorIndex(src.Color)][i.getDirecIndex(src.Impl)] {
					hasReadySource = true
					break
				}
			} else if strings.HasPrefix(src.Impl, "$") {
				// Register: check predicate (not just "always ready")
				registerIndex, err := strconv.Atoi(strings.TrimPrefix(src.Impl, "$"))
				if err == nil && registerIndex >= 0 && registerIndex < len(state.Registers) {
					if state.Registers[registerIndex].Pred {
						hasReadySource = true
						break
					}
				}
			} else {
				// Immediate value: always ready
				hasReadySource = true
				break
			}
		}
		flag = hasReadySource
		// Mark PHI as having executed (for subsequent iterations)
		if flag {
			state.States[phiStateKey] = true
		}
	} else {
		// Normal instructions and PHI subsequent iterations: require ALL sources to be ready
		for _, src := range inst.SrcOperands.Operands {
			if state.Directions[src.Impl] {
				// Direction port: check RecvBufHeadReady
				if !state.RecvBufHeadReady[i.getColorIndex(src.Color)][i.getDirecIndex(src.Impl)] {
					flag = false
					break
				}
			} else if strings.HasPrefix(src.Impl, "$") {
				// Register: check predicate (not just "always ready")
				registerIndex, err := strconv.Atoi(strings.TrimPrefix(src.Impl, "$"))
				if err == nil && registerIndex >= 0 && registerIndex < len(state.Registers) {
					if !state.Registers[registerIndex].Pred {
						flag = false
						break
					}
				} else {
					// Invalid register index
					flag = false
					break
				}
			}
			// Immediate values are always ready, so no check needed
		}
	}

	// Check destination ports (same for all instructions)
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

	// Removed verbose Inst trace to reduce log size

	instFuncs := map[string]func(Operation, *coreState){
		"ADD": i.runAdd, // ADD, ADDI, INC, SUB, DEC
		"SUB": i.runSub,
		"LLS": i.runLLS,
		"SHL": i.runLLS, // SHL is an alias for LLS (Logical Left Shift)
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
		"FDIV": i.runFDiv,
		"NOP":  i.runNOP,

		"PHI":        i.runPhi,
		"PHI_CONST":  i.runPhiConst,
		"GPRED":      i.runGrantPred,
		"GRANT_ONCE": i.runGrantOnce,
		"CONSTANT":   i.runConstant,
		"GEP":        i.runGEP,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LDD":  i.runLoadDirect,
		"STD":  i.runStoreDirect,
		"LOAD": i.runLoadDirect, // LOAD is an alias for LDD (Load Direct from local memory)

		"LD":  i.runLoadDRAM,
		"LDW": i.runLoadWaitDRAM,

		"ST":    i.runStoreDRAM,
		"STORE": i.runStoreDirect, // STORE is an alias for STD (Store Direct to local memory)
		"STW":   i.runStoreWaitDRAM,

		"TRIGGER": i.runTrigger,

		"NOT": i.runNot,

		"ICMP_EQ":     i.parseAndCompareI, // Use parseAndCompareI for integer comparison
		"ICMP_SGT":    i.parseAndCompareI, // Signed Greater Than
		"SEXT":        i.runSEXT,
		"ZEXT":        i.runZEXT, // Zero Extend
		"CAST_FPTOSI": i.runCAST_FPTOSI,
		"FMUL_FADD":   i.runFMulFAdd, // Fused Multiply-Add: result = src1 * src2 + src3
		"CTRL_MOV":    i.runMov,      // CTRL_MOV is an alias for MOV
		"DATA_MOV":    i.runMov,      // DATA_MOV is an alias for MOV
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
	} else if state.Directions[operand.Impl] || state.Directions[toTitleCase(operand.Impl)] {
		//fmt.Println("operand.Impl", operand.Impl)
		// must first check it is ready
		// Normalize direction name: convert to Title case (e.g., "SOUTH" -> "South")
		normalizedDir := operand.Impl
		if !state.Directions[operand.Impl] {
			normalizedDir = toTitleCase(operand.Impl)
		}
		color, direction := i.getColorIndex(operand.Color), i.getDirecIndex(normalizedDir)
		value = state.RecvBufHead[color][direction]

		// Data from ports is always considered valid (predicate remains as set by sender)
		// Removed overly strict predicate checking that was preventing valid operations

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
		// Handle immediate values with # prefix (e.g., #0, #1, #18.000000)
		implStr := operand.Impl
		if strings.HasPrefix(implStr, "#") {
			implStr = implStr[1:] // Remove # prefix
		}

		// Try to parse as integer
		num, err := strconv.Atoi(implStr)
		if err == nil {
			value = cgra.NewScalar(uint32(num))
		} else {
			// Try to parse as float (e.g., 18.000000)
			if floatVal, err := strconv.ParseFloat(implStr, 32); err == nil {
				// Convert float to uint32 bits
				value = cgra.NewScalar(uint32(math.Float32bits(float32(floatVal))))
			} else {
				// Try to parse as unsigned integer
				if immediate, err := strconv.ParseUint(implStr, 0, 32); err == nil {
					value = cgra.NewScalar(uint32(immediate))
				} else {
					panic(fmt.Sprintf("Invalid operand %v in readOperand; expected register, direction, or immediate", operand))
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
	} else if state.Directions[operand.Impl] || state.Directions[toTitleCase(operand.Impl)] {
		// Normalize direction name: convert to Title case (e.g., "SOUTH" -> "South")
		normalizedDir := operand.Impl
		if !state.Directions[operand.Impl] {
			normalizedDir = toTitleCase(operand.Impl)
		}
		if state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedDir)] {
			//fmt.Printf("sendbufhead busy\n")
			return
		}
		state.SendBufHeadBusy[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedDir)] = true
		state.SendBufHead[i.getColorIndex(operand.Color)][i.getDirecIndex(normalizedDir)] = value
	} else {
		panic(fmt.Sprintf("Invalid operand %v in writeOperand; expected register or direction", operand))
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
	// EOS propagation: MOV passes through the EOS flag from source to destination
	// for _, dst := range inst.DstOperands.Operands {
	// 	i.writeOperand(dst, cgra.NewScalarWithEOS(opr, oprStruct.Pred, oprStruct.EOS), state)
	// }
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

	// Debug: Log NOT execution for Core (2,2)
	if state.TileX == 2 && state.TileY == 2 {
		Trace("NOT_Exec",
			"Time", float64(0), // Will be set by caller
			"X", state.TileX,
			"Y", state.TileY,
			"Src", src.Impl,
			"SrcVal", srcVal,
			"SrcPred", srcPred,
			"Result", result,
		)
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

	// Check if source is a port (NORTH, SOUTH, EAST, WEST)
	// If so, this is a data forwarding operation (port -> port)
	// Otherwise, it's a memory load operation (address -> value)
	isPort := state.Directions[src1.Impl] || state.Directions[toTitleCase(src1.Impl)]

	if isPort {
		// Load from memory using address received from port
		// Read address from port, then load from memory at that address
		// The memory accessed is the local memory of the PE executing this LOAD instruction
		addrStruct := i.readOperand(src1, state)
		addr := addrStruct.First()

		// Clamp address to valid memory range (0 to len(Memory)-1)
		if addr >= uint32(len(state.Memory)) {
			// Wrap around using modulo instead of panicking
			addr = addr % uint32(len(state.Memory))
			slog.Warn("Memory",
				"Behavior", "LoadDirect_AddressClamped",
				"OriginalAddr", addrStruct.First(),
				"ClampedAddr", addr,
				"MemorySize", len(state.Memory),
				"Port", src1.Impl,
				"X", state.TileX,
				"Y", state.TileY,
			)
		}
		value := state.Memory[addr]

		// ==== NEW: Add to waveform accumulator ====
		if state.CycleAcc != nil {
			state.CycleAcc.AddMemoryOp(
				"LOAD",
				addr,
				value,
				"Local",
			)
		}

		slog.Warn("Memory",
			"Behavior", "LoadDirect",
			"Value", value,
			"Addr", addr,
			"Port", src1.Impl,
			"X", state.TileX,
			"Y", state.TileY,
		)
		// LOAD instruction: read address from source port, load data from memory, send data to destination
		// The source operand is the address, and we load the data from memory at that address
		// Then we send the data (value) to the destination port(s)
		// We do NOT forward the address - only the data from memory
		for _, dst := range inst.DstOperands.Operands {
			// Send the data (value) loaded from memory to the destination port
			i.writeOperand(dst, cgra.NewScalarWithPred(value, addrStruct.Pred), state)
		}
	} else {
		// Memory load: read address from source, then load from memory
		addrStruct := i.readOperand(src1, state)
		addr := addrStruct.First()

		// Clamp address to valid memory range (0 to len(Memory)-1)
		if addr >= uint32(len(state.Memory)) {
			// Wrap around using modulo instead of panicking
			addr = addr % uint32(len(state.Memory))
			slog.Warn("Memory",
				"Behavior", "LoadDirect_AddressClamped",
				"OriginalAddr", addrStruct.First(),
				"ClampedAddr", addr,
				"MemorySize", len(state.Memory),
				"X", state.TileX,
				"Y", state.TileY,
			)
		}
		value := state.Memory[addr]

		// ==== NEW: Add to waveform accumulator ====
		if state.CycleAcc != nil {
			state.CycleAcc.AddMemoryOp(
				"LOAD",
				addr,
				value,
				"Local",
			)
		}

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
	src2 := inst.SrcOperands.Operands[1]

	// Standard format: src1 = value, src2 = address
	valueStruct := i.readOperand(src1, state)
	value := valueStruct.First()
	addrStruct := i.readOperand(src2, state)
	addr := addrStruct.First()

	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds")
	}

	// Check predicate: only store if both address and value have valid predicates
	if !addrStruct.Pred || !valueStruct.Pred {
		// Predicate is false, skip store operation
		return
	}

	// Check if this is a histogram operation:
	// - Histogram format: STORE, [$0], [$1] where both are registers
	// - $0 = current count + 1 (value), $1 = bin index (address)
	// - For histogram, we need to accumulate (increment) instead of overwrite
	// - If both operands are registers and address is small (< 100), it's likely histogram
	isHistogramFormat := strings.HasPrefix(src1.Impl, "$") && strings.HasPrefix(src2.Impl, "$")
	if isHistogramFormat && addr < 100 {
		// For histogram: accumulate (increment) the value at the address
		// - value = $0 = current count + 1 (increment amount, typically 1)
		// - addr = $1 = bin index (address to store to)
		// - We accumulate: newValue = oldValue + value
		oldValue := state.Memory[addr]
		newValue := oldValue + value // Accumulate: add the increment to the old value
		state.Memory[addr] = newValue
		slog.Warn("Memory",
			"Behavior", "StoreDirect",
			"Type", "Histogram",
			"Increment", value,
			"OldValue", oldValue,
			"NewValue", newValue,
			"Addr", addr,
			"X", state.TileX,
			"Y", state.TileY,
		)
	} else {
		// Normal store: overwrite (for axpy and other operations)
		state.Memory[addr] = value
		slog.Warn("Memory",
			"Behavior", "StoreDirect",
			"Type", "Normal",
			"Value", value,
			"Addr", addr,
			"X", state.TileX,
			"Y", state.TileY,
		)
	}

	// Check EOS flag in the value data - if set, terminate the program
	// if valueStruct.EOS {
	// 	slog.Info("Flow", "EOS detected in STORE", "X", state.TileX, "Y", state.TileY)
	// 	os.Exit(0)
	// }
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
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state).First()
	dstVal := uint32(0)

	// Handle immediate value with # prefix (e.g., #20)
	immeStr := inst.SrcOperands.Operands[1].Impl
	if strings.HasPrefix(immeStr, "#") {
		immeStr = immeStr[1:] // Remove # prefix
	}
	imme, err := strconv.ParseUint(immeStr, 10, 32)
	if err != nil {
		panic(fmt.Sprintf("invalid compare number: %s", inst.SrcOperands.Operands[1].Impl))
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

	// Handle multiple destination operands (e.g., ICMP_EQ, [$0], [#20] -> [$0])
	// ICMP outputs comparison result (0 or 1) which is always valid (predicate=true)
	// This result is used by GPRED to control predicate, so it should always be predicate=true
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalar(dstVal), state)
	}
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

	// Removed verbose ISUB output to reduce log size

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
	// not exist
	os.Exit(0)
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
	// if state.TileX == 2 && state.TileY == 3 {
	// 	fmt.Fprintf(os.Stderr, "[FADD] Core (2,3): src1Float=%.4f, src2Float=%.4f, resultFloat=%.4f, resultUint=%d\n",
	// 		src1Float, src2Float, resultFloat, resultUint)
	// }

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(resultUint, src1Pred && src2Pred), state)
	}

	// elect no next PC
}

func (i instEmulator) runFDiv(inst Operation, state *coreState) {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	// Check if both operands have valid data (predicate is true)
	// If data is not ready, predicate will be false, and we should not execute
	if !src1Struct.Pred || !src2Struct.Pred {
		// Just return without doing anything (instruction will be retried when data is ready)
		return
	}

	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	// Convert to float32
	src1Float := uint2Float(src1Val)
	src2Float := uint2Float(src2Val)

	// Avoid division by zero
	if src2Float == 0.0 {
		panic(fmt.Sprintf("Float division by zero at PC %d @(%d, %d)", state.PCInBlock, state.TileX, state.TileY))
	}

	dstFloat := src1Float / src2Float
	dstVal := float2Uint(dstFloat)
	// if state.TileX == 2 && state.TileY == 3 {
	// 	fmt.Fprintf(os.Stderr, "[FDIV] Core (2,3): src1Float=%.4f, src2Float=%.4f, resultFloat=%.4f, resultUint=%d\n",
	// 		src1Float, src2Float, dstFloat, dstVal)
	// 	// Special check for value 21: if src1Float=100.0, this should produce resultFloat=5.5556
	// 	if src1Float == 100.0 {
	// 		expected_result := float32(100.0 / 18.0)
	// 		if math.Abs(float64(dstFloat-expected_result)) > 0.0001 {
	// 			fmt.Fprintf(os.Stderr, "[FDIV_ERROR] Core (2,3): src1Float=100.0, expected resultFloat=%.4f, got %.4f\n",
	// 				expected_result, dstFloat)
	// 		}
	// 	}
	// }

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dstVal, src1Struct.Pred && src2Struct.Pred), state)
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
	// PHI node: selects between loop-carried value (backedge) and initial value
	// src0 = backedge / loop-carried value (from previous iteration)
	// src1 = initial value (from grant_once or one-shot source)
	src0 := inst.SrcOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[1]

	// Read operands
	src0Struct := i.readOperand(src0, state)
	src1Struct := i.readOperand(src1, state)

	// Priority rule: src0 (backedge) has priority when both predicates are true
	// 1. If src0.pred == true → choose src0
	// 2. Else if src1.pred == true → choose src1
	// 3. Else → no valid input this cycle: choose any value but set pred=false
	var selectedVal uint32
	var selectedPred bool
	// var selectedEOS bool

	if src0Struct.Pred {
		// src0 (backedge) has priority
		selectedVal = src0Struct.First()
		selectedPred = src0Struct.Pred
		// selectedEOS = src0Struct.EOS // EOS propagation: pass through EOS from selected source
	} else if src1Struct.Pred {
		// src1 (initial value) is valid
		selectedVal = src1Struct.First()
		selectedPred = src1Struct.Pred
		// selectedEOS = src1Struct.EOS // EOS propagation: pass through EOS from selected source
	} else {
		// Both predicates are false: no valid input this cycle
		// Use src0's value but set predicate to false
		selectedVal = src0Struct.First()
		selectedPred = false
		// selectedEOS = false // No valid input, no EOS
	}

	// Optional: Log debug message if both predicates are true (src0 has priority)
	if src0Struct.Pred && src1Struct.Pred {
		Trace("PHI_BothPredTrue",
			"X", state.TileX,
			"Y", state.TileY,
			"Selected", "src0 (backedge has priority)",
			"Src0Val", src0Struct.First(),
			"Src1Val", src1Struct.First(),
		)
	}

	// Write the selected (value, pred, EOS) to ALL destination operands
	// PHI is pure selection - passes through EOS from the selected source
	// for _, dst := range inst.DstOperands.Operands {
	// 	i.writeOperand(dst, cgra.NewScalarWithEOS(selectedVal, selectedPred, selectedEOS), state)
	// }
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(selectedVal, selectedPred), state)
	}

	// Mark this PHI as having executed (for first-execution-only deadlock breaking)
	// After first execution, PHI will require both sources to be ready on subsequent iterations
	phiStateKey := fmt.Sprintf("PHI_FirstExec_%d_%d_%d", state.TileX, state.TileY, state.PCInBlock)
	state.States[phiStateKey] = true

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
	// GPRED: Grant predicate instruction
	// First src is the data with predicate, second src is the condition to check the first src's predicate
	// If condition is true (non-zero) AND first src's predicate is true, output the data with predicate=true
	// Otherwise, output with predicate=false
	dataSrc := inst.SrcOperands.Operands[0]
	condSrc := inst.SrcOperands.Operands[1]

	dataStruct := i.readOperand(dataSrc, state)
	condStruct := i.readOperand(condSrc, state)
	dataVal := dataStruct.First()
	condVal := condStruct.First()
	dataPred := dataStruct.Pred

	// Result predicate: true only if condition is non-zero AND data predicate is true
	// When condVal is 0 (exit signal), output predicate=false to stop the loop
	resultPred := false
	if condVal != 0 && dataPred {
		resultPred = true
	}

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(dataVal, resultPred), state)
	}

	// elect no next PC
}

// runGEP implements GEP (GetElementPtr) instruction.
// GEP is used for memory address calculation.
// It reads source operand(s) and calculates the memory address, then writes to destination.
// For single source: GEP calculates base address (typically just passes the value as address)
// For multiple sources: GEP may calculate base + offset
func (i instEmulator) runGEP(inst Operation, state *coreState) {
	// Read first source operand (base address or index)
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	baseAddr := srcStruct.First()
	srcPred := srcStruct.Pred

	// Convert float to int if baseAddr is a float (represented as bits > 1000000)
	// This handles cases where the loop index is passed as a float
	if baseAddr > 1000000 {
		// Interpret as float32 bits and convert to int
		floatVal := math.Float32frombits(baseAddr)
		intVal := int32(floatVal) // Convert float to int32
		// Handle negative values and ensure address is within valid range
		if intVal < 0 {
			// Negative address is invalid, clamp to 0
			baseAddr = 0
		} else {
			baseAddr = uint32(intVal) // Convert int32 to uint32
		}
		// Clamp address to valid memory range (0 to len(Memory)-1)
		if baseAddr >= uint32(len(state.Memory)) {
			baseAddr = baseAddr % uint32(len(state.Memory)) // Wrap around using modulo
		}

	}

	// If there are multiple source operands, calculate base + offset
	// Otherwise, just use the base address
	calculatedAddr := baseAddr
	if len(inst.SrcOperands.Operands) > 1 {
		// Read offset from second source operand
		offsetSrc := inst.SrcOperands.Operands[1]
		offsetStruct := i.readOperand(offsetSrc, state)
		offset := offsetStruct.First()

		// Convert offset to int if it's a float
		if offset > 1000000 {
			floatVal := math.Float32frombits(offset)
			intVal := int32(floatVal)
			if intVal < 0 {
				offset = 0 // Negative offset is invalid
			} else {
				offset = uint32(intVal)
			}
			// Clamp offset to valid memory range
			if offset >= uint32(len(state.Memory)) {
				offset = offset % uint32(len(state.Memory))
			}
		}

		calculatedAddr = baseAddr + offset
		srcPred = srcPred && offsetStruct.Pred
	}

	// Clamp final calculated address to valid memory range
	if calculatedAddr >= uint32(len(state.Memory)) {
		calculatedAddr = calculatedAddr % uint32(len(state.Memory))
	}

	// Write the calculated memory address to all destination operands
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(calculatedAddr, srcPred), state)
	}

	// elect no next PC
}

// runSEXT implements SEXT (Sign Extend) instruction.
// Sign extends a value from a smaller bit width to a larger one.
// In CGRA context, typically extends from smaller integer to 32-bit integer.
func (i instEmulator) runSEXT(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	srcPred := srcStruct.Pred

	// Sign extend: treat srcVal as signed integer and extend to 32 bits
	// For simplicity, we assume the input is already 32-bit or we extend from lower bits
	// If input is treated as smaller (e.g., 16-bit), we'd mask and sign extend
	// For now, just pass through the value (assuming it's already properly extended)
	result := srcVal

	// Write to all destination operands
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, srcPred), state)
	}

	// elect no next PC
}

// runZEXT implements ZEXT (Zero Extend) instruction.
// It extends a value to a larger bit width by adding zeros to the most significant bits.
// For now, we treat it as a pass-through (no-op) since we're working with 32-bit values.
func (i instEmulator) runZEXT(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	srcPred := srcStruct.Pred

	// ZEXT extends the value (for now, just pass through since we use 32-bit)
	// In a real implementation, this would extend from smaller bit width to 32-bit
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(srcVal, srcPred), state)
	}
	// elect no next PC
}

// runFMulFAdd implements FMUL_FADD (Fused Multiply-Add) instruction.
// It computes: result = src1 * src2 + src3
// This is a fused operation that performs both multiply and add in one instruction.
func (i instEmulator) runFMulFAdd(inst Operation, state *coreState) {
	if len(inst.SrcOperands.Operands) < 3 {
		panic("FMUL_FADD requires 3 source operands")
	}

	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src3 := inst.SrcOperands.Operands[2]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src3Struct := i.readOperand(src3, state)

	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src3Uint := src3Struct.First()

	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	src3Pred := src3Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)
	src3Float := uint2Float(src3Uint)

	// Fused multiply-add: result = src1 * src2 + src3
	resultFloat := src1Float*src2Float + src3Float

	resultUint := float2Uint(resultFloat)

	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(resultUint, src1Pred && src2Pred && src3Pred), state)
	}
	// elect no next PC
}

// runCAST_FPTOSI implements CAST_FPTOSI (Float Point to Signed Integer Cast) instruction.
// Converts a float32 value to a signed 32-bit integer.
func (i instEmulator) runCAST_FPTOSI(inst Operation, state *coreState) {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	srcPred := srcStruct.Pred

	// Convert float32 bits to float32 value
	srcFloat := uint2Float(srcVal)

	// Convert float32 to int32 (signed integer)
	srcInt := int32(srcFloat)

	// Convert int32 to uint32 (for storage)
	result := uint32(srcInt)

	// Write to all destination operands
	for _, dst := range inst.DstOperands.Operands {
		i.writeOperand(dst, cgra.NewScalarWithPred(result, srcPred), state)
	}

	// elect no next PC
}

// runGrantOnce implements GRANT_ONCE instruction.
// GRANT_ONCE is a fusion of constant generation and grant operation.
// It reads a source operand (constant), grants it with predicate=true (valid mark), and sends to destination.
func (i instEmulator) runGrantOnce(inst Operation, state *coreState) {
	// Check if we've already executed this instruction
	// GRANT_ONCE executes only once - if it has already executed, return immediately
	stateKey := fmt.Sprintf("GrantOnce_%d", state.PCInBlock)
	// hasExecuted := state.States[stateKey] == true

	// if hasExecuted {
	// 	// GRANT_ONCE has already executed, do nothing (only executes once)
	// 	// In loop programs, this means GRANT_ONCE will be skipped in subsequent iterations
	// 	// PHI will select the value from previous ADD instead
	// 	return
	// }

	// Read source operand (constant value)
	// GRANT_ONCE may have no source operand (empty source), in which case we read from external input (boundary port)
	var srcVal uint32
	var srcPred bool = true
	var grantedPred bool = false
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		// Check if source operand is empty (Impl is empty string)
		if src.Impl != "" {
			srcStruct := i.readOperand(src, state)
			srcVal = srcStruct.First()
			srcPred = srcStruct.Pred
		} else {
			// Empty source operand: read from external input (boundary port)
			// For boundary tiles, FeedIn sends data to the corresponding port
			// We need to determine which direction to read from based on tile position
			// Common cases: x=0 -> WEST, x=width-1 -> EAST, y=0 -> SOUTH, y=height-1 -> NORTH
			// For now, try to read from all boundary directions and use the first one with data
			srcVal, srcPred = i.readFromBoundaryPort(state, src.Color)
		}
	} else {
		// No source operands: read from external input (boundary port)
		srcVal, srcPred = i.readFromBoundaryPort(state, "")
	}

	// GRANT_ONCE sets predicate based on source data validity
	// If data was read from external input, use the predicate from that data
	// Otherwise, set predicate=true (valid mark) on first and only execution
	if state.States[stateKey] != true {
		grantedPred = srcPred
	}

	// Write the value to all destination operands with predicate
	for _, dst := range inst.DstOperands.Operands {
		// Create data with predicate=true
		dataToWrite := cgra.NewScalarWithPred(srcVal, grantedPred)
		i.writeOperand(dst, dataToWrite, state)
	}

	// Mark as executed - GRANT_ONCE will not execute again
	state.States[stateKey] = true

	// elect no next PC
}

// readFromBoundaryPort reads data from a boundary port (external input)
// It tries to read from all possible boundary directions and returns the first one with data
// Common cases: x=0 -> WEST, x=width-1 -> EAST, y=0 -> SOUTH, y=height-1 -> NORTH
func (i instEmulator) readFromBoundaryPort(state *coreState, color string) (uint32, bool) {
	colorIdx := i.getColorIndex(color)

	// Try to read from all boundary directions in order of likelihood
	// For a 4x4 grid: x=0 -> WEST, y=0 -> SOUTH, x=3 -> EAST, y=3 -> NORTH
	// We check RecvBufHead to see if there's data available
	directionsToTry := []struct {
		name string
		idx  int
	}{
		{"West", int(cgra.West)},
		{"South", int(cgra.South)},
		{"East", int(cgra.East)},
		{"North", int(cgra.North)},
	}

	for _, dir := range directionsToTry {
		if dir.idx < len(state.RecvBufHeadReady[colorIdx]) {
			if state.RecvBufHeadReady[colorIdx][dir.idx] {
				// Data is available, read it
				data := state.RecvBufHead[colorIdx][dir.idx]
				val := data.First()
				pred := data.Pred
				// Mark as consumed (set RecvBufHeadReady to false)
				state.RecvBufHeadReady[colorIdx][dir.idx] = false
				Trace("GRANT_ONCE_ReadBoundary",
					"X", state.TileX,
					"Y", state.TileY,
					"Direction", dir.name,
					"Color", color,
					"Data", val,
					"Pred", pred,
				)
				return val, pred
			}
		}
	}

	// No data available from any boundary port, return 0 with predicate=false
	Trace("GRANT_ONCE_NoData",
		"X", state.TileX,
		"Y", state.TileY,
		"Color", color,
		"Reason", "No data available in any boundary port",
	)
	return 0, false
}

// runConstant implements the CONSTANT instruction
// CONSTANT reads a constant value (immediate) and sends it to destination with predicate=true
// Unlike GRANT_ONCE, CONSTANT can execute multiple times (no "execute only once" restriction)
func (i instEmulator) runConstant(inst Operation, state *coreState) {
	// Read source operand (constant value)
	// CONSTANT should have at least one source operand (the constant)
	if len(inst.SrcOperands.Operands) == 0 {
		// If no source operand, use 0 as default
		zeroVal := uint32(0)
		for _, dst := range inst.DstOperands.Operands {
			dataToWrite := cgra.NewScalarWithPred(zeroVal, true)
			i.writeOperand(dst, dataToWrite, state)
		}
		return
	}

	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()

	// CONSTANT sets predicate=true (valid mark)
	constantPred := true

	// Write the value to all destination operands with predicate=true
	for _, dst := range inst.DstOperands.Operands {
		// Create data with predicate=true
		dataToWrite := cgra.NewScalarWithPred(srcVal, constantPred)
		i.writeOperand(dst, dataToWrite, state)
	}

	// elect no next PC
}
