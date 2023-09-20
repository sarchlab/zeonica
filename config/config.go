// Package config provides a default configuration for the CGRA device.
package config

import (
	"fmt"

	"github.com/sarchlab/akita/v3/noc/networking/mesh"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
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
		Tiles:  make([][]*tile, d.height),
	}

	nocConnector := mesh.NewConnector().
		WithEngine(d.engine).
		WithFreq(d.freq).
		WithSwitchLatency(1).
		WithBandwidth(1)
	nocConnector.CreateNetwork(name + ".Mesh")

	d.createTiles(dev, name, nocConnector)
	d.setRemovePorts(dev)

	nocConnector.EstablishNetwork()

	return dev
}

func (d DeviceBuilder) createTiles(
	dev *device,
	name string,
	nocConnector *mesh.Connector,
) {
	for y := 0; y < d.height; y++ {
		dev.Tiles[y] = make([]*tile, d.width)
		for x := 0; x < d.width; x++ {
			tile := &tile{}
			coreName := fmt.Sprintf("%s.Tile[%d][%d].Core", name, x, y)
			tile.Core = core.Builder{}.
				WithEngine(d.engine).
				WithFreq(d.freq).
				Build(coreName)

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
}

func (d DeviceBuilder) setRemovePorts(dev *device) {
	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			tile := dev.Tiles[y][x]

			if x > 0 {
				westTile := dev.Tiles[y][x-1]
				tile.SetRemotePort(cgra.West,
					westTile.Core.GetPortByName(cgra.East.Name()))
			}

			if y > 0 {
				northTile := dev.Tiles[y-1][x]
				tile.SetRemotePort(cgra.North,
					northTile.Core.GetPortByName(cgra.South.Name()))
			}

			if x < d.width-1 {
				eastTile := dev.Tiles[y][x+1]
				tile.SetRemotePort(cgra.East,
					eastTile.Core.GetPortByName(cgra.West.Name()))
			}

			if y < d.height-1 {
				southTile := dev.Tiles[y+1][x]
				tile.SetRemotePort(cgra.South,
					southTile.Core.GetPortByName(cgra.North.Name()))
			}
		}
	}
}
