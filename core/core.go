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
	// madeProgress = c.AlwaysPart() || madeProgress
	madeProgress = c.emu.runRoutingRules(&c.state) || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doSend() || madeProgress
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
				WithSrc(c.ports[cgra.Side(i)].local).
				WithData(c.state.SendBufHead[color][i]).
				WithSendTime(c.Engine.CurrentTime()).
				WithColor(color).
				Build()

			err := c.ports[cgra.Side(i)].remote.Send(msg)
			if err != nil {
				continue
			}

			fmt.Printf("%10f, %s, Send %d %s->%s, Color %d\n",
				c.Engine.CurrentTime()*1e9,
				c.Name(),
				msg.Data, msg.Src.Name(), msg.Dst.Name(),
				color)
			c.state.SendBufHeadBusy[color][i] = false
		}
	}

	return madeProgress
}

func (c *Core) doRecv() bool {
	madeProgress := false

	for i := 0; i < 4; i++ { //direction
		item := c.ports[cgra.Side(i)].local.Peek()
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
				msg.Data, msg.Src.Name(), msg.Dst.Name(),
				color)

			c.ports[cgra.Side(i)].local.Retrieve(c.Engine.CurrentTime())
			madeProgress = true
		}
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
	madeProgress := true //If madeprogress, tick, otherwise, wait
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
			madeProgress = c.Router(parts[1], parts[2], parts[3]) || madeProgress
		case "WAIT_AND":
			c.WaitAnd(parts[1], parts[2], parts[3]) //Pending modification
		default:
			panic("Invalid Instruction")
		}

		c.state.PC++
		nextPC := c.state.PC
		if prevPC == nextPC {
			return false
		}

		fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)
		if int(c.state.PC) >= len(c.state.Code) {
			return false
		}

		inst = c.state.Code[c.state.PC]
	}

	return madeProgress
}

// If data from two sources is not ready, wait to ready.
func (c *Core) WaitAnd(src1 string, src2 string, color string) {
	src1Index := c.getIndex(src1)
	src2Index := c.getIndex(src2)
	colorIndex := c.emu.getColorIndex(color)

	if !c.state.RecvBufHeadReady[colorIndex][src1Index] || !c.state.RecvBufHeadReady[colorIndex][src2Index] {
		//c.state.PC = uint32(len(c.state.Code))
		fmt.Printf("%10f, %s, Data from %s and %s is not both available\n", c.Engine.CurrentTime()*1e9, c.Name(), src1, src2)
		//return false
	}
	fmt.Printf("%10f, %s, Wait data from %s and %s\n", c.Engine.CurrentTime()*1e9, c.Name(), src1, src2)
	c.state.PC++
	//return true
}

// Wait for data is ready and send.
func (c *Core) Router(dst string, src string, color string) bool {

	srcIndex := c.getIndex(src)
	dstIndex := c.getIndex(dst)
	colorIndex := c.emu.getColorIndex(color)
	//The data is not ready.
	if !c.state.RecvBufHeadReady[colorIndex][srcIndex] {
		fmt.Printf("Router Src not READY %s\n", c.Name())
		return false
	}

	//The receiver is not ready.
	if c.state.SendBufHeadBusy[colorIndex][dstIndex] {
		fmt.Printf("Router Dst not READY %s\n", c.Name())
		return false
	}

	c.state.SendBufHeadBusy[colorIndex][dstIndex] = true
	c.state.SendBufHead[colorIndex][dstIndex] = c.state.RecvBufHead[colorIndex][srcIndex]
	fmt.Printf("%10f, %s, ROUTER %d %s->%s\n",
		c.Engine.CurrentTime()*1e9,
		c.Name(),
		c.state.RecvBufHead[colorIndex][srcIndex], c.Name(), dst)
	return true
}

// If the source data is available, send the result to next core after computation.
// If the source data is not available, do nothing.
func (c *Core) ConditionSend(dst string, src string, resister int, srcColor int, dstColor int) {

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
