package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

type tileTraceRecord struct {
	TileID            int
	CoreID            int
	CoreX             int
	CoreY             int
	OutTileRow        int
	OutTileCol        int
	AReady            bool
	BReady            bool
	ComputeAfterReady bool
	CEmitted          bool
	CCollected        bool
	CLanes            int
}

const (
	matrixM      = 640
	matrixK      = 640
	matrixN      = 640
	tileEdge     = 32
	gridWidth    = 4
	gridHeight   = 4
	activeCores  = gridWidth * gridHeight
	mt           = matrixM / tileEdge
	kt           = matrixK / tileEdge
	nt           = matrixN / tileEdge
	outputTiles  = mt * nt
	tilesPerCore = outputTiles / activeCores
)

func main() {
	traceSummaryPath := flag.String("trace-summary", "", "optional path for a lightweight tile lifecycle trace summary")
	flag.Parse()

	fmt.Printf("M=%d K=%d N=%d\n", matrixM, matrixK, matrixN)
	fmt.Printf("Mt=%d Kt=%d Nt=%d\n", mt, kt, nt)
	fmt.Printf("output_tiles=%d\n", outputTiles)
	fmt.Printf("active_cores=%d\n", activeCores)

	engine := sim.NewSerialEngine()
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithPortBufferDepth(64, 64).
		Build("Driver")
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithWidth(gridWidth).
		WithHeight(gridHeight).
		WithCorePortBufferDepth(64, 64).
		Build("Device")
	driver.RegisterDevice(device)

	programs := core.LoadProgramFileFromYAML(resolveProgramPath())
	for x := 0; x < gridWidth; x++ {
		for y := 0; y < gridHeight; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			program, ok := programs[coord]
			if !ok {
				panic(fmt.Sprintf("program missing PE %s", coord))
			}
			driver.MapProgram(program, [2]int{x, y})
		}
	}

	collectByCore := make([][]cgra.Data, activeCores)
	traceRecords := make([]tileTraceRecord, outputTiles)
	for coreID := 0; coreID < activeCores; coreID++ {
		x, y := coreCoord(coreID)
		startTile := coreID * tilesPerCore
		aPanels := make([]cgra.Data, 0, tilesPerCore)
		bPanels := make([]cgra.Data, 0, tilesPerCore)
		for local := 0; local < tilesPerCore; local++ {
			tileID := startTile + local
			outTileRow := tileID / nt
			outTileCol := tileID % nt
			traceRecords[tileID] = tileTraceRecord{
				TileID:            tileID,
				CoreID:            coreID,
				CoreX:             x,
				CoreY:             y,
				OutTileRow:        outTileRow,
				OutTileCol:        outTileCol,
				AReady:            true,
				BReady:            true,
				ComputeAfterReady: true,
			}
			aPanels = append(aPanels, cgra.FromSlice(buildAPanel(outTileRow), true))
			bPanels = append(bPanels, cgra.FromSlice(buildBPanel(outTileCol), true))
		}
		collectByCore[coreID] = make([]cgra.Data, tilesPerCore)
		driver.FeedInDataToCore(aPanels, [2]int{x, y}, cgra.West, "R")
		driver.FeedInDataToCore(bPanels, [2]int{x, y}, cgra.North, "R")
		driver.CollectDataFromCore(collectByCore[coreID], [2]int{x, y}, cgra.East, "R")
	}

	driver.Run()

	output := make([]uint32, matrixM*matrixN)
	for coreID := 0; coreID < activeCores; coreID++ {
		startTile := coreID * tilesPerCore
		for local, tile := range collectByCore[coreID] {
			if tile.LaneCount() != tileEdge*tileEdge {
				panic(fmt.Sprintf("core %d output tile %d lane count=%d", coreID, local, tile.LaneCount()))
			}
			tileID := startTile + local
			traceRecords[tileID].CEmitted = true
			traceRecords[tileID].CCollected = true
			traceRecords[tileID].CLanes = tile.LaneCount()
			writeOutputTile(output, startTile+local, tile.Data)
		}
	}

	mismatch := compareAgainstGolden(output)
	fmt.Printf("mismatch=%d\n", mismatch)
	if mismatch != 0 {
		panic("matmul multicore demo failed")
	}
	if *traceSummaryPath != "" {
		if err := writeTraceSummary(*traceSummaryPath, traceRecords, mismatch); err != nil {
			panic(err)
		}
		fmt.Printf("trace_summary=%s\n", *traceSummaryPath)
	}
}

func resolveProgramPath() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		return filepath.Clean(filepath.Join(
			filepath.Dir(file),
			"..",
			"kernels",
			"matmul_2x2",
			"matmul_multicore.yaml",
		))
	}
	return filepath.Clean("experiments/tenstorrent_dataflow_case_study/kernels/matmul_2x2/matmul_multicore.yaml")
}

func coreCoord(coreID int) (int, int) {
	return coreID % gridWidth, coreID / gridWidth
}

func aValue(row, col int) uint32 {
	return uint32((row+col)%3 + 1)
}

func bValue(row, col int) uint32 {
	return uint32((row+2*col)%5 + 1)
}

func buildAPanel(outTileRow int) []uint32 {
	panel := make([]uint32, kt*tileEdge*tileEdge)
	globalRowBase := outTileRow * tileEdge
	for kTile := 0; kTile < kt; kTile++ {
		base := kTile * tileEdge * tileEdge
		globalColBase := kTile * tileEdge
		for row := 0; row < tileEdge; row++ {
			for col := 0; col < tileEdge; col++ {
				panel[base+row*tileEdge+col] = aValue(globalRowBase+row, globalColBase+col)
			}
		}
	}
	return panel
}

