package main

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestAddOperationWithRandomData(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 16

	// Create test data
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// Generate random test data
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFileFromYAML("./test_add.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, height}, height, "R")

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
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// Verify results: output should be input+2
	t.Log("=== ADD Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] + 2
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ ADD tests passed!")
	} else {
		t.Fatal("❌ ADD tests failed!")
	}
}

func TestSubOperationWithRandomData(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 16

	// Create test data
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// Generate random test data
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFileFromYAML("./test_sub.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from north, output to south
	driver.FeedIn(src, cgra.South, [2]int{0, width}, width, "R")
	driver.Collect(dst, cgra.North, [2]int{0, width}, width, "R")

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
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// Verify results: output should be input-2
	t.Log("=== SUB Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] - 2
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ SUB tests passed!")
	} else {
		t.Fatal("❌ SUB tests failed!")
	}
}

func TestMulOperationWithRandomData(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 16

	// Create test data
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// Generate random test data
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFileFromYAML("./test_mul.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from east, output to west
	driver.FeedIn(src, cgra.East, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.West, [2]int{0, height}, height, "R")

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
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// Verify results: output should be input*2
	t.Log("=== MUL Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] * 4
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ MUL tests passed!")
	} else {
		t.Fatal("❌ MUL tests failed!")
	}
}

func TestDivOperationWithRandomData(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2
	length := 16

	// Create test data
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// Generate random test data (avoid division by zero)
	rand.Seed(time.Now().UnixNano())
	minI := int32(-20)
	maxI := int32(20)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		// Ensure data is a multiple of 4 to avoid division precision issues
		if INum%4 != 0 {
			INum = INum - INum%4 + 4
		}
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFileFromYAML("./test_div.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from south, output to north
	driver.FeedIn(src, cgra.North, [2]int{0, width}, width, "R")
	driver.Collect(dst, cgra.South, [2]int{0, width}, width, "R")

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
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// Verify results: output should be input/2
	t.Log("=== DIV Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] / 4
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ DIV tests passed!")
	} else {
		t.Fatal("❌ DIV tests failed!")
	}
}
