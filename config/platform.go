package config

import (
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type tileCore interface {
	sim.Component
	MapProgram(program []string, x int, y int)
	SetRemotePort(side cgra.Side, port sim.Port)
}

type tile struct {
	Core tileCore
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
	default:
		panic("invalid side")
	}
}

// SetRemotePort sets the port that the core can send data to.
func (t tile) SetRemotePort(side cgra.Side, port sim.Port) {
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
func (d *device) GetSidePorts(side cgra.Side, portRange [2]int) []sim.Port {
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
