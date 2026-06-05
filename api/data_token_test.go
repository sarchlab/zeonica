package api_test

import (
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestDriverCoreDataFeedCollectCarriesVectorToken(t *testing.T) {
	const tileEdge = 32
	tileElems := tileEdge * tileEdge
	aPanel := make([]uint32, tileElems)
	bPanel := make([]uint32, tileElems)
	for row := 0; row < tileEdge; row++ {
		aPanel[row*tileEdge+row] = 1
		for col := 0; col < tileEdge; col++ {
			bPanel[row*tileEdge+col] = uint32(row + col + 1)
		}
	}

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
	driver.MapProgram(core.Program{EntryBlocks: []core.EntryBlock{{
		InstructionGroups: []core.InstructionGroup{{
			Operations: []core.Operation{{
				OpCode: "TT_MATMUL_TILE_U32",
				SrcOperands: core.OperandList{Operands: []core.Operand{
					{Impl: "West", Color: "R"},
					{Impl: "North", Color: "R"},
				}},
				DstOperands: core.OperandList{Operands: []core.Operand{
					{Impl: "East", Color: "R"},
				}},
			}},
			RefCount: map[string]int{"WestR": 1, "NorthR": 1},
		}},
	}}}, [2]int{0, 0})

	out := make([]cgra.Data, 1)
	driver.FeedInDataToCore([]cgra.Data{cgra.FromSlice(aPanel, true)}, [2]int{0, 0}, cgra.West, "R")
	driver.FeedInDataToCore([]cgra.Data{cgra.FromSlice(bPanel, true)}, [2]int{0, 0}, cgra.North, "R")
	driver.CollectDataFromCore(out, [2]int{0, 0}, cgra.East, "R")
	driver.Run()

	if got := out[0].LaneCount(); got != tileElems {
		t.Fatalf("unexpected lane count: got %d want %d", got, tileElems)
	}
	for idx, want := range bPanel {
		if out[0].Data[idx] != want {
			t.Fatalf("unexpected output lane %d: got %d want %d", idx, out[0].Data[idx], want)
		}
	}
}
