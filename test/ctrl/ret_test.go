package main

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestRetOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2

	// Generate random test data
	src := make([]uint32, 1)
	src[0] = 114514

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
	program := core.LoadProgramFileFromYAML("./test_ret.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")

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

	retVal := device.GetTile(0, 0).GetRetVal()

	if retVal == 114514 {
		fmt.Println("retVal:", retVal)
		t.Log("✅ Ret tests passed!")
	} else {
		fmt.Println("retVal:", retVal)
		t.Fatal("❌ Ret tests failed!")
	}
}

func TestFireOperation(t *testing.T) {
	// Set test parameters
	width := 2
	height := 2

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
	program := core.LoadProgramFileFromYAML("./test_fire.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// fire all the cores in the beginning
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			// convert to tileCore
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

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

	retVal := device.GetTile(0, 0).GetRetVal()

	if retVal == 1 {
		fmt.Println("retVal:", retVal)
		t.Log("✅ Fire tests passed!")
	} else {
		fmt.Println("retVal:", retVal)
		t.Fatal("❌ Fire tests failed!")
	}
}
