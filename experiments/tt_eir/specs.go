package main

// Kernel shapes, CB sizes and DRAM page orders of the TT-Metal gtests.

import (
	"fmt"
	"math"
)

type caseB struct {
	a, b, shared int
	stage        string
	delay        int
}

type kernelSpec struct {
	build        func(Latencies) *build
	fixed        func(Fixed) (float64, map[string]float64)
	inPageBytes  int
	outPages     int
	outPageBytes int
	tolerance    func(a, b float32) bool
	caseB        bool
}

func isClose(rtol, atol float32) func(a, b float32) bool {
	return func(a, b float32) bool {
		d := float32(math.Abs(float64(a - b)))
		return d <= atol || d <= rtol*float32(math.Max(math.Abs(float64(a)), math.Abs(float64(b))))
	}
}

// unswizzle: writer_unswizzle / writer_matmul_tile_layout page of the n-th written tile.
func unswizzle(n, nt, subM, subN, rowOff, colOff, globalN int) int {
	sT := subM * subN
	sub, within := n/sT, n%sT
	nSubN := nt / subN
	return (rowOff+(sub/nSubN)*subM+within/subN)*globalN + colOff + (sub%nSubN)*subN + within%subN
}

func matmulFixed(class string, rows, cols int) func(Fixed) (float64, map[string]float64) {
	return func(f Fixed) (float64, map[string]float64) {
		parts := map[string]float64{"dispatch": f.Dispatch, "class": f.ClassFixed[class]}
		if rows*cols > 1 {
			line := 0.0
			if rows == 1 || cols == 1 {
				line = math.Max(float64(rows+cols-3), 0)
			}
			mesh := math.Max(float64(rows-1), 0) * math.Max(float64(cols-1), 0)
			parts["multicore"] = f.MulticoreBase + line*f.MulticoreLine + mesh*f.MulticoreMesh
		}
		t := 0.0
		for _, v := range parts {
			t += v
		}
		return t, parts
	}
}

func blocked(m, n, kt, w, subM, subN, aCap, bCap, sCap int, bfp8 bool) MatmulKernel {
	aT, bT := m*w, w*n
	return MatmulKernel{Rows: 1, Cols: 1, MTiles: m, NTiles: n, SubM: subM, SubN: subN, KWidth: w, Blocks: kt / w,
		BFP8: bfp8, ACap: aCap, BCap: bCap, SharedCap: sCap, SingleCoreGtest: true,
		In0Page: func(r, c, k, i int) int { return k*aT + i },
		In1Page: func(r, c, k, i int) int { return k*bT + i },
		OutPage: func(r, c, x int) int { return unswizzle(x, n, subM, subN, 0, 0, n) }}
}

func multicore(rows, cols int, cb caseB) MatmulKernel {
	const m, n, kt, w = 4, 4, 8, 2
	globalN := n * cols
	k := MatmulKernel{Rows: rows, Cols: cols, MTiles: m, NTiles: n, SubM: 4, SubN: 2, KWidth: w, Blocks: kt / w,
		ACap: cb.a, BCap: cb.b, SharedCap: cb.shared,
		In0Page: func(r, c, k, i int) int { return (r*m+i/w)*kt + k*w + i%w },
		In1Page: func(r, c, k, i int) int { return (k*w+i/n)*globalN + c*n + i%n },
		OutPage: func(r, c, x int) int { return unswizzle(x, n, 4, 2, r*m, c*n, globalN) }}
	switch cb.stage {
	case "reader":
		k.ReaderDelay = cb.delay
	case "compute":
		k.ComputeDelay = cb.delay
	case "writer":
		k.WriterDelay = cb.delay
	}
	return k
}

func hifi4(actRM, outRM bool) MatmulKernel {
	k := blocked(4, 2, 2, 2, 2, 1, 16, 8, 8, false)
	k.ActivationRowMajor, k.OutRowMajor = actRM, outRM
	return k
}

func mm(k MatmulKernel, class string, pages int) kernelSpec {
	pb := 2048
	if k.BFP8 {
		pb = 1088
	}
	return kernelSpec{build: k.build, fixed: matmulFixed(class, k.Rows, k.Cols), inPageBytes: pb, outPages: pages, outPageBytes: pb}
}

