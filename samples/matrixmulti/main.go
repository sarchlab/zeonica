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

var width = 3
var height = 3

//go:embed matrixmulti.cgraasm
var matrixMulti string

//go:embed output.cgraasm
var output string

func passThrough(driver api.Driver) {

	//1 2 3
	//4 5 6
	//7 8 9
	src1 := [15]uint32{1, 0, 0, 2, 4, 0, 3, 5, 7, 0, 6, 8, 0, 0, 9}
	//9 8 7
	//6 5 4
	//3 2 1
	src2 := [15]uint32{9, 0, 0, 6, 8, 0, 3, 5, 7, 0, 2, 4, 0, 0, 1}
	dst := make([]uint32, 9)

	driver.FeedIn(src1[:], cgra.West, [2]int{0, height}, height)
	driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			driver.MapProgram(matrixMulti, [2]int{x, y})
		}
	}
	driver.Run()
	driver.Collect(dst, cgra.North, [2]int{0, height}, height)
	for x := width - 1; x > -1; x-- {
		for y := 0; y < height; y++ {
			driver.MapProgram(output, [2]int{x, y})
		}
	}
	driver.Run()
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