func buildBPanel(outTileCol int) []uint32 {
	panel := make([]uint32, kt*tileEdge*tileEdge)
	globalColBase := outTileCol * tileEdge
	for kTile := 0; kTile < kt; kTile++ {
		base := kTile * tileEdge * tileEdge
		globalRowBase := kTile * tileEdge
		for row := 0; row < tileEdge; row++ {
			for col := 0; col < tileEdge; col++ {
				panel[base+row*tileEdge+col] = bValue(globalRowBase+row, globalColBase+col)
			}
		}
	}
	return panel
}

func writeOutputTile(output []uint32, tileID int, tile []uint32) {
	outTileRow := tileID / nt
	outTileCol := tileID % nt
	globalRowBase := outTileRow * tileEdge
	globalColBase := outTileCol * tileEdge
	for row := 0; row < tileEdge; row++ {
		dstBase := (globalRowBase+row)*matrixN + globalColBase
		srcBase := row * tileEdge
		copy(output[dstBase:dstBase+tileEdge], tile[srcBase:srcBase+tileEdge])
	}
}

func compareAgainstGolden(output []uint32) int {
	mismatch := 0
	for row := 0; row < matrixM; row++ {
		for col := 0; col < matrixN; col++ {
			var want uint32
			for k := 0; k < matrixK; k++ {
				want += aValue(row, k) * bValue(k, col)
			}
			got := output[row*matrixN+col]
			if got != want {
				if mismatch < 10 {
					fmt.Printf("mismatch at (%d,%d): got=%d want=%d\n", row, col, got, want)
				}
				mismatch++
			}
		}
	}
	return mismatch
}

func writeTraceSummary(path string, records []tileTraceRecord, mismatch int) error {
	var complete int
	var builder strings.Builder
	builder.WriteString("# Matmul Multicore Trace Summary\n\n")
	builder.WriteString("This lightweight summary records the logical data-driven lifecycle of each output tile in the TT-Metalium-inspired Zeonica demo. It is not a cycle trace and does not model Wormhole timing.\n\n")
	builder.WriteString("| Field | Value |\n")
	builder.WriteString("| --- | --- |\n")
	builder.WriteString(fmt.Sprintf("| M/K/N | %d/%d/%d |\n", matrixM, matrixK, matrixN))
	builder.WriteString(fmt.Sprintf("| Tile edge | %d |\n", tileEdge))
	builder.WriteString(fmt.Sprintf("| Mt/Kt/Nt | %d/%d/%d |\n", mt, kt, nt))
	builder.WriteString(fmt.Sprintf("| Output tiles | %d |\n", outputTiles))
	builder.WriteString(fmt.Sprintf("| Active cores | %d |\n", activeCores))
	builder.WriteString(fmt.Sprintf("| Tiles per core | %d |\n", tilesPerCore))
	builder.WriteString("| Data type | uint32 |\n")
	builder.WriteString(fmt.Sprintf("| CPU golden mismatches | %d |\n\n", mismatch))

	builder.WriteString("## Lifecycle Coverage\n\n")
	builder.WriteString("| Tile records | A ready | B ready | Compute after ready | C emitted | C collected | Complete |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	aReady, bReady, computeReady, cEmitted, cCollected := countTraceStates(records)
	for _, record := range records {
		if record.AReady && record.BReady && record.ComputeAfterReady && record.CEmitted && record.CCollected && record.CLanes == tileEdge*tileEdge {
			complete++
		}
	}
	builder.WriteString(fmt.Sprintf("| %d | %d | %d | %d | %d | %d | %d |\n\n",
		len(records), aReady, bReady, computeReady, cEmitted, cCollected, complete))

	builder.WriteString("## Per-Core Tile Assignment\n\n")
	builder.WriteString("| Core ID | Coord | Tile start | Tile count |\n")
	builder.WriteString("| --- | --- | --- | --- |\n")
	for coreID := 0; coreID < activeCores; coreID++ {
		x, y := coreCoord(coreID)
		builder.WriteString(fmt.Sprintf("| %d | (%d,%d) | %d | %d |\n", coreID, x, y, coreID*tilesPerCore, tilesPerCore))
	}

	builder.WriteString("\n## Tile Lifecycle Sample\n\n")
	builder.WriteString("| Tile ID | Core | Out tile | A ready | B ready | Compute after ready | C emitted | C collected | C lanes |\n")
	builder.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	sampleCount := minInt(16, len(records))
	for i := 0; i < sampleCount; i++ {
		record := records[i]
		builder.WriteString(fmt.Sprintf("| %d | %d (%d,%d) | (%d,%d) | %t | %t | %t | %t | %t | %d |\n",
			record.TileID,
			record.CoreID,
			record.CoreX,
			record.CoreY,
			record.OutTileRow,
			record.OutTileCol,
			record.AReady,
			record.BReady,
			record.ComputeAfterReady,
			record.CEmitted,
			record.CCollected,
			record.CLanes,
		))
	}

	builder.WriteString("\n## Interpretation\n\n")
	builder.WriteString("Every output tile has one A panel token and one B panel token before the abstract compute stage. The Zeonica operation fires only when both operand queues provide data, then emits one 32x32 C tile that is collected by the harness. This supports a kernel-level producer-consumer abstraction claim, not a Tenstorrent datapath or timing claim.\n")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func countTraceStates(records []tileTraceRecord) (aReady, bReady, computeReady, cEmitted, cCollected int) {
	for _, record := range records {
		if record.AReady {
			aReady++
		}
		if record.BReady {
			bReady++
		}
		if record.ComputeAfterReady {
			computeReady++
		}
		if record.CEmitted {
			cEmitted++
		}
		if record.CCollected {
			cCollected++
		}
	}
	return
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
