package core

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/cgra"
)

// Builder can create new cores.
type Builder struct {
	engine sim.Engine
	freq   sim.Freq
}

// WithEngine sets the engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the core.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// Build creates a core.
func (b Builder) Build(name string) *Core {
	c := &Core{}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)
	c.state = coreState{
		Registers:        make([]uint32, 64),
		Memory:			  make([]uint32, 1024),
		RecvBufHead:      make([][]uint32, 4),
		RecvBufHeadReady: make([][]bool, 4),
		SendBufHead:      make([][]uint32, 4),
		SendBufHeadBusy:  make([][]bool, 4),
	}

	for i := 0; i < 4; i++ {
		c.state.RecvBufHead[i] = make([]uint32, 4)
		c.state.RecvBufHeadReady[i] = make([]bool, 4)
		c.state.SendBufHead[i] = make([]uint32, 4)
		c.state.SendBufHeadBusy[i] = make([]bool, 4)
	}

	c.ports = make(map[cgra.Side]*portPair)

	b.makePort(c, cgra.North)
	b.makePort(c, cgra.West)
	b.makePort(c, cgra.South)
	b.makePort(c, cgra.East)

	return c
}

func (b *Builder) makePort(c *Core, side cgra.Side) {
	localPort := sim.NewLimitNumMsgPort(c, 1, c.Name()+"."+side.Name())
	c.ports[side] = &portPair{
		local: localPort,
	}
	c.AddPort(side.Name(), localPort)
}
