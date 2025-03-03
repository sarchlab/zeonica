// Package cgra defines the commonly used data structure for CGRAs.
package cgra

import (
	"github.com/sarchlab/akita/v4/sim"
)

// Side defines the side of a tile.
type Side int

const (
	North Side = iota
	East
	South
	West
)

// Name returns the name of the side.
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
	default:
		panic("invalid side")
	}
}

// Tile defines a tile in the CGRA.
type Tile interface {
	GetPort(dir interface{}) sim.Port
	SetRemotePort(side Side, port sim.RemotePort)

	MapProgram(program []string, x int, y int)
	GetMemory(x int, y int, addr uint32) uint32
	WriteMemory(x int, y int, data uint32, baseAddr uint32)
	GetTileX() int
	GetTileY() int
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
