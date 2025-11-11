package core

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type portPair struct {
	local  sim.Port
	remote sim.RemotePort
}

type Core struct {
	*sim.TickingComponent

	ports map[cgra.Side]*portPair

	state coreState
	emu   instEmulator
}

func (c *Core) GetTileX() int {
	return int(c.state.TileX)
}

func (c *Core) GetTileY() int {
	return int(c.state.TileY)
}

// get memory
func (c *Core) GetMemory(x int, y int, addr uint32) uint32 {
	if x == int(c.state.TileX) && y == int(c.state.TileY) {
		return c.state.Memory[addr]
	} else {
		panic("Invalid Tile")
	}
}

// write memory
func (c *Core) WriteMemory(x int, y int, data uint32, baseAddr uint32) {
	//fmt.Printf("Core [%d][%d] receive WriteMemory(x=%d, y=%d)\n", c.state.TileX, c.state.TileY, x, y)
	if x == int(c.state.TileX) && y == int(c.state.TileY) {
		c.state.Memory[baseAddr] = data

		// ==== NEW: Add to waveform accumulator ====
		if c.state.CycleAcc != nil {
			c.state.CycleAcc.AddMemoryOp(
				"STORE",
				baseAddr,
				data,
				"Local",
			)
		}

		//fmt.Printf("Core [%d][%d] write memory[%d] = %d\n", c.state.TileX, c.state.TileY, baseAddr, c.state.Memory[baseAddr])
		Trace("Memory",
			"Behavior", "WriteMemory",
			"Time", float64(c.Engine.CurrentTime()*1e9),
			"Data", data,
			"X", x,
			"Y", y,
			"Addr", baseAddr,
		)
	} else {
		panic(fmt.Sprintf("Invalid Tile: Expect (%d, %d)，but get (%d, %d)", c.state.TileX, c.state.TileY, x, y))
	}
}

func (c *Core) SetRemotePort(side cgra.Side, remote sim.RemotePort) {
	c.ports[side].remote = remote
}

// MapProgram sets the program that the core needs to run.
func (c *Core) MapProgram(program interface{}, x int, y int) {
	if prog, ok := program.(Program); ok {
		c.state.Code = prog
		// Removed verbose MapProgram trace to reduce log size
	} else {
		panic("MapProgram expects core.Program type")
	}
	c.state.PCInBlock = -1
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
	// Removed verbose MapProgramAfter trace to reduce log size

	// CRITICAL: If this core has a program with instructions that don't need external input,
	// schedule the first tick event explicitly.
	// TickingComponent doesn't automatically schedule the first tick, so we need to do it manually
	// for instructions like GRANT_ONCE that can execute without external input.
	if len(c.state.Code.EntryBlocks) > 0 {
		// Check if the first instruction group contains instructions that don't need external input
		needsExplicitTick := false
		if len(c.state.Code.EntryBlocks[0].InstructionGroups) > 0 {
			firstInstGroup := c.state.Code.EntryBlocks[0].InstructionGroups[0]
			for _, op := range firstInstGroup.Operations {
				// Instructions that don't need external input (constants, grants, etc.)
				// These instructions can execute immediately without waiting for external data
				if op.OpCode == "GRANT_ONCE" || op.OpCode == "GRANT_ONCE_CONST" {
					needsExplicitTick = true
					break
				}
				// Check if all source operands are immediate values (constants)
				// If so, this instruction doesn't need external input
				allImmediate := true
				for _, src := range op.SrcOperands.Operands {
					// Check if operand is an immediate value (starts with # or is a number)
					if strings.HasPrefix(src.Impl, "#") {
						continue // Immediate value
					}
					// Check if it's a register (starts with $)
					if strings.HasPrefix(src.Impl, "$") {
						allImmediate = false
						break
					}
					// Check if it's a direction (needs external input)
					if c.state.Directions[src.Impl] || c.state.Directions[strings.Title(strings.ToLower(src.Impl))] {
						allImmediate = false
						break
					}
					// Try to parse as number (immediate value)
					if _, err := strconv.Atoi(src.Impl); err != nil {
						// Not a number, might need external input
						allImmediate = false
						break
					}
				}
				if allImmediate && len(op.SrcOperands.Operands) > 0 {
					needsExplicitTick = true
					break
				}
			}
		}

		if needsExplicitTick {
			c.TickNow()
			// Removed verbose MapProgramTickNow trace to reduce log size
		}
	}
}

