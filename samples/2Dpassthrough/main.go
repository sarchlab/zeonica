package main

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 1
var height = 1

//go:embed 2Dpassthrough.cgraasm
var passThroughKernel string

func passThrough(driver api.Driver) {
	length := 2
	src1 := make([]uint32, length)
	src2 := make([]uint32, length)
	dst1 := make([]uint32, length)
	dst2 := make([]uint32, length)

	for i := 0; i < length; i++ {
		src1[i] = uint32(i + 1)
		src2[i] = uint32(length - i)
	}

	driver.FeedIn(src1, cgra.West, [2]int{0, height}, height, "R")
	driver.FeedIn(src2, cgra.North, [2]int{0, height}, width, "R")
	driver.Collect(dst1, cgra.East, [2]int{0, height}, height, "R")
	driver.Collect(dst2, cgra.South, [2]int{0, height}, width, "R")

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
	monitor := monitoring.NewMonitor()

	engine := sim.NewSerialEngine()
	monitor.RegisterEngine(engine)

	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")
	monitor.RegisterComponent(driver)

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		WithMonitor(monitor).
		Build("Device")

	driver.RegisterDevice(device)

	monitor.StartServer()

	passThrough(driver)

	time.Sleep(100 * time.Hour)
	atexit.Exit(0)
}
