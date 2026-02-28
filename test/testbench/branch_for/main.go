package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

// BranchFor runs the branch_for testbench on the configured runtime.
func BranchFor(rt *runtimecfg.Runtime) int {
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
	mismatch := 0
	if retVal == expected {
		fmt.Printf("✅ branch_for test passed: retVal=%f expected=%f\n", retVal, expected)
	} else {
		fmt.Printf("❌ branch_for test failed: retVal=%f expected=%f\n", retVal, expected)
		mismatch = 1
	}
	return mismatch
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
	const testName = "branch_for"

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

	mismatch := BranchFor(rt)

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}

	passed := mismatch == 0
	if rt.Config.LoggingEnabled {
		reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
		if err != nil {
			panic(err)
		}
		fmt.Printf("report saved: %s\n", reportPath)
	} else {
		fmt.Println("logging disabled in arch spec, skipped report generation")
	}
}
