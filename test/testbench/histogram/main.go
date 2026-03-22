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
	"gopkg.in/yaml.v3"
)

// YAML shapes mirror core/program.go (LoadProgramFileFromYAML) for patching only.

type histogramYAMLRoot struct {
	ArrayConfig histogramArrayConfig `yaml:"array_config"`
}

type histogramArrayConfig struct {
	Rows       int                    `yaml:"rows"`
	Cols       int                    `yaml:"columns"`
	CompiledII int                    `yaml:"compiled_ii"`
	Cores      []histogramYAMLCore    `yaml:"cores"`
}

type histogramYAMLCore struct {
	Row     int               `yaml:"row"`
	Column  int               `yaml:"column"`
	CoreID  string            `yaml:"core_id"`
	Entries []histogramYAMLEntry `yaml:"entries"`
}

type histogramYAMLEntry struct {
	EntryID           string                        `yaml:"entry_id"`
	Type              string                        `yaml:"type"`
	InstructionGroups []histogramYAMLInstGroup      `yaml:"instructions"`
}

type histogramYAMLInstGroup struct {
	Operations []histogramYAMLOperation `yaml:"operations"`
	IndexPerII int                      `yaml:"index_per_ii"`
}

type histogramYAMLOperation struct {
	OpCode            string               `yaml:"opcode"`
	SrcOperands       []histogramYAMLOperand `yaml:"src_operands"`
	DstOperands       []histogramYAMLOperand `yaml:"dst_operands"`
	ID                int                  `yaml:"id"`
	InvalidIterations int                  `yaml:"invalid_iterations"`
	TimeStep          int                  `yaml:"time_step"`
}

type histogramYAMLOperand struct {
	Operand string `yaml:"operand"`
	Color   string `yaml:"color"`
}

// gepArgReplacements maps LLVM-style kernel parameters to immediates for GEP.
//
// Matches histogram_int.cpp:
//
//	void kernel(int input[], int histogram[])
//
// arg0 — base of input[] (input_data); testbench preloads at tile (3,2) starting offset 0.
// arg1 — base of histogram[]; preloads at tile (2,1) starting offset 0.
//
// Override with ZEONICA_GEP_ARG0 / ZEONICA_GEP_ARG1 (e.g. "0" or "#0").
func gepArgReplacements() map[string]string {
	m := make(map[string]string)
	if v := strings.TrimSpace(os.Getenv("ZEONICA_GEP_ARG0")); v != "" {
		m["arg0"] = normalizeImmediateYAMLOperand(v)
	} else {
		m["arg0"] = "#0"
	}
	if v := strings.TrimSpace(os.Getenv("ZEONICA_GEP_ARG1")); v != "" {
		m["arg1"] = normalizeImmediateYAMLOperand(v)
	} else {
		m["arg1"] = "#0"
	}
	return m
}

func normalizeImmediateYAMLOperand(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		return s
	}
	return "#" + s
}

// patchGEPArgOperands replaces arg0/arg1 in GEP source operands with immediates.
// Zeonica's readOperand only accepts $reg, ports, or numeric immediates — not symbolic args.
func patchGEPArgOperands(root *histogramYAMLRoot, repl map[string]string) bool {
	changed := false
	for ci := range root.ArrayConfig.Cores {
		core := &root.ArrayConfig.Cores[ci]
		for ei := range core.Entries {
			entry := &core.Entries[ei]
			for gi := range entry.InstructionGroups {
				group := &entry.InstructionGroups[gi]
				for oi := range group.Operations {
					op := &group.Operations[oi]
					if op.OpCode != "GEP" {
						continue
					}
					for si := range op.SrcOperands {
						src := &op.SrcOperands[si]
						if newOp, ok := repl[src.Operand]; ok {
							src.Operand = newOp
							changed = true
						}
					}
				}
			}
		}
	}
	return changed
}

// resolveProgramYAMLWithGEPArgs reads compiler-generated YAML, patches GEP arg operands, and
// returns a path suitable for core.LoadProgramFileFromYAML. If nothing changed, returns the
// original path and a no-op cleanup.
func resolveProgramYAMLWithGEPArgs(programPath string) (resolved string, cleanup func()) {
	data, err := os.ReadFile(programPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to read program file %q: %v", programPath, err))
	}

	var root histogramYAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		panic(fmt.Sprintf("Failed to parse YAML %q: %v", programPath, err))
	}

	repl := gepArgReplacements()
	if !patchGEPArgOperands(&root, repl) {
		return programPath, func() {}
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		panic(err)
	}

	tmp, err := os.CreateTemp("", "zeonica-histogram-patched-*.yaml")
	if err != nil {
		panic(err)
	}
	path := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		panic(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		panic(err)
	}

	return path, func() { _ = os.Remove(path) }
}

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
		programPath = "tmp-generated-instructions.yaml"
	}
	resolvedPath, cleanupYAML := resolveProgramYAMLWithGEPArgs(programPath)
	defer cleanupYAML()

	program := core.LoadProgramFileFromYAML(resolvedPath)
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

	// Tile layout must match the mapped kernel in tmp-generated-instructions.yaml (and .asm):
	//   LOAD / input GEP live on core column=1, row=0  -> tile (1,0)
	//   STORE / histogram GEP on column=0, row=1      -> tile (0,1)
	// Older hand-tuned testbenches used (3,2)/(2,1); compiler-generated mapping differs.
	const (
		inputTileX   = 1
		inputTileY   = 0
		histTileX    = 0
		histTileY    = 1
		histBins     = 5
		inputDataLen = 20
	)

	for addr := 0; addr < histBins; addr++ {
		driver.PreloadMemory(histTileX, histTileY, 0, uint32(addr))
	}
	for addr, val := range inputData {
		if addr >= inputDataLen {
			break
		}
		driver.PreloadMemory(inputTileX, inputTileY, val, uint32(addr))
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

	// Histogram results written by STORE on tile (0,1); read same tile.
	outputTile := [2]int{histTileX, histTileY}
	fmt.Printf("output memory @ tile (%d,%d):\n", outputTile[0], outputTile[1])
	scanLimit := histBins
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
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("report saved: %s\n", reportPath)
}