// Tick runs the program for one cycle.
func (c *Core) Tick() (madeProgress bool) {
	// ==== NEW: Initialize accumulator for this cycle ====
	c.state.CycleAcc = NewCycleAccumulator()
	currentTime := float64(c.Engine.CurrentTime() * 1e9)

	// Removed verbose CoreTick trace to reduce log size

	// Original execution order:
	// 1. First send data to other cores (doSend)
	// 2. Then run program (runProgram)
	// 3. Finally receive data from other cores (doRecv)
	madeProgress = c.doSend() || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doRecv() || madeProgress

	// For cores with programs that haven't started yet, ensure they make progress
	// This is important for instructions like GRANT_ONCE that don't need input
	// If a core has a program but hasn't made progress and hasn't started, force it to start
	// BUT: Only do this ONCE - if the program has already executed (PCInBlock == -1 and hasExecuted),
	// don't force progress to avoid infinite loops
	if len(c.state.Code.EntryBlocks) > 0 && !madeProgress && c.state.PCInBlock == -1 {
		// Check if program has already been executed (for single-instruction programs like GRANT_ONCE)
		// This should match the logic in runProgram() - only true single-instruction programs
		hasExecuted := false
		if len(c.state.Code.EntryBlocks) > 0 && len(c.state.Code.EntryBlocks[0].InstructionGroups) == 1 {
			firstInstGroup := c.state.Code.EntryBlocks[0].InstructionGroups[0]
			if len(firstInstGroup.Operations) == 1 && firstInstGroup.Operations[0].OpCode == "GRANT_ONCE" {
				// Check if GRANT_ONCE has already executed
				stateKey := fmt.Sprintf("GrantOnce_0")
				if c.state.States[stateKey] == true {
					hasExecuted = true
				}
			}
		}

		// Only force progress if program hasn't executed yet
		if !hasExecuted {
			// Core has program but hasn't started yet, force it to start by returning true
			// This ensures the engine continues to tick this core
			madeProgress = true
		}
		// Removed verbose CoreForceProgress trace to reduce log size
	}

	// REMOVED: Code that was causing infinite loops - cores with finished programs (PCInBlock == -1)
	// were constantly returning madeProgress=true, causing the engine to never stop.
	// The logic above (lines 153-176) already handles the case where programs haven't started yet.

	// ==== NEW: Emit canonical PEState log at cycle end ====
	if EnableWaveformLog && c.state.CycleAcc != nil {
		LogPEState(currentTime, c.state.TileX, c.state.TileY, c.state.CycleAcc)
	}

	return madeProgress
}

func makeBytesFromUint32(data uint32) []byte {
	return []byte{byte(data >> 24), byte(data >> 16), byte(data >> 8), byte(data)}
}

