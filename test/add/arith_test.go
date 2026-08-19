package main

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestArithmeticOperations(t *testing.T) {
	tests := []struct {
		name        string
		programPath string
		feedSide    cgra.Side
		collectSide cgra.Side
		input       []int32
		want        func(int32) int32
	}{
		{
			name:        "add",
			programPath: "test_add.yaml",
			feedSide:    cgra.West,
			collectSide: cgra.East,
			input:       integerSequence(-10, 16),
			want:        func(value int32) int32 { return value + 2 },
		},
		{
			name:        "subtract",
			programPath: "test_sub.yaml",
			feedSide:    cgra.South,
			collectSide: cgra.North,
			input:       integerSequence(-10, 16),
			want:        func(value int32) int32 { return value - 2 },
		},
		{
			name:        "multiply",
			programPath: "test_mul.yaml",
			feedSide:    cgra.East,
			collectSide: cgra.West,
			input:       integerSequence(-10, 16),
			want:        func(value int32) int32 { return value * 4 },
		},
		{
			name:        "divide",
			programPath: "test_div.yaml",
			feedSide:    cgra.North,
			collectSide: cgra.South,
			input:       integerMultiples(-8, 4, 16),
			want:        func(value int32) int32 { return value / 4 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runArithmeticProgram(t, tt.programPath, tt.feedSide, tt.collectSide, tt.input)
			for idx, input := range tt.input {
				want := tt.want(input)
				if got[idx] != want {
					t.Errorf("result[%d] = %d, want %d for input %d", idx, got[idx], want, input)
				}
			}
		})
	}
}

func runArithmeticProgram(
	t *testing.T,
	programPath string,
	feedSide cgra.Side,
	collectSide cgra.Side,
	input []int32,
) []int32 {
	t.Helper()
	const width = 2
	const height = 2

	engine := sim.NewSerialEngine()
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		Build("Device")
	driver.RegisterDevice(device)

	programs := core.LoadProgramFileFromYAML(programPath)
	if len(programs) == 0 {
		t.Fatalf("program %q contains no core programs", programPath)
	}
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			if program, ok := programs[fmt.Sprintf("(%d,%d)", x, y)]; ok {
				driver.MapProgram(program, [2]int{x, y})
			}
		}
	}

	src := signedToUint32(input)
	dst := make([]uint32, len(src))
	feedSpan := sideSpan(feedSide, width, height)
	collectSpan := sideSpan(collectSide, width, height)
	driver.FeedIn(src, feedSide, [2]int{0, feedSpan}, feedSpan, "R")
	driver.Collect(dst, collectSide, [2]int{0, collectSpan}, collectSpan, "R")
	driver.Run()

	return uint32ToSigned(dst)
}

func sideSpan(side cgra.Side, width, height int) int {
	if side == cgra.North || side == cgra.South {
		return width
	}
	return height
}

func integerSequence(start int32, length int) []int32 {
	values := make([]int32, length)
	for idx := range values {
		values[idx] = start + int32(idx)
	}
	return values
}

func integerMultiples(start, step int32, length int) []int32 {
	values := make([]int32, length)
	for idx := range values {
		values[idx] = (start + int32(idx)) * step
	}
	return values
}

func signedToUint32(values []int32) []uint32 {
	result := make([]uint32, len(values))
	for idx, value := range values {
		result[idx] = uint32(value)
	}
	return result
}

func uint32ToSigned(values []uint32) []int32 {
	result := make([]int32, len(values))
	for idx, value := range values {
		result[idx] = int32(value)
	}
	return result
}
