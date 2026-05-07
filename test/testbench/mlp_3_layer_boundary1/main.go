package main

import (
	"encoding/json"
	"fmt"
	"math"
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
	testName      = "mlp_3_layer_boundary1"
	fabricRows    = 16
	fabricColumns = 16
)

type colRange struct {
	start int
	end   int // exclusive
}

type stageConfig struct {
	name       string
	program    string
	gemmRanges []colRange
	reluRanges []colRange
}

type stageRunStats struct {
	Name             string `json:"name"`
	Program          string `json:"program"`
	DeltaCycles      int64  `json:"deltaCycles"`
	BestOffset       int    `json:"bestOffset"`
	StageMismatch    int    `json:"stageMismatch"`
	StageReportPath  string `json:"stageReportPath"`
	StageReportCycle int64  `json:"stageReportCycles"`
}

type stagedRunSummary struct {
	Seed             int64           `json:"seed"`
	Rounds           int             `json:"rounds"`
	WeightMin        int             `json:"weightMin"`
	WeightMax        int             `json:"weightMax"`
	Stages           []stageRunStats `json:"stages"`
	StageCyclesTotal int64           `json:"stageCyclesTotal"`
	EngineEndCycle   int64           `json:"engineEndCycle"`
	FinalMismatch    int             `json:"finalMismatch"`
	TotalMismatch    int             `json:"totalMismatch"`
}

type unifiedStatsReport struct {
	TestName      string           `json:"testName"`
	TotalCycles   int64            `json:"totalCycles"`
	Passed        bool             `json:"passed"`
	MismatchCount int              `json:"mismatchCount"`
	Staged        stagedRunSummary `json:"staged"`
}

func mapStageProgram(
	driver interface {
		MapProgram(program interface{}, core [2]int)
	},
	program map[string]core.Program,
	width, height int,
) {
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}
}