func (c *Core) doSend() bool {
	madeProgress := false
	for i := 0; i < 8; i++ { // only 8 directions
		for color := 0; color < 4; color++ {

			if !c.state.SendBufHeadBusy[color][i] {
				continue
			}

			msg := cgra.MoveMsgBuilder{}.
				WithDst(c.ports[cgra.Side(i)].remote).
				WithSrc(c.ports[cgra.Side(i)].local.AsRemote()).
				WithData(c.state.SendBufHead[color][i]).
				WithSendTime(c.Engine.CurrentTime()).
				WithColor(color).
				Build()

			err := c.ports[cgra.Side(i)].local.Send(msg)
			if err != nil {
				// Backpressure: Port send failed - log for analysis
				direction := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
				Trace("Backpressure",
					"Type", "SendFailed",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Direction", direction,
					"Color", color,
					"Time", float64(c.Engine.CurrentTime()*1e9),
					"Error", fmt.Sprintf("%v", err),
					"Data", c.state.SendBufHead[color][i].First(),
					"Pred", c.state.SendBufHead[color][i].Pred,
				)
				continue
			}

			// DataFlow Send trace for link utilization analysis
			Trace("DataFlow",
				"Behavior", "Send",
				slog.Float64("Time", float64(c.Engine.CurrentTime()*1e9)),
				"Data", c.state.SendBufHead[color][i].First(),
				"Pred", c.state.SendBufHead[color][i].Pred,
				"Color", color,
				"From", c.ports[cgra.Side(i)].local.AsRemote(),
				"To", c.ports[cgra.Side(i)].remote,
				"X", c.state.TileX,
				"Y", c.state.TileY,
				"Direction", []string{"North", "East", "South", "West", "Router", "", "", ""}[i],
			)

			// ==== NEW: Add to waveform accumulator ====
			if c.state.CycleAcc != nil {
				direction := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
				c.state.CycleAcc.AddOutputPort(
					direction,
					true, // hasData
					c.state.SendBufHead[color][i].First(),
					c.state.SendBufHead[color][i].Pred,
					color,
					true, // sent
				)
			}

			c.state.SendBufHeadBusy[color][i] = false
		}
	}

	// handle the memory request

	if c.state.SendBufHeadBusy[c.emu.getColorIndex("R")][cgra.Router] { // only one port, must be Router-red

		if c.state.IsToWriteMemory {
			msg := mem.WriteReqBuilder{}.
				WithAddress(uint64(c.state.AddrBuf)).
				WithData(makeBytesFromUint32(c.state.SendBufHead[c.emu.getColorIndex("R")][cgra.Router].First())).
				WithSrc(c.ports[cgra.Side(cgra.Router)].local.AsRemote()).
				WithDst(c.ports[cgra.Side(cgra.Router)].remote).
				Build()

			err := c.ports[cgra.Side(cgra.Router)].local.Send(msg)
			if err != nil {
				// Backpressure: Memory write send failed
				Trace("Backpressure",
					"Type", "MemoryWriteFailed",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Time", float64(c.Engine.CurrentTime()*1e9),
					"Address", c.state.AddrBuf,
					"Error", fmt.Sprintf("%v", err),
				)
				return madeProgress
			}

			Trace("Memory",
				"Behavior", "Send",
				slog.Float64("Time", float64(c.Engine.CurrentTime()*1e9)),
				"Data", c.state.SendBufHead[c.emu.getColorIndex("R")][cgra.Router].First(),
				"Pred", c.state.SendBufHead[c.emu.getColorIndex("R")][cgra.Router].Pred,
				"Color", "R",
				"Src", msg.Src,
				"Dst", msg.Dst,
			)
			c.state.SendBufHeadBusy[c.emu.getColorIndex("R")][cgra.Router] = false
		} else {
			msg := mem.ReadReqBuilder{}.
				WithAddress(uint64(c.state.AddrBuf)).
				WithSrc(c.ports[cgra.Side(cgra.Router)].local.AsRemote()).
				WithDst(c.ports[cgra.Side(cgra.Router)].remote).
				WithByteSize(4).
				Build()

			err := c.ports[cgra.Side(cgra.Router)].local.Send(msg)
			if err != nil {
				// Backpressure: Memory read send failed
				Trace("Backpressure",
					"Type", "MemoryReadFailed",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Time", float64(c.Engine.CurrentTime()*1e9),
					"Address", c.state.AddrBuf,
					"Error", fmt.Sprintf("%v", err),
				)
				return madeProgress
			}

			Trace("Memory",
				"Behavior", "Send",
				slog.Float64("Time", float64(c.Engine.CurrentTime()*1e9)),
				"Data", c.state.AddrBuf,
				"Color", "R",
				"Src", msg.Src,
				"Dst", msg.Dst,
			)
			c.state.SendBufHeadBusy[c.emu.getColorIndex("R")][cgra.Router] = false
		}
	}

	return madeProgress
}

