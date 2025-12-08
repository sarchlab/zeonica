package main

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestPhiOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 5

	// Generate random test data
	cmpSrcData1 := []uint32{6, 7, 3, 4, 8}
	cmpSrcData2 := []uint32{1, 2, 3, 4, 5}
	SrcData1 := []uint32{1, 2, 3, 4, 5}
	SrcData2 := []uint32{6, 7, 8, 9, 10}
	dst := make([]uint32, length)

	// Create simulation engine
	engine := sim.NewSerialEngine()

	// Create driver
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")

	// Create device
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		Build("Device")

	driver.RegisterDevice(device)

	// Load program
	program := core.LoadProgramFileFromYAML("./test_gpred.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(cmpSrcData1, cgra.West, [2]int{0, 1}, 1, "R")
	driver.FeedIn(cmpSrcData2, cgra.South, [2]int{0, 1}, 1, "R")
	driver.FeedIn(SrcData1, cgra.West, [2]int{1, 2}, 1, "R")
	driver.FeedIn(SrcData2, cgra.South, [2]int{1, 2}, 1, "R")
	driver.Collect(dst, cgra.East, [2]int{1, 2}, 1, "R")

	// Map program to all cores
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// Run simulation
	driver.Run()

	// Convert results and verify
	cmpSrcIData1 := make([]int32, length)
	cmpSrcIData2 := make([]int32, length)
	srcIData1 := make([]int32, length)
	srcIData2 := make([]int32, length)
	dstI := make([]int32, length)

	for i := 0; i < length; i++ {
		cmpSrcIData1[i] = *(*int32)(unsafe.Pointer(&cmpSrcData1[i]))
		cmpSrcIData2[i] = *(*int32)(unsafe.Pointer(&cmpSrcData2[i]))
		srcIData1[i] = *(*int32)(unsafe.Pointer(&SrcData1[i]))
		srcIData2[i] = *(*int32)(unsafe.Pointer(&SrcData2[i]))
	}

	expected := []int32{6, 7, 3, 4, 10}
	for i := 0; i < 5; i++ {
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	t.Log("=== Gpred Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		actual := dstI[i]
		if actual != expected[i] {
			t.Errorf("Index %d:, cmpSrc1=%d, cmpSrc2=%d, src1=%d, src2=%d, Expected=%d, Actual=%d",
				i, cmpSrcIData1[i], cmpSrcIData2[i], srcIData1[i], srcIData2[i], expected[i], actual)
			allPassed = false
		}
	}

	if allPassed {
		t.Log("✅ Gpred tests passed!")
	} else {
		t.Fatal("❌ Gpred tests failed!")
	}
}
