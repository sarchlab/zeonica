package api

import "github.com/sarchlab/akita/v4/sim"

type defaultPortFactory struct {
	incomingBufCap int
	outgoingBufCap int
}

func (f defaultPortFactory) make(c sim.Component, name string) sim.Port {
	incoming := f.incomingBufCap
	if incoming <= 0 {
		incoming = 1
	}
	outgoing := f.outgoingBufCap
	if outgoing <= 0 {
		outgoing = 1
	}
	return sim.NewPort(c, incoming, outgoing, name)
}

// DriverBuilder creates a new instance of Driver.
type DriverBuilder struct {
	engine                sim.Engine
	freq                  sim.Freq
	portIncomingBufferCap int
	portOutgoingBufferCap int
}

// WithEngine sets the engine.
func (b DriverBuilder) WithEngine(engine sim.Engine) DriverBuilder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the driver.
func (b DriverBuilder) WithFreq(freq sim.Freq) DriverBuilder {
	b.freq = freq
	return b
}

// WithPortBufferDepth configures driver boundary-port incoming/outgoing capacity.
func (b DriverBuilder) WithPortBufferDepth(incoming, outgoing int) DriverBuilder {
	b.portIncomingBufferCap = incoming
	b.portOutgoingBufferCap = outgoing
	return b
}

// Build create a driver.
func (b DriverBuilder) Build(name string) Driver {
	d := &driverImpl{
		portFactory: defaultPortFactory{
			incomingBufCap: b.portIncomingBufferCap,
			outgoingBufCap: b.portOutgoingBufferCap,
		},
	}

	d.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, d)

	return d
}
