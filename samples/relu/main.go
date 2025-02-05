package main

import (
	_ "embed"
	"fmt"
	"math/rand"
	"time"
	"unsafe"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 16
var height = 16

// For float test, change reluI.cgraasm to reluF.cgraasm
//
//go:embed reluI.cgraasm
var program string

func relu(driver api.Driver) {
	length := 16

	rand.Seed(time.Now().UnixNano())
	src := make([]uint32, length)
	dst := make([]uint32, length)

	//For float test
	// minF := float32(-10.0)
	// maxF := float32(10.0)
	// for i := 0; i < length; i++ {
	// 	FNum := minF + rand.Float32()*(maxF-minF)
	// 	src[i] = *(*uint32)(unsafe.Pointer(&FNum))
	// }

	//For Int test
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
			driver.MapProgram(program, [2]int{x, y})
		}
	}

	driver.Run()

	//For float test
	// srcF := make([]float32, length)
	// dstF := make([]float32, length)
	// for i := 0; i < length; i++ {
	// 	srcF[i] = *(*float32)(unsafe.Pointer(&src[i]))
	// 	dstF[i] = *(*float32)(unsafe.Pointer(&dst[i])) // Convert each element to float.
	// }
	// fmt.Println(srcF)
	// fmt.Println(dstF)

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
	relu(driver)
	atexit.Exit(0)
}
