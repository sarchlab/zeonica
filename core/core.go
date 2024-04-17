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
	// fmt.Println("Before Recv")
	// fmt.Print("Recv Buffer Head(Recv)")
	// fmt.Println(c.state.RecvBufHead)
	// fmt.Print("Recv Buffer Head Ready(Recv)")
	// fmt.Println(c.state.RecvBufHeadReady)
	// fmt.Print("Send Buffer Head(SEND)")
	// fmt.Println(c.state.SendBufHead)
	// fmt.Print("Send Buffer Head Busy(SEND)")
	// fmt.Println(c.state.SendBufHeadBusy)
	madeProgress = c.doRecv() || madeProgress
	// fmt.Println("After Recv")
	// fmt.Print("Recv Buffer Head(Recv)")
	// fmt.Println(c.state.RecvBufHead)
	// fmt.Print("Recv Buffer Head Ready(Recv)")
	// fmt.Println(c.state.RecvBufHeadReady)
	// fmt.Print("Send Buffer Head(SEND)")
	// fmt.Println(c.state.SendBufHead)
	// fmt.Print("Send Buffer Head Busy(SEND)")
	// fmt.Println(c.state.SendBufHeadBusy)
	c.AlwaysPart()
	// fmt.Println("After Always")
	// fmt.Print("Recv Buffer Head(Recv)")
	// fmt.Println(c.state.RecvBufHead)
	// fmt.Print("Recv Buffer Head Ready(Recv)")
	// fmt.Println(c.state.RecvBufHeadReady)
	// fmt.Print("Send Buffer Head(SEND)")
	// fmt.Println(c.state.SendBufHead)
	// fmt.Print("Send Buffer Head Busy(SEND)")
	// fmt.Println(c.state.SendBufHeadBusy)
	madeProgress = c.runProgram() || madeProgress
	// fmt.Println("After Run")
	// fmt.Print("Recv Buffer Head(Recv)")
	// fmt.Println(c.state.RecvBufHead)
	// fmt.Print("Recv Buffer Head Ready(Recv)")
	// fmt.Println(c.state.RecvBufHeadReady)
	// fmt.Print("Send Buffer Head(SEND)")
	// fmt.Println(c.state.SendBufHead)
	// fmt.Print("Send Buffer Head Busy(SEND)")
	// fmt.Println(c.state.SendBufHeadBusy)
	madeProgress = c.doSend() || madeProgress
	// fmt.Println("After Send")
	// fmt.Print("Recv Buffer Head(Recv)")
	// fmt.Println(c.state.RecvBufHead)
	// fmt.Print("Recv Buffer Head Ready(Recv)")
	// fmt.Println(c.state.RecvBufHeadReady)
	// fmt.Print("Send Buffer Head(SEND)")
	// fmt.Println(c.state.SendBufHead)
	// fmt.Print("Send Buffer Head Busy(SEND)")
	// fmt.Println(c.state.SendBufHeadBusy)
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
			// fmt.Print("Send Buffer Head(SEND)")
			// fmt.Println(c.state.SendBufHead)
			// fmt.Print("Send Buffer Head Busy(SEND)")
			// fmt.Println(c.state.SendBufHeadBusy)
			c.state.SendBufHeadBusy[color][i] = false
		}
	}

	return madeProgress
}

func (c *Core) doRecv() bool {
	madeProgress := false

	for i := 0; i < 4; i++ {
		for color := 0; color < 4; color++ {
			if c.state.RecvBufHeadReady[color][i] {
				continue
			}

			item := c.ports[cgra.Side(i)].local.Retrieve(c.Engine.CurrentTime())
			if item == nil {
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
			//fmt.Print("Recv Buffer Head(Recv)")
			//fmt.Println(c.state.RecvBufHead)
			//fmt.Print("Recv Buffer Head Ready(Recv)")
			//fmt.Println(c.state.RecvBufHeadReady)
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
func (c *Core) AlwaysPart() {
	//madeProgress := false //If madeprogress, tick, otherwise, wait
	if int(c.state.PC) >= len(c.state.Code) {
		return
	}

	inst := c.state.Code[c.state.PC]
	for inst[len(inst)-1] == ':' {
		c.state.PC++
		inst = c.state.Code[c.state.PC]
	}

	for strings.HasPrefix(inst, "@") {
		if int(c.state.PC) >= len(c.state.Code) {
			return
		}
		inst := c.state.Code[c.state.PC]
		prevPC := c.state.PC
		parts := strings.Split(inst, ",")
		instName := parts[0]
		instName = strings.TrimLeft(instName, "@")
		switch instName {
		case "ROUTER_FORWARD":
			c.Router(parts[1], parts[2], parts[3])
		case "WAIT_AND":
			c.WaitAnd(parts[1], parts[2], parts[3])
		default:
			panic("Invalid Instruction")
		}
		c.state.PC++
		nextPC := c.state.PC
		if prevPC == nextPC {
			return
		}
		fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)
	}
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
func (c *Core) Router(dst string, src string, color string) {

	srcIndex := c.getIndex(src)
	dstIndex := c.getIndex(dst)
	colorIndex := c.emu.getColorIndex(color)

	//The data is not ready.
	if !c.state.RecvBufHeadReady[colorIndex][srcIndex] {
		fmt.Println("Router Src not READY")
		fmt.Print(c.state.RecvBufHeadReady[colorIndex])
		fmt.Println()
		//c.state.PC = uint32(len(c.state.Code))
		//return false
		return
	}

	//The receiver is not ready.
	if c.state.SendBufHeadBusy[colorIndex][dstIndex] {
		fmt.Println("Router Dst not READY")
		fmt.Print(c.state.SendBufHead[colorIndex])
		fmt.Println()
		//c.state.PC = uint32(len(c.state.Code))
		//return false
		return
	}

	c.state.SendBufHeadBusy[colorIndex][dstIndex] = true
	c.state.SendBufHead[colorIndex][dstIndex] = c.state.RecvBufHead[colorIndex][srcIndex]
	fmt.Printf("%10f, %s, ROUTER %d %s->%s\n",
		c.Engine.CurrentTime()*1e9,
		c.Name(),
		c.state.RecvBufHead[colorIndex][srcIndex], src, dst)
	fmt.Print("Send Buffer Head(ROUTER)")
	fmt.Println(c.state.SendBufHead)
	//return true
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
