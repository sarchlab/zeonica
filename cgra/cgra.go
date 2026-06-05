// Package cgra defines the commonly used data structure for CGRAs.
package cgra

import (
	"github.com/sarchlab/akita/v4/sim"
)

// Side defines the side of a tile.
type Side int

const (
	// North is the top cardinal side.
	North Side = iota
	// East is the right cardinal side.
	East
	// South is the bottom cardinal side.
	South
	// West is the left cardinal side.
	West
	// NorthEast is the upper-right diagonal side.
	NorthEast
	// NorthWest is the upper-left diagonal side.
	NorthWest
	// SouthEast is the lower-right diagonal side.
	SouthEast
	// SouthWest is the lower-left diagonal side.
	SouthWest
	// Router is the logical router port side.
	Router
	// Dummy1 is an auxiliary placeholder side.
	Dummy1
	// Dummy2 is an auxiliary placeholder side.
	Dummy2
	// Dummy3 is an auxiliary placeholder side.
	Dummy3
)

// Name returns the name of the side.
//
//nolint:gocyclo
func (s Side) Name() string {
	switch s {
	case North:
		return "North"
	case West:
		return "West"
	case South:
		return "South"
	case East:
		return "East"
	case NorthEast:
		return "NorthEast"
	case NorthWest:
		return "NorthWest"
	case SouthEast:
		return "SouthEast"
	case SouthWest:
		return "SouthWest"
	case Router:
		return "Router"
	case Dummy1:
		return "Dummy1"
	case Dummy2:
		return "Dummy2"
	case Dummy3:
		return "Dummy3"
	default:
		panic("invalid side")
	}
}

// Tile defines a tile in the CGRA.
type Tile interface {
	GetPort(side Side) sim.Port
	SetRemotePort(side Side, port sim.RemotePort)
	MapProgram(program interface{}, x int, y int)
	GetMemory(x int, y int, addr uint32) uint32
	WriteMemory(x int, y int, data uint32, baseAddr uint32)
	WriteSharedMemory(x int, y int, data []byte, baseAddr uint32)
	ReadSharedMemory(x int, y int, addr uint32) uint32
	InjectData(side Side, color int, data Data) bool
	DrainData(side Side, color int) (Data, bool)
	EnableHostDrain(side Side)
	GetTileX() int
	GetTileY() int
	GetRetVal() uint32
	GetTickingComponent() sim.Component
}

// A Device is a CGRA device.
type Device interface {
	GetSize() (width, height int)
	GetTile(x, y int) Tile
	GetSidePorts(side Side, portRange [2]int) []sim.Port
}

// Platform is the hardware platform that may include multiple CGRA devices.
type Platform struct {
	Devices []*Device
}