func convert4BytesToUint32(data []byte) uint32 {
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
}

func (c *Core) doRecv() bool {
	madeProgress := false
	for i := 0; i < 8; i++ { //direction
		item := c.ports[cgra.Side(i)].local.PeekIncoming()
		if item == nil {
			continue
		}

		// fmt.Printf("%10f, %s, %d retrieved\n",
		// 	c.Engine.CurrentTime()*1e9,
		// 	c.Name(), cgra.Side(i))

		//fmt.Printf("%s Scanning direction %d(0 is North, 3 is West)\n", c.Name(), i)
		for color := 0; color < 4; color++ {
			//fmt.Printf("%s Receiving Data with color %d. Recv buffer head: %+v\n",
			//	c.Name(), color, c.state.RecvBufHeadReady[color][i])
			// In AsyncOp mode, we should allow new data to overwrite old data even if RecvBufHeadReady is true
			// This is because different instructions in different instruction groups may need different data
			// from the same port. The RefCount mechanism will ensure that data is not consumed prematurely.
			// In SyncOp mode, if RecvBufHeadReady is true, it means old data hasn't been consumed yet,
			// so we should skip receiving new data to avoid overwriting the old data.
			if c.state.RecvBufHeadReady[color][i] && c.state.Mode == SyncOp {
				// Backpressure: Data reception skipped due to RecvBufHeadReady being true
				oldData := c.state.RecvBufHead[color][i].First()
				oldPred := c.state.RecvBufHead[color][i].Pred
				// Try to peek at the incoming message to see what data we're skipping
				msg := item.(*cgra.MoveMsg)
				if color == msg.Color {
					newData := msg.Data.First()
					direction := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
					Trace("Backpressure",
						"Type", "RecvSkipped",
						"X", c.state.TileX,
						"Y", c.state.TileY,
						"Direction", direction,
						"Color", color,
						"Time", float64(c.Engine.CurrentTime()*1e9),
						"Reason", "RecvBufHeadReady=true (old data not consumed)",
						"OldData", oldData,
						"OldPred", oldPred,
						"NewData", newData,
						"NewPred", msg.Data.Pred,
					)
					// if (c.state.TileX == 1 && c.state.TileY == 2 && i == int(cgra.West)) ||
					// 	(c.state.TileX == 2 && c.state.TileY == 3 && i == int(cgra.South)) ||
					// 	(c.state.TileX == 2 && c.state.TileY == 2 && i == int(cgra.East)) {
					// 	sideName := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
					// 	fmt.Fprintf(os.Stderr, "[Recv_SKIP] Core (%d,%d): %s[color=%d] skipped newData=%d, oldData=%d, oldPred=%v\n",
					// 		c.state.TileX, c.state.TileY, sideName, color, newData, oldData, oldPred)
					// }
				}
				continue
			}

			msg := item.(*cgra.MoveMsg)
			if color != msg.Color {
				continue
			}

			// In AsyncOp mode, always update RecvBufHead with new data if available
			// This allows new data to overwrite old data, which is necessary when different
			// instructions in different instruction groups need different data from the same port
			// Log backpressure if old data is being overwritten (RecvBufHeadReady was true)
			if c.state.RecvBufHeadReady[color][i] && c.state.Mode == AsyncOp {
				// Backpressure: Old data overwritten by new data in AsyncOp mode
				oldData := c.state.RecvBufHead[color][i].First()
				oldPred := c.state.RecvBufHead[color][i].Pred
				newData := msg.Data.First()
				direction := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
				Trace("Backpressure",
					"Type", "DataOverwritten",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Direction", direction,
					"Color", color,
					"Time", float64(c.Engine.CurrentTime()*1e9),
					"Reason", "RecvBufHeadReady=true (old data overwritten by new data in AsyncOp)",
					"OldData", oldData,
					"OldPred", oldPred,
					"NewData", newData,
					"NewPred", msg.Data.Pred,
				)
			}
			c.state.RecvBufHeadReady[color][i] = true
			c.state.RecvBufHead[color][i] = msg.Data

			// ==== NEW: Add to waveform accumulator ====
			if c.state.CycleAcc != nil {
				direction := []string{"North", "East", "South", "West", "Router", "", "", ""}[i]
				c.state.CycleAcc.AddInputPort(
					direction,
					true, // hasData
					msg.Data.First(),
					msg.Data.Pred,
					color,
					true, // ready
				)
			}

			// DataFlow Recv trace for link utilization analysis
			Trace("DataFlow",
				"Behavior", "Recv",
				slog.Float64("Time", float64(c.Engine.CurrentTime()*1e9)),
				"Data", msg.Data.First(),
				"Pred", msg.Data.Pred,
				"Color", color,
				"From", msg.Src,
				"To", msg.Dst,
				"X", c.state.TileX,
				"Y", c.state.TileY,
				"Direction", []string{"North", "East", "South", "West", "Router", "", "", ""}[i],
			)

			c.ports[cgra.Side(i)].local.RetrieveIncoming()
			madeProgress = true
		}
	}

	item := c.ports[cgra.Side(cgra.Router)].local.PeekIncoming()
	if item == nil {
		return madeProgress
	} else {
		if c.state.RecvBufHeadReady[c.emu.getColorIndex("R")][cgra.Router] {
			return madeProgress
		}

		// if msg is DataReadyRsp, then the data is ready
		if msg, ok := item.(*mem.DataReadyRsp); ok {
			c.state.RecvBufHeadReady[c.emu.getColorIndex("R")][cgra.Router] = true
			c.state.RecvBufHead[c.emu.getColorIndex("R")][cgra.Router] = cgra.NewScalar(convert4BytesToUint32(msg.Data))

			Trace("Memory",
				"Behavior", "Recv",
				"Time", float64(c.Engine.CurrentTime()*1e9),
				"Data", msg.Data,
				"Src", msg.Src,
				"Dst", msg.Dst,
				"Pred", c.state.RecvBufHead[c.emu.getColorIndex("R")][cgra.Router].Pred,
				"Color", "R",
			)

			c.ports[cgra.Side(cgra.Router)].local.RetrieveIncoming()
			madeProgress = true
		} else if msg, ok := item.(*mem.WriteDoneRsp); ok {
			c.state.RecvBufHeadReady[c.emu.getColorIndex("R")][cgra.Router] = true
			c.state.RecvBufHead[c.emu.getColorIndex("R")][cgra.Router] = cgra.NewScalar(0)

			Trace("Memory",
				"Behavior", "Recv",
				"Time", float64(c.Engine.CurrentTime()*1e9),
				"Src", msg.Src,
				"Dst", msg.Dst,
				"Pred", c.state.RecvBufHead[c.emu.getColorIndex("R")][cgra.Router].Pred,
				"Color", "R",
			)

			c.ports[cgra.Side(cgra.Router)].local.RetrieveIncoming()
			madeProgress = true
		}
	}

	return madeProgress
}

