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

	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

const (
	testName = "stonne_gemm32x32"
	kDim     = 32
)

// Gemm runs the 32x32 stonne GEMM testbench on the configured runtime.
//
//nolint:gocyclo,funlen
func Gemm(rt *runtimecfg.Runtime) int {
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

	programPath, err := resolveProgramPath()
	if err != nil {
		panic(err)
	}
	program := core.LoadProgramFileFromYAML(programPath)
	if len(program) == 0 {
		panic(fmt.Sprintf("failed to load program from %s", programPath))
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	A := make([][]uint32, height)
	for i := 0; i < height; i++ {
		A[i] = make([]uint32, kDim)
		for k := 0; k < kDim; k++ {
			A[i][k] = uint32(rng.Intn(10))
		}
	}

	B := make([][]uint32, kDim)
	for k := 0; k < kDim; k++ {
		B[k] = make([]uint32, width)
		for j := 0; j < width; j++ {
			B[k][j] = uint32(rng.Intn(10))
		}
	}

	expected := computeGemm(A, B, height, width, kDim)

	// Feed A from WEST (rows) and B from NORTH (columns).
	aData := make([]uint32, 0, kDim*height)
	for k := 0; k < kDim; k++ {
		for row := 0; row < height; row++ {
			aData = append(aData, A[row][k])
		}
	}
	bData := make([]uint32, 0, kDim*width)
	for k := 0; k < kDim; k++ {
		for col := 0; col < width; col++ {
			bData = append(bData, B[k][col])
		}
	}

	driver.FeedIn(aData, cgra.West, [2]int{0, height}, height, "R")
	driver.FeedIn(bData, cgra.North, [2]int{0, width}, width, "R")

	// Drain boundary outputs to avoid edge DATA_MOV backpressure.
	feedRounds := len(aData) / height
	collectRounds := feedRounds + 128
	dummySouth := make([]uint32, collectRounds*width)
	dummyEast := make([]uint32, collectRounds*height)
	driver.Collect(dummySouth, cgra.South, [2]int{0, width}, width, "R")
	driver.Collect(dummyEast, cgra.East, [2]int{0, height}, height, "R")

	driver.Run()

	mismatch := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			val := driver.ReadMemory(x, y, 0)
			exp := expected[y][x]
			fmt.Printf("C[%d][%d] -> %d (expected %d)\n", x, y, val, exp)
			if val != exp {
				mismatch++
			}
		}
	}

	if mismatch == 0 {
		fmt.Println("✅ GEMM 32x32 output matches expected values")
	} else {
		fmt.Printf("❌ GEMM 32x32 mismatches: %d\n", mismatch)
	}
	return mismatch
}

func computeGemm(A, B [][]uint32, rows, cols, kdim int) [][]uint32 {
	C := make([][]uint32, rows)
	for i := 0; i < rows; i++ {
		C[i] = make([]uint32, cols)
		for j := 0; j < cols; j++ {
			var sum uint32
			for k := 0; k < kdim; k++ {
				sum += A[i][k] * B[k][j]
			}
			C[i][j] = sum
		}
	}
	return C
}

func resolveProgramPath() (string, error) {
	fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_PROGRAM_YAML"))
	if fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_PROGRAM_YAML points to a missing file: %s", fromEnv)
	}

	candidates := []string{
		"gemm.yaml",
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "gemm.yaml")),
		)
	}

	return firstExistingPath("program yaml", candidates)
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
		"arch_spec.yaml",
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "arch_spec", "arch_spec.yaml")),
		)
	}

	return firstExistingPath("arch spec", candidates)
}

func firstExistingPath(label string, candidates []string) (string, error) {
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

	mismatch := Gemm(rt)

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
