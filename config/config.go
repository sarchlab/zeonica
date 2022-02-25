// Package config provides a default configuration for the CGRA device.
package config

import (
	"github.com/kaustubhcs/cgra_sim/core"
	"gitlab.com/akita/akita/v2/sim"
	"gitlab.com/akita/mem/v2/idealmemcontroller"
	"gitlab.com/akita/mem/v2/mem"
	"gitlab.com/akita/noc/v2/networking/mesh"
)

type Tile struct {
	Core *core.Core
	Mem  *idealmemcontroller.Comp
}

type Device struct {
	Tiles []*Tile
}

func CreateDevice(engine sim.Engine) *Device {
	d := &Device{}

	nocConnector := mesh.NewConnector().
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithSwitchLatency(1).
		WithBandwidth(1)
	nocConnector.CreateNetwork("Mesh")

	memTable := &mem.InterleavedLowModuleFinder{
		InterleavingSize: 64,
	}

	d.Tiles = make([]*Tile, 0)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			t := &Tile{}

			t.Core = core.NewCore("core", engine)
			t.Core.TileX = x
			t.Core.TileY = y
			t.Core.MemTable = memTable

			t.Mem = idealmemcontroller.New("mem", engine, 40*mem.KB)
			memTable.LowModules = append(memTable.LowModules,
				t.Mem.GetPortByName("Top"))

			d.Tiles = append(d.Tiles, t)

			nocConnector.AddTile(
				[3]int{x, y, 0},
				[]sim.Port{
					t.Core.GetPortByName("Mem"),
					t.Mem.GetPortByName("Top"),
				},
			)
		}
	}

	nocConnector.EstablishNetwork()

	return d
}
