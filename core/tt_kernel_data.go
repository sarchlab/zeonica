package core

// Wormhole tile data: L1 page <-> register tile conversion, tilize/untilize and
// FPU/SFPU tile compute, bit-exact with TT-Sim (the previous round's data plane).

import (
	"encoding/binary"
	"fmt"
)

// TT L1 data formats used by the modelled kernels.
const (
	TTFormatBF16  = "bf16"
	TTFormatBFP8B = "bfp8b"
)

// TTTileBytes returns the L1 size of one 32x32 tile.
func TTTileBytes(format string) int {
	if format == TTFormatBFP8B {
		return 64 + 1024
	}
	return 2048
}

// ---------------------------------------------------------------------------
// Tile encode/decode between L1 pages and fp32-bit element arrays.

type ttTileValues [ttTileElems]uint32

func ttUnpackTile(page []byte, format string) (*ttTileValues, error) {
	var t ttTileValues
	switch format {
	case TTFormatBF16:
		if len(page) != 2048 {
			return nil, fmt.Errorf("bf16 tile has %d bytes", len(page))
		}
		for i := 0; i < ttTileElems; i++ {
			t[i] = ttBF16ToSrc(binary.LittleEndian.Uint16(page[2*i:]))
		}
	case TTFormatBFP8B:
		if len(page) != 1088 {
			return nil, fmt.Errorf("bfp8b tile has %d bytes", len(page))
		}
		for i := 0; i < ttTileElems; i++ {
			t[i] = ttBF16ToSrc(ttBFP8ToBF16(page[64+i], page[i/16]))
		}
	default:
		return nil, fmt.Errorf("unsupported tile format %q", format)
	}
	return &t, nil
}

func ttPackTile(dst *ttTileValues, format string) []byte {
	switch format {
	case TTFormatBFP8B:
		page := make([]byte, 1088)
		for g := 0; g < 64; g++ {
			exp, mans := ttPackBFP8Block(dst[16*g : 16*g+16])
			page[g] = exp
			copy(page[64+16*g:], mans[:])
		}
		return page
	default:
		page := make([]byte, 2048)
		for i := 0; i < ttTileElems; i++ {
			binary.LittleEndian.PutUint16(page[2*i:], ttPackBF16(dst[i]))
		}
		return page
	}
}

// Row-major <-> tile conversion for one tile row (32 rows x widthTiles tiles)
// of bf16 data. Tilize/untilize are pure permutations in the unpacker/packer.
func ttTilizeRow(rows []byte, widthTiles int) [][]byte {
	out := make([][]byte, widthTiles)
	w := widthTiles * 32
	for t := 0; t < widthTiles; t++ {
		page := make([]byte, 2048)
		for r := 0; r < 32; r++ {
			for c := 0; c < 32; c++ {
				src := 2 * (r*w + t*32 + c)
				copy(page[2*ttFaceIndex(r, c):], rows[src:src+2])
			}
		}
		out[t] = page
	}
	return out
}

func ttUntilizeRow(tiles [][]byte) []byte {
	w := len(tiles) * 32
	rows := make([]byte, 2*32*w)
	for t, page := range tiles {
		for r := 0; r < 32; r++ {
			for c := 0; c < 32; c++ {
				dst := 2 * (r*w + t*32 + c)
				copy(rows[dst:dst+2], page[2*ttFaceIndex(r, c):2*ttFaceIndex(r, c)+2])
			}
		}
	}
	return rows
}

// ---------------------------------------------------------------------------
// Tile compute.

// ttMatmulTileAccumulate performs one matmul_tiles call: dst += a(in0) x b(in1)
// with all fidelity phases. LLK order: for each fidelity phase, a full sweep of
// MVMULs over both 16-wide inner halves; one MVMUL reduces 16 inner lanes.
func ttMatmulTileAccumulate(dst, a, b *ttTileValues, phases int) {
	var aOp, bOp ttTileValues
	for i := range a {
		aOp[i] = ttFPUOperand(a[i])
		bOp[i] = ttFPUOperand(b[i])
	}
	var srcA, srcB [16]uint32
	for r := 0; r < 32; r++ {
		for c := 0; c < 32; c++ {
			idx := ttFaceIndex(r, c)
			v := dst[idx]
			for p := 0; p < phases; p++ {
				for h := 0; h < 2; h++ {
					for i := 0; i < 16; i++ {
						k := 16*h + i
						srcB[i] = aOp[ttFaceIndex(r, k)] // SrcB holds in0 rows
						srcA[i] = bOp[ttFaceIndex(k, c)] // SrcA holds in1 columns
					}
					v = ttDst16(ttMvMul(&srcA, &srcB, v, uint32(p)))
				}
			}
			dst[idx] = v
		}
	}
}

// ttEltwiseTile computes one ELWADD/ELWSUB/ELWMUL tile. bAt maps an output
// element to the SrcB element used (broadcast).
func ttEltwiseTile(op string, a, b *ttTileValues, bAt func(r, c int) int, phases int) *ttTileValues {
	var out ttTileValues
	for r := 0; r < 32; r++ {
		for c := 0; c < 32; c++ {
			idx := ttFaceIndex(r, c)
			x := ttFPUOperand(a[idx])
			y := ttFPUOperand(b[bAt(r, c)])
			switch op {
			case "add":
				out[idx] = ttDst16(ttElwAdd(x, y))
			case "sub":
				out[idx] = ttDst16(ttElwAdd(x, y^0x40000))
			case "mul":
				v := uint32(0)
				for p := 0; p < phases; p++ {
					v = ttDst16(ttElwMul(x, y, v, uint32(p)))
				}
				out[idx] = v
			default:
				panic("unsupported eltwise op " + op)
			}
		}
	}
	return &out
}

func ttCopyTileToDst(a *ttTileValues) *ttTileValues {
	var out ttTileValues
	for i := range a {
		out[i] = ttMovA2D(a[i])
	}
	return &out
}

func ttSFPUTile(op string, a *ttTileValues) *ttTileValues {
	d := ttCopyTileToDst(a)
	for i := range d {
		d[i] = ttSFPU(op, d[i])
	}
	return d
}

