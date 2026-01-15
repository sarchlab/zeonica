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

func TestCmpExOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 5

	// Create test data
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// Generate random test data
	src = []uint32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}

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
	program := core.LoadProgramFileFromYAML("./test_cmpex.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")

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
	srcI := make([]int32, length*2)
	dstI := make([]int32, length*2)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	expected := []int32{0, 1, 1, 0, 0}
	// Verify results: output should be input+2
	t.Log("=== CmpEx Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		actual := dstI[i]

		if actual != expected[i] {
			t.Errorf("Index %d: Input=%d, %d, Expected=%d, Actual=%d",
				i, srcI[2*i], srcI[2*i+1], expected[i], actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, %d, Output=%d ✓", i, srcI[2*i], srcI[2*i+1], actual)
		}
	}

	if allPassed {
		t.Log("✅ CmpEx tests passed!")
	} else {
		t.Fatal("❌ CmpEx tests failed!")
	}
}

/*
func TestPhiOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 5

	// Generate random test data
	srcData := []uint32{5, 5, 5, 5, 5}
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
	program := core.LoadProgramFileFromYAML("./test_phiconst.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(srcData, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.West, [2]int{1, 2}, 1, "R")

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
	srcIData := make([]int32, length)
	dstI := make([]int32, length)

	for i := 0; i < length; i++ {
		srcIData[i] = *(*int32)(unsafe.Pointer(&srcData[i]))
	}

	expected := []int32{1, 5, 5, 5, 5}
	for i := 0; i < 5; i++ {
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	t.Log("=== Phi-Const Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		actual := dstI[i]
		if actual != expected[i] {
			t.Errorf("Index %d:, Expected=%d, Actual=%d",
				i, expected[i], actual)
			allPassed = false
		}
	}

	if allPassed {
		t.Log("✅ Phi-Const tests passed!")
	} else {
		t.Fatal("❌ Gpred tests failed!")
	}
} */
