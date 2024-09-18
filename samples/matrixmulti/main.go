package main

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 2
var height = 2

//go:embed matrixmulti.cgraasm
var matrixMultiKernal string

//go:embed output.cgraasm
var output string

func matrixMulti(driver api.Driver) {

	//1 2 3
	//4 5 6
	//7 8 9
	//src1 := []uint32{1, 0, 0, 2, 4, 0, 3, 5, 7, 0, 6, 8, 0, 0, 9}
	src1 := []uint32{1, 0, 2, 4, 0, 5}
	//9 8 7
	//6 5 4
	//3 2 1
	//src2 := []uint32{9, 0, 0, 6, 8, 0, 3, 5, 7, 0, 2, 4, 0, 0, 1} //no need zeros
	src2 := []uint32{9, 0, 8, 6, 0, 5}
	// src1Collect := make([]uint32, len(src1))
	// src2Collect := make([]uint32, len(src1))
	dst := make([]uint32, 6)

	driver.FeedIn(src1[:], cgra.West, [2]int{0, height}, height, "R")
	driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width, "R")
	// driver.Collect(src1Collect, cgra.North, [2]int{0, height}, height, "R") //for matrix source data.
	// driver.Collect(src2Collect, cgra.North, [2]int{0, height}, height, "R") //
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			driver.MapProgram(matrixMultiKernal, [2]int{x, y})
		}
	}
	driver.Run()
	driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width, "B") //for output signal
	driver.Collect(dst, cgra.South, [2]int{0, height}, height, "B")  //for output
	// for x := width - 1; x > -1; x-- {
	// 	for y := 0; y < height; y++ {
	// 		driver.MapProgram(output, [2]int{x, y})
	// 	}
	// }
	driver.Run()
	fmt.Println(dst)
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

	matrixMulti(driver)

	time.Sleep(100 * time.Hour)
	atexit.Exit(0)
}
