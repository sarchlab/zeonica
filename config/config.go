package config

import (
	"fmt"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
)

type ConnectionRule struct {
	FromX, FromY int // src tile coordinates
	FromDir      int // src direction
	ToX, ToY     int // dst tile coordinates
	ToDir        int // dst direction
}

// DeviceBuilder can build CGRA devices.
type DeviceBuilder struct {
	engine            sim.Engine
	freq              sim.Freq
	monitor           *monitoring.Monitor
	width, height     int
	tileDirections    int
	customConnections []ConnectionRule
}

func (b DeviceBuilder) WithCustomConnection(fromX, fromY, fromDir int, toX, toY, toDir int) DeviceBuilder {
	b.customConnections = append(b.customConnections, ConnectionRule{
		FromX:   fromX,
		FromY:   fromY,
		FromDir: fromDir,
		ToX:     toX,
		ToY:     toY,
		ToDir:   toDir,
	})
	return b
}

func (b DeviceBuilder) WithTileDirections(
	total int,
) DeviceBuilder {
	b.tileDirections = total
	return b
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
				WithDirections(d.tileDirections).
				WithEngine(d.engine).
				WithFreq(d.freq).
				Build(coreName)

			if d.monitor != nil {
				d.monitor.RegisterComponent(tile.Core)
			}

			tile.Core.MapProgram(nil, x, y)

			dev.Tiles[y][x] = tile
		}
	}
}

func (d DeviceBuilder) connectTiles(dev *device) {
	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			currentTile := dev.Tiles[y][x]

			// default 4 way
			if x < d.width-1 {
				eastTile := dev.Tiles[y][x+1]
				d.connectTilePorts(currentTile, cgra.East, eastTile, cgra.West)
			}
			if y < d.height-1 {
				southTile := dev.Tiles[y+1][x]
				d.connectTilePorts(currentTile, cgra.South, southTile, cgra.North)
			}

			// customize direction
			if d.tileDirections > 4 {
				for _, rule := range d.customConnections {
					if rule.FromX >= 0 && rule.FromX < d.width &&
						rule.FromY >= 0 && rule.FromY < d.height &&
						rule.ToX >= 0 && rule.ToX < d.width &&
						rule.ToY >= 0 && rule.ToY < d.height {

						srcTile := dev.Tiles[rule.FromY][rule.FromX]
						dstTile := dev.Tiles[rule.ToY][rule.ToX]

						// Connect the tiles with custom ports
						d.connectTilePorts(srcTile, cgra.Side(rule.FromDir), dstTile, cgra.Side(rule.ToDir))
					}
				}
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
