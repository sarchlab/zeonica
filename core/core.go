package core

import (
	"fmt"
	"strings"

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

type Operand struct {
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

// MapProgram sets the program that the core needs to run.
func (c *Core) MapProgram(program []string, x int, y int) {
	c.state.Code = program
	c.state.PC = 0
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
}

// Tick runs the program for one cycle.
func (c *Core) Tick() (madeProgress bool) {
	madeProgress = c.doSend() || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doRecv() || madeProgress
	return madeProgress
}

func (c *Core) Comment(code []string) []string {
	var filteredCode []string
	for _, line := range code {
		if idx := strings.Index(line, "//"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		// Skip empty lines after comment removal.
		if line == "" {
			continue
		}
		filteredCode = append(filteredCode, line)
	}
	return filteredCode
}

func (c *Core) convertBlockToSingleInstruction(lines []string) string {
	return "BLOCK: " + strings.Join(lines, "; ")
}

func (c *Core) convertCombineInstruction(code []string) []string {
	var parsedInstructions []string
	var blockLines []string
	insideBlock := false

	for _, line := range code {
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		if trimmedLine == "{" {
			insideBlock = true
			blockLines = []string{}
			continue
		}

		if trimmedLine == "}" {
			insideBlock = false
			if len(blockLines) > 0 {
				parsedInstructions = append(parsedInstructions, c.convertBlockToSingleInstruction(blockLines))
			}
			continue
		}

		if insideBlock {
			blockLines = append(blockLines, trimmedLine)
		} else {
			parsedInstructions = append(parsedInstructions, trimmedLine)
		}
	}

	return parsedInstructions
}

func (c *Core) runProgram() bool {
	filteredCode := c.Comment(c.state.Code)
	parsedCode := c.convertCombineInstruction(filteredCode)

	if int(c.state.PC) >= len(parsedCode) {
		return false
	}
	inst := parsedCode[c.state.PC]

	// fmt.Printf("%10f, %s, inst: %s inst_length: %d\n", c.Engine.CurrentTime()*1e9, c.Name(), inst, len(inst))
	for inst[len(inst)-1] == ':' {
		c.state.PC++
		inst = parsedCode[c.state.PC]
	}

	//need to determin the entry: block

	prevPC := c.state.PC
	c.state.blockMode = true

	if strings.HasPrefix(inst, "BLOCK: ") {

		blockInst := strings.TrimPrefix(inst, "BLOCK: ")
		subInstructions := strings.Split(blockInst, "; ")
		c.state.blockMode = true

		blockStalled := false

		for _, subInst := range subInstructions {
			c.state.instStalled = false
			c.emu.RunInst(subInst, &c.state)
			if c.state.instStalled {
				blockStalled = true
				fmt.Printf("BLOCK stalled on sub-instruction: %s\n", subInst)
				break
			}
			fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), subInst)
		}
		c.state.blockMode = false
		// NOTE: In blockMode, we do not consume RecvBufHead immediately.
		if !blockStalled {
			for dir := 0; dir < 4; dir++ {
				for color := 0; color < 4; color++ {
					c.state.RecvBufHeadReady[color][dir] = false
				}
			}
			c.state.PC++
		} else {
			return false
		}
	} else {
		c.emu.RunInst(inst, &c.state)
		nextPC := c.state.PC
		if prevPC == nextPC {
			return false
		}
		fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)
	}

	return true
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
