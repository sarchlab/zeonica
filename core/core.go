package core

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/confignew"
)

type portPair struct {
	local  sim.Port
	remote sim.RemotePort
}

type Core struct {
	*sim.TickingComponent

	ports map[cgra.Side]*portPair

	internalInfo map[int]func()
	// a map to store internal information, like the coordinates of the core in mesh.

	freq    sim.Freq
	binding confignew.IDImplBinding

	state coreState
	emu   instEmulator
}

// Ask Yuchao to try the sim to run a main diagram mesh.

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

func (c *Core) WriteMemory(x int, y int, data uint32, baseAddr uint32) {
	fmt.Printf("Core [%d][%d] receive WriteMemory(x=%d, y=%d)\n", c.state.TileX, c.state.TileY, x, y)
	if x == int(c.state.TileX) && y == int(c.state.TileY) {
		c.state.Memory[baseAddr] = data
		fmt.Printf("Core [%d][%d] write memory[%d] = %d\n", c.state.TileX, c.state.TileY, baseAddr, c.state.Memory[baseAddr])
	} else {
		panic(fmt.Sprintf("Invalid Tile: Expect (%d, %d)，but get (%d, %d)", c.state.TileX, c.state.TileY, x, y))
	}
}

func (c *Core) SetRemotePort(side cgra.Side, remote sim.RemotePort) {
	c.ports[side].remote = remote
}

func (c *Core) MapProgram(program []string, x int, y int) {
	c.state.Code = program
	c.state.PC = 0
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
}

func (c *Core) runProgram() bool {
	madeProgress := false
	//run()

	return madeProgress
}

func (c *Core) Tick() (madeProgress bool) {
	madeProgress = c.doSend() || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doRecv() || madeProgress
	return madeProgress
}

func (c *Core) doSend() bool {
	madeProgress := false
	for i := 0; i < 4; i++ {
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
				continue
			}

			fmt.Printf("%10f, %s, Send %d %s->%s, Color %d\n",
				c.Engine.CurrentTime()*1e9,
				c.Name(),
				msg.Data, msg.Src, msg.Dst,
				color)
			c.state.SendBufHeadBusy[color][i] = false
		}
	}

	return madeProgress
}

func (c *Core) doRecv() bool {
	madeProgress := false
	for i := 0; i < 4; i++ { //direction
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

			fmt.Printf("%10f, %s, Recv %d %s->%s, Color %d\n",
				c.Engine.CurrentTime()*1e9,
				c.Name(),
				msg.Data, msg.Src, msg.Dst,
				color)

			c.ports[cgra.Side(i)].local.RetrieveIncoming()
			madeProgress = true
		}
	}

	return madeProgress
}
