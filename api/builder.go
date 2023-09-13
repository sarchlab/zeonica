package api

import "github.com/sarchlab/akita/v3/sim"

type defaultPortFactory struct {
}

func (f defaultPortFactory) make(c sim.Component, name string) sim.Port {
	return sim.NewLimitNumMsgPort(c, 1, name)
}

// DriverBuilder creates a new instance of Driver.
type DriverBuilder struct {
	engine sim.Engine
	freq   sim.Freq
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

// Build create a driver.
func (b DriverBuilder) Build(name string) Driver {
	d := &driverImpl{
		portFactory: defaultPortFactory{},
	}

	d.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, d)

	return d
}
