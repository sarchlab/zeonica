package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
	"gopkg.in/yaml.v3"
)

const (
	testName = "conv"
	width    = 4
	height   = 4
	kSteps   = 9
)

func makeBytesFromUint32(data uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, data)
	return buf
}

func resolveLocalPath(name string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve local path")
	}
	return filepath.Join(filepath.Dir(thisFile), name)
}

func resolveProgramPath() string {
	if fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_CONV_PROGRAM")); fromEnv != "" {
		return fromEnv
	}
	return filepath.Clean(filepath.Join(
		resolveLocalPath(".."),
		"..",
		"Zeonica_Testbench",
		"kernel",
		"conv",
		"tmp-generated-instructions.yaml",
	))
}

//nolint:gocyclo
func patchGEPArgs(programPath string, repl map[string]string) (string, func()) {
	data, err := os.ReadFile(programPath)
	if err != nil {
		panic(err)
	}
	var root core.YAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		panic(err)
	}
	changed := false
	for ci := range root.ArrayConfig.Cores {
		for ei := range root.ArrayConfig.Cores[ci].Entries {
			for gi := range root.ArrayConfig.Cores[ci].Entries[ei].InstructionGroups {
				for oi := range root.ArrayConfig.Cores[ci].Entries[ei].InstructionGroups[gi].Operations {
					op := &root.ArrayConfig.Cores[ci].Entries[ei].InstructionGroups[gi].Operations[oi]
					if op.OpCode != "GEP" {
						continue
					}
					for si := range op.SrcOperands {
						src := &op.SrcOperands[si]
						if replacement, ok := repl[src.Operand]; ok {
							src.Operand = replacement
							changed = true
						}
					}
				}
			}
		}
	}
	if !changed {
		return programPath, func() {}
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		panic(err)
	}
	tmp, err := os.CreateTemp("", "zeonica-conv-patched-*.yaml")
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

func appendRegisterExport(programs map[string]core.Program) {
	for coord, program := range programs {
		if len(program.EntryBlocks) == 0 {
			continue
		}
		lastTimeStep := 0
		for _, group := range program.EntryBlocks[0].InstructionGroups {
			for _, op := range group.Operations {
				if op.TimeStep > lastTimeStep {
					lastTimeStep = op.TimeStep
				}
			}
		}
		export := core.InstructionGroup{
			RefCount:   map[string]int{"$0RED": 1},
			IndexPerII: lastTimeStep + 1,
			Operations: []core.Operation{
				{
					OpCode: "DATA_MOV",
					SrcOperands: core.OperandList{Operands: []core.Operand{{
						Color: "R",
						Impl:  "$0",
					}}},
					DstOperands: core.OperandList{Operands: []core.Operand{{
						Color: "R",
						Impl:  "SOUTHEAST",
					}}},
					ID:       900000 + lastTimeStep,
					TimeStep: lastTimeStep + 1,
				},
			},
		}
		program.EntryBlocks[0].InstructionGroups = append(program.EntryBlocks[0].InstructionGroups, export)
		programs[coord] = program
	}
}

func scheduleMappedTiles(device cgra.Device, engine sim.Engine, programs map[string]core.Program) {
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if _, exists := programs[fmt.Sprintf("(%d,%d)", x, y)]; !exists {
				continue
			}
			tile := device.GetTile(x, y)
			engine.Schedule(sim.MakeTickEvent(tile.GetTickingComponent(), 0))
		}
	}
}

// expectedGeneratedConvReturn is the CPU reference for the compiler-generated
// conv kernel: sum_x A[x/70][x%70] * B[x/70][x%70] over x in [0, 4200), with A
// preloaded as aOnes ones and B as bOnes ones (all other words zero).
func expectedGeneratedConvReturn(aOnes, bOnes int) uint32 {
	const total = 4200
	expected := uint32(0)
	for x := 0; x < total; x++ {
		if x < aOnes && x < bOnes {
			expected++
		}
	}
	return expected
}

