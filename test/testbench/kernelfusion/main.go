package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
	"gopkg.in/yaml.v3"
)

const (
	testName            = "kernelfusion"
	defaultProgramYAML  = "tmp-generated-instructions.yaml"
	defaultPreloadWords = 64
	defaultDumpWords    = 32
	m                   = 4
	k                   = 4
	n                   = 4
)

type programLayout struct {
	arrayRows  int
	arrayCols  int
	compiledII int
	loadTiles  [][2]int
	storeTiles [][2]int
	returnTile *[2]int
}

// KernelFusion runs the generated YAML and compares against the C reference.
//
//nolint:gocyclo,funlen
func KernelFusion(rt *runtimecfg.Runtime) int {
	width := rt.Config.Columns
	height := rt.Config.Rows
	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine

	programPath, err := resolveProgramPath()
	if err != nil {
		panic(err)
	}

	program := core.LoadProgramFileFromYAML(programPath)
	if len(program) == 0 {
		panic(fmt.Sprintf("failed to load program from %s", programPath))
	}

	fixCount := sanitizeGrantOnceEmptySrc(program)
	if fixCount > 0 {
		fmt.Printf("patched %d GRANT_ONCE empty src operand(s) to #0\n", fixCount)
	}

	layout, err := loadProgramLayout(programPath, program)
	if err != nil {
		panic(err)
	}

	fmt.Printf("program yaml: %s\n", programPath)
	fmt.Printf("runtime arch: %dx%d\n", width, height)
	if layout.arrayCols > 0 || layout.arrayRows > 0 {
		fmt.Printf("yaml arch: %dx%d, compiled_ii=%d\n", layout.arrayCols, layout.arrayRows, layout.compiledII)
		if layout.arrayCols != width || layout.arrayRows != height {
			fmt.Printf("warning: runtime arch differs from yaml arch\n")
		}
	}
	fmt.Printf("load tiles: %s\n", formatCoords(layout.loadTiles))
	fmt.Printf("store tiles: %s\n", formatCoords(layout.storeTiles))
	if layout.returnTile != nil {
		fmt.Printf("return tile: (%d,%d)\n", layout.returnTile[0], layout.returnTile[1])
	} else {
		fmt.Println("return tile: <none>")
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	a, xVec, b := buildFuseKernelInputs()
	expectedY, expectedZ, expectedChecksum := cpuFuseKernel(a, xVec, b)
	packed := packFuseKernelMemory(a, xVec, b)

	fmt.Printf("reference checksum: %d\n", expectedChecksum)
	fmt.Printf("reference y: %v\n", expectedY)
	fmt.Printf("reference z: %v\n", expectedZ)

	preloadWords := getEnvInt("ZEONICA_PRELOAD_WORDS", len(packed))
	preloadPackedInputs(driver, width, height, packed, preloadWords)

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			engine.Schedule(sim.MakeTickEvent(tile.GetTickingComponent(), 0))
		}
	}

	driver.Run()

	mismatchCount := 0
	if layout.returnTile != nil {
		retBits := device.GetTile(layout.returnTile[0], layout.returnTile[1]).GetRetVal()
		retVal := int32(retBits)
		fmt.Printf("retVal(bits=0x%08x) -> %d\n", retBits, retVal)
		if retVal != expectedChecksum {
			fmt.Printf("checksum mismatch: expected=%d got=%d\n", expectedChecksum, retVal)
			mismatchCount++
		}
	}

	dumpWords := getEnvInt("ZEONICA_DUMP_WORDS", defaultDumpWords)
	for _, tile := range layout.storeTiles {
		fmt.Printf("store tile (%d,%d) memory dump:\n", tile[0], tile[1])
		for addr := 0; addr < dumpWords; addr++ {
			val := driver.ReadMemory(tile[0], tile[1], uint32(addr))
			fmt.Printf("  addr %d -> 0x%08x (%d)\n", addr, val, int32(val))
		}
	}

	if len(layout.storeTiles) == 0 && layout.returnTile == nil {
		fmt.Println("warning: yaml has no STORE/RETURN_VALUE, nothing observable to dump")
	}

	return mismatchCount
}

