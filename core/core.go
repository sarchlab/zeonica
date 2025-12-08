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

type Core struct {
	*sim.TickingComponent

	ports map[cgra.Side]*portPair

	state coreState
	emu   instEmulator
}

func (c *Core) GetRetVal() uint32 {
	return *c.state.retVal
}

func (c *Core) GetTileX() int {
	return int(c.state.TileX)
}

func (c *Core) GetTileY() int {
	return int(c.state.TileY)
}

func (c *Core) GetTickingComponent() sim.Component {
	return c.TickingComponent
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
	} else {
		panic("MapProgram expects core.Program type")
	}
	c.state.PCInBlock = -1
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
}

// Tick runs the program for one cycle.
func (c *Core) Tick() (madeProgress bool) {
	madeProgress = c.doSend() || madeProgress
	// madeProgress = c.AlwaysPart() || madeProgress
	// madeProgress = c.emu.runRoutingRules(&c.state) || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doRecv() || madeProgress
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

			//fmt.Printf("\033[31m (%d, %d) Sending data %d to %s\033[0m\n", c.state.TileX, c.state.TileY, c.state.SendBufHead[color][i].First(), c.ports[cgra.Side(i)].remote)

			msg := cgra.MoveMsgBuilder{}.
				WithDst(c.ports[cgra.Side(i)].remote).
				WithSrc(c.ports[cgra.Side(i)].local.AsRemote()).
				WithData(c.state.SendBufHead[color][i]).
				WithSendTime(c.Engine.CurrentTime()).
				WithColor(color).
				Build()

			err := c.ports[cgra.Side(i)].local.Send(msg)
			if err != nil {
				continue
			}

			Trace("DataFlow",
				"Behavior", "Send",
				slog.Float64("Time", float64(c.Engine.CurrentTime()*1e9)),
				"Data", msg.Data.First(),
				"Pred", c.state.SendBufHead[color][i].Pred,
				"Color", color,
				"Src", msg.Src,
				"Dst", msg.Dst,
			)
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
			if c.state.RecvBufHeadReady[color][i] {
				continue
			}

			msg := item.(*cgra.MoveMsg)
			if color != msg.Color {
				continue
			}

			c.state.RecvBufHeadReady[color][i] = true
			c.state.RecvBufHead[color][i] = msg.Data

			Trace("DataFlow",
				"Behavior", "Recv",
				"Time", float64(c.Engine.CurrentTime()*1e9),
				"Data", msg.Data.First(),
				"Pred", c.state.RecvBufHead[color][i].Pred,
				"Src", msg.Src,
				"Dst", msg.Dst,
				"Color", color,
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
