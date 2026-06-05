//nolint:funlen,lll,whitespace
package core

import (
	"fmt"
	"log/slog"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type portPair struct {
	local  sim.Port
	remote sim.RemotePort
}

// Core models one CGRA compute core with local state and ports.
type Core struct {
	*sim.TickingComponent

	ports map[cgra.Side]*portPair

	state coreState
	emu   instEmulator
}

// GetRetVal returns the shared return value.
func (c *Core) GetRetVal() uint32 {
	return *c.state.retVal
}

// GetTileX returns the x coordinate of the core.
func (c *Core) GetTileX() int {
	return int(c.state.TileX)
}

// GetTileY returns the y coordinate of the core.
func (c *Core) GetTileY() int {
	return int(c.state.TileY)
}

// GetTickingComponent returns the ticking component wrapper.
func (c *Core) GetTickingComponent() sim.Component {
	return c.TickingComponent
}

// GetMemory returns the memory value at addr for this core.
func (c *Core) GetMemory(x int, y int, addr uint32) uint32 {
	if x == int(c.state.TileX) && y == int(c.state.TileY) {
		return c.state.Memory[addr]
	}
	panic("Invalid Tile")
}

// WriteMemory writes one memory word at baseAddr for this core.
func (c *Core) WriteMemory(x int, y int, data uint32, baseAddr uint32) {
	//fmt.Printf("Core [%d][%d] receive WriteMemory(x=%d, y=%d)\n", c.state.TileX, c.state.TileY, x, y)
	if x == int(c.state.TileX) && y == int(c.state.TileY) {
		c.state.Memory[baseAddr] = data
		//fmt.Printf("Core [%d][%d] write memory[%d] = %d\n", c.state.TileX, c.state.TileY, baseAddr, c.state.Memory[baseAddr])
		timeValue := float64(c.Engine.CurrentTime() * 1e9)
		if TraceEnabled() {
			Trace("Memory",
				"Behavior", "WriteMemory",
				"Time", timeValue,
				"Data", data,
				"X", x,
				"Y", y,
				"Addr", baseAddr,
			)
		} else {
			ObserveMemory("WriteMemory", timeValue, x, y, "", "")
		}
	} else {
		panic(fmt.Sprintf("Invalid Tile: Expect (%d, %d)，but get (%d, %d)", c.state.TileX, c.state.TileY, x, y))
	}
}

// SetRemotePort connects a side to a remote port endpoint.
func (c *Core) SetRemotePort(side cgra.Side, remote sim.RemotePort) {
	c.ports[side].remote = remote
}

func (c *Core) SetSharedSRAMAccessor(accessor SharedSRAMAccessor) {
	c.state.SharedSRAMAccessor = accessor
}

func (c *Core) InjectData(side cgra.Side, color int, data cgra.Data) bool {
	return c.state.recvQueuePush(color, int(side), data)
}

func (c *Core) DrainData(side cgra.Side, color int) (cgra.Data, bool) {
	return c.state.sendQueueConsume(color, int(side))
}

func (c *Core) EnableHostDrain(side cgra.Side) {
	c.state.HostDrainDirections[int(side)] = true
}

// MapProgram sets the program that the core needs to run.
func (c *Core) MapProgram(program interface{}, x int, y int) {
	if prog, ok := program.(Program); ok {
		c.state.Code = prog
	} else {
		panic("MapProgram expects core.Program type")
	}
	c.state.PCInBlock = -1
	c.state.CurrentCycle = 0
	c.state.OpTimingCursor = make(map[int]int)
	c.state.OpTimingLate = make(map[int]bool)
	c.state.OpTimingRollCycle = make(map[int]int64)
	c.state.PendingSyncGroup = nil
	c.state.TimingWaitBlocked = false
	c.state.StallReason = ""
	c.state.StallOpID = 0
	c.state.StallOpCode = ""
	c.state.OpInputReadCache = make(map[string]cgra.Data)
	c.state.resetPortQueues()
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
	c.state.WatchedQueues = matchingQueueWatchesForTile(c.state.EnableQueueWatches, c.state.ConfiguredQueueWatches, x, y)
}

// Tick runs the program for one cycle.
func (c *Core) Tick() (madeProgress bool) {
	madeProgress = c.doRecv() || madeProgress
	// madeProgress = c.AlwaysPart() || madeProgress
	// madeProgress = c.emu.runRoutingRules(&c.state) || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doSend() || madeProgress
	c.state.observeWatchedQueues(float64(c.Engine.CurrentTime() * 1e9))
	c.state.CurrentCycle++
	return madeProgress
}

func makeBytesFromUint32(data uint32) []byte {
	return []byte{byte(data >> 24), byte(data >> 16), byte(data >> 8), byte(data)}
}

//nolint:gocyclo
func (c *Core) doSend() bool {
	madeProgress := false
	for i := 0; i < 8; i++ { // only 8 directions
		if c.state.HostDrainDirections[i] {
			continue
		}
		for color := 0; color < 4; color++ {
			if !c.state.sendQueueHasData(color, i) {
				continue
			}
			head, ok := c.state.sendQueuePeek(color, i)
			if !ok {
				continue
			}

			//fmt.Printf("\033[31m (%d, %d) Sending data %d to %s\033[0m\n", c.state.TileX, c.state.TileY, c.state.SendBufHead[color][i].First(), c.ports[cgra.Side(i)].remote)

			msg := cgra.MoveMsgBuilder{}.
				WithDst(c.ports[cgra.Side(i)].remote).
				WithSrc(c.ports[cgra.Side(i)].local.AsRemote()).
				WithData(head).
				WithSendTime(c.Engine.CurrentTime()).
				WithColor(color).
				Build()

			err := c.ports[cgra.Side(i)].local.Send(msg)
			if err != nil {
				continue
			}

			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				Trace("DataFlow",
					"Behavior", "Send",
					slog.Float64("Time", timeValue),
					"Data", msg.Data.First(),
					"Pred", head.Pred,
					"Color", color,
					"Src", msg.Src,
					"Dst", msg.Dst,
				)
			} else {
				ObserveDataFlow("Send", timeValue, "", "", string(msg.Src), string(msg.Dst))
			}
			c.state.sendQueueConsume(color, i)
			madeProgress = true
		}
	}

	// handle the memory request

	routerColor := c.emu.getColorIndex("R")
	if c.state.sendQueueHasData(routerColor, int(cgra.Router)) { // only one port, must be Router-red
		head, ok := c.state.sendQueuePeek(routerColor, int(cgra.Router))
		if !ok {
			return madeProgress
		}
		if c.state.IsToWriteMemory {
			physAddr := c.state.SharedMemoryBase + c.state.AddrBuf
			msg := mem.WriteReqBuilder{}.
				WithAddress(uint64(physAddr) * 4).
				WithData(makeBytesFromUint32(head.First())).
				WithSrc(c.ports[cgra.Router].local.AsRemote()).
				WithDst(c.ports[cgra.Router].remote).
				Build()

			err := c.ports[cgra.Router].local.Send(msg)
			if err != nil {
				return madeProgress
			}
			if c.state.PendingMemoryOp != nil && c.state.PendingMemoryOp.IsWrite && !c.state.PendingMemoryOp.RequestSent {
				c.state.PendingMemoryOp.RequestID = msg.ID
				c.state.PendingMemoryOp.RequestSent = true
			}

			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				opID := 0
				addr := c.state.AddrBuf
				if c.state.PendingMemoryOp != nil {
					opID = c.state.PendingMemoryOp.OpID
					addr = c.state.PendingMemoryOp.Address
				}
				physAddr = c.state.SharedMemoryBase + addr
				Trace("Memory",
					"Behavior", "Send",
					slog.Float64("Time", timeValue),
					"OpID", opID,
					"OpCode", "STORE",
					"Addr", addr,
					"PhysAddr", physAddr,
					"Data", head.First(),
					"Pred", head.Pred,
					"Color", "R",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Src", msg.Src,
					"Dst", msg.Dst,
				)
			} else {
				ObserveMemory("Send", timeValue, int(c.state.TileX), int(c.state.TileY), string(msg.Src), string(msg.Dst))
			}
			c.state.sendQueueConsume(routerColor, int(cgra.Router))
			madeProgress = true
		} else {
			physAddr := c.state.SharedMemoryBase + c.state.AddrBuf
			msg := mem.ReadReqBuilder{}.
				WithAddress(uint64(physAddr) * 4).
				WithSrc(c.ports[cgra.Router].local.AsRemote()).
				WithDst(c.ports[cgra.Router].remote).
				WithByteSize(4).
				Build()

			err := c.ports[cgra.Router].local.Send(msg)
			if err != nil {
				return madeProgress
			}
			if c.state.PendingMemoryOp != nil && !c.state.PendingMemoryOp.IsWrite && !c.state.PendingMemoryOp.RequestSent {
				c.state.PendingMemoryOp.RequestID = msg.ID
				c.state.PendingMemoryOp.RequestSent = true
			}

			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				opID := 0
				addr := c.state.AddrBuf
				if c.state.PendingMemoryOp != nil {
					opID = c.state.PendingMemoryOp.OpID
					addr = c.state.PendingMemoryOp.Address
				}
				physAddr = c.state.SharedMemoryBase + addr
				Trace("Memory",
					"Behavior", "Send",
					slog.Float64("Time", timeValue),
					"OpID", opID,
					"OpCode", "LOAD",
					"Addr", addr,
					"PhysAddr", physAddr,
					"Data", c.state.AddrBuf,
					"Color", "R",
					"X", c.state.TileX,
					"Y", c.state.TileY,
					"Src", msg.Src,
					"Dst", msg.Dst,
				)
			} else {
				ObserveMemory("Send", timeValue, int(c.state.TileX), int(c.state.TileY), string(msg.Src), string(msg.Dst))
			}
			c.state.sendQueueConsume(routerColor, int(cgra.Router))
			madeProgress = true
		}
	}

	return madeProgress
}

