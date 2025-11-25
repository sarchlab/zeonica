// Package config provides a default configuration for the CGRA device.
package config

import (
	"fmt"
	"strconv"

	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/mem/mem"
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
	memoryMode    string         // simple or shared or local
	memoryShare   map[[2]int]int //map[[x, y]]GroupID
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

// WithMemoryMode sets the memory mode (simple or shared).
func (d DeviceBuilder) WithMemoryMode(mode string) DeviceBuilder {
	if mode != "simple" && mode != "shared" && mode != "local" {
		panic("Invalid memory mode: " + mode)
	}
	d.memoryMode = mode
	return d
}

// WithMemoryShare sets the memory sharing configuration.
func (d DeviceBuilder) WithMemoryShare(share map[[2]int]int) DeviceBuilder {
	d.memoryShare = share
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
	d.createSharedMemory(dev)

	return dev
}

func (d DeviceBuilder) createSharedMemory(dev *device) {
	if d.memoryMode == "shared" {
		// Create shared memory controller

		controllers := make(map[int]*idealmemcontroller.Comp)
		connections := make(map[int]*directconnection.Comp)

		for x := 0; x < d.width; x++ {
			for y := 0; y < d.height; y++ {
				tile := dev.Tiles[y][x]
				// if has mapping
				if _, ok := d.memoryShare[[2]int{x, y}]; !ok {
					panic("No mapping for tile " + strconv.Itoa(x) + "," + strconv.Itoa(y))
				}
				groupID := d.memoryShare[[2]int{x, y}]
				if _, ok := controllers[groupID]; !ok {
					// has not been created yet, create it
					controller := idealmemcontroller.MakeBuilder().
						WithEngine(d.engine).
						WithNewStorage(4 * mem.GB).
						WithLatency(5).
						Build("SharedMemory")
					controllers[groupID] = controller

					name := fmt.Sprintf("SharedMemory%d%d", x, y)

					conn := directconnection.MakeBuilder().
						WithEngine(d.engine).
						WithFreq(d.freq).
						Build(name)
					conn.PlugIn(controller.GetPortByName("Top"))
					conn.PlugIn(tile.Core.GetPortByName("Router"))
					connections[groupID] = conn
					tile.SetRemotePort(cgra.Router, controller.GetPortByName("Top").AsRemote())
					tile.SharedMemoryController = controller
					dev.SharedMemoryControllers = append(dev.SharedMemoryControllers, controller)

					fmt.Println("Connect Tile (", x, ",", y, ") to SharedMemory Controller (", groupID, ") (new-created)")
				} else {
					// plug in the controller to the tile
					fmt.Println("Connect Tile (", x, ",", y, ") to SharedMemory Controller (", groupID, ") (already-created)")
					connections[groupID].PlugIn(tile.Core.GetPortByName("Router"))
					tile.SetRemotePort(cgra.Router, controllers[groupID].GetPortByName("Top").AsRemote())
					tile.SharedMemoryController = controllers[groupID]
					dev.SharedMemoryControllers = append(dev.SharedMemoryControllers, controllers[groupID])
				}
			}
		}
	} else if d.memoryMode == "local" {
		// create DRAM for each of the tiles
		for y := 0; y < d.height; y++ {
			for x := 0; x < d.width; x++ {
				tile := dev.Tiles[y][x]
				drams := idealmemcontroller.MakeBuilder().
					WithEngine(d.engine).
					WithNewStorage(4 * mem.GB).
					WithLatency(5).
					Build("DRAM")

				conn := directconnection.MakeBuilder().
					WithEngine(d.engine).
					WithFreq(d.freq).
					Build("DRAMConn")

				conn.PlugIn(drams.GetPortByName("Top"))
				conn.PlugIn(tile.Core.GetPortByName("Router")) // use router as the memory port

				// set the remote port of the tile to the DRAM port
				tile.SetRemotePort(cgra.Router, drams.GetPortByName("Top").AsRemote())

				tile.SharedMemoryController = drams
				dev.SharedMemoryControllers = append(dev.SharedMemoryControllers, drams)

				fmt.Println("Init DRAM for tile", tile.Core.Name(), "at", x, y)
			}
		}
	}
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
			// connect to the North tile
			if y < d.height-1 {
				northTile := dev.Tiles[y+1][x]
				d.connectTilePorts(currentTile, cgra.North, northTile, cgra.South)
			}
			// connect to the North East tile
			if y < d.height-1 && x < d.width-1 {
				northEastTile := dev.Tiles[y+1][x+1]
				d.connectTilePorts(currentTile, cgra.NorthEast, northEastTile, cgra.SouthWest)
			}
			// connect to the North West tile
			if y < d.height-1 && x > 0 {
				northWestTile := dev.Tiles[y+1][x-1]
				d.connectTilePorts(currentTile, cgra.NorthWest, northWestTile, cgra.SouthEast)
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
