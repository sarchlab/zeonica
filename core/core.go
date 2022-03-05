package core

import (
	"github.com/sarchlab/zeonica/cgra"
	"gitlab.com/akita/akita/v2/sim"
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
	inst := c.state.Code[c.state.PC]

	prevPC := c.state.PC
	c.emu.RunInst(inst, &c.state)
	nextPC := c.state.PC

	if prevPC == nextPC {
		return false
	}

	return true
}
