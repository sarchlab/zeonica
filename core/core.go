package core

import (
	"fmt"
	"strings"

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
	madeProgress = c.AlwaysPart() || madeProgress
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

// Distributor for always executing part, these parts are not controlled by cycles.
func (c *Core) AlwaysPart() bool {
	madeProgress := false
	if int(c.state.PC) >= len(c.state.Code) {
		return false
	}

	inst := c.state.Code[c.state.PC]
	for inst[len(inst)-1] == ':' {
		c.state.PC++
		inst = c.state.Code[c.state.PC]
	}

	for strings.HasPrefix(inst, "@") {
		prevPC := c.state.PC
		parts := strings.Split(inst, ",")
		instName := parts[0]
		instName = strings.TrimLeft(instName, "@")
		switch instName {
		case "ROUTER_FORWARD":
			madeProgress = c.Router(parts[1], parts[2])
		case "WAIT_AND":
			madeProgress = c.WaitAnd(parts[1], parts[2])
		default:
			panic("Invalid Instruction")
		}
		c.state.PC++
		nextPC := c.state.PC
		if prevPC == nextPC {
			return false
		}
	}

	fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)

	return madeProgress
}

// If data from two sources is not ready, jump to tail and do nothing.
func (c *Core) WaitAnd(src1 string, src2 string) bool {
	src1Index := c.getIndex(src1)
	src2Index := c.getIndex(src2)

	if !c.state.RecvBufHeadReady[src1Index] || !c.state.RecvBufHeadReady[src2Index] {
		c.state.PC = uint32(len(c.state.Code))
		fmt.Printf("%10f, %s, Data from %s and %s is not both available\n", c.Engine.CurrentTime()*1e9, c.Name(), src1, src2)
		return false
	}
	fmt.Printf("%10f, %s, Wait data from %s and %s\n", c.Engine.CurrentTime()*1e9, c.Name(), src1, src2)
	return true
}

// Pending modify.
// Just rec the buffer and set the send buffer. Do not do the rec and send staff. Do not
// set the buffer flag to false. The MAC operator will do that
func (c *Core) Router(dst string, src string) bool {

	srcIndex := c.getIndex(src)
	dstIndex := c.getIndex(dst)

	//The data is not ready.
	if !c.state.RecvBufHeadReady[srcIndex] {
		c.state.PC = uint32(len(c.state.Code))
		return false
	}

	//The receiver is not ready.
	if c.state.SendBufHeadBusy[dstIndex] {
		c.state.PC = uint32(len(c.state.Code))
		return false
	}

	c.state.SendBufHeadBusy[dstIndex] = true
	c.state.SendBufHead[dstIndex] = c.state.RecvBufHead[srcIndex]
	return true
}

// If the source data is available, send the result to next core after computation.
// If the source data is not available, do nothing.
func (c *Core) ConditionSend(dst string, src string) {

}

func (c *Core) RouterSrcMustBeDirection(src string) {
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
	default:
		panic("invalid side")
	}

	return srcIndex
}
