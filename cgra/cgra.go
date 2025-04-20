// Package cgra defines the commonly used data structure for CGRAs.
package cgra

import (
	"fmt"
	"github.com/sarchlab/akita/v4/sim"
	"sync"
)

// Side defines the side of a tile.
type Side int

const (
	North Side = iota
	West
	South
	East
)

var (
	sideNames   = []string{"North", "West", "South", "East"}
	sideNamesMu sync.RWMutex
)

// Name returns the name of the side.
func (s Side) Name() string {
	sideNamesMu.RLock()
	defer sideNamesMu.RUnlock()
	if int(s) < len(sideNames) {
		return sideNames[s]
	}
	return fmt.Sprintf("Side %d", s)
}

func AddSide(name string) Side {
	sideNamesMu.Lock()
	defer sideNamesMu.Unlock()
	sideNames = append(sideNames, name)
	return Side(len(sideNames) - 1)
}

func SetSideName(s Side, name string) {
	sideNamesMu.Lock()
	defer sideNamesMu.Unlock()
	if int(s) < len(sideNames) {
		sideNames[s] = name
	} else {
		for i := len(sideNames); i <= int(s); i++ {
			sideNames = append(sideNames, fmt.Sprintf("Side %d", i))
		}
		sideNames[s] = name
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
