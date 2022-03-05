package main

import (
	_ "embed"
	"fmt"

	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
)

//go:embed passthrough.cgraasm
var passThroughKernel string

func passThrough(driver api.Driver) {
	length := 8
	src := make([]uint32, length)
	dst := make([]uint32, length)

	for i := 0; i < length; i++ {
		src[i] = uint32(i)
	}

	driver.FeedIn(src, cgra.West, [2]int{0, 4}, 4)
	driver.Collect(dst, cgra.East, [2]int{0, 4}, 4)

	for x := 0; x < 1; x++ {
		for y := 0; y < 4; y++ {
			driver.MapProgram(passThroughKernel, [2]int{x, y})
		}
	}

	driver.Run()

	fmt.Println(src)
	fmt.Println(dst)
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
		WithWidth(1).
		WithHeight(4).
		Build("Device")

	driver.RegisterDevice(device)

	passThrough(driver)

	atexit.Exit(0)
}
