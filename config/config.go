// Package config provides a default configuration for the CGRA device.
package config

import (
	"fmt"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
)

// DeviceBuilder can build CGRA devices.
type DeviceBuilder struct {
	engine  sim.Engine
	freq    sim.Freq
	monitor *monitoring.Monitor
	//portFactory   portFactory
	width, height int
}

// type portFactory interface {
// 	make(c sim.Component, name string) sim.Port
// }

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

	d.createTiles(dev, name)
	d.connectTiles(dev)

	return dev
}

func (d DeviceBuilder) createTiles(
	dev *device,
	name string,
) {
	for y := 0; y < d.height; y++ {
		dev.Tiles[y] = make([]*tile, d.width)
		for x := 0; x < d.width; x++ {
			tile := &tile{}
			coreName := fmt.Sprintf("%s.Tile[%d][%d].Core", name, y, x)
			tile.Core = core.Builder{}.
				WithEngine(d.engine).
				WithFreq(d.freq).
				Build(coreName)

			if d.monitor != nil {
				d.monitor.RegisterComponent(tile.Core)
			}

			tile.Core.MapProgram(core.Program{}, x, y)

			dev.Tiles[y][x] = tile
		}
	}
}

func (d DeviceBuilder) connectTiles(dev *device) {
	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			currentTile := dev.Tiles[y][x]
			// connect to the East tile
			if x < d.width-1 {
				eastTile := dev.Tiles[y][x+1]
				d.connectTilePorts(currentTile, cgra.East, eastTile, cgra.West)
			}
			// connect to the South tile
			if y < d.height-1 {
				southTile := dev.Tiles[y+1][x]
				d.connectTilePorts(currentTile, cgra.South, southTile, cgra.North)
			}
			// connect to the south east tile
			if y < d.height-1 && x < d.width-1 {
				southEastTile := dev.Tiles[y+1][x+1]
				d.connectTilePorts(currentTile, cgra.SouthEast, southEastTile, cgra.NorthWest)
			}
			// connect to the south west tile
			if y < d.height-1 && x > 0 {
				southWestTile := dev.Tiles[y+1][x-1]
				d.connectTilePorts(currentTile, cgra.SouthWest, southWestTile, cgra.NorthEast)
			}
		}
	}
}

func (d DeviceBuilder) connectTilePorts(srcTile *tile,
	srcSide cgra.Side,
	dstTile *tile,
	dstSide cgra.Side) {

	srcPort := srcTile.GetPort(srcSide)
	dstPort := dstTile.GetPort(dstSide)

	connName := fmt.Sprintf("%s.%s.%s.%s",
		srcTile.Core.Name(), srcSide.Name(),
		dstTile.Core.Name(), dstSide.Name(),
	)
	conn := directconnection.MakeBuilder().
		WithEngine(d.engine).
		WithFreq(d.freq).
		Build(connName)

	conn.PlugIn(srcPort)
	conn.PlugIn(dstPort)

	srcTile.SetRemotePort(srcSide, dstPort.AsRemote())
	dstTile.SetRemotePort(dstSide, srcPort.AsRemote())
}