func convert4BytesToUint32(data []byte) uint32 {
	return uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
}

//nolint:gocyclo
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
			if c.state.recvQueueIsFull(color, i) {
				continue
			}

			msg := item.(*cgra.MoveMsg)
			if color != msg.Color {
				continue
			}

			if !c.state.recvQueuePush(color, i, msg.Data) {
				continue
			}

			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				Trace("DataFlow",
					"Behavior", "Recv",
					"Time", timeValue,
					"Data", msg.Data.First(),
					"Pred", msg.Data.Pred,
					"Src", msg.Src,
					"Dst", msg.Dst,
					"Color", color,
				)
			} else {
				ObserveDataFlow("Recv", timeValue, "", "", string(msg.Src), string(msg.Dst))
			}

			c.ports[cgra.Side(i)].local.RetrieveIncoming()
			madeProgress = true
		}
	}

	item := c.ports[cgra.Router].local.PeekIncoming()
	if item == nil {
		return madeProgress
	}
	routerColor := c.emu.getColorIndex("R")
	routerDir := int(cgra.Router)
	if c.state.recvQueueIsFull(routerColor, routerDir) {
		return madeProgress
	}

	// if msg is DataReadyRsp, then the data is ready
	if msg, ok := item.(*mem.DataReadyRsp); ok {
		if c.state.PendingMemoryOp != nil &&
			!c.state.PendingMemoryOp.IsWrite &&
			c.state.PendingMemoryOp.RequestSent &&
			msg.RespondTo == c.state.PendingMemoryOp.RequestID {
			value := cgra.NewScalar(convert4BytesToUint32(msg.Data))
			c.state.PendingMemoryOp.DataReady = &value
			c.ports[cgra.Router].local.RetrieveIncoming()
			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				Trace("Memory",
					"Behavior", "Recv",
					"Time", timeValue,
					"OpID", c.state.PendingMemoryOp.OpID,
					"OpCode", c.state.PendingMemoryOp.OpCode,
					"Addr", c.state.PendingMemoryOp.Address,
					"PhysAddr", c.state.SharedMemoryBase+c.state.PendingMemoryOp.Address,
					"Data", msg.Data,
					"Src", msg.Src,
					"Dst", msg.Dst,
					"Pred", value.Pred,
					"Color", "R",
					"X", c.state.TileX,
					"Y", c.state.TileY,
				)
			}
			madeProgress = true
			return madeProgress
		}
		value := cgra.NewScalar(convert4BytesToUint32(msg.Data))
		if !c.state.recvQueuePush(routerColor, routerDir, value) {
			return madeProgress
		}

		timeValue := float64(c.Engine.CurrentTime() * 1e9)
		if TraceEnabled() {
			Trace("Memory",
				"Behavior", "Recv",
				"Time", timeValue,
				"Data", msg.Data,
				"Src", msg.Src,
				"Dst", msg.Dst,
				"Pred", value.Pred,
				"Color", "R",
			)
		} else {
			ObserveMemory("Recv", timeValue, int(c.state.TileX), int(c.state.TileY), string(msg.Src), string(msg.Dst))
		}

		c.ports[cgra.Router].local.RetrieveIncoming()
		madeProgress = true
	} else if msg, ok := item.(*mem.WriteDoneRsp); ok {
		if c.state.PendingMemoryOp != nil &&
			c.state.PendingMemoryOp.IsWrite &&
			c.state.PendingMemoryOp.RequestSent &&
			msg.RespondTo == c.state.PendingMemoryOp.RequestID {
			c.state.PendingMemoryOp.WriteDone = true
			c.ports[cgra.Router].local.RetrieveIncoming()
			timeValue := float64(c.Engine.CurrentTime() * 1e9)
			if TraceEnabled() {
				Trace("Memory",
					"Behavior", "Recv",
					"Time", timeValue,
					"OpID", c.state.PendingMemoryOp.OpID,
					"OpCode", c.state.PendingMemoryOp.OpCode,
					"Addr", c.state.PendingMemoryOp.Address,
					"PhysAddr", c.state.SharedMemoryBase+c.state.PendingMemoryOp.Address,
					"Data", c.state.PendingMemoryOp.Value,
					"Src", msg.Src,
					"Dst", msg.Dst,
					"Pred", c.state.PendingMemoryOp.Pred,
					"Color", "R",
					"X", c.state.TileX,
					"Y", c.state.TileY,
				)
			}
			madeProgress = true
			return madeProgress
		}
		value := cgra.NewScalar(0)
		if !c.state.recvQueuePush(routerColor, routerDir, value) {
			return madeProgress
		}

		timeValue := float64(c.Engine.CurrentTime() * 1e9)
		if TraceEnabled() {
			Trace("Memory",
				"Behavior", "Recv",
				"Time", timeValue,
				"Src", msg.Src,
				"Dst", msg.Dst,
				"Pred", value.Pred,
				"Color", "R",
			)
		} else {
			ObserveMemory("Recv", timeValue, int(c.state.TileX), int(c.state.TileY), string(msg.Src), string(msg.Dst))
		}

		c.ports[cgra.Router].local.RetrieveIncoming()
		madeProgress = true
	}

	return madeProgress
}

func (c *Core) runProgram() bool {
	if len(c.state.Code.EntryBlocks) == 0 {
		return false
	}

	if c.state.PCInBlock == -1 {
		c.state.PCInBlock = 0
		c.state.SelectedBlock = &c.state.Code.EntryBlocks[0] // just temp, only one block\
		if c.state.Mode == AsyncOp {
			c.emu.SetUpInstructionGroup(0, &c.state)
		}
		c.state.NextPCInBlock = -1
	}
	//print("Op2Exec: ", c.state.CurrReservationState.OpToExec, "\n")

	iGroup := c.state.SelectedBlock.InstructionGroups[c.state.PCInBlock]

	makeProgress := c.emu.RunInstructionGroup(iGroup, &c.state, float64(c.Engine.CurrentTime()*1e9))

	return makeProgress
}
