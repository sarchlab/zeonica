package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

// Relu runs the ReLU testbench on the configured runtime.
//
//nolint:gocyclo
func Relu(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows

	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine

	programPath := strings.TrimSpace(os.Getenv("ZEONICA_PROGRAM_YAML"))
	if programPath == "" {
		if _, err := os.Stat("relu.yaml"); err == nil {
			programPath = "relu.yaml"
		} else {
			programPath = "relu/relu.yaml"
		}
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

	// preload input data at tile (3,2): 32 int32 values
	inputData := []int32{1, -2, 3, -4, 5, -6, 7, -8, 9, -10, 11, -12, 13, 14, -15, 16, 17, 18, 19, 20, -21, 22, 23, 24, -25, 26, 27, 28, -29, 30, -31, 32}
	for i := 0; i < len(inputData); i++ {
		driver.PreloadMemory(3, 2, uint32(inputData[i]), uint32(i))
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

	// output tile (1,3), 32 elements
	outputTile := [2]int{1, 3}
	scanLimit := 32
	fmt.Printf("output memory @ tile (%d,%d):\n", outputTile[0], outputTile[1])
	outputData := make([]uint32, scanLimit)
	for addr := 0; addr < scanLimit; addr++ {
		val := driver.ReadMemory(outputTile[0], outputTile[1], uint32(addr))
		outputData[addr] = val
		fmt.Printf("  addr %d -> %d\n", addr, val)
	}

	expected := computeReLU(inputData)
	fmt.Println("expected ReLU (CPU):")
	reluMismatch := 0
	for i, val := range expected {
		fmt.Printf("  addr %d -> %d\n", i, val)
		if i < len(outputData) && outputData[i] != val {
			reluMismatch++
		}
	}
	if reluMismatch == 0 {
		fmt.Println("✅ output matches expected ReLU")
	} else {
		fmt.Printf("❌ output mismatches ReLU: %d\n", reluMismatch)
	}
	return reluMismatch
}

func computeReLU(input []int32) []uint32 {
	out := make([]uint32, len(input))
	for i, v := range input {
		if v > 0 {
			out[i] = uint32(v)
		} else {
			out[i] = 0
		}
	}
	return out
}

func resolveArchSpecPath() (string, error) {
	fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_ARCH_SPEC"))
	if fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_ARCH_SPEC points to a missing file: %s", fromEnv)
	}

	candidates := []string{
		"test/arch_spec/arch_spec.yaml",
		"../../arch_spec/arch_spec.yaml",
	}

	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "arch_spec", "arch_spec.yaml")),
		)
	}

	seen := make(map[string]struct{}, len(candidates))
	normalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
		if _, err := os.Stat(clean); err == nil {
			return clean, nil
		}
	}

	return "", fmt.Errorf("cannot locate arch spec, tried: %s", strings.Join(normalized, ", "))
}

func main() {
	const testName = "relu"

	archSpecPath, err := resolveArchSpecPath()
	if err != nil {
		panic(err)
	}

	rt, err := runtimecfg.LoadRuntime(archSpecPath, testName)
	if err != nil {
		panic(err)
	}

	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		panic(err)
	}

	mismatch := Relu(rt)

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}

	passed := mismatch == 0
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("report saved: %s\n", reportPath)
}
