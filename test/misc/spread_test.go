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

func TestSpreadOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 10

	// Create test data
	src := make([]uint32, length)
	dst1 := make([]uint32, length)
	dst2 := make([]uint32, length)
	dst3 := make([]uint32, length)
	dst4 := make([]uint32, length)

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
	program := core.LoadProgramFileFromYAML("./test_spread.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst1, cgra.West, [2]int{1, 2}, 1, "R")
	driver.Collect(dst2, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Collect(dst3, cgra.East, [2]int{1, 2}, 1, "R")
	driver.Collect(dst4, cgra.West, [2]int{0, 1}, 1, "R")

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
	srcI := make([]int32, length)
	dstI1 := make([]int32, length)
	dstI2 := make([]int32, length)
	dstI3 := make([]int32, length)
	dstI4 := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI1[i] = *(*int32)(unsafe.Pointer(&dst1[i]))
		dstI2[i] = *(*int32)(unsafe.Pointer(&dst2[i]))
		dstI3[i] = *(*int32)(unsafe.Pointer(&dst3[i]))
		dstI4[i] = *(*int32)(unsafe.Pointer(&dst4[i]))
	}

	expected := []int32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}
	// Verify results: output should be input+2
	t.Log("=== Spread Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		actual1 := dstI1[i]
		actual2 := dstI2[i]
		actual3 := dstI3[i]
		actual4 := dstI4[i]
		if actual1 != expected[i] || actual2 != expected[i] || actual3 != expected[i] || actual4 != expected[i] {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], actual1)
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], actual2)
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], actual3)
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], actual4)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual1)
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual2)
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual3)
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual4)
		}
	}

	if allPassed {
		t.Log("✅ Spread tests passed!")
	} else {
		t.Fatal("❌ Spread tests failed!")
	}
}
