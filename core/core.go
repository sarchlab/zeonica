package core

import (
	"fmt"
	"strconv"
	"strings"

	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/mem"
)

type Core struct {
	*sim.TickingComponent

	MemPort sim.Port

	MemTable mem.LowModuleFinder

	Code []string

	State state

	Waiting     bool
	WaitingInst []string
}

func (c *Core) Tick(now sim.VTimeInSec) (madeProgress bool) {

	instStr := c.Code[c.PC]
	tokens := strings.Split(instStr, ",")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	switch tokens[0] {
	case "TID_X":
		fmt.Printf("Executing TID_X\n")
		c.writeOperand(tokens[1], uint32(c.TileX))
	case "SHL":
		fmt.Printf("Executing SHL\n")
		c.writeOperand(tokens[1],
			c.readOperand(tokens[2])<<c.readOperand(tokens[3]))
	case "ADD":
		fmt.Printf("Executing ADD\n")
		src1 := c.readOperand(tokens[2])
		src2 := c.readOperand(tokens[3])
		dst := src1 + src2
		c.writeOperand(tokens[1], dst)
	case "LD":
		fmt.Printf("Executing LD\n")
		addr := c.readOperand(tokens[2])
		memDst := c.MemTable.Find(uint64(addr))
		req := mem.ReadReqBuilder{}.
			WithAddress(uint64(addr)).
			WithByteSize(4).
			WithSrc(c.MemPort).
			WithDst(memDst).
			WithPID(0).
			WithSendTime(now).
			Build()

		err := c.MemPort.Send(req)
		if err != nil {
			return false
		}

		c.Waiting = true
		c.WaitingInst = tokens
		return true
	}

	fmt.Printf("%+v\n", c.Registers)
	c.PC++

	return true
}

func (c *Core) readOperand(operand string, state threadState) uint32 {
	if operand[0] == '$' {
		regIndex, err := strconv.Atoi(operand[1:])
		if err != nil {
			panic(err)
		}

		return state.Registers[regIndex]
	}

	imm, err := strconv.ParseUint(operand, 10, 32)
	if err != nil {
		panic(err)
	}

	return uint32(imm)
}

func (c *Core) writeOperand(operand string, value uint32) {
	if operand[0] == '$' {
		c.Registers[operand] = value
		return
	}

	if operand[0] == '@' {
		c.Arguments[operand[1:]] = value
		return
	}

	panic("Unknown operand " + operand)
}

func NewCore(name string, engine sim.Engine) *Core {
	c := &Core{
		State: threadState{
			PC:        0,
			Registers: make([]uint32, 256),
		},
	}

	c.TickingComponent = sim.NewTickingComponent(name, engine, 1*sim.GHz, c)
	c.MemPort = sim.NewLimitNumMsgPort(c, 1, name+".MemPort")
	c.AddPort("Mem", c.MemPort)

	return c
}
