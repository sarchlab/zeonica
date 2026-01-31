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

func Fir() (int32, int32) {
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

	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "tmp-generated-instructions.yaml"
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

	const NTAPS = 32
	input := make([]int32, NTAPS)
	coeff := []int32{
		0, 1, 3, -2, 0, 0, -3, 1,
		0, 1, 3, -2, 0, 0, -3, 1,
		0, 1, 3, -2, 0, 0, -3, 1,
		0, 1, 3, -2, 0, 0, -3, 1,
	}

	for i := 0; i < NTAPS; i++ {
		input[i] = int32(i + 1)
	}

	var expected int32
	for i := 0; i < NTAPS; i++ {
		expected += input[i] * coeff[i]
	}

	// From tmp-generated-instructions.yaml:
	// LOAD (arg0/input) on tile (2,1); LOAD (arg2/coeff) on tile (3,2).
	inputTile := [2]int{2, 1}
	coeffTile := [2]int{3, 2}

	for addr, val := range input {
		driver.PreloadMemory(inputTile[0], inputTile[1], uint32(val), uint32(addr))
	}
	for addr, val := range coeff {
		driver.PreloadMemory(coeffTile[0], coeffTile[1], uint32(val), uint32(addr))
	}

	driver.Run()

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	// RETURN_VALUE is on tile (1,2) in tmp-generated-instructions.yaml.
	retBits := device.GetTile(1, 2).GetRetVal()
	retVal := int32(retBits)
	fmt.Printf("retVal(bits=0x%08x) -> %d\n", retBits, retVal)

	if retVal == expected {
		fmt.Println("✅ Fir tests passed!")
	} else {
		fmt.Printf("❌ Fir tests failed! expected=%d\n", expected)
	}
	return retVal, expected
}

func main() {
	f, err := os.Create("fir.json.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))
	Fir()
}
