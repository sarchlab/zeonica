package core

import (
	"github.com/sarchlab/akita/v4/sim"
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
	c.emu = instEmulator{
		CareFlags: true,
	}
	c.state = coreState{
		SelectedBlock: nil,
		PCInBlock:     -1,
		Directions: map[string]bool{
			"North":     true,
			"East":      true,
			"South":     true,
			"West":      true,
			"NorthEast": true,
			"SouthEast": true,
			"SouthWest": true,
			"NorthWest": true,
			"Router":    true,
		},
		Registers:        make([]cgra.Data, 64),
		Memory:           make([]uint32, 1024),
		RecvBufHead:      make([][]cgra.Data, 4),
		RecvBufHeadReady: make([][]bool, 4),
		SendBufHead:      make([][]cgra.Data, 4),
		SendBufHeadBusy:  make([][]bool, 4),
		AddrBuf:          0,
		IsToWriteMemory:  false,
		States:           make(map[string]interface{}),
		Mode:             SyncOp,
		CurrReservationState: ReservationState{
			ReservationMap:  make(map[int]bool),
			OpToExec:        0,
			RefCountRuntime: make(map[string]int),
		},
		CycleAcc: NewCycleAccumulator(),
	}

	for i := 0; i < 4; i++ {
		c.state.RecvBufHead[i] = make([]cgra.Data, 12)
		c.state.RecvBufHeadReady[i] = make([]bool, 12)
		c.state.SendBufHead[i] = make([]cgra.Data, 12)
		c.state.SendBufHeadBusy[i] = make([]bool, 12)
	}

	c.state.States["Phiconst"] = false

	c.ports = make(map[cgra.Side]*portPair)

	b.makePort(c, cgra.North)
	b.makePort(c, cgra.West)
	b.makePort(c, cgra.South)
	b.makePort(c, cgra.East)
	b.makePort(c, cgra.NorthEast)
	b.makePort(c, cgra.SouthEast)
	b.makePort(c, cgra.SouthWest)
	b.makePort(c, cgra.NorthWest)
	b.makePort(c, cgra.Router)
	b.makePort(c, cgra.Dummy1)
	b.makePort(c, cgra.Dummy2)
	b.makePort(c, cgra.Dummy3)

	return c
}

func (b *Builder) makePort(c *Core, side cgra.Side) {
	localPort := sim.NewPort(c, 1, 1, c.Name()+"."+side.Name())
	c.ports[side] = &portPair{
		local: localPort,
	}
	c.AddPort(side.Name(), localPort)
}
