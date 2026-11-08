package core

// Wormhole Tensix numerics used by the TT data plane. These are ports of the
// bit-level reference behavior in tt-sim (src/tensix.cpp: fpu_accum_normalize_encode,
// elwadd, elwmul, mvmul, MOVA2D, PACR and UNPACR format conversion) restricted
// to the configuration the modelled kernels use: bf16/bfp8b sources, 16-bit
// destination in bf16 mode (fp32_dest_acc_en=false), no stochastic rounding.
//
// Representation: "src" and "dst" values are fp32 bit patterns. Values that
// live in a 16-bit destination always have their low 16 bits clear.

import (
	"math"
	"math/bits"
)

const ttTileElems = 1024

// ttFaceIndex returns the L1 (tile_nfaces) position of tile element (r, c).
func ttFaceIndex(r, c int) int {
	return ((r/16)*2+c/16)*256 + (r%16)*16 + c%16
}

// ---------------------------------------------------------------------------
// Unpacker: L1 format -> fp32 bits as held in SrcA/SrcB.

func ttBF16ToSrc(v uint16) uint32 { return uint32(v) << 16 }

// ttBFP8ToBF16 ports tt-sim bfp8_to_bf16.
func ttBFP8ToBF16(x uint8, exp uint8) uint16 {
	sign := uint32(x >> 7)
	mag := uint8(x << 1)
	if mag == 0 {
		if sign != 0 {
			return 0xFF80
		}
		return 0
	}
	lz := uint8(bits.LeadingZeros8(mag))
	mag <<= lz
	exp -= lz // may wrap, as in the reference
	return uint16(sign<<15) | uint16(exp)<<7 | uint16(mag&0x7E)
}

// ttFPUOperand converts a Src register value to the 19-bit FPU operand used by
// ELW*/MVMUL for bf16-family formats (low mantissa bits dropped).
func ttFPUOperand(v uint32) uint32 { return (v & 0xFFFF0000) >> 13 }

func ttFPUZero(v19 uint32) bool { return v19&0x3FFFF < 0x400 }

// ---------------------------------------------------------------------------
// FPU (bf16 destination mode: fp32_acc=false, fp_exponent_8b=true).

func ttAccumNormalizeEncode(signSop0 uint32, expSop0 int32, manSop0 uint32,
	signSop1 uint32, expSop1 int32, manSop1 uint32, dstValue uint32, fp32Acc, fpExp8b bool) uint32 {
	signDst := dstValue >> 31
	expDst := int32((dstValue >> 23) & 255)
	manDst := (dstValue & 0x7FFFFF) | 0x800000
	if expDst == 0 {
		manDst = 0
	}
	exp := expSop0
	if expSop1 > exp {
		exp = expSop1
	}
	if expDst > exp {
		exp = expDst
	}
	if exp <= 0 {
		return 0
	}
	align := func(man uint32, e int32, sign uint32) uint32 {
		if e >= exp {
			return man
		}
		d := uint32(exp - e)
		if d < 31 {
			return (man + (1 << (d - 1)) - sign) >> d
		}
		return 0
	}
	manSop0 = align(manSop0, expSop0, signSop0)
	manSop1 = align(manSop1, expSop1, signSop1)
	if expDst < exp {
		d := uint32(exp - expDst)
		if d < 31 {
			manDst = (manDst + (1 << (d - 1))) >> d
		} else {
			manDst = 0
		}
	}
	if !fp32Acc {
		manSop0 = (manSop0 + (1 << 12) - signSop0) &^ 0x1FFF
		manSop1 = (manSop1 + (1 << 12) - signSop1) &^ 0x1FFF
		manDst = (manDst + (1 << 12)) &^ 0x1FFF
	}
	sign := signDst
	man := int32(manDst)
	if signSop0 == sign {
		man += int32(manSop0)
	} else {
		man -= int32(manSop0)
	}
	if signSop1 == sign {
		man += int32(manSop1)
	} else {
		man -= int32(manSop1)
	}
	if man < 0 {
		man = -man
		sign ^= 1
	}
	if man == 0 {
		return 0
	}
	shift := int32(bits.LeadingZeros32(uint32(man))) - 8
	// Wormhole (TT_ARCH_VERSION == 0) renormalization quirk for -1.
	if sign != 0 && man == 1 {
		shift -= 27
	}
	exp -= shift
	if !fp32Acc {
		if fpExp8b {
			shift -= 16
		} else {
			shift -= 13
		}
	}
	if shift < 0 {
		s := -shift
		man = (man + (1 << (s - 1))) >> s
	} else {
		man <<= shift
	}
	if !fp32Acc {
		if fpExp8b {
			man <<= 16
		} else {
			man <<= 13
		}
	}
	if man&0x1000000 != 0 {
		exp++
	}
	m := uint32(man) & 0x7FFFFF
	if exp <= 0 {
		return 0
	}
	if fp32Acc || fpExp8b {
		if exp >= 255 {
			return sign<<31 | 255<<23
		}
		return sign<<31 | uint32(exp)<<23 | m
	}
	if exp >= 32 {
		return sign<<31 | 31<<23 | 0x3FF<<13
	}
	return sign<<31 | uint32(exp)<<23 | m
}

