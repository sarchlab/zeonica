package core

import (
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

// Builder can create new cores.
type Builder struct {
	engine        sim.Engine
	freq          sim.Freq
	numDirections int
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

func NewBuilder() Builder {
	return Builder{
		//numDirections: 4, // default 4 direction
	}
}

// Build creates a core.
func (b Builder) Build(name string) *Core {
	c := &Core{}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)
	c.state = coreState{
		Registers:        make([]uint32, 64),
		Memory:           make([]uint32, 1024),
		RecvBufHead:      make([][]uint32, b.numDirections),
		RecvBufHeadReady: make([][]bool, b.numDirections),
		SendBufHead:      make([][]uint32, b.numDirections),
		SendBufHeadBusy:  make([][]bool, b.numDirections),
	}

	for i := 0; i < b.numDirections; i++ {
		c.state.RecvBufHead[i] = make([]uint32, b.numDirections)
		c.state.RecvBufHeadReady[i] = make([]bool, b.numDirections)
		c.state.SendBufHead[i] = make([]uint32, b.numDirections)
		c.state.SendBufHeadBusy[i] = make([]bool, b.numDirections)
	}

	c.ports = make(map[cgra.Side]*portPair)

	for i := 0; i < b.numDirections; i++ {
		b.makePort(c, cgra.Side(i))
	}

	return c
}

func (b *Builder) makePort(c *Core, side cgra.Side) {
	localPort := sim.NewPort(c, 1, 1, c.Name()+"."+side.Name()) //string
	c.ports[side] = &portPair{
		local: localPort,
	}
	c.AddPort(side.Name(), localPort)
}

/*
create a port for core's each id

*/
