package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestBranchForKernel(t *testing.T) {
	width := 4
	height := 4

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

	program := core.LoadProgramFileFromYAML("./tmp-generated-instructions.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load branch_for program")
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

	driver.Run()

	retBits := device.GetTile(1, 1).GetRetVal()
	retVal := math.Float32frombits(retBits)
	expected := cpuBranchFor()

	if retVal != expected {
		t.Fatalf("branch_for failed: retVal=%f expected=%f", retVal, expected)
	}
}