func runCompilerGeneratedConv(rt *runtimecfg.Runtime, programPath string) (bool, int) {
	const (
		inputTileX = 3
		inputTileY = 0
		arg6Base   = 0
		arg7Base   = 5000
		inputWords = 4200
	)

	patchedPath, cleanup := patchGEPArgs(programPath, map[string]string{
		"arg6": fmt.Sprintf("#%d", arg6Base),
		"arg7": fmt.Sprintf("#%d", arg7Base),
	})
	defer cleanup()

	programs := core.LoadProgramFileFromYAML(patchedPath)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if prog, exists := programs[fmt.Sprintf("(%d,%d)", x, y)]; exists {
				rt.Driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	for i := 0; i < inputWords; i++ {
		if rt.Config.MemoryMode == "shared" {
			rt.Driver.PreloadSharedMemory(inputTileX, inputTileY, makeBytesFromUint32(1), uint32(arg6Base+i))
		} else {
			rt.Driver.PreloadMemory(inputTileX, inputTileY, 1, uint32(arg6Base+i))
		}
	}
	for i := 0; i < 140; i++ {
		if rt.Config.MemoryMode == "shared" {
			rt.Driver.PreloadSharedMemory(inputTileX, inputTileY, makeBytesFromUint32(1), uint32(arg7Base+i))
		} else {
			rt.Driver.PreloadMemory(inputTileX, inputTileY, 1, uint32(arg7Base+i))
		}
	}

	scheduleMappedTiles(rt.Device, rt.Engine, programs)
	rt.Driver.Run()

	ret := rt.Device.GetTile(2, 0).GetRetVal()
	fmt.Printf("tmp-generated conv return value: %d\n", ret)

	expected := expectedGeneratedConvReturn(inputWords, 140)
	fmt.Printf("expected return value: %d\n", expected)
	if ret != expected {
		return false, 1
	}
	return true, 0
}

//nolint:gocyclo
func runSystolicConv(rt *runtimecfg.Runtime, programPath string) (bool, int) {
	programs := core.LoadProgramFileFromYAML(programPath)
	appendRegisterExport(programs)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if prog, exists := programs[fmt.Sprintf("(%d,%d)", x, y)]; exists {
				rt.Driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	rowInputs := [height][kSteps]int32{
		{1, 2, 3, 4, 5, 6, 7, 8, 9},
		{2, 3, 4, 5, 6, 7, 8, 9, 10},
		{3, 4, 5, 6, 7, 8, 9, 10, 11},
		{4, 5, 6, 7, 8, 9, 10, 11, 12},
	}
	colWeights := [width][kSteps]int32{
		{1, 1, 1, 1, 1, 1, 1, 1, 1},
		{1, 2, 1, 2, 1, 2, 1, 2, 1},
		{2, 2, 2, 2, 2, 2, 2, 2, 2},
		{3, 1, 3, 1, 3, 1, 3, 1, 3},
	}

	for y := 0; y < height; y++ {
		data := make([]cgra.Data, kSteps)
		for k := 0; k < kSteps; k++ {
			data[k] = cgra.NewScalar(uint32(rowInputs[y][k]))
		}
		rt.Driver.FeedInDataToCore(data, [2]int{0, y}, cgra.West, "R")
	}
	for x := 0; x < width; x++ {
		data := make([]cgra.Data, kSteps)
		for k := 0; k < kSteps; k++ {
			data[k] = cgra.NewScalar(uint32(colWeights[x][k]))
		}
		rt.Driver.FeedInDataToCore(data, [2]int{x, height - 1}, cgra.North, "R")
	}

	outputs := make(map[[2]int][]cgra.Data, width*height)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			buf := make([]cgra.Data, 1)
			outputs[[2]int{x, y}] = buf
			rt.Driver.CollectDataFromCore(buf, [2]int{x, y}, cgra.SouthEast, "R")
		}
	}

	scheduleMappedTiles(rt.Device, rt.Engine, programs)
	rt.Driver.Run()

	mismatch := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var expected int32
			for k := 0; k < kSteps; k++ {
				expected += rowInputs[y][k] * colWeights[x][k]
			}
			got := int32(outputs[[2]int{x, y}][0].First())
			fmt.Printf("out[%d][%d] = %d expected=%d\n", y, x, got, expected)
			if got != expected {
				mismatch++
			}
		}
	}

	return mismatch == 0, mismatch
}

func main() {
	rt, err := runtimecfg.LoadRuntime(resolveLocalPath("arch_spec.yaml"), testName)
	if err != nil {
		panic(err)
	}
	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		panic(err)
	}

	programPath := resolveProgramPath()
	var passed bool
	var mismatch int
	if strings.Contains(filepath.Base(programPath), "tmp-generated") {
		passed, mismatch = runCompilerGeneratedConv(rt, programPath)
	} else {
		passed, mismatch = runSystolicConv(rt, programPath)
	}

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("conv report saved: %s\n", reportPath)
	if mismatch != 0 {
		os.Exit(1)
	}
}
