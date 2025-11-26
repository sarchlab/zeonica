package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func Fir() {
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

	program := core.LoadProgramFileFromYAML("test/testbench/fir/fir4x4.yaml")
	if len(program) == 0 {
		panic("Failed to load program")
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
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

	driver.PreloadMemory(3, 3, 3, 0)
	driver.PreloadMemory(3, 3, 1, 1)
	driver.PreloadMemory(2, 1, 2, 0)
	driver.PreloadMemory(2, 1, 4, 1) // addr has ERRORS !!!!!!

	driver.Run()

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	// get the returned value
	retVal := device.GetTile(0, 0).GetRetVal()
	fmt.Println("retVal:", retVal)

	if retVal == 12 {
		fmt.Println("✅ Fire tests passed!")
	} else {
		fmt.Println("❌ Fire tests failed!")
	}
}

func main() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))
	Fir()
}
