package config

import (
	"fmt"

	"github.com/sarchlab/akita/v4/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
)

type tileCore interface {
	sim.Component
	MapProgram(program interface{}, x int, y int)
	SetRemotePort(side cgra.Side, port sim.RemotePort)
	GetMemory(x int, y int, addr uint32) uint32
	WriteMemory(x int, y int, data uint32, baseAddr uint32)
	GetTileX() int
	GetTileY() int
}

type tile struct {
	Core                   tileCore
	SharedMemoryController *idealmemcontroller.Comp
}

// GetPort returns the of the tile by the side.
func (t tile) GetPort(side cgra.Side) sim.Port {
	switch side {
	case cgra.North:
		return t.Core.GetPortByName("North")
	case cgra.West:
		return t.Core.GetPortByName("West")
	case cgra.South:
		return t.Core.GetPortByName("South")
	case cgra.East:
		return t.Core.GetPortByName("East")
	case cgra.NorthEast:
		return t.Core.GetPortByName("NorthEast")
	case cgra.SouthEast:
		return t.Core.GetPortByName("SouthEast")
	case cgra.SouthWest:
		return t.Core.GetPortByName("SouthWest")
	case cgra.NorthWest:
		return t.Core.GetPortByName("NorthWest")
	case cgra.Router:
		return t.Core.GetPortByName("Router")
	default:
		panic("invalid side")
	}
}

func (t tile) GetTileX() int {
	return t.Core.GetTileX()
}

func (t tile) GetTileY() int {
	return t.Core.GetTileY()
}
func (t tile) String() string {
	return fmt.Sprintf("Tile(%d, %d)", t.Core.GetTileX(), t.Core.GetTileY())
}

// getMemory returns the memory of the tile.
func (t tile) GetMemory(x int, y int, addr uint32) uint32 {
	return t.Core.GetMemory(x, y, addr)
}

// writeMemory writes the memory of the tile.
func (t tile) WriteMemory(x int, y int, data uint32, baseAddr uint32) {
	t.Core.WriteMemory(x, y, data, baseAddr)
}

func (t tile) WriteSharedMemory(x int, y int, data []byte, baseAddr uint32) { // x, y is useless here
	fmt.Println("WriteSharedMemory(", x, ",", y, ") ", baseAddr, " <- ", data)
	err := t.SharedMemoryController.Storage.Write(uint64(baseAddr), data)
	if err != nil {
		panic(err)
	}
}

// SetRemotePort sets the port that the core can send data to.
func (t tile) SetRemotePort(side cgra.Side, port sim.RemotePort) {
	t.Core.SetRemotePort(side, port)
}

// MapProgram sets the program that the tile needs to run.
func (t tile) MapProgram(program interface{}, x int, y int) {
	t.Core.MapProgram(program, x, y)
}

// A Device is a CGRA device that includes a large number of tiles. Tiles can be
// retrieved using d.Tiles[y][x].
type device struct {
	Name                    string
	Width, Height           int
	Tiles                   [][]*tile
	SharedMemoryControllers []*idealmemcontroller.Comp
}

// GetSize returns the width and height of the device.
func (d *device) GetSize() (int, int) {
	return d.Width, d.Height
}

// GetTile returns the tile at the given coordinates.
// Coordinate system: (0,0) at bottom-left, y increases upward, x increases rightward
// Internal storage: Tiles[y_array][x] where y_array = height - 1 - y (0=top, height-1=bottom)
func (d *device) GetTile(x, y int) cgra.Tile {
	// Convert from user coordinate (0=bottom) to array index (0=top)
	yArray := d.Height - 1 - y
	return d.Tiles[yArray][x]
}

// GetSidePorts returns the ports on the given side of the device.
// Coordinate system: (0,0) at bottom-left, y increases upward, x increases rightward
func (d *device) GetSidePorts(
	side cgra.Side,
	portRange [2]int,
) []sim.Port {
	ports := make([]sim.Port, 0)

	switch side {
	case cgra.North:
		// North side: top row in user coordinates (y=height-1), which is array index 0
		for x := portRange[0]; x < portRange[1]; x++ {
			ports = append(ports, d.Tiles[0][x].GetPort(side))
		}
	case cgra.West:
		// West side: y-coordinate based, need to convert from user coord to array index
		for yUser := portRange[0]; yUser < portRange[1]; yUser++ {
			yArray := d.Height - 1 - yUser
			ports = append(ports, d.Tiles[yArray][0].GetPort(side))
		}
	case cgra.South:
		// South side: bottom row in user coordinates (y=0), which is array index height-1
		for x := portRange[0]; x < portRange[1]; x++ {
			ports = append(ports, d.Tiles[d.Height-1][x].GetPort(side))
		}
	case cgra.East:
		// East side: y-coordinate based, need to convert from user coord to array index
		for yUser := portRange[0]; yUser < portRange[1]; yUser++ {
			yArray := d.Height - 1 - yUser
			ports = append(ports, d.Tiles[yArray][d.Width-1].GetPort(side))
		}
	default:
		panic("invalid side")
	}

	return ports
}

// GetAllCores returns all Core instances from a device.
// This is used by the driver to access all cores for unified startup.
func GetAllCores(d cgra.Device) []*core.Core {
	// Type assert to the concrete device type to access internal Tiles structure
	if dev, ok := d.(*device); ok {
		cores := make([]*core.Core, 0, dev.Width*dev.Height)
		for yArray := 0; yArray < dev.Height; yArray++ {
			for x := 0; x < dev.Width; x++ {
				if dev.Tiles[yArray][x] != nil {
					// Type assert Core to *core.Core
					if c, ok := dev.Tiles[yArray][x].Core.(*core.Core); ok {
						cores = append(cores, c)
					}
				}
			}
		}
		return cores
	}
	// If device is not a *device, return empty slice
	// This handles mock devices in tests
	return []*core.Core{}
}
