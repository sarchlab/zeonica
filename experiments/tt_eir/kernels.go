package main

// E-IR programs of the TT-Metal kernels. The reader/compute/writer kernels map
// onto the PEs of one cluster (see gen.go); the PE programs follow the TT
// kernel source loop by loop.

import "math"

// Latencies are opcode latencies (cycles) written to the operation latency
// sidecar, plus the NOP-loop delay costs of the Case B probes.
type Latencies struct {
	DRAMRead       int `yaml:"dram_read_tile"`
	DRAMWrite      int `yaml:"dram_write_tile"`
	Unpack         int `yaml:"unpack_tile"`
	Reload         int `yaml:"reload_tile"`
	Zero           int `yaml:"zero_tile"`
	Mov            int `yaml:"mov_tile"`
	Matmul         int `yaml:"matmul_tile"`
	Pack           int `yaml:"pack_tile"`
	Tilize         int `yaml:"tilize_tile"`
	Untilize       int `yaml:"untilize_page"`
	Copy           int `yaml:"copy_tile"`
	EltwiseBinary  int `yaml:"eltwise_binary_tile"`
	BcastH         int `yaml:"bcast_h_tile"`
	BcastW         int `yaml:"bcast_w_tile"`
	BcastHW        int `yaml:"bcast_hw_tile"`
	BcastMulAdjust int `yaml:"bcast_mul_adjust"`
	SFPU           int `yaml:"sfpu_tile"`
	CB             int `yaml:"cb_op"`
	// NOP-loop cost per iteration as seen on each stage's clock (TT-Sim's
	// progress clock retires several RISC-V steps per tick while compute runs).
	ReaderDelaySteps  float64 `yaml:"reader_delay_steps"`
	ComputeDelaySteps float64 `yaml:"compute_delay_steps"`
	WriterDelaySteps  float64 `yaml:"writer_delay_steps"`
	// Per-stage NOP-loop entry/exit cost (the loop also absorbs pipeline slack
	// in the model that TT-Sim's progress clock does not have).
	ReaderDelaySetup int `yaml:"reader_delay_setup"`
	// Per-subblock cost of each compute stage (dst acquire/release handshake
	// between the unpack, math and pack TRISCs); TT-Sim's time per subblock is
	// nearly independent of the tiles in it.
	UnpackSubblock     int `yaml:"unpack_subblock"`
	SingleCoreSubblock int `yaml:"single_core_subblock_extra"` // single-core gtests (reader_matmul_blocked / writer_unswizzle)
	ReloadInit         int `yaml:"reload_init"`                // copy_tile_to_dst_init_short before the partial reload
	MMInit             int `yaml:"mm_init_short"`              // mm_init_short after the reload (c24 already released)
	MathSubblock       int `yaml:"math_subblock"`
	PackSubblock       int `yaml:"pack_subblock"`
	// TT-Sim progress clock (blocked matmul): reader/writer work in steps per
	// tile, shortened by progress credits from the compute bursts (per subblock).
	ReaderWorkPerTile     float64 `yaml:"reader_work_per_tile"`
	WriterWorkPerTile     float64 `yaml:"writer_work_per_tile"`
	ComputeBurst          int     `yaml:"compute_burst_steps"`
	FirstBurstExtra       int     `yaml:"first_burst_extra"`
	ReleaseBeforeFraction float64 `yaml:"release_before_fraction"`
	UnpackProgressSteps   int     `yaml:"unpack_progress_steps"`
	ReloadProgressSteps   int     `yaml:"reload_progress_steps"`
	PackBurstSteps        int     `yaml:"pack_burst_steps"`
	ResumeWork            int     `yaml:"resume_work"`
	ComputeDelaySetup     int     `yaml:"compute_delay_setup"`
	WriterDelaySetup      int     `yaml:"writer_delay_setup"`
}

