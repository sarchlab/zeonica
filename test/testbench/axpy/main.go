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

// Axpy runs the AXPY testbench on the configured runtime.
//
//nolint:gocyclo,funlen
func Axpy(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows
	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine

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

	// Mapping uses tile (3,2) for x load/MUL and tile (2,3) for y load.
	// Store happens at tile (3,3).
	xTile := [2]int{3, 2}
	yTile := [2]int{2, 3}

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
	outputTile := [2]int{3, 3}
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
	return mismatch
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
	const testName = "axpy"

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

	mismatch := Axpy(rt)

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