func ttExpProdAdj(fp32Acc bool, phase uint32, fpExp8b bool) int32 {
	adj := int32(-15)
	if fpExp8b {
		adj = -127
	} else if fp32Acc {
		adj = 127 - 30
	}
	if phase&1 != 0 {
		adj -= 5
	}
	if phase&2 != 0 {
		adj -= 7
	}
	return adj
}

func ttFidelityMantissas(manA, manB uint32, phase uint32) (uint32, uint32) {
	if phase&1 != 0 {
		manA = (manA >> 1) & 31
	} else {
		manA >>= 6
	}
	if phase&2 != 0 {
		manB = (manB & 15) << 3
	} else {
		manB >>= 4
	}
	return manA, manB
}

// ttElwMul ports tt-sim elwmul for one fidelity phase (dst accumulates).
func ttElwMul(a, b, dst uint32, phase uint32) uint32 {
	const fp32Acc, fpExp8b = false, true
	adj := ttExpProdAdj(fp32Acc, phase, fpExp8b)
	zeroExp := adj
	if zeroExp < 0 {
		zeroExp = 0
	}
	signA, signB := a>>18, b>>18
	expA, expB := (a>>10)&255, (b>>10)&255
	manA, manB := (a&0x3FF)|0x400, (b&0x3FF)|0x400
	expSop := int32(expA+expB) + adj
	manA, manB = ttFidelityMantissas(manA, manB, phase)
	signSop := (signA ^ signB) & 1
	manSop := (manA * manB) << 13
	if ttFPUZero(a) || ttFPUZero(b) || expSop <= 0 {
		signSop, expSop, manSop = 0, zeroExp, 0
	}
	return ttAccumNormalizeEncode(signSop, expSop, manSop, 0, 0, 0, dst, fp32Acc, fpExp8b)
}

// ttElwAdd ports tt-sim elwadd with dest_accum_en=false (dst ignored).
func ttElwAdd(a, b uint32) uint32 {
	const fp32Acc, fpExp8b = false, true
	if ttFPUZero(a) {
		a = 0
	}
	if ttFPUZero(b) {
		b = 0
	}
	if a&0x3FFFF < b&0x3FFFF {
		a, b = b, a
	}
	signA, signB := (a>>18)&1, (b>>18)&1
	expA, expB := int32((a>>10)&255), int32((b>>10)&255)
	var manA, manB uint32
	if a != 0 {
		manA = (a & 0x3FF) | 0x400
	}
	if b != 0 {
		manB = (b & 0x3FF) | 0x400
	}
	expSop := expA
	if expB < expSop {
		d := uint32(expSop - expB)
		if d < 12 {
			manB = (manB + (1 << (d - 1))) >> d
		} else {
			manB = 0
		}
	}
	var manSop uint32
	if signA == signB {
		manSop = manA + manB
	} else {
		manSop = manA - manB
	}
	manSop <<= 13
	return ttAccumNormalizeEncode(signA, expSop, manSop, 0, 0, 0, 0, fp32Acc, fpExp8b)
}

// ttMvMul ports tt-sim mvmul: dst += sum_i a[i]*b[i] over 16 lanes, where a
// holds SrcA (in1) and b holds SrcB (in0) 19-bit operands.
func ttMvMul(a, b *[16]uint32, dst uint32, phase uint32) uint32 {
	const fp32Acc, fpExp8b = false, true
	adj := ttExpProdAdj(fp32Acc, phase, fpExp8b)
	zeroExp := adj
	if zeroExp < 0 {
		zeroExp = 0
	}
	var signs [16]uint32
	var exps [16]int32
	var mans [16]uint32
	for i := 0; i < 16; i++ {
		x, y := a[i], b[i]
		if ttFPUZero(x) || ttFPUZero(y) {
			signs[i], exps[i], mans[i] = 0, zeroExp, 0
			continue
		}
		manA, manB := ttFidelityMantissas((x&0x3FF)|0x400, (y&0x3FF)|0x400, phase)
		signs[i] = ((x >> 18) ^ (y >> 18)) & 1
		exps[i] = int32((x>>10)&255) + int32((y>>10)&255) + adj
		mans[i] = manA * manB
	}
	var expSops [2]int32
	var manSops [2]uint32
	var signSops [2]uint32
	for g := 0; g < 2; g++ {
		e := exps[8*g]
		for i := 1; i < 8; i++ {
			if exps[8*g+i] > e {
				e = exps[8*g+i]
			}
		}
		if e <= 0 {
			continue
		}
		sop := int32(0)
		for i := 0; i < 8; i++ {
			lane := 8*g + i
			d := uint32(e - exps[lane])
			if d > 30 {
				d = 30
			}
			m := int32(((mans[lane] << 1) + (1 << d)) >> (d + 1))
			if signs[lane] != 0 {
				m = -m
			}
			sop += m
		}
		if sop < 0 {
			signSops[g] = 1
			sop = -sop
		}
		manSops[g] = uint32(sop) << 13
		expSops[g] = e
	}
	return ttAccumNormalizeEncode(signSops[0], expSops[0], manSops[0], signSops[1], expSops[1], manSops[1], dst, fp32Acc, fpExp8b)
}