//nolint:gocyclo,funlen
func runBoundary1MLP(archSpecPath string) (int, stagedRunSummary) {
	summary := stagedRunSummary{}

	rounds := 64
	if roundsStr := strings.TrimSpace(os.Getenv("ZEONICA_MLP_ROUNDS")); roundsStr != "" {
		if parsed, err := strconv.Atoi(roundsStr); err == nil && parsed > 0 {
			rounds = parsed
		}
	}

	seed := time.Now().UnixNano()
	if seedStr := strings.TrimSpace(os.Getenv("ZEONICA_RAND_SEED")); seedStr != "" {
		if parsed, err := strconv.ParseInt(seedStr, 10, 64); err == nil {
			seed = parsed
		}
	}

	weightMin := 0
	if value := strings.TrimSpace(os.Getenv("ZEONICA_MLP_WEIGHT_MIN")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			weightMin = parsed
		}
	}
	weightMax := 3
	if value := strings.TrimSpace(os.Getenv("ZEONICA_MLP_WEIGHT_MAX")); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			weightMax = parsed
		}
	}
	if weightMax < weightMin {
		panic(fmt.Sprintf("invalid north weight range: min=%d max=%d (expect max >= min)", weightMin, weightMax))
	}

	rng := rand.New(rand.NewSource(seed))
	summary.Seed = seed
	summary.Rounds = rounds
	summary.WeightMin = weightMin
	summary.WeightMax = weightMax
	fmt.Printf("Using random seed: %d\n", seed)
	fmt.Printf("MLP rounds: %d\n", rounds)
	fmt.Printf("MLP north weight range: [%d, %d]\n", weightMin, weightMax)

	width := fabricColumns
	height := fabricRows
	inputPerRow := make([][]int32, height)
	for row := 0; row < height; row++ {
		inputPerRow[row] = make([]int32, rounds)
		for t := 0; t < rounds; t++ {
			inputPerRow[row][t] = int32(rng.Intn(16))
		}
	}

	northPerCol := make([][]int32, width)
	for col := 0; col < width; col++ {
		northPerCol[col] = make([]int32, rounds)
		w := int32(weightMin + rng.Intn(weightMax-weightMin+1))
		for t := 0; t < rounds; t++ {
			northPerCol[col][t] = w
		}
	}

	stages := []stageConfig{
		{
			name:    "stage12",
			program: "stage12_gemm_relu_gemm_relu.yaml",
			gemmRanges: []colRange{
				{start: 0, end: 4},
				{start: 6, end: 10},
			},
			reluRanges: []colRange{
				{start: 4, end: 6},
				{start: 10, end: 12},
			},
		},
		{
			name:    "stage3",
			program: "stage3_gemm.yaml",
			gemmRanges: []colRange{
				{start: 12, end: 16},
			},
			reluRanges: nil,
		},
	}

	cpuStageExpected := make([][][]int32, 0, len(stages))
	currCPU := clone2D(inputPerRow)
	for _, st := range stages {
		currCPU = applyStageCPU(currCPU, northPerCol, st, rounds, height)
		cpuStageExpected = append(cpuStageExpected, currCPU)
	}
	cpuFullExpected := computeExpectedFullMLP3Layer(inputPerRow, northPerCol, width, height, rounds)

	currInput := clone2D(inputPerRow)
	totalMismatch := 0
	collectRounds := rounds + width + height + 32

	for idx, st := range stages {
		rt, err := runtimecfg.LoadRuntime(archSpecPath, fmt.Sprintf("%s_%s", testName, st.name))
		if err != nil {
			panic(err)
		}
		if rt.Config.Columns != fabricColumns || rt.Config.Rows != fabricRows {
			panic(fmt.Sprintf(
				"%s requires %dx%d fabric, got %dx%d",
				st.name,
				fabricColumns,
				fabricRows,
				rt.Config.Columns,
				rt.Config.Rows,
			))
		}
		traceLog, err := rt.InitTraceLogger(core.LevelTrace)
		if err != nil {
			panic(err)
		}

		programPath, err := resolveProgramPath(st.program)
		if err != nil {
			panic(err)
		}
		program := core.LoadProgramFileFromYAML(programPath)
		if len(program) == 0 {
			panic(fmt.Sprintf("failed to load stage program from %s", programPath))
		}
		mapStageProgram(rt.Driver, program, rt.Config.Columns, rt.Config.Rows)
		fmt.Printf("mapped stage: %s\n", st.name)

		westData := flattenByRound(currInput, rounds, height)
		northByRange := make([][]uint32, len(st.gemmRanges))
		for idxRange, gr := range st.gemmRanges {
			stream := make([]uint32, 0, rounds*(gr.end-gr.start))
			for t := 0; t < rounds; t++ {
				for col := gr.start; col < gr.end; col++ {
					stream = append(stream, uint32(northPerCol[col][t]))
				}
			}
			northByRange[idxRange] = stream
		}

		southData := make([]uint32, collectRounds*width)
		eastData := make([]uint32, collectRounds*height)

		driver := rt.Driver
		driver.FeedIn(westData, cgra.West, [2]int{0, height}, height, "R")
		if len(st.gemmRanges) > 0 {
			for idxRange, gr := range st.gemmRanges {
				driver.FeedIn(northByRange[idxRange], cgra.North, [2]int{gr.start, gr.end}, gr.end-gr.start, "R")
			}
		}
		driver.Collect(southData, cgra.South, [2]int{0, width}, width, "R")
		driver.Collect(eastData, cgra.East, [2]int{0, height}, height, "R")
		driver.Run()

		observedByRow := make([][]int32, height)
		for row := 0; row < height; row++ {
			observedByRow[row] = make([]int32, collectRounds)
			for r := 0; r < collectRounds; r++ {
				observedByRow[row][r] = int32(eastData[r*height+row])
			}
		}

		offset, rowMismatch, stageMismatch := bestGlobalOffset(observedByRow, cpuStageExpected[idx], rounds, collectRounds, height)
		for row := 0; row < height; row++ {
			fmt.Printf("%s row %d best offset=%d mismatches=%d/%d\n", st.name, row, offset, rowMismatch[row], rounds)
		}
		fmt.Printf("%s total mismatches=%d\n", st.name, stageMismatch)
		totalMismatch += stageMismatch

		nextInput := make([][]int32, height)
		for row := 0; row < height; row++ {
			nextInput[row] = make([]int32, rounds)
			copy(nextInput[row], observedByRow[row][offset:offset+rounds])
		}
		currInput = nextInput

		if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
			panic(err)
		}

		passed := stageMismatch == 0
		reportResult, reportPath, err := rt.GenerateAndSaveReport(5, &passed, &stageMismatch)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s report saved: %s (cycles=%d)\n", st.name, reportPath, reportResult.TotalCycles)
		summary.Stages = append(summary.Stages, stageRunStats{
			Name:             st.name,
			Program:          st.program,
			DeltaCycles:      reportResult.TotalCycles,
			BestOffset:       offset,
			StageMismatch:    stageMismatch,
			StageReportPath:  reportPath,
			StageReportCycle: reportResult.TotalCycles,
		})
		summary.StageCyclesTotal += reportResult.TotalCycles
	}

	finalMismatch := 0
	for row := 0; row < height; row++ {
		for t := 0; t < rounds; t++ {
			if currInput[row][t] != cpuFullExpected[row][t] {
				finalMismatch++
			}
		}
	}
	fmt.Printf("final output mismatches against full CPU=%d\n", finalMismatch)
	totalMismatch += finalMismatch
	summary.FinalMismatch = finalMismatch
	summary.TotalMismatch = totalMismatch
	summary.EngineEndCycle = summary.StageCyclesTotal

	if totalMismatch == 0 {
		fmt.Println("Boundary1 MLP output stream matches CPU reference")
	} else {
		fmt.Printf("Boundary1 MLP mismatches: %d\n", totalMismatch)
	}
	return totalMismatch, summary
}

