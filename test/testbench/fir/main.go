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

// Fir runs the FIR testbench on the configured runtime.
//
//nolint:gocyclo,funlen
func Fir(rt *runtimecfg.Runtime) int {
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

	mismatchCount := 0
	if retVal == expected {
		fmt.Println("✅ Fir tests passed!")
	} else {
		fmt.Printf("❌ Fir tests failed! expected=%d\n", expected)
		mismatchCount = 1
	}
	return mismatchCount
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
	const testName = "fir"

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

	mismatchCount := Fir(rt)

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}

	passed := mismatchCount == 0
	if rt.Config.LoggingEnabled {
		reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatchCount)
		if err != nil {
			panic(err)
		}
		fmt.Printf("report saved: %s\n", reportPath)
	} else {
		fmt.Println("logging disabled in arch spec, skipped report generation")
	}
}
