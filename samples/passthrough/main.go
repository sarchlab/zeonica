package main

import (
	_ "embed"
	"fmt"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 4
var height = 4

//go:embed passthrough.cgraasm
var passThroughKernel string

func passThrough(driver api.Driver) {
	length := 8
	src := make([]uint32, length)
	dst := make([]uint32, length)

	for i := 0; i < length; i++ {
		src[i] = uint32(i)
	}

	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, height}, height, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
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
		WithWidth(width).
		WithHeight(height).
		Build("Device")

	driver.RegisterDevice(device)

	passThrough(driver)

	atexit.Exit(0)
}