// Fixed are setup terms outside the PE programs (added to the endpoint).
type Fixed struct {
	Dispatch          float64            `yaml:"dispatch"`
	ClassFixed        map[string]float64 `yaml:"class_fixed"`
	MulticoreBase     float64            `yaml:"multicore_base"`
	MulticoreLine     float64            `yaml:"multicore_line"`
	MulticoreMesh     float64            `yaml:"multicore_mesh"`
	SFPUDispatch      float64            `yaml:"sfpu_dispatch"`
	ProgramWindowBase float64            `yaml:"program_window_base"`
}

type Profile struct {
	Latencies Latencies `yaml:"latencies"`
	Fixed     Fixed     `yaml:"fixed"`
}

func delayCycles(iterations int, steps float64, setup int) int {
	if iterations <= 0 {
		return 0
	}
	return int(math.Round(float64(iterations)*steps)) + setup
}

// opcodeLatencies is the operation latency sidecar of one run; kernel-specific
// latencies (work sizes, NOP-loop probes) override the profile's.
func (pr Profile) opcodeLatencies(readerDelay, computeDelay, writerDelay int, kernel map[string]int) map[string]int {
	l := pr.Latencies
	m := map[string]int{
		"CB_WAIT": l.CB, "CB_POP": l.CB,
		"TT_DRAM_READ_TILE": l.DRAMRead, "TT_DRAM_WRITE_TILE": l.DRAMWrite,
		"TT_UNPACK_BF16": l.Unpack, "TT_UNPACK_BFP8": l.Unpack,
		"TT_PACK_BF16": l.Pack, "TT_PACK_BFP8": l.Pack,
		"TT_RELOAD": l.Reload, "TT_ZERO_TILE": l.Zero, "TT_MOV_TILE": l.Mov,
		"TT_MATMUL_BF16": l.Matmul, "TT_TILIZE": l.Tilize, "TT_UNTILIZE": l.Untilize, "TT_COPY_TILE": l.Copy,
		"TT_DELAY_READER":    delayCycles(readerDelay, l.ReaderDelaySteps, l.ReaderDelaySetup),
		"TT_DELAY_COMPUTE":   delayCycles(computeDelay, l.ComputeDelaySteps, l.ComputeDelaySetup),
		"TT_DELAY_WRITER":    delayCycles(writerDelay, l.WriterDelaySteps, l.WriterDelaySetup),
		"TT_PROGRESS_CREDIT": l.CB, "CB_RESERVE": l.CB, "TT_RESUME_WORK": l.ResumeWork,
		"TT_SUBBLOCK_UNPACK": l.UnpackSubblock, "TT_RELOAD_INIT": l.ReloadInit, "TT_MM_INIT": l.MMInit, "TT_SUBBLOCK_MATH": l.MathSubblock, "TT_SUBBLOCK_PACK": l.PackSubblock,
	}
	for _, op := range []string{"ADD", "SUB", "MUL"} {
		adj := 0
		if op == "MUL" {
			adj = l.BcastMulAdjust
		}
		m["TT_ELTWISE_"+op] = l.EltwiseBinary
		m["TT_BCAST_H_"+op] = l.BcastH + adj
		m["TT_BCAST_W_"+op] = l.BcastW + adj
		m["TT_BCAST_HW_"+op] = l.BcastHW + adj
	}
	for _, op := range []string{"RELU", "EXP", "SIGMOID", "TANH"} {
		m["TT_SFPU_"+op] = l.SFPU
	}
	for op, v := range kernel {
		m[op] = v
	}
	for op, v := range m {
		if v <= 0 {
			delete(m, op) // unused (e.g. no NOP loop); the sidecar requires > 0
		}
	}
	return m
}

// feed lists the DRAM pages the host streams into one reader port, in read order.
type feed struct {
	x, y  int
	color string
	pages []int // page index into the input DRAM image
	in    int   // 0: in0, 1: in1
}