func loadProgramLayout(programPath string, program map[string]core.Program) (programLayout, error) {
	data, err := os.ReadFile(programPath)
	if err != nil {
		return programLayout{}, fmt.Errorf("read program yaml: %w", err)
	}

	var root core.YAMLRoot
	if err := yaml.Unmarshal(data, &root); err != nil {
		return programLayout{}, fmt.Errorf("parse program yaml: %w", err)
	}

	layout := programLayout{
		arrayRows:  root.ArrayConfig.Rows,
		arrayCols:  root.ArrayConfig.Cols,
		compiledII: root.ArrayConfig.CompiledII,
	}

	loadSet := make(map[[2]int]struct{})
	storeSet := make(map[[2]int]struct{})

	for coord, prog := range program {
		pos, err := parseCoord(strings.Trim(coord, "()"))
		if err != nil {
			return programLayout{}, fmt.Errorf("invalid program coord %q: %w", coord, err)
		}

		for _, entry := range prog.EntryBlocks {
			for _, group := range entry.InstructionGroups {
				for _, op := range group.Operations {
					switch op.OpCode {
					case "LOAD":
						loadSet[pos] = struct{}{}
					case "STORE":
						storeSet[pos] = struct{}{}
					case "RETURN_VALUE":
						if layout.returnTile != nil && *layout.returnTile != pos {
							return programLayout{}, fmt.Errorf(
								"multiple RETURN_VALUE tiles found: (%d,%d) and (%d,%d)",
								layout.returnTile[0], layout.returnTile[1], pos[0], pos[1],
							)
						}
						ret := pos
						layout.returnTile = &ret
					}
				}
			}
		}
	}

	layout.loadTiles = sortCoords(loadSet)
	layout.storeTiles = sortCoords(storeSet)
	return layout, nil
}

func buildFuseKernelInputs() ([]int32, []int32, []int32) {
	a := make([]int32, m*k)
	xVec := make([]int32, k)
	b := make([]int32, n*m)

	for i := range a {
		a[i] = int32((i & 3) - 1)
	}
	for i := range xVec {
		if i&1 == 1 {
			xVec[i] = 2
		} else {
			xVec[i] = -1
		}
	}
	for i := range b {
		b[i] = int32((i & 3) - 2)
	}

	return a, xVec, b
}

func cpuFuseKernel(a, xVec, b []int32) ([]int32, []int32, int32) {
	y := make([]int32, m)
	z := make([]int32, n)

	for i := 0; i < m; i++ {
		var acc int32
		for j := 0; j < k; j++ {
			acc += a[i*k+j] * xVec[j]
		}
		y[i] = acc
	}

	for i := 0; i < m; i++ {
		if y[i] < 0 {
			y[i] = 0
		}
	}

	for i := 0; i < n; i++ {
		var acc int32
		for j := 0; j < m; j++ {
			acc += b[i*m+j] * y[j]
		}
		z[i] = acc
	}

	var checksum int32
	for i := 0; i < n; i++ {
		checksum += z[i]
	}

	return y, z, checksum & 0xFF
}

func packFuseKernelMemory(a, xVec, b []int32) []int32 {
	// Match the original static buffer layout used by the fused C kernel:
	// A[M*K], x[K], y[M], B[N*M], z[N].
	packed := make([]int32, 0, len(a)+len(xVec)+m+len(b)+n)
	packed = append(packed, a...)
	packed = append(packed, xVec...)
	packed = append(packed, make([]int32, m)...)
	packed = append(packed, b...)
	packed = append(packed, make([]int32, n)...)
	return packed
}

func preloadPackedInputs(driver api.Driver, width, height int, packed []int32, words int) {
	limit := words
	if limit > len(packed) {
		limit = len(packed)
	}

	fmt.Printf("preload packed memory words=%d to all tiles\n", limit)
	for tx := 0; tx < width; tx++ {
		for ty := 0; ty < height; ty++ {
			for addr := 0; addr < limit; addr++ {
				driver.PreloadMemory(tx, ty, uint32(packed[addr]), uint32(addr))
			}
		}
	}
}

