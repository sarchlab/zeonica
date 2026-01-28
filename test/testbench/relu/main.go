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

func Relu() {
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

	programPath := "test/testbench/relu/relu.yaml"

	// preload data

	data := []int32{1, -2, 3, -4, 5, -6, 7, -8, 9, -10, 11, -12, 13, 14, -15, 16, 17, 18, 19, 20, -21, 22, 23, 24, -25, 26, 27, 28, -29, 30, -31, 32} // length is 32

	for i := 0; i < len(data); i++ {
		driver.PreloadMemory(3, 2, uint32(data[i]), uint32(i))
	}

	program := core.LoadProgramFileFromYAML(programPath)

	fmt.Println("program:", program)

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

	// TODO: Add PreloadMemory calls if needed for relu test
	// driver.PreloadMemory(x, y, data, baseAddr)

	driver.Run()

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	// get memory values in (2,3) from 0x0-0x31
	for i := 0; i < 32; i++ {
		value := driver.ReadMemory(1, 3, uint32(i))
		fmt.Println("memory[", i, "]:", value)
	}
}

func main() {
	f, err := os.Create("relu.json.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))
	Relu()
}
