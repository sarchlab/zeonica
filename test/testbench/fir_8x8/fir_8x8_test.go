package main

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

const (
	samples = 64
	taps    = 8
	xBase   = 0
	hBase   = 256
	yBase   = 512
)

var (
	x = []int32{
		-5, 2, -7, 0, 7, -2, 5, -4, 3, -6, 1, -8, -1, 6, -3, 4,
		-5, 2, -7, 0, 7, -2, 5, -4, 3, -6, 1, -8, -1, 6, -3, 4,
		-5, 2, -7, 0, 7, -2, 5, -4, 3, -6, 1, -8, -1, 6, -3, 4,
		-5, 2, -7, 0, 7, -2, 5, -4, 3, -6, 1, -8, -1, 6, -3, 4,
	}
	h = []int32{-3, 2, -2, 3, -1, 4, 0, -4}
)

func word(v int32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(v))
	return buf
}

// TestFir8x8 runs a mapping whose loop PHIs receive their back-edge over a port:
// PHI 26 on tile (0,1) has EAST as its back-edge and $0 as its initial value.
func TestFir8x8(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "fir_8x8.json.log"))
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer f.Close()
	slog.SetDefault(slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: core.LevelTrace})))

	rt, err := runtimecfg.LoadRuntime("arch_spec.yaml", "fir_8x8")
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}

	program := core.LoadProgramFileFromYAML("fir-8x8-instructions.yaml")
	for col := 0; col < rt.Config.Columns; col++ {
		for row := 0; row < rt.Config.Rows; row++ {
			if prog, ok := program[fmt.Sprintf("(%d,%d)", col, row)]; ok {
				rt.Driver.MapProgram(prog, [2]int{col, row})
			}
		}
	}

	for i, v := range x {
		rt.Driver.PreloadSharedMemory(0, 0, word(v), uint32(xBase+i))
	}
	for i, v := range h {
		rt.Driver.PreloadSharedMemory(0, 0, word(v), uint32(hBase+i))
	}

	for col := 0; col < rt.Config.Columns; col++ {
		for row := 0; row < rt.Config.Rows; row++ {
			tile := rt.Device.GetTile(col, row)
			rt.Engine.Schedule(sim.MakeTickEvent(tile.GetTickingComponent(), 0))
		}
	}
	rt.Driver.Run()

	mismatches := 0
	for i := taps - 1; i < samples; i++ {
		var want int32
		for k := 0; k < taps; k++ {
			want += h[k] * x[i-k]
		}
		if got := int32(rt.Driver.ReadSharedMemory(0, 0, uint32(yBase+i))); got != want {
			mismatches++
		}
	}
	if mismatches != 0 {
		t.Fatalf("fir mismatch count: got=%d of %d expected=0", mismatches, samples-taps+1)
	}
}