func sanitizeGrantOnceEmptySrc(program map[string]core.Program) int {
	fixCount := 0

	for coord, prog := range program {
		for bi := range prog.EntryBlocks {
			entry := &prog.EntryBlocks[bi]
			for gi := range entry.InstructionGroups {
				group := &entry.InstructionGroups[gi]
				for oi := range group.Operations {
					op := &group.Operations[oi]
					if op.OpCode != "GRANT_ONCE" {
						continue
					}
					if len(op.SrcOperands.Operands) == 0 {
						continue
					}
					if strings.TrimSpace(op.SrcOperands.Operands[0].Impl) != "" {
						continue
					}

					op.SrcOperands.Operands[0].Impl = "#0"
					if strings.TrimSpace(op.SrcOperands.Operands[0].Color) == "" {
						op.SrcOperands.Operands[0].Color = "RED"
					}
					fixCount++
				}
			}
		}
		program[coord] = prog
	}

	return fixCount
}

func sortCoords(coordSet map[[2]int]struct{}) [][2]int {
	coords := make([][2]int, 0, len(coordSet))
	for coord := range coordSet {
		coords = append(coords, coord)
	}

	sort.Slice(coords, func(i, j int) bool {
		if coords[i][1] != coords[j][1] {
			return coords[i][1] < coords[j][1]
		}
		return coords[i][0] < coords[j][0]
	})

	return coords
}

func formatCoords(coords [][2]int) string {
	if len(coords) == 0 {
		return "<none>"
	}

	parts := make([]string, 0, len(coords))
	for _, coord := range coords {
		parts = append(parts, fmt.Sprintf("(%d,%d)", coord[0], coord[1]))
	}
	return strings.Join(parts, ", ")
}

func getEnvInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		fmt.Printf("warning: invalid %s=%q, fallback to %d\n", name, raw, fallback)
		return fallback
	}
	return value
}

func resolveProgramPath() (string, error) {
	fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_PROGRAM_YAML"))
	if fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_PROGRAM_YAML points to a missing file: %s", fromEnv)
	}

	candidates := []string{
		defaultProgramYAML,
		"tmp-generated-instructions.yaml",
	}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), defaultProgramYAML)),
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "tmp-generated-instructions.yaml")))
	}
	return firstExistingPath("program yaml", candidates)
}

func parseCoord(raw string) ([2]int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	if len(parts) != 2 {
		return [2]int{}, fmt.Errorf("expect x,y, got %q", raw)
	}

	x, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return [2]int{}, fmt.Errorf("invalid x: %w", err)
	}
	y, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return [2]int{}, fmt.Errorf("invalid y: %w", err)
	}
	return [2]int{x, y}, nil
}

func resolveArchSpecPath() (string, error) {
	fromEnv := strings.TrimSpace(os.Getenv("ZEONICA_ARCH_SPEC"))
	if fromEnv != "" {
		if _, err := os.Stat(fromEnv); err == nil {
			return fromEnv, nil
		}
		return "", fmt.Errorf("ZEONICA_ARCH_SPEC points to a missing file: %s", fromEnv)
	}

	candidates := make([]string, 0, 5)
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates,
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "arch_spec.yaml")),
			filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "arch_spec", "arch_spec.yaml")),
		)
	}
	candidates = append(candidates,
		"arch_spec.yaml",
		"test/arch_spec/arch_spec.yaml",
		"../../arch_spec/arch_spec.yaml",
	)

	return firstExistingPath("arch spec", candidates)
}

func firstExistingPath(label string, candidates []string) (string, error) {
	seen := make(map[string]struct{}, len(candidates))
	normalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		clean := filepath.Clean(candidate)
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		normalized = append(normalized, clean)
		if _, err := os.Stat(clean); err == nil {
			return clean, nil
		}
	}
	return "", fmt.Errorf("cannot locate %s, tried: %s", label, strings.Join(normalized, ", "))
}

func main() {
	archSpecPath, err := resolveArchSpecPath()
	if err != nil {
		panic(err)
	}

	rt, err := runtimecfg.LoadRuntime(archSpecPath, testName)
	if err != nil {
		panic(err)
	}

	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		panic(err)
	}

	mismatch := KernelFusion(rt)

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		panic(err)
	}

	passed := mismatch == 0
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		panic(err)
	}
	fmt.Printf("report saved: %s\n", reportPath)

	if mismatch != 0 {
		os.Exit(1)
	}
}
