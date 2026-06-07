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
	"gopkg.in/yaml.v3"
)

const (
	npoints      = 256
	nstages      = 8
	dataRealBase = 0
	dataImagBase = dataRealBase + npoints
	coefRealBase = dataImagBase + npoints
	coefImagBase = coefRealBase + npoints
)

type fftValue struct {
	addr int
	val  int32
	name string
}

// Fft runs the 256-point integer FFT kernel compiled in
// test/Zeonica_Testbench/kernel/fft/tmp-generated-instructions.yaml.
//
// The kernel is validated against the source-level fft_int.c semantics using
// shared memory and non-overlapping argument bases:
// arg0=data_real@0, arg1=data_imag@256, arg2=coef_real@512, arg3=coef_imag@768.
//
//nolint:gocyclo,funlen
func Fft(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows
	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine

	programPath, err := resolveProgramPath()
	if err != nil {
		panic(err)
	}
	patchedProgramPath, cleanupProgram := resolveProgramYAMLWithGEPArgs(programPath)
	defer cleanupProgram()
	program := core.LoadProgramFileFromYAML(patchedProgramPath)
	fmt.Println("program:", patchedProgramPath)

	if len(program) == 0 {
		panic("failed to load FFT program")
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	dataReal, dataImag, coefReal, coefImag := initFFTInputs()
	preloadFFTMemory(driver.PreloadSharedMemory, dataReal, dataImag, coefReal, coefImag)
	expectedReal, expectedImag := simulateSourceFFT(dataReal, dataImag, coefReal, coefImag)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

	driver.Run()

	mismatch := 0
	expectedValues := expectedFFTValues(expectedReal, expectedImag)
	for _, expected := range expectedValues {
		got := int32(driver.ReadSharedMemory(0, 0, uint32(expected.addr)))
		if got != expected.val {
			if mismatch < 20 {
				fmt.Printf(
					"FFT mismatch %s[%d] addr=%d: got=%d expected=%d\n",
					expected.name,
					expected.addr%npoints,
					expected.addr,
					got,
					expected.val,
				)
			}
			mismatch++
		}
	}

	if mismatch == 0 {
		fmt.Println("FFT output matches expected")
	} else {
		fmt.Printf("FFT output mismatches: %d\n", mismatch)
	}
	return mismatch
}

func initFFTInputs() ([]int32, []int32, []int32, []int32) {
	dataReal := make([]int32, npoints)
	dataImag := make([]int32, npoints)
	coefReal := make([]int32, npoints)
	coefImag := make([]int32, npoints)

	for i := 0; i < npoints; i++ {
		dataReal[i] = int32(i)
		dataImag[i] = 1
		coefReal[i] = 2
		coefImag[i] = 2
	}

	return dataReal, dataImag, coefReal, coefImag
}

func preloadFFTMemory(
	preload func(x int, y int, data []byte, baseAddr uint32),
	dataReal []int32,
	dataImag []int32,
	coefReal []int32,
	coefImag []int32,
) {
	for i := 0; i < npoints; i++ {
		preload(0, 0, uint32Bytes(uint32(dataReal[i])), uint32(dataRealBase+i))
		preload(0, 0, uint32Bytes(uint32(dataImag[i])), uint32(dataImagBase+i))
		preload(0, 0, uint32Bytes(uint32(coefReal[i])), uint32(coefRealBase+i))
		preload(0, 0, uint32Bytes(uint32(coefImag[i])), uint32(coefImagBase+i))
	}
}

func simulateSourceFFT(
	inputReal []int32,
	inputImag []int32,
	coefReal []int32,
	coefImag []int32,
) ([]int32, []int32) {
	dataReal := append([]int32(nil), inputReal...)
	dataImag := append([]int32(nil), inputImag...)
	groupsPerStage := 1
	buttersPerGroup := npoints / 2
	coefBase := 0

	for stage := 0; stage < nstages; stage++ {
		for j := 0; j < groupsPerStage; j++ {
			coefIdx := coefBase + j
			wr := coefReal[coefIdx]
			wi := coefImag[coefIdx]
			for k := 0; k < buttersPerGroup; k++ {
				lower := 2*j*buttersPerGroup + k
				upper := lower + buttersPerGroup

				tempReal := wr*dataReal[upper] - wi*dataImag[upper]
				tempImag := wi*dataReal[upper] + wr*dataImag[upper]
				dataReal[upper] = dataReal[lower] - tempReal
				dataReal[lower] += tempReal
				dataImag[upper] = dataImag[lower] - tempImag
				dataImag[lower] += tempImag
			}
		}
		groupsPerStage *= 2
		buttersPerGroup /= 2
		coefBase = (coefBase << 1) + 1
	}

	return dataReal, dataImag
}

func expectedFFTValues(dataReal []int32, dataImag []int32) []fftValue {
	values := make([]fftValue, 0, npoints*2)
	for i := 0; i < npoints; i++ {
		values = append(values,
			fftValue{addr: dataRealBase + i, val: dataReal[i], name: "data_real"},
			fftValue{addr: dataImagBase + i, val: dataImag[i], name: "data_imag"},
		)
	}
	return values
}

func uint32Bytes(value uint32) []byte {
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}

func gepArgReplacements() map[string]string {
	return map[string]string{
		"arg0": fmt.Sprintf("#%d", dataRealBase),
		"arg1": fmt.Sprintf("#%d", dataImagBase),
		"arg2": fmt.Sprintf("#%d", coefRealBase),
		"arg3": fmt.Sprintf("#%d", coefImagBase),
	}
}

func resolveProgramYAMLWithGEPArgs(programPath string) (resolved string, cleanup func()) {
	data, err := os.ReadFile(programPath)
	if err != nil {
		panic(fmt.Sprintf("read FFT program file %q: %v", programPath, err))
	}

	var root core.YAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		panic(fmt.Sprintf("parse FFT program file %q: %v", programPath, err))
	}

	repl := gepArgReplacements()
	changed := false
	for ci := range root.ArrayConfig.Cores {
		for ei := range root.ArrayConfig.Cores[ci].Entries {
			for gi := range root.ArrayConfig.Cores[ci].Entries[ei].InstructionGroups {
				group := &root.ArrayConfig.Cores[ci].Entries[ei].InstructionGroups[gi]
				for oi := range group.Operations {
					op := &group.Operations[oi]
					if op.OpCode != "GEP" {
						continue
					}
					for si := range op.SrcOperands {
						if replacement, ok := repl[op.SrcOperands[si].Operand]; ok {
							op.SrcOperands[si].Operand = replacement
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

	tmp, err := os.CreateTemp("", "zeonica-fft-patched-*.yaml")
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

	return path, func() {
		_ = os.Remove(path)
	}
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
		"test/Zeonica_Testbench/kernel/fft/tmp-generated-instructions.yaml",
		"../../Zeonica_Testbench/kernel/fft/tmp-generated-instructions.yaml",
	}

	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "Zeonica_Testbench", "kernel", "fft", "tmp-generated-instructions.yaml")),
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

	return "", fmt.Errorf("cannot locate FFT program, tried: %s", strings.Join(normalized, ", "))
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
		"test/testbench/fft/arch_spec.yaml",
		"arch_spec.yaml",
		"test/arch_spec/arch_spec.yaml",
		"../../arch_spec/arch_spec.yaml",
	}

	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "arch_spec.yaml")),
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
	const testName = "fft"

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

	mismatch := Fft(rt)

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
