package main

import (
	_ "embed"
	"fmt"
	"math/rand"
	"time"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
	"github.com/tebeka/atexit"
)

var width = 2
var height = 2

func test_through(driver api.Driver, program map[string]core.Program) {
	length := 16

	rand.Seed(time.Now().UnixNano())
	src := make([]uint32, length)
	dst := make([]uint32, length)

	//For Int test

	/*
		for i := 0; i < length; i++ {
			src[i] = uint32(i - 7)
		}*/

	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, height}, height, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			driver.MapProgram(program[fmt.Sprintf("(%d,%d)", x, y)], [2]int{x, y})
		}
	}

	driver.Run()

	//For int test
	srcI := make([]int32, length)
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i])) // Convert each element to float.
	}
	fmt.Println(srcI)
	fmt.Println(dstI)
}

func main() {

	// Load the program - use path relative to workspace root
	program := core.LoadProgramFile("./demo_instructions.yaml")
	fmt.Println(program)

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
	test_through(driver, program)
	atexit.Exit(0)
}