// drain lists the DRAM pages one writer produces, in write order.
type drain struct {
	x, y  int
	pages []int
}

type build struct {
	prog    *eirProgram
	feeds   []feed
	drains  []drain
	latency map[string]int // kernel-specific opcode latencies
	// Instrumentation: CB name of a CB operation, keyed "x,y,op,port,color";
	// pool reservations of a pack PE in program order.
	labels     map[string]string
	reserveSeq map[[2]int][]string
	blockTiles map[string]int // CB publication block size (tiles)
}

// MatmulKernel: matmul_large_block_zm (blocked, partial spill/reload through
// c24) and the HiFi4 matmul_large_block variants. All blocked matmuls use
// credit-based CBs and TT-Sim progress-clock reader/writer work; the work and
// credit amounts are per tile, so every kernel shares the same constants.
type MatmulKernel struct {
	Rows, Cols                      int
	MTiles, NTiles, SubM, SubN      int
	KWidth, Blocks                  int
	BFP8                            bool
	ACap, BCap, SharedCap           int
	In0Page, In1Page                func(r, c, k, i int) int
	OutPage                         func(r, c, n int) int
	ActivationRowMajor, OutRowMajor bool
	ReaderDelay, ComputeDelay       int
	WriterDelay                     int
	SingleCoreGtest                 bool // single-core reader/writer kernels: per-subblock extra
}

func (k MatmulKernel) build(l Latencies) *build {
	b := &build{prog: &eirProgram{width: 3 * k.Cols, height: 2 * k.Rows}, latency: map[string]int{}, labels: map[string]string{}, reserveSeq: map[[2]int][]string{}}
	sT := k.SubM * k.SubN
	aT, bT := k.MTiles*k.KWidth, k.KWidth*k.NTiles
	b.blockTiles = map[string]int{"c0": aT, "c1": bT, "c24": sT, "c16": sT}
	writeUnit := sT
	if k.OutRowMajor {
		writeUnit = 1
	}
	b.latency["TT_PROGRESS_WORK_READER"] = round(l.ReaderWorkPerTile*float64(aT+bT)) + delayCycles(k.ReaderDelay, l.ReaderDelaySteps, l.ReaderDelaySetup)
	b.latency["TT_PROGRESS_WORK_WRITER"] = round(l.WriterWorkPerTile*float64(writeUnit)) + delayCycles(k.WriterDelay, l.WriterDelaySteps, l.WriterDelaySetup)
	b.latency["TT_DELAY_COMPUTE"] = delayCycles(k.ComputeDelay, l.ComputeDelaySteps, l.ComputeDelaySetup)
	if k.SingleCoreGtest {
		b.latency["TT_SUBBLOCK_UNPACK"] = l.UnpackSubblock + l.SingleCoreSubblock
	}
	for r := 0; r < k.Rows; r++ {
		for c := 0; c < k.Cols; c++ {
			k.core(b, l, r, c)
		}
	}
	return b
}

func round(x float64) int { return int(math.Round(x)) }

