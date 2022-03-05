package main

import (
	_ "embed"

	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
)

//go:embed passthrough.cgraasm
var passThroughKernel string

func passThrough(driver api.Driver) {
	src := make([]uint32, 1024)
	dst := make([]uint32, 1024)

	driver.FeedIn(src, cgra.East, [2]int{0, 4}, 4)
	driver.Collect(dst, cgra.West, [2]int{0, 4}, 4)

	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			driver.MapProgram(passThroughKernel, [2]int{x, y})
		}
	}

	driver.Run()
}

func main() {
	engine := sim.NewSerialEngine()

	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(4).
		WithHeight(4).
		Build("Device")

	driver.RegisterDevice(device)

	passThrough(driver)

	atexit.Exit(0)
}
