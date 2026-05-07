package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

const testName = "buffer_depth_4x4"

func runBufferDepth4x4(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows
	if width != 4 || height != 4 {
		panic(fmt.Sprintf("this testbench requires 4x4, got %dx%d", width, height))
	}

	rounds := 256
	if roundsStr := strings.TrimSpace(os.Getenv("ZEONICA_BD_ROUNDS")); roundsStr != "" {
		if parsed, err := strconv.Atoi(roundsStr); err == nil && parsed > 0 {
			rounds = parsed
		}
	}

	programPath, err := resolveProgramPath()
	if err != nil {
		panic(err)
	}
	program := core.LoadProgramFileFromYAML(programPath)
	if len(program) == 0 {
		panic(fmt.Sprintf("failed to load program from %s", programPath))
	}

	driver := rt.Driver
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// Feed one token per west port every round.
	feedData := make([]uint32, 0, rounds*height)
	expectedByRow := make([][]uint32, height)
	for row := 0; row < height; row++ {
		expectedByRow[row] = make([]uint32, rounds)
	}
	for r := 0; r < rounds; r++ {
		for row := 0; row < height; row++ {
			// Unique per (round,row) to validate ordering and completeness.
			value := uint32(r*64 + row)
			feedData = append(feedData, value)
			expectedByRow[row][r] = value
		}
	}
	driver.FeedIn(feedData, cgra.West, [2]int{0, height}, height, "R")

	driver.Run()

	mismatch := 0
	// Sink tiles are at x=3 and STORE always targets address 0.
	// If all rounds are consumed, memory[0] should equal the last fed token.
	lastRound := rounds - 1
	for row := 0; row < height; row++ {
		got := driver.ReadMemory(3, row, 0)
		want := expectedByRow[row][lastRound]
		if got != want {
			fmt.Printf("mismatch row=%d addr=0 got=%d want=%d\n", row, got, want)
			mismatch++
		}
	}

	fmt.Printf("rounds=%d width=%d height=%d mismatches=%d\n", rounds, width, height, mismatch)
	return mismatch
}

func resolveProgramPath() (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_PROGRAM_YAML")); fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_PROGRAM_YAML points to a missing file: %s", fromEnv)
	}

	candidates := []string{"buffer_depth_4x4.yaml"}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), "buffer_depth_4x4.yaml"))
	}
	return firstExistingPath("program yaml", candidates)
}

func resolveArchSpecPath() (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_ARCH_SPEC")); fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_ARCH_SPEC points to a missing file: %s", fromEnv)
	}

	candidates := []string{"arch_spec.yaml"}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), "arch_spec.yaml"))
	}
	return firstExistingPath("arch spec", candidates)
}

func firstExistingPath(label string, candidates []string) (string, error) {
	seen := make(map[string]struct{}, len(candidates))
	normalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
		if _, err := os.Stat(clean); err == nil {
			return clean, nil
		}
	}
	return "", fmt.Errorf("cannot locate %s, tried: %s", label, strings.Join(normalized, ", "))
}

func main() {
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

	mismatch := runBufferDepth4x4(rt)

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}

	passed := mismatch == 0
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("report saved: %s\n", reportPath)
	if mismatch != 0 {
		os.Exit(1)
	}
}
