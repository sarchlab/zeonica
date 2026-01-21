package main

import (
	"fmt"
	"log/slog"
	"math"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func BranchFor() {
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
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

	driver.Run()

	// RETURN_VALUE is on tile (1,1) in tmp-generated-instructions.yaml
	retBits := device.GetTile(1, 1).GetRetVal()
	retVal := math.Float32frombits(retBits)
	fmt.Printf("retVal(bits=0x%08x) -> %f\n", retBits, retVal)

	expected := cpuBranchFor()
	if retVal == expected {
		fmt.Printf("✅ branch_for test passed: retVal=%f expected=%f\n", retVal, expected)
	} else {
		fmt.Printf("❌ branch_for test failed: retVal=%f expected=%f\n", retVal, expected)
	}
}

func cpuBranchFor() float32 {
	n := int64(10)
	i := int64(0)
	acc := float32(0.0)
	for {
		nextAcc := acc + 3.0
		iNext := i + 1
		if iNext < n {
			i = iNext
			acc = nextAcc
			continue
		}
		return nextAcc
	}
}

func main() {
	f, err := os.Create("branch_for.json.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})
	slog.SetDefault(slog.New(handler))

	BranchFor()
}