// ttDst16 keeps the value a 16-bit bf16 destination stores.
func ttDst16(result uint32) uint32 { return result & 0xFFFF0000 }

// ttMovA2D: datacopy from SrcA into a 16-bit bf16 destination (copy_tile).
func ttMovA2D(v uint32) uint32 {
	if v&0x7F800000 == 0 { // flush denormals (Zero_Flag_disabled_src == 0)
		return 0
	}
	return v & 0xFFFF0000
}

// ---------------------------------------------------------------------------
// Packer: 16-bit bf16 destination -> L1.

// ttPackBF16 converts a destination value to an L1 bf16 word (intermediate
// format bf16): NaN saturates to Inf, denormals and signed zero flush to +0.
func ttPackBF16(dst uint32) uint16 {
	v := uint16(dst >> 16)
	switch {
	case v&0x7FFF > 0x7F80:
		return (v & 0x8000) | 0x7F80
	case v&0x7FFF < 0x80:
		return 0
	}
	return v
}

// ttPackBFP8Block encodes 16 destination values into one shared exponent and
// 16 sign-magnitude mantissa bytes (intermediate format bfp8: m6 rounding).
func ttPackBFP8Block(vals []uint32) (uint8, [16]uint8) {
	var buf [16]uint32
	for j := 0; j < 16; j++ {
		v := uint32(ttPackBF16(vals[j]))
		v = (v + 1) &^ 1 // reduce to m6 instead of m7
		buf[j] = v
	}
	maxExp := uint32(0)
	for j := 0; j < 16; j++ {
		if e := (buf[j] >> 7) & 255; e > maxExp {
			maxExp = e
		}
	}
	var out [16]uint8
	for j := 0; j < 16; j++ {
		s := buf[j] >> 15
		e := (buf[j] >> 7) & 255
		m := buf[j] & 127
		if e != 0 {
			m |= 128
		}
		shift := maxExp - e
		if shift >= 8 {
			m = 0
		} else {
			m = (m + (1 << shift)) >> (shift + 1)
			if m == 1<<7 {
				m = 1<<7 - 1
			}
		}
		if m != 0 {
			m |= s << 7
		}
		out[j] = uint8(m)
	}
	return uint8(maxExp), out
}

// ---------------------------------------------------------------------------
// SFPU (approx_mode=false). Load bf16 from dst as fp32, operate in fp32,
// store with denormal flush and bf16 truncation (SFPSTORE instr_mod0=2).
// exp/sigmoid/tanh use libm here rather than the LLK polynomial sequences,
// so they are judged against the gtest tolerance, not bit-exactness.

func ttSFPUStoreBF16(v uint32) uint32 {
	if v&0x7FFFFFFF < 0x800000 {
		v &= 0x80000000
	}
	return v & 0xFFFF0000
}

func ttSFPU(op string, dst uint32) uint32 {
	x := math.Float32frombits(dst)
	var y float32
	switch op {
	case "relu":
		if dst&0x80000000 != 0 {
			return 0
		}
		return dst
	case "exponential":
		y = float32(math.Exp(float64(x)))
	case "sigmoid":
		y = float32(1 / (1 + math.Exp(-float64(x))))
	case "tanh":
		y = float32(math.Tanh(float64(x)))
	default:
		panic("unsupported SFPU op " + op)
	}
	return ttSFPUStoreBF16(math.Float32bits(y))
}

// ttFidelityPhases returns the number of fidelity phases for a MathFidelity
// enum value (LoFi=0 -> 1, HiFi2=2, HiFi3=3, HiFi4=4).
func ttFidelityPhases(mathFidelity int) int {
	if mathFidelity <= 1 {
		return 1
	}
	return mathFidelity
}
