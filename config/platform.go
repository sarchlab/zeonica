package config

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type tileCore interface {
	sim.Component
	MapProgram(program []string, x int, y int)
	SetRemotePort(side cgra.Side, port sim.RemotePort)
	GetMemory(x int, y int, addr uint32) uint32
	WriteMemory(x int, y int, data uint32, baseAddr uint32)
	GetTileX() int
	GetTileY() int
}

type tile struct {
	Core tileCore
}

func (t tile) GetPort(dir interface{}) sim.Port {
	switch v := dir.(type) {
	case cgra.Side:
		// default direction
		switch v {
		case cgra.North:
			return t.Core.GetPortByName("North")
		case cgra.West:
			return t.Core.GetPortByName("West")
		case cgra.South:
			return t.Core.GetPortByName("South")
		case cgra.East:
			return t.Core.GetPortByName("East")
		default:
			panic("invalid cgra.Side")
		}
	case int:
		// custom direction
		if v >= 4 {
			return t.Core.GetPortByName(fmt.Sprintf("CustomDir%d", v))
		}
		panic(fmt.Sprintf("invalid direction number: %d (0-3 are reserved)", v))
	default:
		panic("invalid direction type")
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

// SetRemotePort sets the port that the core can send data to.
func (t tile) SetRemotePort(side cgra.Side, port sim.RemotePort) {
	t.Core.SetRemotePort(side, port)
}

// MapProgram sets the program that the tile needs to run.
func (t tile) MapProgram(program []string, x int, y int) {
	t.Core.MapProgram(program, x, y)
}

// A Device is a CGRA device that includes a large number of tiles. Tiles can be
// retrieved using d.Tiles[y][x].
type device struct {
	Name          string
	Width, Height int
	Tiles         [][]*tile
	Directions    int
}

// GetSize returns the width and height of the device.
func (d *device) GetSize() (int, int) {
	return d.Width, d.Height
}

// GetTile returns the tile at the given coordinates.
func (d *device) GetTile(x, y int) cgra.Tile {
	return d.Tiles[y][x]
}

// GetSidePorts returns the ports on the given side of the device.
func (d *device) GetSidePorts(
	side cgra.Side,
	portRange [2]int,
) []sim.Port {
	ports := make([]sim.Port, 0)

	switch side {
	case cgra.North:
		for x := portRange[0]; x < portRange[1]; x++ {
			ports = append(ports, d.Tiles[0][x].GetPort(side))
		}
	case cgra.West:
		for y := portRange[0]; y < portRange[1]; y++ {
			ports = append(ports, d.Tiles[y][0].GetPort(side))
		}
	case cgra.South:
		for x := portRange[0]; x < portRange[1]; x++ {
			ports = append(ports, d.Tiles[d.Height-1][x].GetPort(side))
		}
	case cgra.East:
		for y := portRange[0]; y < portRange[1]; y++ {
			ports = append(ports, d.Tiles[y][d.Width-1].GetPort(side))
		}
	default:
		panic("invalid side")
	}

	return ports
}
