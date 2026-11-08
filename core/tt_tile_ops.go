package core

// Tile instructions for TT-Metal-style kernels, next to TT_MATMUL_TILE_U32.
// Operands are ordinary E-IR operands (ports, indexed ports, registers,
// immediates); tiles travel as cgra.Data vectors:
//
//	L1 page tile:  words of the page bytes (bf16: 512, bfp8b: 272, little endian)
//	register tile: 1024 lanes of fp32 bit patterns (Src/Dst register contents)
//
// Arithmetic uses the Wormhole numerics in tt_numerics.go (bit-exact with
// TT-Sim for the FPU paths). Timing comes from the opcode latency table.

import (
	"encoding/binary"
	"fmt"

	"github.com/sarchlab/zeonica/cgra"
)

func (i instEmulator) ttTileOpcodeFuncs() map[string]opcodeFunc {
	m := map[string]opcodeFunc{
		"CB_WAIT":                 i.runCBWait,
		"CB_POP":                  i.runCBPop,
		"CB_RESERVE":              i.runCBReserve,
		"TT_PROGRESS_WORK_READER": i.runTTDelay,
		"TT_PROGRESS_WORK_WRITER": i.runTTDelay,
		"TT_PROGRESS_CREDIT":      i.runTTProgressCredit,
		"TT_RESUME_WORK":          i.runTTResume,
		"TT_DRAM_READ_TILE":       i.runTTMovTile,
		"TT_DRAM_WRITE_TILE":      i.runTTMovTile,
		// Per-tile writer with a write barrier (writer_unswizzle, writer_matmul_tile_layout).
		"TT_DRAM_WRITE_TILE_SYNC": i.runTTMovTile,
		"TT_MOV_TILE":             i.runTTMovTile,
		"TT_UNPACK_BF16":          func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTUnpack(o, s, TTFormatBF16) },
		"TT_UNPACK_BFP8":          func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTUnpack(o, s, TTFormatBFP8B) },
		"TT_PACK_BF16":            func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTPack(o, s, TTFormatBF16) },
		"TT_PACK_BFP8":            func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTPack(o, s, TTFormatBFP8B) },
		"TT_RELOAD":               i.runTTReload,
		"TT_COPY_TILE":            i.runTTCopyTile,
		"TT_ZERO_TILE":            i.runTTZeroTile,
		"TT_MATMUL_BF16":          i.runTTMatmulBF16,
		"TT_TILIZE":               i.runTTTilize,
		"TT_UNTILIZE":             i.runTTUntilize,
		"TT_DELAY_READER":         i.runTTDelay,
		"TT_DELAY_COMPUTE":        i.runTTDelay,
		"TT_DELAY_WRITER":         i.runTTDelay,
		// Per-subblock compute-stage handshakes (timing only).
		"TT_SUBBLOCK_UNPACK": i.runTTDelay,
		"TT_RELOAD_INIT":     i.runTTDelay,
		"TT_MM_INIT":         i.runTTDelay,
		"TT_SUBBLOCK_MATH":   i.runTTToken,
		"TT_SUBBLOCK_PACK":   i.runTTDelay,
	}
	for _, op := range []string{"ADD", "SUB", "MUL"} {
		op := op
		m["TT_ELTWISE_"+op] = func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTEltwise(o, s, op, "") }
		for _, b := range []string{"H", "W", "HW"} {
			b := b
			m["TT_BCAST_"+b+"_"+op] = func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTEltwise(o, s, op, b) }
		}
	}
	for name, op := range map[string]string{"RELU": "relu", "EXP": "exponential", "SIGMOID": "sigmoid", "TANH": "tanh"} {
		op := op
		m["TT_SFPU_"+name] = func(o Operation, s *coreState) map[Operand]cgra.Data { return i.runTTSFPU(o, s, op) }
	}
	return m
}

// ttHiFi4Phases: all modelled kernels use the default MathFidelity::HiFi4.
const ttHiFi4Phases = 4

func ttAllDst(inst Operation, v cgra.Data) map[Operand]cgra.Data {
	out := map[Operand]cgra.Data{}
	for _, d := range inst.DstOperands.Operands {
		out[d] = v
	}
	return out
}

func ttWordsFromBytes(b []byte) cgra.Data {
	w := make([]uint32, (len(b)+3)/4)
	for k := range w {
		var tmp [4]byte
		copy(tmp[:], b[4*k:])
		w[k] = binary.LittleEndian.Uint32(tmp[:])
	}
	return cgra.FromSlice(w, true)
}

func ttBytesFromWords(d cgra.Data, n int) []byte {
	b := make([]byte, 4*len(d.Data))
	for k, v := range d.Data {
		binary.LittleEndian.PutUint32(b[4*k:], v)
	}
	if n > 0 && n < len(b) {
		b = b[:n]
	}
	return b
}

func ttRegTile(d cgra.Data) *ttTileValues {
	if len(d.Data) != ttTileElems {
		panic(fmt.Sprintf("register tile must have %d lanes, got %d", ttTileElems, len(d.Data)))
	}
	var t ttTileValues
	copy(t[:], d.Data)
	return &t
}