func (k MatmulKernel) core(b *build, l Latencies, r, c int) {
	e := b.prog
	cl := e.cluster(r, c)
	R, U, M, P, W := cl.reader, cl.unpack, cl.math, cl.pack, cl.writer
	sT := k.SubM * k.SubN
	nSubN := k.NTiles / k.SubN
	nSub := (k.MTiles / k.SubM) * nSubN
	aT, bT := k.MTiles*k.KWidth, k.KWidth*k.NTiles
	unpackIn, packOut := "TT_UNPACK_BF16", "TT_PACK_BF16"
	if k.BFP8 {
		unpackIn, packOut = "TT_UNPACK_BFP8", "TT_PACK_BFP8"
	}
	hifi4 := k.ActivationRowMajor || k.OutRowMajor
	pool := !hifi4 // c16 and c24 share one credit pool (matmul_large_block_zm)
	M.loop = !hifi4
	label := func(pe *peProgram, op, port, color, cb string) {
		b.labels[fmtS("%d,%d,%s,%s,%s", pe.x, pe.y, op, port, color)] = cb
	}

	// Circular buffers = receive queues of the consumer ports.
	e.cap(U, "West", "R", k.ACap, "c0")
	e.cap(U, "West", "Y", k.BCap, "c1")
	e.cap(M, "West", "R", 2, "srcA")
	e.cap(M, "West", "Y", 2, "srcB")
	e.cap(M, "West", "B", sT, "dst init")
	e.cap(P, "NorthEast", "R", 2*sT, "dst")
	e.cap(W, "East", "R", k.SharedCap, "c16")
	switch {
	case k.ActivationRowMajor:
		e.cap(U, "South", "R", aT, "c24 (tilized in0)")
	case !k.OutRowMajor:
		e.cap(U, "South", "R", k.SharedCap, "c24")
	}
	if k.OutRowMajor {
		e.cap(U, "South", "Y", k.SubM*k.NTiles, "c26")
	}
	for _, x := range [][5]string{{"CB_WAIT", "West", "R", "c0"}, {"CB_POP", "West", "R", "c0"}, {"CB_WAIT", "West", "Y", "c1"}, {"CB_POP", "West", "Y", "c1"},
		{"CB_WAIT", "South", "R", "c24"}, {"CB_POP", "South", "R", "c24"}} {
		label(U, x[0], x[1], x[2], x[3])
	}
	label(W, "CB_WAIT", "East", "R", "c16")
	label(W, "CB_POP", "East", "R", "c16")
	label(R, "CB_RESERVE", "East", "R", "c0")
	label(R, "CB_RESERVE", "East", "Y", "c1")

	// Credits: free space of each CB at its producer, returned by CB_POP.
	e.credits = append(e.credits, credit{R.x, R.y, "East", "R", k.ACap}, credit{R.x, R.y, "East", "Y", k.BCap})
	credits := []struct {
		pe          *peProgram
		side, color string
	}{{R, "East", "R"}, {R, "East", "Y"}, {R, "East", "B"}, {R, "SouthEast", "B"}, {W, "NorthEast", "B"}, {W, "East", "B"}}
	if pool {
		e.credits = append(e.credits, credit{P.x, P.y, "North", "R", k.SharedCap})
		credits = append(credits, struct {
			pe          *peProgram
			side, color string
		}{P, "North", "R"}, struct {
			pe          *peProgram
			side, color string
		}{P, "West", "R"})
	}
	for _, q := range credits {
		// Credit queues act as counters: never a source of backpressure.
		e.cap(q.pe, q.side, q.color, 1<<16, "credits")
	}
	// Progress credits: unpack -> reader (West B), writer (SouthWest B);
	// pack -> reader (NorthWest B), writer (West B).
	e.progress = append(e.progress, queueCap{x: R.x, y: R.y, side: "East", color: "B"}, queueCap{x: R.x, y: R.y, side: "SouthEast", color: "B"},
		queueCap{x: W.x, y: W.y, side: "NorthEast", color: "B"}, queueCap{x: W.x, y: W.y, side: "East", color: "B"})
	sendCredit := func(pe *peProgram, steps int, dsts ...operand) {
		if steps > 0 {
			pe.op("TT_PROGRESS_CREDIT", []operand{imm(steps)}, dsts...)
		}
	}
	fromU := []operand{p("West", "B"), p("SouthWest", "B")}
	fromP := []operand{p("NorthWest", "B"), p("West", "B")}
	popTo := func(port, color string, credit bool) []operand {
		if credit {
			return []operand{p(port, color)}
		}
		return nil
	}

	// Reader (NCRISC): one K block per loop iteration.
	R.note(fmtS("cb_reserve_back(c0, %d); cb_reserve_back(c1, %d)", aT, bT))
	R.op("CB_RESERVE", []operand{p("East", "R"), imm(aT)})
	R.op("CB_RESERVE", []operand{p("East", "Y"), imm(bT)})
	R.note("block read work (incl. NOP-loop probe), progress-clock steps")
	R.op("TT_PROGRESS_WORK_READER", nil)
	R.note("noc reads complete: push the in0 block into c0")
	for i := 0; i < aT; i++ {
		R.op("TT_DRAM_READ_TILE", []operand{p("West", "R")}, p("East", "R"))
	}
	R.note("push the in1 block into c1")
	for i := 0; i < bT; i++ {
		R.op("TT_DRAM_READ_TILE", []operand{p("West", "Y")}, p("East", "Y"))
	}
	f0 := feed{x: R.x, y: R.y, color: "R", in: 0}
	f1 := feed{x: R.x, y: R.y, color: "Y", in: 1}
	for blk := 0; blk < k.Blocks; blk++ {
		for i := 0; i < aT; i++ {
			f0.pages = append(f0.pages, k.In0Page(r, c, blk, i))
		}
		for i := 0; i < bT; i++ {
			f1.pages = append(f1.pages, k.In1Page(r, c, blk, i))
		}
	}
	b.feeds = append(b.feeds, f0, f1)

	// Compute kernel: unpack (U), math (M), pack (P). Math runs the same code
	// for every subblock (the unpacker feeds zero or reloaded partials as the
	// dst init), so its program is one subblock body in a loop.
	mathSubblock := func() {
		M.note("tile_regs_acquire: dst acquired for this subblock (token to unpack, which reloads partials into it)")
		M.op("TT_SUBBLOCK_MATH", nil, p("West", "B"))
		for t := 0; t < sT; t++ {
			M.note(fmtS("output tile %d of the subblock: dst = init (zero or reloaded) + sum over the K block of A*B", t))
			delay := t == sT-1 && k.ComputeDelay > 0
			for kk := 0; kk < k.KWidth; kk++ {
				acc, dst := p("$1", "R"), p("$1", "R")
				if kk == 0 {
					acc = p("West", "B")
				}
				if kk == k.KWidth-1 && !delay {
					dst = p("SouthWest", "R")
				}
				M.op("TT_MATMUL_BF16", []operand{acc, p("West", "R"), p("West", "Y")}, dst)
			}
			if delay {
				M.note("NOP loop on the math TRISC before tile_regs_commit (Case B compute probe)")
				M.op("TT_DELAY_COMPUTE", nil)
				M.op("TT_MOV_TILE", []operand{p("$1", "R")}, p("SouthWest", "R"))
			}
		}
	}
	if !hifi4 {
		mathSubblock()
	}
	burstBase := float64(l.ComputeBurst)
	acquired := 0 // dst-acquire tokens consumed by unpack
	e.cap(U, "East", "B", nSub+2, "dst acquire tokens")
	for blk := 0; blk < k.Blocks; blk++ {
		U.note(fmtS("block %d: cb_wait_front(c0, %d)", blk, aT))
		U.op("CB_WAIT", []operand{p("West", "R"), imm(aT)})
		act, actFmt := "West", unpackIn
		actColor := "R"
		if k.ActivationRowMajor {
			for row := 0; row < k.MTiles; row++ {
				for j := 0; j < k.KWidth; j++ {
					U.note(fmtS("tilize_block row %d tile %d", row, j))
					U.op("TT_TILIZE", []operand{at("West", 2*row, "R"), at("West", 2*row+1, "R"), imm(j)}, p("East", "R"))
					M.op("TT_COPY_TILE", []operand{p("West", "R")}, p("SouthWest", "R"))
					P.op("TT_PACK_BF16", []operand{p("NorthEast", "R")}, p("North", "R"))
				}
			}
			U.op("CB_POP", []operand{p("West", "R"), imm(aT)}, popTo("West", "R", true)...)
			U.note("cb_wait_front(c24, tilized in0)")
			U.op("CB_WAIT", []operand{p("South", "R"), imm(aT)})
			act, actFmt = "South", "TT_UNPACK_BF16"
		}
		U.note(fmtS("cb_wait_front(c1, %d)", bT))
		U.op("CB_WAIT", []operand{p("West", "Y"), imm(bT)})
		if blk > 0 {
			U.note("resume after waiting for the reader (paid only if a wait above blocked)")
			U.op("TT_RESUME_WORK", nil)
		}
		final := blk == k.Blocks-1
		for s := 0; s < nSub; s++ {
			sm, sn := s/nSubN, s%nSubN
			if hifi4 {
				mathSubblock()
			}
			U.note(fmtS("subblock %d: per-subblock unpack handshake", s))
			U.op("TT_SUBBLOCK_UNPACK", nil)
			if blk > 0 {
				g := blk*nSub + s + 1 // dst-acquire tokens math has sent once it starts this subblock
				U.note(fmtS("subblock %d: wait until math acquired dst, then reload partials (copy_tile from c24)", s))
				U.op("CB_WAIT", []operand{p("East", "B"), imm(g - acquired)})
				U.op("CB_POP", []operand{p("East", "B"), imm(g - acquired)})
				acquired = g
				U.op("CB_WAIT", []operand{p("South", "R"), imm(sT)})
				U.op("TT_RELOAD_INIT", nil)
				for t := 0; t < sT; t++ {
					U.op("TT_RELOAD", []operand{at("South", t, "R")}, p("East", "B"))
				}
				U.op("CB_POP", []operand{p("South", "R"), imm(sT)}, popTo("South", "R", pool)...)
				sendCredit(U, l.ReloadProgressSteps, fromU...)
				U.op("TT_MM_INIT", nil)
			}
			if blk > 0 {
				U.note("unpack the in0/in1 tiles of every output tile of the subblock")
			} else {
				U.note("unpack the in0/in1 tiles of every output tile; the tile's zero dst init travels in the same group")
			}
			for t := 0; t < sT; t++ {
				m, n := sm*k.SubM+t/k.SubN, sn*k.SubN+t%k.SubN
				for kk := 0; kk < k.KWidth; kk++ {
					ops := []yop{mk(actFmt, []operand{at(act, m*k.KWidth+kk, actColor)}, p("East", "R")),
						mk(unpackIn, []operand{at("West", kk*k.NTiles+n, "Y")}, p("East", "Y"))}
					if blk == 0 && kk == 0 {
						ops = append(ops, mk("TT_ZERO_TILE", nil, p("East", "B")))
					}
					U.group(ops...)
				}
			}
			burst := round(burstBase)
			if blk == 0 && s == 0 {
				burst += l.FirstBurstExtra
			}
			if blk > 0 {
				burst += l.UnpackProgressSteps
			}
			before := burst
			if s == nSub-1 {
				before = round(l.ReleaseBeforeFraction * burstBase)
				if blk > 0 {
					before += l.UnpackProgressSteps
				}
			}
			U.note("compute burst progress credit")
			sendCredit(U, before, fromU...)
			if s == nSub-1 {
				U.note("cb_pop_front(in0, in1)")
				U.op("CB_POP", []operand{p(act, actColor), imm(aT)}, popTo(act, actColor, act == "West")...)
				U.op("CB_POP", []operand{p("West", "Y"), imm(bT)}, popTo("West", "Y", true)...)
			}
			sendCredit(U, burst-before, fromU...)
			packCredit := l.PackBurstSteps
			switch {
			case final && k.OutRowMajor:
				P.note(fmtS("subblock %d: tile_regs_wait, pack to c26", s))
				P.op("CB_WAIT", []operand{p("NorthEast", "R"), imm(sT)})
				P.op("TT_SUBBLOCK_PACK", nil)
				for t := 0; t < sT; t++ {
					P.op("TT_PACK_BF16", []operand{p("NorthEast", "R")}, p("North", "Y"))
				}
				sendCredit(P, packCredit, fromP...)
				if sn == nSubN-1 {
					rowTiles := k.SubM * k.NTiles
					U.note("reblock_and_untilize: cb_wait_front(c26)")
					U.op("CB_WAIT", []operand{p("South", "Y"), imm(rowTiles)})
					for h := 0; h < k.SubM; h++ {
						for j := 0; j < nSubN; j++ {
							U.op("TT_RELOAD", []operand{at("South", j*sT+h*k.SubN, "Y")}, p("East", "R"))
							M.op("TT_MOV_TILE", []operand{p("West", "R")}, p("SouthWest", "R"))
						}
						P.note(fmtS("untilize tile row %d", h))
						for j := 0; j < 2; j++ {
							P.op("TT_UNTILIZE", []operand{at("NorthEast", 0, "R"), at("NorthEast", 1, "R"), imm(j)}, p("West", "R"))
						}
						P.op("CB_POP", []operand{p("NorthEast", "R"), imm(2)})
					}
					U.op("CB_POP", []operand{p("South", "Y"), imm(rowTiles)})
				}
			default:
				cb, dst, out := "c24", p("North", "R"), "TT_PACK_BF16"
				if final {
					cb, dst, out = "c16", p("West", "R"), packOut
				}
				P.note(fmtS("block %d subblock %d: tile_regs_wait (whole subblock committed), pack to %s", blk, s, cb))
				P.op("CB_WAIT", []operand{p("NorthEast", "R"), imm(sT)})
				P.op("TT_SUBBLOCK_PACK", nil)
				if pool {
					P.op("CB_RESERVE", []operand{p("North", "R"), p("West", "R"), imm(sT)})
					b.reserveSeq[[2]int{P.x, P.y}] = append(b.reserveSeq[[2]int{P.x, P.y}], cb)
				}
				for t := 0; t < sT; t++ {
					P.op(out, []operand{p("NorthEast", "R")}, dst)
				}
				sendCredit(P, packCredit, fromP...)
			}
		}
	}

	// Writer (BRISC): one output subblock (or one row-major page) per iteration.
	d := drain{x: W.x, y: W.y}
	unit := sT
	if k.OutRowMajor {
		unit = 1
	}
	W.note(fmtS("cb_wait_front(c16, %d)", unit))
	W.op("CB_WAIT", []operand{p("East", "R"), imm(unit)})
	W.note("write work (incl. NOP-loop probe), progress-clock steps")
	W.op("TT_PROGRESS_WORK_WRITER", nil)
	for t := 0; t < unit; t++ {
		W.op("TT_DRAM_WRITE_TILE", []operand{at("East", t, "R")}, p("West", "R"))
	}
	W.op("CB_POP", []operand{p("East", "R"), imm(unit)}, popTo("East", "R", pool)...)
	if k.OutRowMajor {
		for x := 0; x < 2*k.MTiles; x++ {
			d.pages = append(d.pages, x)
		}
	} else {
		for x := 0; x < nSub*sT; x++ {
			d.pages = append(d.pages, k.OutPage(r, c, x))
		}
	}
	b.drains = append(b.drains, d)
}

