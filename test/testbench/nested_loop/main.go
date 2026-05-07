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

// NestedLoop runs the nested loop testbench.
//
//nolint:gocyclo,funlen
func NestedLoop() {
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

	// Kernel: y[i] = sum_{j}(A[i*N+j]), N=4
	const N = 4
	A := make([]uint32, N*N)

	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			A[i*N+j] = uint32((i+1)*10 + (j + 1))
		}
	}
	expected := computeNestedLoop(N, A)

	fmt.Println("A matrix (row-major):")
	for i := 0; i < N; i++ {
		fmt.Printf("  row %d:", i)
		for j := 0; j < N; j++ {
			fmt.Printf(" %d", A[i*N+j])
		}
		fmt.Println()
	}

	// LOAD happens on tile (3,2); preload A there.
	inputTile := [2]int{3, 2}
	for addr, val := range A {
		driver.PreloadMemory(inputTile[0], inputTile[1], val, uint32(addr*4))
	}

	// STORE happens on tile (3,1); preload output buffer to 0.
	outputTile := [2]int{3, 1}
	for addr := 0; addr < N; addr++ {
		driver.PreloadMemory(outputTile[0], outputTile[1], 0, uint32(addr))
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

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	// print output memory data
	fmt.Printf("output memory @ tile (%d,%d):\n", outputTile[0], outputTile[1])
	outputData := make([]uint32, N)
	mismatch := 0
	for addr := 0; addr < N; addr++ {
		val := driver.ReadMemory(outputTile[0], outputTile[1], uint32(addr))
		outputData[addr] = val
		fmt.Printf("  addr %d -> %d\n", addr, val)
		if val != expected[addr] {
			mismatch++
		}
	}

	fmt.Println("expected nested_loop (CPU):")
	for i, val := range expected {
		fmt.Printf("  addr %d -> %d\n", i, val)
	}
	if mismatch == 0 {
		fmt.Println("✅ output matches expected nested_loop")
	} else {
		fmt.Printf("❌ output mismatches nested_loop: %d\n", mismatch)
	}
}

func computeNestedLoop(n int, A []uint32) []uint32 {
	result := make([]uint32, n)
	for i := 0; i < n; i++ {
		var sum uint32
		for j := 0; j < n; j++ {
			sum += A[i*n+j]
		}
		result[i] = sum
	}
	return result
}

func main() {
	f, err := os.Create("nested_loop.json.log")
	if err != nil {
		panic(err)
	}
	defer func() { _ = f.Close() }()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})
	slog.SetDefault(slog.New(handler))

	NestedLoop()
}
