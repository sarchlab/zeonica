// Command tt_eir runs TT-Metal kernels as E-IR programs on the common Zeonica
// Driver/Core path: the PE programs are E-IR YAML loaded with
// core.LoadProgramFileFromYAML, opcode timing comes from the operation latency
// sidecar, DRAM is the Driver's data feed/collect, and circular buffers are
// the cores' receive queues. TT-Sim input DRAM images are fed in and the output
// DRAM image is compared with TT-Sim's readback.
package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
	"gopkg.in/yaml.v3"
)

func fmtS(f string, a ...any) string { return fmt.Sprintf(f, a...) }

type summary struct {
	Kernel          string             `json:"kernel"`
	Path            string             `json:"execution_path"`
	PEs             int                `json:"pes"`
	Instructions    int                `json:"eir_instructions"`
	CBQueues        int                `json:"cb_queues"`
	EIRCycles       float64            `json:"eir_cycles"`
	FixedCycles     float64            `json:"fixed_cycles"`
	ZeonicaCycles   float64            `json:"zeonica_cycles"`
	ProgramWindow   float64            `json:"program_window,omitempty"`
	Data            dataCheck          `json:"data_check"`
	Passed          bool               `json:"passed"`
	WallSeconds     float64            `json:"wall_seconds"`
	FixedComponents map[string]float64 `json:"fixed_components"`
}

type dataCheck struct {
	Source    string  `json:"source"`
	Mode      string  `json:"mode"`
	Elements  int     `json:"elements"`
	BitExact  int     `json:"bit_exact"`
	WithinTol int     `json:"within_tolerance"`
	MaxULP    int     `json:"max_ulp"`
	Fraction  float64 `json:"bit_exact_fraction"`
	Passed    bool    `json:"passed"`
}

func readFirst(dir string, names ...string) []byte {
	for _, n := range names {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil {
			return b
		}
	}
	return nil
}

func main() {
	kernel := flag.String("kernel", "", "kernel name (or caseb)")
	cfgPath := flag.String("config", "", "timing profile (YAML)")
	dataDir := flag.String("data-dir", "", "TT-Sim dump directory (in0/in1/out or input_a/input_b/output .u32)")
	sumPath := flag.String("summary-json", "", "summary output")
	outPath := flag.String("output-u32", "", "Zeonica output DRAM image")
	eirPath := flag.String("eir", "", "write the E-IR program here (default: temp file)")
	stateDir := flag.String("state-dir", "", "write the internal-state log (CB pages and events per TT core) here")
	a := flag.Int("a", 16, "Case B: c0 capacity (tiles)")
	b := flag.Int("b", 16, "Case B: c1 capacity (tiles)")
	shared := flag.Int("shared", 16, "Case B: c16/c24 shared capacity (tiles)")
	stage := flag.String("stage", "none", "Case B: reader|compute|writer NOP-loop stage")
	delay := flag.Int("delay", 0, "Case B: NOP-loop iterations")
	flag.Parse()
	slog.SetLogLoggerLevel(slog.LevelWarn)

	var prof Profile
	raw, err := os.ReadFile(*cfgPath)
	check(err)
	check(yaml.Unmarshal(raw, &prof))
	ref := readFirst(*dataDir, "out.u32", "output.u32")
	ins := [2][]byte{readFirst(*dataDir, "in0.u32", "input_a.u32"), readFirst(*dataDir, "in1.u32", "input_b.u32")}
	if ins[0] == nil || ref == nil {
		check(fmt.Errorf("missing TT-Sim dump in %s", *dataDir))
	}
	spec := lookup(*kernel, caseB{a: *a, b: *b, shared: *shared, stage: *stage, delay: *delay})
	bd := spec.build(prof.Latencies)

	// E-IR program and latency sidecar.
	dir, err := os.MkdirTemp("", "tt_eir")
	check(err)
	defer os.RemoveAll(dir)
	progFile := *eirPath
	if progFile == "" {
		progFile = filepath.Join(dir, "program.yaml")
	}
	check(os.WriteFile(progFile, []byte(bd.prog.yaml()), 0o644))
	rd, cd, wd := 0, 0, 0
	switch *stage {
	case "reader":
		rd = *delay
	case "compute":
		cd = *delay
	case "writer":
		wd = *delay
	}
	latFile := filepath.Join(dir, "latency.yaml")
	lat, err := yaml.Marshal(map[string]any{"default_latency": 1, "opcodes": prof.opcodeLatencies(rd, cd, wd, bd.latency)})
	check(err)
	check(os.WriteFile(latFile, lat, 0o644))
	check(os.Setenv("ZEONICA_OPERATION_LATENCY_FILE", latFile))
	if os.Getenv("ZEONICA_DATA_COLLECT_TIMEOUT_CYCLES") == "" {
		check(os.Setenv("ZEONICA_DATA_COLLECT_TIMEOUT_CYCLES", "5000000"))
	}

	start := time.Now()
	cycles, out := run(bd, progFile, ins, spec.inPageBytes, spec.outPageBytes, len(ref), *stateDir)
	fixed, parts := spec.fixed(prof.Fixed)
	s := summary{Kernel: *kernel, Path: "E-IR YAML -> LoadProgramFileFromYAML -> Driver.MapProgram -> Core/Akita",
		PEs: len(bd.prog.pes), CBQueues: len(bd.prog.caps), EIRCycles: cycles, FixedCycles: fixed, ZeonicaCycles: cycles + fixed,
		WallSeconds: time.Since(start).Seconds(), FixedComponents: parts}
	for _, pe := range bd.prog.pes {
		s.Instructions += len(pe.ops)
	}
	if spec.caseB {
		s.ProgramWindow = cycles + prof.Fixed.ProgramWindowBase
	}
	s.Data = compare(out, ref, spec)
	s.Data.Source = *dataDir
	s.Passed = s.Data.Passed
	if *outPath != "" {
		check(os.WriteFile(*outPath, out, 0o644))
	}
	if *sumPath != "" {
		writeJSON(*sumPath, s)
	}
	fmt.Printf("kernel=%s pes=%d instructions=%d eir_cycles=%.0f zeonica_cycles=%.2f bit_exact=%d/%d passed=%t wall=%.1fs\n",
		s.Kernel, s.PEs, s.Instructions, s.EIRCycles, s.ZeonicaCycles, s.Data.BitExact, s.Data.Elements, s.Passed, s.WallSeconds)
	if !s.Passed {
		os.Exit(1)
	}
}