func countColumns(ranges []colRange) int {
	total := 0
	for _, r := range ranges {
		total += r.end - r.start
	}
	return total
}

func inRanges(x int, ranges []colRange) bool {
	for _, r := range ranges {
		if x >= r.start && x < r.end {
			return true
		}
	}
	return false
}

func applyStageCPU(input [][]int32, northPerCol [][]int32, st stageConfig, rounds, height int) [][]int32 {
	out := make([][]int32, height)
	for row := 0; row < height; row++ {
		out[row] = make([]int32, rounds)
		for t := 0; t < rounds; t++ {
			v := input[row][t]
			for col := 0; col < fabricColumns; col++ {
				switch {
				case inRanges(col, st.gemmRanges):
					v = v * northPerCol[col][t]
				case inRanges(col, st.reluRanges):
					if v < 0 {
						v = 0
					}
				default:
					// relay: v unchanged
				}
			}
			out[row][t] = v
		}
	}
	return out
}

func computeExpectedFullMLP3Layer(inputPerRow [][]int32, northPerCol [][]int32, width, height, rounds int) [][]int32 {
	out := make([][]int32, height)
	for row := 0; row < height; row++ {
		out[row] = make([]int32, rounds)
		for t := 0; t < rounds; t++ {
			v := inputPerRow[row][t]
			for col := 0; col < width; col++ {
				switch col {
				case 4, 5, 10, 11:
					if v < 0 {
						v = 0
					}
				default:
					v = v * northPerCol[col][t]
				}
			}
			out[row][t] = v
		}
	}
	return out
}

func flattenByRound(data [][]int32, rounds, height int) []uint32 {
	out := make([]uint32, 0, rounds*height)
	for t := 0; t < rounds; t++ {
		for row := 0; row < height; row++ {
			out = append(out, uint32(data[row][t]))
		}
	}
	return out
}

func bestGlobalOffset(observed [][]int32, expected [][]int32, rounds, collectRounds, height int) (int, []int, int) {
	if rounds == 0 {
		return 0, make([]int, height), 0
	}
	lastStart := collectRounds - rounds
	if lastStart < 0 {
		lastStart = 0
	}

	bestOffset := 0
	bestMismatch := math.MaxInt
	bestByRow := make([]int, height)

	for start := 0; start <= lastStart; start++ {
		mismatchByRow := make([]int, height)
		total := 0
		for row := 0; row < height; row++ {
			m := 0
			for i := 0; i < rounds; i++ {
				if observed[row][start+i] != expected[row][i] {
					m++
				}
			}
			mismatchByRow[row] = m
			total += m
		}
		if total < bestMismatch {
			bestMismatch = total
			bestOffset = start
			copy(bestByRow, mismatchByRow)
			if total == 0 {
				break
			}
		}
	}

	return bestOffset, bestByRow, bestMismatch
}

func clone2D(in [][]int32) [][]int32 {
	out := make([][]int32, len(in))
	for i := range in {
		out[i] = make([]int32, len(in[i]))
		copy(out[i], in[i])
	}
	return out
}

func resolveProgramPath(fileName string) (string, error) {
	if fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_PROGRAM_DIR")); fromEnv != "" {
		candidate := filepath.Join(fromEnv, fileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	candidates := []string{fileName}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(thisFile), fileName)))
	}
	return firstExistingPath("program kernel", candidates)
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
		candidates = append(candidates, filepath.Clean(filepath.Join(filepath.Dir(thisFile), "arch_spec.yaml")))
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
	outputDir := filepath.Dir(archSpecPath)
	if err := os.Chdir(outputDir); err != nil {
		panic(err)
	}

	mismatch, summary := runBoundary1MLP(archSpecPath)
	passed := mismatch == 0
	unified := unifiedStatsReport{
		TestName:      testName,
		TotalCycles:   summary.StageCyclesTotal,
		Passed:        passed,
		MismatchCount: mismatch,
		Staged:        summary,
	}
	unifiedBytes, err := json.MarshalIndent(unified, "", "  ")
	if err != nil {
		panic(err)
	}
	reportPath := filepath.Join(outputDir, fmt.Sprintf("%s.report.json", testName))
	if err := os.WriteFile(reportPath, append(unifiedBytes, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("report saved: %s\n", reportPath)

	unifiedPath := filepath.Join(outputDir, fmt.Sprintf("%s.unified.report.json", testName))
	if err := os.WriteFile(unifiedPath, append(unifiedBytes, '\n'), 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("unified boundary1 stats saved: %s\n", unifiedPath)
	fmt.Printf("total cycles (sum of stage reports): %d\n", summary.StageCyclesTotal)

	if mismatch != 0 {
		os.Exit(1)
	}
}
