package cgra

import (
	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
)

type device struct {
	Name      string
	TileSum   int
	FuncUnits []FuncUnit
}

func (d *device) GetFuncUnit(id int) *FuncUnit {
	return &d.FuncUnits[id]
}

type DeviceBuilder struct {
	engine  sim.Engine
	freq    sim.Freq
	client  *device
	monitor *monitoring.Monitor
}

// WithEngine sets the engine that drives the device simulation.
func (d DeviceBuilder) WithEngine(engine sim.Engine) DeviceBuilder {
	d.engine = engine
	return d
}

// WithFreq sets the frequency of the device.
func (d DeviceBuilder) WithFreq(freq sim.Freq) DeviceBuilder {
	d.freq = freq
	return d
}

// WithMonitor sets the monitor that monitors the device.
func (d DeviceBuilder) WithMonitor(monitor *monitoring.Monitor) DeviceBuilder {
	d.monitor = monitor
	return d
}

// AddFuncUnit, return the FU and its FUid
func (d DeviceBuilder) AddFuncUnit(name string) (FuncUnit, int) {
	d.client.TileSum++
	// new FU and add to the device
	fu := FuncUnit{Name: name}
	d.client.FuncUnits = append(d.client.FuncUnits, fu)
	return fu, d.client.TileSum - 1
}