var sides = map[string]cgra.Side{"North": cgra.North, "East": cgra.East, "South": cgra.South, "West": cgra.West,
	"NorthEast": cgra.NorthEast, "NorthWest": cgra.NorthWest, "SouthEast": cgra.SouthEast, "SouthWest": cgra.SouthWest}

// run maps the E-IR program onto a Zeonica device and streams DRAM pages
// through the Driver. It returns the cycle the last output page left the
// writer, the output DRAM image and the per-page output cycles.
func run(bd *build, progFile string, ins [2][]byte, inPageBytes, outPageBytes, outBytes int, stateDir string) (float64, []byte) {
	engine := sim.NewSerialEngine()
	driver := api.DriverBuilder{}.WithEngine(engine).WithFreq(1*sim.GHz).WithPortBufferDepth(1, 1).Build("Driver")
	builder := config.DeviceBuilder{}.WithEngine(engine).WithFreq(1*sim.GHz).
		WithWidth(bd.prog.width).WithHeight(bd.prog.height).
		WithCorePortBufferDepth(1, 1).WithEnableFIFOModel(true)
	var st *stateLog
	if stateDir != "" {
		var watches []core.QueueWatchSpec
		st, watches = newStateLog(bd)
		builder = builder.WithEnableQueueWatches(true).WithQueueWatches(watches)
		core.SetTraceObserver(func(o core.TraceObservation) {
			if o.Msg == "Queue" {
				st.onQueue(o)
			}
		})
		core.TTCBObserver = st.onCB
		defer func() { core.SetTraceObserver(nil); core.TTCBObserver = nil }()
	}
	device := builder.Build("Device")
	driver.RegisterDevice(device)
	if st != nil {
		st.dev = device
	}

	programs := core.LoadProgramFileFromYAML(progFile)
	for _, pe := range bd.prog.pes {
		driver.MapProgram(programs[fmt.Sprintf("(%d,%d)", pe.x, pe.y)], [2]int{pe.x, pe.y})
	}
	for _, q := range bd.prog.caps {
		config.CoreOf(device.GetTile(q.x, q.y)).SetRecvQueueCapacity(sides[q.side], q.color, q.tiles)
	}
	colors := map[string]int{"R": 0, "Y": 1, "B": 2}
	for _, c := range bd.prog.credits {
		config.CoreOf(device.GetTile(c.x, c.y)).InjectData(sides[c.side], colors[c.color], cgra.NewScalar(uint32(c.value)))
	}
	for _, q := range bd.prog.progress {
		config.CoreOf(device.GetTile(q.x, q.y)).SetProgressCreditPort(sides[q.side], q.color)
	}
	for _, f := range bd.feeds {
		// DRAM: every page of the reader's read sequence is available up front.
		config.CoreOf(device.GetTile(f.x, f.y)).SetRecvQueueCapacity(cgra.West, f.color, len(f.pages)+1)
		data := make([]cgra.Data, len(f.pages))
		for i, pg := range f.pages {
			data[i] = pageData(ins[f.in], pg, inPageBytes)
		}
		driver.FeedInDataToCore(data, [2]int{f.x, f.y}, cgra.West, f.color)
	}
	collected := make([][]cgra.Data, len(bd.drains))
	for i, d := range bd.drains {
		collected[i] = make([]cgra.Data, len(d.pages))
		driver.CollectDataFromCore(collected[i], [2]int{d.x, d.y}, cgra.West, "R")
	}
	defer func() {
		if r := recover(); r != nil {
			for _, pe := range bd.prog.pes {
				fmt.Fprintf(os.Stderr, "%-16s (%d,%d) %s\n", pe.name, pe.x, pe.y, config.CoreOf(device.GetTile(pe.x, pe.y)).QueueSummary())
			}
			panic(r)
		}
	}()
	driver.Run()
	cycles := float64(engine.CurrentTime() * 1e9)

	out := make([]byte, outBytes)
	for i, d := range bd.drains {
		for j, pg := range d.pages {
			if collected[i][j].LaneCount() == 0 {
				check(fmt.Errorf("writer (%d,%d) produced %d of %d pages", d.x, d.y, j, len(d.pages)))
			}
			b := make([]byte, 4*len(collected[i][j].Data))
			for k, w := range collected[i][j].Data {
				binary.LittleEndian.PutUint32(b[4*k:], w)
			}
			if off := pg * outPageBytes; off < len(out) {
				copy(out[off:], b[:outPageBytes])
			}
		}
	}
	if st != nil {
		st.write(stateDir, cycles)
	}
	return math.Round(cycles), out
}

