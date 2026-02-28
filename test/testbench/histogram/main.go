package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

// Histogram runs the histogram testbench on the configured runtime.
//
//nolint:gocyclo,funlen
func Histogram(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows
	seed := time.Now().UnixNano()
	if seedStr := os.Getenv("ZEONICA_RAND_SEED"); seedStr != "" {
		if parsed, err := strconv.ParseInt(seedStr, 10, 64); err == nil {
			seed = parsed
		}
	}
	rng := rand.New(rand.NewSource(seed))
	fmt.Printf("Using random seed: %d\n", seed)

	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine

	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		//programPath = "test/Zeonica_Testbench/kernel/histogram/histogram-instructions.yaml"
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

	// preload randomized input data (DATA_LEN=20, value range [1, 19])
	inputData := make([]uint32, 20)
	for i := range inputData {
		inputData[i] = uint32(rng.Intn(19) + 1)
	}
	expected := computeHistogram(inputData, 5)

	// histogram tile (2,1): initialize histogram[0..4] to 0
	for addr := 0; addr < 5; addr++ {
		driver.PreloadMemory(2, 1, 0, uint32(addr))
	}
	// data tile (3,2): input_data[0..19]
	for addr, val := range inputData {
		driver.PreloadMemory(3, 2, val, uint32(addr))
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
	outputTile := [2]int{2, 1}
	fmt.Printf("output memory @ tile (%d,%d):\n", outputTile[0], outputTile[1])
	scanLimit := 5
	outputData := make([]uint32, scanLimit)
	for addr := 0; addr < scanLimit; addr++ {
		val := driver.ReadMemory(outputTile[0], outputTile[1], uint32(addr))
		outputData[addr] = val
		fmt.Printf("  addr %d -> %d\n", addr, val)
	}

	fmt.Println("expected histogram (CPU):")
	histMismatch := 0
	for i, val := range expected {
		fmt.Printf("  addr %d -> %d\n", i, val)
		if i < len(outputData) && outputData[i] != val {
			histMismatch++
		}
	}
	if histMismatch == 0 {
		fmt.Println("✅ output matches expected histogram")
	} else {
		fmt.Printf("❌ output mismatches histogram: %d\n", histMismatch)
	}
	return histMismatch
}

func computeHistogram(input []uint32, bins int) []uint32 {
	// Match histogram_int.cpp semantics:
	// b = BUCKET_LEN * (input[i] - MIN) / (MAX - MIN)
	const minVal = 1
	const maxVal = 19
	const dataLen = 20
	delta := maxVal - minVal
	result := make([]uint32, bins)
	for i := 0; i < len(input) && i < dataLen; i++ {
		v := int(input[i])
		if v < minVal || v > maxVal {
			continue
		}
		b := bins * (v - minVal) / delta
		if b >= 0 && b < bins {
			result[b]++
		}
	}
	return result
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
	const testName = "histogram"

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

	mismatch := Histogram(rt)

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
