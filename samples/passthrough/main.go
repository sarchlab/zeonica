package main

import (
	_ "embed"

	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
)

var (
	engine sim.Engine
	driver api.Driver
)

//go:embed passthrough.cgraasm
var passThroughKernel string

func passThrough() {
	src := make([]uint32, 1024)
	dst := make([]uint32, 1024)

	driver.FeedIn(src, int(config.East), [2]int{0, 32}, 32)
	driver.Collect(dst, int(config.East), [2]int{0, 32}, 32)

	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			driver.MapProgram(passThroughKernel, [2]int{x, y})
		}
	}

	driver.Run()
}

func main() {
	passThrough()

	atexit.Exit(0)
}
