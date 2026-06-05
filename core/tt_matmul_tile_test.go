package core

import (
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func TestTTMatmulTileU32SingleKTile(t *testing.T) {
	a := make([]uint32, ttMatmulTileEdge*ttMatmulTileEdge)
	b := make([]uint32, ttMatmulTileEdge*ttMatmulTileEdge)
	for row := 0; row < ttMatmulTileEdge; row++ {
		a[row*ttMatmulTileEdge+row] = 1
		for col := 0; col < ttMatmulTileEdge; col++ {
			b[row*ttMatmulTileEdge+col] = uint32(row + col + 1)
		}
	}

	got := ttMatmulTileU32(cgra.FromSlice(a, true), cgra.FromSlice(b, true))
	if len(got.Data) != ttMatmulTileEdge*ttMatmulTileEdge {
		t.Fatalf("unexpected lane count: %d", len(got.Data))
	}
	for idx := range b {
		if got.Data[idx] != b[idx] {
			t.Fatalf("identity matmul mismatch at lane %d: got %d want %d", idx, got.Data[idx], b[idx])
		}
	}
}

func TestTTMatmulTileU32TwoKTiles(t *testing.T) {
	aPanel := make([]uint32, 2*ttMatmulTileEdge*ttMatmulTileEdge)
	bPanel := make([]uint32, 2*ttMatmulTileEdge*ttMatmulTileEdge)
	fillPanelTile(aPanel, 0, func(row, col int) uint32 {
		if row == col {
			return 1
		}
		return 0
	})
	fillPanelTile(bPanel, 0, func(row, col int) uint32 {
		return uint32(row + 1)
	})
	fillPanelTile(aPanel, 1, func(row, col int) uint32 {
		if row == col {
			return 2
		}
		return 0
	})
	fillPanelTile(bPanel, 1, func(row, col int) uint32 {
		return uint32(col + 1)
	})

	got := ttMatmulTileU32(cgra.FromSlice(aPanel, true), cgra.FromSlice(bPanel, true))
	for row := 0; row < ttMatmulTileEdge; row++ {
		for col := 0; col < ttMatmulTileEdge; col++ {
			want := uint32(row+1) + 2*uint32(col+1)
			if got.Data[row*ttMatmulTileEdge+col] != want {
				t.Fatalf("mismatch at (%d,%d): got %d want %d", row, col, got.Data[row*ttMatmulTileEdge+col], want)
			}
		}
	}
}

func TestTTMatmulTileU32OfficialKTShape(t *testing.T) {
	const kt = 20
	aPanel := make([]uint32, kt*ttMatmulTileEdge*ttMatmulTileEdge)
	bPanel := make([]uint32, kt*ttMatmulTileEdge*ttMatmulTileEdge)
	for kTile := 0; kTile < kt; kTile++ {
		fillPanelTile(aPanel, kTile, func(row, col int) uint32 {
			return uint32((row+col+kTile)%5 + 1)
		})
		fillPanelTile(bPanel, kTile, func(row, col int) uint32 {
			return uint32((row*2+col+kTile)%7 + 1)
		})
	}

	got := ttMatmulTileU32(cgra.FromSlice(aPanel, true), cgra.FromSlice(bPanel, true))
	want := cpuPanelMatmul(aPanel, bPanel, kt)
	for idx := range want {
		if got.Data[idx] != want[idx] {
			t.Fatalf("mismatch at lane %d: got %d want %d", idx, got.Data[idx], want[idx])
		}
	}
}

func fillPanelTile(panel []uint32, kTile int, fn func(row, col int) uint32) {
	base := kTile * ttMatmulTileEdge * ttMatmulTileEdge
	for row := 0; row < ttMatmulTileEdge; row++ {
		for col := 0; col < ttMatmulTileEdge; col++ {
			panel[base+row*ttMatmulTileEdge+col] = fn(row, col)
		}
	}
}

func cpuPanelMatmul(aPanel, bPanel []uint32, kt int) []uint32 {
	out := make([]uint32, ttMatmulTileEdge*ttMatmulTileEdge)
	for kTile := 0; kTile < kt; kTile++ {
		base := kTile * ttMatmulTileEdge * ttMatmulTileEdge
		for row := 0; row < ttMatmulTileEdge; row++ {
			for inner := 0; inner < ttMatmulTileEdge; inner++ {
				a := aPanel[base+row*ttMatmulTileEdge+inner]
				for col := 0; col < ttMatmulTileEdge; col++ {
					out[row*ttMatmulTileEdge+col] += a * bPanel[base+inner*ttMatmulTileEdge+col]
				}
			}
		}
	}
	return out
}