func eltwise(kind, op string, tiles, inCap, outCap int) kernelSpec {
	const ht, wt = 2, 3
	k := EltwiseKernel{Tiles: tiles, Kind: kind, Op: op, Ht: ht, Wt: wt, InCap: inCap, OutCap: outCap}
	switch kind {
	case "bcast_h":
		k.In1Page = func(i int) int { return (i/(ht*wt))*wt + i%wt }
	case "bcast_w":
		k.In1Page = func(i int) int { return i / wt }
	case "bcast_hw":
		k.In1Page = func(i int) int { return i / (ht * wt) }
	default:
		k.In1Page = func(i int) int { return i }
	}
	s := kernelSpec{build: k.build, inPageBytes: 2048, outPages: tiles, outPageBytes: 2048}
	s.fixed = func(f Fixed) (float64, map[string]float64) {
		if kind == "sfpu" {
			return f.SFPUDispatch, map[string]float64{"sfpu_dispatch": f.SFPUDispatch}
		}
		return f.Dispatch, map[string]float64{"dispatch": f.Dispatch}
	}
	return s
}

func lookup(name string, cb caseB) kernelSpec {
	grids := map[string][2]int{"": {2, 2}, "_1x2": {1, 2}, "_2x1": {2, 1}, "_1x3": {1, 3}, "_3x1": {3, 1}, "_2x3": {2, 3}, "_3x2": {3, 2},
		"_2x4": {2, 4}, "_4x2": {4, 2}, "_3x3": {3, 3}}
	for suffix, g := range grids {
		if name == "matmul_multicore_multi_dram_small"+suffix {
			return mm(multicore(g[0], g[1], caseB{a: 16, b: 16, shared: 16}), "multicore_multi_dram", 16*g[0]*g[1])
		}
	}
	sfpu := map[string]string{"relu": "RELU", "exponential": "EXP", "sigmoid": "SIGMOID", "tanh": "TANH"}
	tol := map[string]func(a, b float32) bool{"exponential": isClose(0.1, 0.1), "sigmoid": isClose(0.06, 0.006), "tanh": isClose(0.175, 0.1)}
	for op, code := range sfpu {
		for _, tiles := range []int{1, 4} {
			if name == fmt.Sprintf("sfpu_%s_%dtile", op, tiles) {
				s := eltwise("sfpu", code, tiles, 1, 1)
				s.tolerance = tol[op]
				return s
			}
		}
	}
	switch name {
	case "caseb":
		s := mm(multicore(2, 2, cb), "multicore_multi_dram", 64)
		s.caseB = true
		return s
	case "matmul_single_tile_l1":
		return mm(blocked(1, 1, 1, 1, 1, 1, 1, 1, 1, false), "single_tile", 1)
	case "matmul_single_tile_bfp8b":
		return mm(blocked(1, 1, 1, 1, 1, 1, 1, 1, 1, true), "single_tile", 1)
	case "matmul_large_block":
		return mm(blocked(2, 18, 18, 1, 2, 3, 4, 36, 36, false), "large_block", 36)
	case "matmul_single_core_small":
		return mm(blocked(4, 4, 4, 2, 4, 4, 16, 16, 16, false), "single_core", 16)
	case "matmul_single_core":
		return mm(blocked(16, 16, 192, 2, 4, 2, 64, 64, 256, false), "single_core", 256)
	case "matmul_large_block_hifi4_tilized_tilized":
		return mm(hifi4(false, false), "large_block_hifi4", 8)
	case "matmul_large_block_hifi4_rowmajor_input":
		return mm(hifi4(true, false), "large_block_hifi4", 8)
	case "matmul_large_block_hifi4_rowmajor_output":
		return mm(hifi4(false, true), "large_block_hifi4", 8)
	case "matmul_large_block_hifi4_rowmajor_rowmajor":
		return mm(hifi4(true, true), "large_block_hifi4", 8)
	case "eltwise_binary_add":
		return eltwise("binary", "ADD", 2048, 2, 2)
	case "eltwise_binary_mul":
		return eltwise("binary", "MUL", 2048, 2, 2)
	case "bcast_h_add":
		return eltwise("bcast_h", "ADD", 48, 2, 2)
	case "bcast_h_mul":
		return eltwise("bcast_h", "MUL", 48, 2, 2)
	case "bcast_w_add":
		return eltwise("bcast_w", "ADD", 48, 2, 2)
	case "bcast_w_mul":
		return eltwise("bcast_w", "MUL", 48, 2, 2)
	case "bcast_hw_add":
		return eltwise("bcast_hw", "ADD", 48, 2, 2)
	}
	check(fmt.Errorf("unknown kernel %q", name))
	return kernelSpec{}
}