func (c *Core) runProgram() bool {
	// Removed verbose CoreRunProgram trace to reduce log size
	// If this core has no program, do nothing
	if len(c.state.Code.EntryBlocks) == 0 {
		// Removed verbose CoreNoProgram trace to reduce log size
		return false
	}

	if c.state.PCInBlock == -1 {
		// Check if this is a single-instruction program (like GRANT_ONCE) that should only execute once
		// For loop programs, we should restart execution
		isSingleInstructionProgram := false
		if len(c.state.Code.EntryBlocks) > 0 && len(c.state.Code.EntryBlocks[0].InstructionGroups) == 1 {
			firstInstGroup := c.state.Code.EntryBlocks[0].InstructionGroups[0]
			if len(firstInstGroup.Operations) == 1 && firstInstGroup.Operations[0].OpCode == "GRANT_ONCE" {
				// Check if GRANT_ONCE has already executed
				stateKey := fmt.Sprintf("GrantOnce_0")
				if c.state.States[stateKey] == true {
					isSingleInstructionProgram = true
					// if (c.state.TileX == 2 && c.state.TileY == 2) || (c.state.TileX == 2 && c.state.TileY == 3) {
					// 	fmt.Fprintf(os.Stderr, "[runProgram] Core (%d,%d) Single-instruction program, returning false\n",
					// 		c.state.TileX, c.state.TileY)
					// }
				}
			}
		}

		if isSingleInstructionProgram {
			// Single-instruction program has already executed, don't restart
			return false
		}

		// For loop programs or programs that haven't started yet, restart execution
		// This allows loops to iterate multiple times
		// Removed verbose CoreAboutToStart and CoreStart trace to reduce log size
		c.state.PCInBlock = 0
		c.state.SelectedBlock = &c.state.Code.EntryBlocks[0] // just temp, only one block\
		if c.state.Mode == AsyncOp {
			c.emu.SetUpInstructionGroup(0, &c.state)
		}
		c.state.NextPCInBlock = -1
		// Note: We do NOT reset GRANT_ONCE state for loop programs
		// GRANT_ONCE will execute again, but with predicate=false on subsequent iterations
		// This allows PHI to correctly select the value from previous ADD instead of GRANT_ONCE
		// The GRANT_ONCE implementation in emu.go handles this by checking hasExecuted
		// and setting predicate=false for subsequent executions
	}
	//print("Op2Exec: ", c.state.CurrReservationState.OpToExec, "\n")

	iGroup := c.state.SelectedBlock.InstructionGroups[c.state.PCInBlock]

	//fmt.Printf("%10f, %s, inst: %v inst_length: %d\n", c.Engine.CurrentTime()*1e9, c.Name(), combInst, len(combInst.Insts))

	/* do not have label in codes
	for inst[len(inst)-1] == ':' {
		c.state.PC++
		inst = c.state.Code[c.state.PC]
	}
	*/
	//fmt.Printf("start run inst \n")
	makeProgress := c.emu.RunInstructionGroup(iGroup, &c.state, float64(c.Engine.CurrentTime()*1e9))
	//fmt.Printf("end run inst, current PC = %d\n", nextPC)

	// Removed verbose InstGroup trace to reduce log size
	// Only log critical events or errors

	return makeProgress
	//debug reg value
	//fmt.Printf("Core (%d, %d) Register values:\n", c.state.TileX, c.state.TileY)
	// for i, val := range c.state.Registers {
	// 	if val != 0 { // Only print registers that are used
	// 		fmt.Printf("  $%-2d: %d\n", i, val) // More readable formatting
	// 	}
	// }
}

// If the source data is available, send the result to next core after computation.
// If the source data is not available, do nothing.
// func (c *Core) ConditionSend(dst string, src string, resister int, srcColor int, dstColor int) {
// }

func (c *Core) RouterSrcMustBeDirection(src string) {
	arr := []string{"NORTH", "SOUTH", "WEST", "EAST", "SOUTHWEST", "SOUTHEAST", "NORTHWEST", "NORTHEAST", "ROUTER"}
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

func (c *Core) getIndex(side string) int {
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
	case "NORTHEAST":
		srcIndex = int(cgra.NorthEast)
	case "NORTHWEST":
		srcIndex = int(cgra.NorthWest)
	case "SOUTHEAST":
		srcIndex = int(cgra.SouthEast)
	case "SOUTHWEST":
		srcIndex = int(cgra.SouthWest)
	case "ROUTER":
		srcIndex = int(cgra.Router)
	// Adding new direction
	default:
		panic("invalid side")
	}

	return srcIndex
}
