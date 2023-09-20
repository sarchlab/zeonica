package core

import (
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type portPair struct {
	local  sim.Port
	remote sim.Port
}

type Core struct {
	*sim.TickingComponent

	ports map[cgra.Side]*portPair

	state coreState
	emu   instEmulator
}

func (c *Core) SetRemotePort(side cgra.Side, remote sim.Port) {
	c.ports[side].remote = remote
}

// MapProgram sets the program that the core needs to run.
func (c *Core) MapProgram(program []string) {
	c.state.Code = program
	c.state.PC = 0
}

// Tick runs the program for one cycle.
func (c *Core) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = c.doRecv() || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doSend() || madeProgress

	return madeProgress
}

func (c *Core) doSend() bool {
	madeProgress := false

	for i := 0; i < 4; i++ {
		if !c.state.SendBufHeadBusy[i] {
			continue
		}

		msg := cgra.MoveMsgBuilder{}.
			WithDst(c.ports[cgra.Side(i)].remote).
			WithSrc(c.ports[cgra.Side(i)].local).
			WithData(c.state.SendBufHead[i]).
			WithSendTime(c.Engine.CurrentTime()).
			Build()

		err := c.ports[cgra.Side(i)].remote.Send(msg)
		if err != nil {
			continue
		}

		fmt.Printf("%10f, %s, Send %d %s->%s\n",
			c.Engine.CurrentTime()*1e9,
			c.Name(),
			msg.Data, msg.Src.Name(), msg.Dst.Name())

		c.state.SendBufHeadBusy[i] = false
	}

	return madeProgress
}

func (c *Core) doRecv() bool {
	madeProgress := false

	for i := 0; i < 4; i++ {
		if c.state.RecvBufHeadReady[i] {
			continue
		}

		item := c.ports[cgra.Side(i)].local.Retrieve(c.Engine.CurrentTime())
		if item == nil {
			continue
		}

		msg := item.(*cgra.MoveMsg)
		c.state.RecvBufHeadReady[i] = true
		c.state.RecvBufHead[i] = msg.Data

		fmt.Printf("%10f, %s, Recv %d %s->%s\n",
			c.Engine.CurrentTime()*1e9,
			c.Name(),
			msg.Data, msg.Src.Name(), msg.Dst.Name())

		madeProgress = true
	}

	return madeProgress
}

func (c *Core) runProgram() bool {
	if int(c.state.PC) >= len(c.state.Code) {
		return false
	}

	inst := c.state.Code[c.state.PC]
	for inst[len(inst)-1] == ':' {
		c.state.PC++
		inst = c.state.Code[c.state.PC]
	}

	prevPC := c.state.PC
	c.emu.RunInst(inst, &c.state)
	nextPC := c.state.PC

	if prevPC == nextPC {
		return false
	}

	fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)

	return true
}
