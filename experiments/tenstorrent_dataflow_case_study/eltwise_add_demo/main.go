package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func main() {
	inputA := []uint32{1, 2, 3}
	inputB := []uint32{10, 20, 30}
	output := make([]uint32, len(inputA))

	engine := sim.NewSerialEngine()
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithPortBufferDepth(8, 8).
		Build("Driver")

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithWidth(1).
		WithHeight(1).
		WithCorePortBufferDepth(8, 8).
		Build("Device")

	driver.RegisterDevice(device)

	programPath := resolveProgramPath()
	programs := core.LoadProgramFileFromYAML(programPath)
	program, ok := programs["(0,0)"]
	if !ok {
		panic(fmt.Sprintf("program %s does not contain PE (0,0)", programPath))
	}
	driver.MapProgram(program, [2]int{0, 0})

	driver.FeedIn(inputA, cgra.West, [2]int{0, 1}, 1, "R")
	driver.FeedIn(inputB, cgra.North, [2]int{0, 1}, 1, "R")
	driver.Collect(output, cgra.East, [2]int{0, 1}, 1, "R")

	driver.Run()

	mismatch := 0
	expected := make([]uint32, len(inputA))
	for i := range inputA {
		expected[i] = inputA[i] + inputB[i]
		if output[i] != expected[i] {
			mismatch++
		}
	}

	fmt.Printf("input_a=%v\n", inputA)
	fmt.Printf("input_b=%v\n", inputB)
	fmt.Printf("expected=%v\n", expected)
	fmt.Printf("output=%v\n", output)
	fmt.Printf("mismatch=%d\n", mismatch)

	if mismatch != 0 {
		panic("eltwise_add demo failed")
	}
}

func resolveProgramPath() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Clean(filepath.Join(
			filepath.Dir(file),
			"..",
			"kernels",
			"eltwise_add",
			"eltwise_add.yaml",
		))
	}
	return filepath.Clean("experiments/tenstorrent_dataflow_case_study/kernels/eltwise_add/eltwise_add.yaml")
}
