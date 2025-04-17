package cgra

import "github.com/sarchlab/akita/v4/sim"

type FUBuilder struct {
	engine       sim.Engine
	freq         sim.Freq
	numRegisters int
}

func (b FUBuilder) WithEngine(engine sim.Engine) FUBuilder {
	b.engine = engine
	return b
}

func (b FUBuilder) WithFreq(freq sim.Freq) FUBuilder {
	b.freq = freq
	return b
}

func (b FUBuilder) WithRegisters(numRegisters int) FUBuilder {
	b.numRegisters = numRegisters
	return b
}
