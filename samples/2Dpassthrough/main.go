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

//go:embed 2Dpassthrough.cgraasm
var passThroughKernel string

func passThrough(driver api.Driver) {
	length := 8
	src1 := make([]uint32, length)
	src2 := make([]uint32, length)
	dst1 := make([]uint32, length)
	dst2 := make([]uint32, length)

	for i := 0; i < length; i++ {
		src1[i] = uint32(i)
		src2[i] = uint32(length - i - 1)
	}

	driver.FeedIn(src1, cgra.West, [2]int{0, height}, height, "R")
	driver.FeedIn(src2, cgra.North, [2]int{0, height}, height, "R")
	driver.Collect(dst1, cgra.East, [2]int{0, height}, height, "R")
	driver.Collect(dst2, cgra.South, [2]int{0, height}, height, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			driver.MapProgram(passThroughKernel, [2]int{x, y})
		}
	}

	driver.Run()

	fmt.Println(src1)
	fmt.Println(src2)
	fmt.Println(dst1)
	fmt.Println(dst2)
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
