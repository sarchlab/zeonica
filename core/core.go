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

// var computeInstructions = map[string]bool{
// 	"MUL":           true,
// 	"ADDI":          true,
// 	"SUB":           true,
// 	"DIV":           true,
// 	"MUL_CONST":     true,
// 	"MUL_CONST_ADD": true,
// 	"MUL_SUB":       true,
// 	"MAC":           true,
// 	"LLS":           true,
// 	"LRS":           true,
// 	"AND":           true,
// 	"OR":            true,
// 	"XOR":           true,
// 	"NOT":           true,
// 	"LD":            true,
// 	"ST":            true,
// 	"FMUL":          true,
// 	"FADD":          true,
// 	"FADD_CONST":    true,
// 	"FSUB":          true,
// 	"FDIV":          true,
// 	"FMUL_CONST":    true,
// 	"FINC":          true,
// 	"PAS":           true,
// 	"START":         true,
// 	"NAH":           true,
// 	"SEL":           true,
// }

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
// core.go
// core.go - MapProgram
func (c *Core) MapProgram(program []string, x int, y int) {
	mergedCode := make([]string, 0)
	var rcvInstructions []string
	var cmpInstructions []string
	var sndInstructions []string

	for _, line := range program {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			block := strings.Trim(line, "[]")
			phaseParts := strings.Split(block, "|")
			for _, phasePart := range phaseParts {
				phasePart = strings.TrimSpace(phasePart)
				if phasePart == "" {
					continue
				}

				parts := strings.SplitN(phasePart, ":", 2)
				if len(parts) != 2 {
					panic(fmt.Sprintf("Block invalid format: %s", phasePart))
				}
				phaseName := strings.TrimSpace(parts[0])
				instructions := strings.TrimSpace(parts[1])

				var phaseInstrs []string
				for _, instr := range strings.Split(instructions, ";") {
					instr = strings.TrimSpace(instr)
					if instr != "" {
						phaseInstrs = append(phaseInstrs, instr)
					}
				}

				switch phaseName {
				case "RCV":
					if len(phaseInstrs) > 4 {
						panic("At most 4 instructions in RCV phase")
					}
					rcvInstructions = phaseInstrs
				case "CMP":
					if len(phaseInstrs) != 1 {
						panic("At most 1 instruction in CMP phase")
					}
					cmpInstructions = phaseInstrs
				case "SND":
					if len(phaseInstrs) > 4 {
						panic("At most 4 instructions in SND phase")
					}
					sndInstructions = phaseInstrs
				default:
					panic(fmt.Sprintf("Unknown phase name: %s", phaseName))
				}
			}

			mergedCode = append(mergedCode, fmt.Sprintf(
				"BLOCK:%s;%s;%s",
				strings.Join(rcvInstructions, ";"),
				strings.Join(cmpInstructions, ";"),
				strings.Join(sndInstructions, ";"),
			))
			//fmt.Printf("Merged code: %s\n", mergedCode[len(mergedCode)-1])
			rcvInstructions, cmpInstructions, sndInstructions = nil, nil, nil
		} else {
			mergedCode = append(mergedCode, line)
		}
	}

	c.state.Code = mergedCode
	c.state.PC = 0
	c.state.TileX = uint32(x)
	c.state.TileY = uint32(y)
}

func (c *Core) runProgram() bool {
	if int(c.state.PC) >= len(c.state.Code) {
		return false
	}

	line := c.state.Code[c.state.PC]
	madeProgress := false

	for line[len(line)-1] == ':' {
		c.state.PC++
		line = c.state.Code[c.state.PC]
	}

	if strings.HasPrefix(line, "BLOCK:") {
		// This is a block of instructions
		block := strings.TrimPrefix(line, "BLOCK:")
		instructions := strings.Split(block, ";")
		startPC := c.state.PC

		for _, inst := range instructions {
			inst = strings.TrimSpace(inst)

			if inst == "" {
				continue
			}
			fmt.Printf("%10f, %s, inst: %s\n",
				c.Engine.CurrentTime()*1e9, c.Name(), inst)

			prevPC := c.state.PC
			c.emu.RunInst(inst, &c.state)
			if c.state.PC == prevPC {
				return madeProgress // wait to continue
			}
			madeProgress = true
		}
		if c.state.PC == startPC {
			c.state.PC = startPC + 1
		}
	} else {
		// This is a normal instruction
		prevPC := c.state.PC
		c.emu.RunInst(line, &c.state)
		nextPC := c.state.PC
		if prevPC == nextPC {
			return madeProgress
		}
		fmt.Printf("%10f, %s, inst: %s\n",
			c.Engine.CurrentTime()*1e9, c.Name(), line)
		madeProgress = true
	}
	return madeProgress
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
	// Adding new direction
	default:
		panic("invalid side")
	}

	return srcIndex
}

// Distributor for always executing part, these parts are not controlled by cycles.
// func (c *Core) AlwaysPart() bool {
// 	madeProgress := true //If madeprogress, tick, otherwise, wait
// 	if int(c.state.PC) >= len(c.state.Code) {
// 		return false
// 	}

// 	inst := c.state.Code[c.state.PC]
// 	for inst[len(inst)-1] == ':' {
// 		c.state.PC++
// 		inst = c.state.Code[c.state.PC]
// 	}

// 	for strings.HasPrefix(inst, "@") {
// 		prevPC := c.state.PC
// 		parts := strings.Split(inst, ",")
// 		instName := parts[0]
// 		instName = strings.TrimLeft(instName, "@")

// 		switch instName {
// 		case "ROUTER_FORWARD":
// 			madeProgress = c.Router(parts[1], parts[2], parts[3]) || madeProgress
// 		case "WAIT_AND":
// 			c.WaitAnd(parts[1], parts[2], parts[3]) //Pending modification
// 		default:
// 			panic("Invalid Instruction")
// 		}

// 		c.state.PC++
// 		nextPC := c.state.PC
// 		if prevPC == nextPC {
// 			return false
// 		}

// 		fmt.Printf("%10f, %s, Inst %s\n", c.Engine.CurrentTime()*1e9, c.Name(), inst)
// 		if int(c.state.PC) >= len(c.state.Code) {
// 			return false
// 		}

// 		inst = c.state.Code[c.state.PC]
// 	}

// 	return madeProgress
// }