func pageData(img []byte, page, size int) cgra.Data {
	b := make([]byte, size)
	if off := page * size; off < len(img) {
		copy(b, img[off:])
	}
	w := make([]uint32, (size+3)/4)
	for k := range w {
		var tmp [4]byte
		copy(tmp[:], b[4*k:])
		w[k] = binary.LittleEndian.Uint32(tmp[:])
	}
	return cgra.FromSlice(w, true)
}

func compare(out, ref []byte, sp kernelSpec) dataCheck {
	c := dataCheck{Mode: "bit_exact"}
	region := sp.outPages * sp.outPageBytes
	if region > len(ref) {
		region = len(ref)
	}
	tol := sp.tolerance
	if tol != nil {
		c.Mode = "gtest_tolerance"
	}
	if sp.outPageBytes == 1088 {
		for i := 0; i < region; i++ {
			c.Elements++
			if out[i] == ref[i] {
				c.BitExact++
			}
		}
		c.WithinTol = c.BitExact
	} else {
		for i := 0; i+1 < region; i += 2 {
			x, y := binary.LittleEndian.Uint16(out[i:]), binary.LittleEndian.Uint16(ref[i:])
			c.Elements++
			if x == y {
				c.BitExact++
				c.WithinTol++
				continue
			}
			if d := ulp(x, y); d > c.MaxULP {
				c.MaxULP = d
			}
			if tol != nil && tol(bf(x), bf(y)) {
				c.WithinTol++
			}
		}
	}
	if c.Elements > 0 {
		c.Fraction = float64(c.BitExact) / float64(c.Elements)
	}
	c.Passed = c.Elements > 0 && c.WithinTol == c.Elements
	return c
}

func bf(v uint16) float32 { return math.Float32frombits(uint32(v) << 16) }

func ulp(a, b uint16) int {
	key := func(v uint16) int {
		if v&0x8000 != 0 {
			return -int(v & 0x7FFF)
		}
		return int(v)
	}
	d := key(a) - key(b)
	if d < 0 {
		d = -d
	}
	return d
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", " ")
	check(err)
	check(os.MkdirAll(filepath.Dir(path), 0o755))
	check(os.WriteFile(path, append(b, '\n'), 0o644))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

var _ = sort.Ints