// EltwiseKernel: binary, broadcast and SFPU unary tile streams on one core.
type EltwiseKernel struct {
	Tiles         int
	Kind          string // binary, bcast_h, bcast_w, bcast_hw, sfpu
	Op            string // ADD, SUB, MUL; or RELU, EXP, SIGMOID, TANH for sfpu
	Ht, Wt        int
	InCap, OutCap int
	In1Page       func(i int) int
}

func (k EltwiseKernel) build(Latencies) *build {
	b := &build{prog: &eirProgram{width: 3, height: 2}, latency: map[string]int{}, labels: map[string]string{}, reserveSeq: map[[2]int][]string{}}
	e := b.prog
	cl := e.cluster(0, 0)
	R, U, M, P, W := cl.reader, cl.unpack, cl.math, cl.pack, cl.writer
	unary := k.Kind == "sfpu"
	reuse := k.Kind == "bcast_w"
	e.cap(U, "West", "R", k.InCap, "c0")
	e.cap(M, "West", "R", 2, "srcA")
	e.cap(P, "NorthEast", "R", 2, "dst")
	e.cap(W, "East", "R", k.OutCap, "c16")
	if !unary {
		e.cap(U, "West", "Y", k.InCap, "c1")
		e.cap(M, "West", "Y", 2, "srcB")
	}
	f0 := feed{x: R.x, y: R.y, color: "R", in: 0}
	f1 := feed{x: R.x, y: R.y, color: "Y", in: 1}
	for i := 0; i < k.Tiles; i++ {
		f0.pages = append(f0.pages, i)
		if !unary && (!reuse || i%k.Wt == 0) {
			f1.pages = append(f1.pages, k.In1Page(i))
		}
	}
	b.feeds = append(b.feeds, f0)
	if !unary {
		b.feeds = append(b.feeds, f1)
	}
	d := drain{x: W.x, y: W.y}
	for i := 0; i < k.Tiles; i++ {
		d.pages = append(d.pages, i)
	}
	b.drains = append(b.drains, d)

	math := "TT_ELTWISE_" + k.Op
	if k.Kind != "binary" && !unary {
		math = map[string]string{"bcast_h": "TT_BCAST_H_", "bcast_w": "TT_BCAST_W_", "bcast_hw": "TT_BCAST_HW_"}[k.Kind] + k.Op
	}
	if reuse {
		// bcast_w: one in1 tile per output row, reused across the row.
		R.note("per output row: read the row's in1 tile, then the in0 tiles")
		R.op("TT_DRAM_READ_TILE", []operand{p("West", "Y")}, p("East", "Y"))
		U.note("per output row: unpack in0 tiles against the resident in1 tile")
		M.note("per output row")
		P.note("per output row")
		for j := 0; j < k.Wt; j++ {
			R.op("TT_DRAM_READ_TILE", []operand{p("West", "R")}, p("East", "R"))
			U.op("TT_UNPACK_BF16", []operand{p("West", "R")}, p("East", "R"))
			U.op("TT_UNPACK_BF16", []operand{at("West", 0, "Y")}, p("East", "Y"))
			M.op(math, []operand{p("West", "R"), p("West", "Y")}, p("SouthWest", "R"))
			P.op("TT_PACK_BF16", []operand{p("NorthEast", "R")}, p("West", "R"))
		}
		U.op("CB_POP", []operand{p("West", "Y"), imm(1)})
	} else {
		R.note("per tile")
		R.op("TT_DRAM_READ_TILE", []operand{p("West", "R")}, p("East", "R"))
		U.note("per tile")
		U.op("TT_UNPACK_BF16", []operand{p("West", "R")}, p("East", "R"))
		M.note("per tile")
		if unary {
			M.op("TT_SFPU_"+k.Op, []operand{p("West", "R")}, p("SouthWest", "R"))
		} else {
			R.op("TT_DRAM_READ_TILE", []operand{p("West", "Y")}, p("East", "Y"))
			U.op("TT_UNPACK_BF16", []operand{p("West", "Y")}, p("East", "Y"))
			M.op(math, []operand{p("West", "R"), p("West", "Y")}, p("SouthWest", "R"))
		}
		P.note("per tile")
		P.op("TT_PACK_BF16", []operand{p("NorthEast", "R")}, p("West", "R"))
	}
	W.note("per tile")
	W.op("TT_DRAM_WRITE_TILE", []operand{p("East", "R")}, p("West", "R"))
	for _, pe := range []*peProgram{R, U, M, P, W} {
		pe.loop = true
	}
	return b
}
