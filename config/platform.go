// Package config defines the hardware platform.
package config

import (
	"github.com/sarchlab/zeonica/core"
	"gitlab.com/akita/akita/v2/sim"
)

// Side defines the side of a tile.
type Side int

const (
	North Side = iota
	West
	South
	East
)

// Tile defines a tile in the CGRA.
type Tile struct {
	Core *core.Core
}

type Device struct {
	Tiles [][]*Tile
}

// GetPort returns the of the tile by the side.
func (t Tile) GetPort(side string) sim.Port {
	return t.Core.GetPortByName(side)
}

// Platform is the hardware platform.
type Platform struct {
	Tiles [][]Tile
}
