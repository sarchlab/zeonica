// Package config provides a default configuration for the CGRA device.
package config

import (
	"fmt"

	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/noc/v2/networking/mesh"
)

// DeviceBuilder can build CGRA devices.
type DeviceBuilder struct {
	engine        sim.Engine
	freq          sim.Freq
	width, height int
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

// WithWidth sets the width of CGRA mesh.
func (d DeviceBuilder) WithWidth(width int) DeviceBuilder {
	d.width = width
	return d
}

// WithHeight sets the height of CGRA mesh.
func (d DeviceBuilder) WithHeight(height int) DeviceBuilder {
	d.height = height
	return d
}

// Build creates a CGRA device.
func (d DeviceBuilder) Build(name string) cgra.Device {
	dev := &device{
		Name:   name,
		Width:  d.width,
		Height: d.height,
		Tiles:  make([][]*cgra.Tile, d.height),
	}

	nocConnector := mesh.NewConnector().
		WithEngine(d.engine).
		WithFreq(d.freq).
		WithSwitchLatency(1).
		WithBandwidth(1)
	nocConnector.CreateNetwork(name + ".Mesh")

	for y := 0; y < d.height; y++ {
		dev.Tiles[y] = make([]*cgra.Tile, d.width)
		for x := 0; x < d.width; x++ {
			tile := &cgra.Tile{}
			tile.Core = core.NewCore(
				fmt.Sprintf("%s.Tile_%d_%d.Core", name, x, y), d.engine)
			dev.Tiles[y][x] = tile

			nocConnector.AddTile(
				[3]int{x, y, 0},
				[]sim.Port{
					tile.Core.GetPortByName(cgra.East.Name()),
					tile.Core.GetPortByName(cgra.West.Name()),
					tile.Core.GetPortByName(cgra.North.Name()),
					tile.Core.GetPortByName(cgra.South.Name()),
				})
		}
	}

	nocConnector.EstablishNetwork()

	return dev
}
