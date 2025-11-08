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
			for yUser := 0; yUser < d.height; yUser++ {
				// Convert user coordinate to array index
				yArray := d.height - 1 - yUser
				tile := dev.Tiles[yArray][x]
				// if has mapping (memoryShare uses user coordinates)
				if _, ok := d.memoryShare[[2]int{x, yUser}]; !ok {
					panic("No mapping for tile " + strconv.Itoa(x) + "," + strconv.Itoa(yUser))
				}
				groupID := d.memoryShare[[2]int{x, yUser}]
				if _, ok := controllers[groupID]; !ok {
					// has not been created yet, create it
					controller := idealmemcontroller.MakeBuilder().
						WithEngine(d.engine).
						WithNewStorage(4 * mem.GB).
						WithLatency(5).
						Build("SharedMemory")
					controllers[groupID] = controller

					name := fmt.Sprintf("SharedMemory%d%d", x, yUser)

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

					fmt.Println("Connect Tile (", x, ",", yUser, ") to SharedMemory Controller (", groupID, ") (new-created)")
				} else {
					// plug in the controller to the tile
					fmt.Println("Connect Tile (", x, ",", yUser, ") to SharedMemory Controller (", groupID, ") (already-created)")
					connections[groupID].PlugIn(tile.Core.GetPortByName("Router"))
					tile.SetRemotePort(cgra.Router, controllers[groupID].GetPortByName("Top").AsRemote())
					tile.SharedMemoryController = controllers[groupID]
					dev.SharedMemoryControllers = append(dev.SharedMemoryControllers, controllers[groupID])
				}
			}
		}
	} else if d.memoryMode == "local" {
		// create DRAM for each of the tiles
		for yUser := 0; yUser < d.height; yUser++ {
			for x := 0; x < d.width; x++ {
				// Convert user coordinate to array index
				yArray := d.height - 1 - yUser
				tile := dev.Tiles[yArray][x]
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

				fmt.Println("Init DRAM for tile", tile.Core.Name(), "at", x, yUser)
			}
		}
	}
}

func (d DeviceBuilder) createTiles(
	dev *device,
	name string,
) {
	// Internal storage: Tiles[y_array][x] where y_array is array index (0=top, height-1=bottom)
	// User coordinate: (x, y_user) where y_user (0=bottom, height-1=top)
	// Conversion: y_array = height - 1 - y_user
	for yArray := 0; yArray < d.height; yArray++ {
		dev.Tiles[yArray] = make([]*tile, d.width)
		for x := 0; x < d.width; x++ {
			tile := &tile{}
			// Convert array index to user coordinate for naming and MapProgram
			yUser := d.height - 1 - yArray
			coreName := fmt.Sprintf("%s.Tile[%d][%d].Core", name, yUser, x)
			tile.Core = core.Builder{}.
				WithEngine(d.engine).
				WithFreq(d.freq).
				Build(coreName)

			if d.monitor != nil {
				d.monitor.RegisterComponent(tile.Core)
			}

			// MapProgram uses user coordinates (x, y_user) where (0,0) is bottom-left
			tile.Core.MapProgram(core.Program{}, x, yUser)

			dev.Tiles[yArray][x] = tile
		}
	}
}

func (d DeviceBuilder) connectTiles(dev *device) {
	// Internal storage: Tiles[y_array][x] where y_array is array index (0=top, height-1=bottom)
	// In user coordinates: South means downward (y decreases), North means upward (y increases)
	// In array indices: South means y_array increases, North means y_array decreases
	for yArray := 0; yArray < d.height; yArray++ {
		for x := 0; x < d.width; x++ {
			currentTile := dev.Tiles[yArray][x]
			// connect to the East tile (same y_array, x+1)
			if x < d.width-1 {
				eastTile := dev.Tiles[yArray][x+1]
				d.connectTilePorts(currentTile, cgra.East, eastTile, cgra.West)
			}
			// connect to the South tile (y_array+1, same x)
			// In user coords: South = downward = y decreases, so y_array increases
			// Note: This also establishes the reverse connection (North direction)
			if yArray < d.height-1 {
				southTile := dev.Tiles[yArray+1][x]
				d.connectTilePorts(currentTile, cgra.South, southTile, cgra.North)
			}
			// connect to the south east tile
			if yArray < d.height-1 && x < d.width-1 {
				southEastTile := dev.Tiles[yArray+1][x+1]
				d.connectTilePorts(currentTile, cgra.SouthEast, southEastTile, cgra.NorthWest)
			}
			// connect to the south west tile
			if yArray < d.height-1 && x > 0 {
				southWestTile := dev.Tiles[yArray+1][x-1]
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

	// Debug: Log port insertion for Core (2,2) and Core (2,3)
	srcX, srcY := srcTile.GetTileX(), srcTile.GetTileY()
	dstX, dstY := dstTile.GetTileX(), dstTile.GetTileY()
	if (srcX == 2 && srcY == 2) || (srcX == 2 && srcY == 3) || (dstX == 2 && dstY == 2) || (dstX == 2 && dstY == 3) {
		fmt.Printf("[PLUGIN] Conn: %s, PlugIn srcPort: Core (%d,%d) %s (%s)\n",
			connName, srcX, srcY, srcSide.Name(), srcPort.Name())
	}
	conn.PlugIn(srcPort)
	if (srcX == 2 && srcY == 2) || (srcX == 2 && srcY == 3) || (dstX == 2 && dstY == 2) || (dstX == 2 && dstY == 3) {
		fmt.Printf("[PLUGIN] Conn: %s, PlugIn dstPort: Core (%d,%d) %s (%s)\n",
			connName, dstX, dstY, dstSide.Name(), dstPort.Name())
	}
	conn.PlugIn(dstPort)

	// Debug: Log connection establishment for Core (2,2) and Core (2,3)
	if (srcX == 2 && srcY == 2) || (srcX == 2 && srcY == 3) || (dstX == 2 && dstY == 2) || (dstX == 2 && dstY == 3) {
		fmt.Printf("[CONN] Connecting: Core (%d,%d) %s -> Core (%d,%d) %s\n",
			srcX, srcY, srcSide.Name(), dstX, dstY, dstSide.Name())
	}

	srcTile.SetRemotePort(srcSide, dstPort.AsRemote())
	dstTile.SetRemotePort(dstSide, srcPort.AsRemote())

	// Debug: Log SetRemotePort calls for Core (2,2) and Core (2,3)
	if (srcX == 2 && srcY == 2) || (srcX == 2 && srcY == 3) || (dstX == 2 && dstY == 2) || (dstX == 2 && dstY == 3) {
		fmt.Printf("[SETREMOTE] Core (%d,%d) %s -> %s\n",
			srcX, srcY, srcSide.Name(), dstPort.Name())
		fmt.Printf("[SETREMOTE] Core (%d,%d) %s -> %s\n",
			dstX, dstY, dstSide.Name(), srcPort.Name())
	}
}