func ttRegData(t *ttTileValues) cgra.Data {
	return cgra.FromSlice(append([]uint32(nil), t[:]...), true)
}

func (i instEmulator) ttArg(inst Operation, k int, state *coreState) cgra.Data {
	return i.readOperand(inst.SrcOperands.Operands[k], state)
}

func (i instEmulator) runTTMovTile(inst Operation, state *coreState) map[Operand]cgra.Data {
	return ttAllDst(inst, i.ttArg(inst, 0, state))
}

func (i instEmulator) runTTDelay(inst Operation, state *coreState) map[Operand]cgra.Data { return nil }

// runTTToken: timing-only operation that signals its destinations (one token).
func (i instEmulator) runTTToken(inst Operation, state *coreState) map[Operand]cgra.Data {
	return ttAllDst(inst, cgra.NewScalar(1))
}

func (i instEmulator) runTTUnpack(inst Operation, state *coreState, format string) map[Operand]cgra.Data {
	t, err := ttUnpackTile(ttBytesFromWords(i.ttArg(inst, 0, state), TTTileBytes(format)), format)
	if err != nil {
		panic(err)
	}
	return ttAllDst(inst, ttRegData(t))
}

func (i instEmulator) runTTPack(inst Operation, state *coreState, format string) map[Operand]cgra.Data {
	return ttAllDst(inst, ttWordsFromBytes(ttPackTile(ttRegTile(i.ttArg(inst, 0, state)), format)))
}

// TT_RELOAD page -> dst: copy_tile of a bf16 partial into the destination.
func (i instEmulator) runTTReload(inst Operation, state *coreState) map[Operand]cgra.Data {
	t, err := ttUnpackTile(ttBytesFromWords(i.ttArg(inst, 0, state), 2048), TTFormatBF16)
	if err != nil {
		panic(err)
	}
	return ttAllDst(inst, ttRegData(ttCopyTileToDst(t)))
}

func (i instEmulator) runTTCopyTile(inst Operation, state *coreState) map[Operand]cgra.Data {
	return ttAllDst(inst, ttRegData(ttCopyTileToDst(ttRegTile(i.ttArg(inst, 0, state)))))
}

func (i instEmulator) runTTZeroTile(inst Operation, state *coreState) map[Operand]cgra.Data {
	return ttAllDst(inst, ttRegData(&ttTileValues{}))
}

// TT_MATMUL_BF16 acc, a(in0), b(in1) -> acc: one matmul_tiles call.
func (i instEmulator) runTTMatmulBF16(inst Operation, state *coreState) map[Operand]cgra.Data {
	acc := ttRegTile(i.ttArg(inst, 0, state))
	ttMatmulTileAccumulate(acc, ttRegTile(i.ttArg(inst, 1, state)), ttRegTile(i.ttArg(inst, 2, state)), ttHiFi4Phases)
	return ttAllDst(inst, ttRegData(acc))
}

func (i instEmulator) runTTEltwise(inst Operation, state *coreState, op, bcast string) map[Operand]cgra.Data {
	ops := map[string]string{"ADD": "add", "SUB": "sub", "MUL": "mul"}
	kinds := map[string]string{"": "", "H": "h", "W": "w", "HW": "hw"}
	a, b := ttRegTile(i.ttArg(inst, 0, state)), ttRegTile(i.ttArg(inst, 1, state))
	return ttAllDst(inst, ttRegData(ttEltwiseTile(ops[op], a, b, ttBcastElem(kinds[bcast]), ttHiFi4Phases)))
}

func (i instEmulator) runTTSFPU(inst Operation, state *coreState, op string) map[Operand]cgra.Data {
	return ttAllDst(inst, ttRegData(ttSFPUTile(op, ttRegTile(i.ttArg(inst, 0, state)))))
}

// TT_TILIZE row0, row1, #j -> tile: tilize one 32-row strip of a 2-tile-wide
// row-major activation (two pages) and unpack tile j.
func (i instEmulator) runTTTilize(inst Operation, state *coreState) map[Operand]cgra.Data {
	rows := append(ttBytesFromWords(i.ttArg(inst, 0, state), 2048), ttBytesFromWords(i.ttArg(inst, 1, state), 2048)...)
	t, err := ttUnpackTile(ttTilizeRow(rows, 2)[ttImmediate(inst.SrcOperands.Operands[2])], TTFormatBF16)
	if err != nil {
		panic(err)
	}
	return ttAllDst(inst, ttRegData(t))
}

// TT_UNTILIZE dst0, dst1, #j -> page: pack two destination tiles of one tile
// row as bf16 and untilize; page j holds 16 rows of the 64-wide strip.
func (i instEmulator) runTTUntilize(inst Operation, state *coreState) map[Operand]cgra.Data {
	t0 := ttPackTile(ttRegTile(i.ttArg(inst, 0, state)), TTFormatBF16)
	t1 := ttPackTile(ttRegTile(i.ttArg(inst, 1, state)), TTFormatBF16)
	j := ttImmediate(inst.SrcOperands.Operands[2])
	return ttAllDst(inst, ttWordsFromBytes(ttUntilizeRow([][]byte{t0, t1})[j*2048:(j+1)*2048]))
}
