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

func computeAxpy(a uint32, x []uint32, y []uint32) []uint32 {
	n := len(y)
	if len(x) < n {
		n = len(x)
	}
	out := make([]uint32, n)
	for i := 0; i < n; i++ {
		out[i] = a*x[i] + y[i]
	}
	return out
}

func Axpy() {
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

	const (
		N = 16
		A = 3
	)

	xData := make([]uint32, N)
	yData := make([]uint32, N)
	for i := 0; i < N; i++ {
		xData[i] = uint32(i + 1)
		yData[i] = uint32(100 + i)
	}
	expected := computeAxpy(A, xData, yData)

	// Mapping uses tile (2,1) for the multiplicand load and store.
	// To match y = a*x + y, preload x at (2,1) and y at (2,0).
	xTile := [2]int{2, 1}
	yTile := [2]int{2, 0}

	for addr, val := range xData {
		driver.PreloadMemory(xTile[0], xTile[1], val, uint32(addr))
	}
	for addr, val := range yData {
		driver.PreloadMemory(yTile[0], yTile[1], val, uint32(addr))
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

	driver.Run()

	fmt.Println("========================")
	fmt.Println("output memory:")
	outputTile := [2]int{2, 1}
	outputData := make([]uint32, N)
	for addr := 0; addr < N; addr++ {
		val := driver.ReadMemory(outputTile[0], outputTile[1], uint32(addr))
		outputData[addr] = val
		fmt.Printf("  addr %d -> %d\n", addr, val)
	}

	fmt.Println("expected (CPU):")
	mismatch := 0
	for i, val := range expected {
		fmt.Printf("  addr %d -> %d\n", i, val)
		if i < len(outputData) && outputData[i] != val {
			mismatch++
		}
	}
	if mismatch == 0 {
		fmt.Println("✅ output matches expected axpy")
	} else {
		fmt.Printf("❌ output mismatches axpy: %d\n", mismatch)
	}
}

func main() {
	f, err := os.Create("axpy.json.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))
	Axpy()
}
