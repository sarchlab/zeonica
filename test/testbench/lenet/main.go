package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

const (
	testName = "lenet"

	inputH  = 28
	inputW  = 28
	conv1C  = 8
	conv2C  = 16
	fc1Out  = 64
	logitsN = 10

	inputSize  = inputH * inputW
	feat1H     = 14
	feat1W     = 14
	feat1Size  = feat1H * feat1W * conv1C
	feat2H     = 7
	feat2W     = 7
	feat2Size  = feat2H * feat2W * conv2C
	fc1In      = feat2Size
	conv1WSize = conv1C * 3 * 3
	conv2WSize = conv2C * conv1C * 3 * 3
	fc1WSize   = fc1Out * fc1In
	fc2WSize   = logitsN * fc1Out
)

const (
	inputBase = 0

	conv1WBase = inputBase + inputSize
	conv1BBase = conv1WBase + conv1WSize
	feat1Base  = conv1BBase + conv1C

	conv2WBase = feat1Base + feat1Size
	conv2BBase = conv2WBase + conv2WSize
	feat2Base  = conv2BBase + conv2C

	fc1WBase   = feat2Base + feat2Size
	fc1BBase   = fc1WBase + fc1WSize
	fc1ActBase = fc1BBase + fc1Out

	fc2WBase   = fc1ActBase + fc1Out
	fc2BBase   = fc2WBase + fc2WSize
	logitsBase = fc2BBase + logitsN

	conv2TmpBase = logitsBase + logitsN
	conv2TmpSize = feat2H * feat2W * 4
)

const (
	packedK1X = 0
	packedK1Y = 1
	packedK2X = 0
	packedK2Y = 5
	packedK3X = 0
	packedK3Y = 7

	seqX = 0
	seqY = 7
)

type testCase struct {
	input  []int8
	conv1W []int8
	conv1B []int32
	feat1  []int8
	conv2W []int8
	conv2B []int32
	feat2  []int8
	fc1W   []int8
	fc1B   []int32
	fc1Act []int8
	fc2W   []int8
	fc2B   []int32
	logits []int32
}

type modeConfig struct {
	name        string
	programPath string
	preload     func(interface {
		PreloadMemory(x int, y int, data uint32, baseAddr uint32)
	}, testCase)
	outputXY [2]int
}

func loadProgram(path string) map[string]core.Program {
	if strings.HasSuffix(strings.ToLower(path), ".asm") {
		return core.LoadProgramFileFromASM(path)
	}
	return core.LoadProgramFileFromYAML(path)
}

func selectProgramPath(stem string) string {
	ext := strings.ToLower(strings.TrimSpace(os.Getenv("ZEONICA_LENET_PROGRAM_EXT")))
	if ext == "yaml" || ext == "yml" {
		return resolveLocalPath(stem + ".yaml")
	}
	return resolveLocalPath(stem + ".asm")
}

func buildTestCase(seed int64) testCase {
	rng := rand.New(rand.NewSource(seed))
	tc := testCase{
		input:  make([]int8, inputSize),
		conv1W: make([]int8, conv1WSize),
		conv1B: make([]int32, conv1C),
		feat1:  make([]int8, feat1Size),
		conv2W: make([]int8, conv2WSize),
		conv2B: make([]int32, conv2C),
		feat2:  make([]int8, feat2Size),
		fc1W:   make([]int8, fc1WSize),
		fc1B:   make([]int32, fc1Out),
		fc1Act: make([]int8, fc1Out),
		fc2W:   make([]int8, fc2WSize),
		fc2B:   make([]int32, logitsN),
		logits: make([]int32, logitsN),
	}

	fillInt8(rng, tc.input, -2, 3)
	fillInt8(rng, tc.conv1W, -2, 3)
	fillInt32(rng, tc.conv1B, -4, 5)
	fillInt8(rng, tc.conv2W, -2, 3)
	fillInt32(rng, tc.conv2B, -4, 5)
	fillInt8(rng, tc.fc1W, -2, 3)
	fillInt32(rng, tc.fc1B, -8, 9)
	fillInt8(rng, tc.fc2W, -2, 3)
	fillInt32(rng, tc.fc2B, -8, 9)

	cpuReference(&tc)
	return tc
}

func fillInt8(rng *rand.Rand, dst []int8, lo, hi int) {
	for i := range dst {
		dst[i] = int8(lo + rng.Intn(hi-lo))
	}
}

func fillInt32(rng *rand.Rand, dst []int32, lo, hi int) {
	for i := range dst {
		dst[i] = int32(lo + rng.Intn(hi-lo))
	}
}

func cpuReference(tc *testCase) {
	conv1Raw := make([]int32, 28*28*conv1C)
	for oy := 0; oy < 28; oy++ {
		for ox := 0; ox < 28; ox++ {
			for oc := 0; oc < conv1C; oc++ {
				acc := tc.conv1B[oc]
				for ky := 0; ky < 3; ky++ {
					iy := oy + ky - 1
					if iy < 0 || iy >= 28 {
						continue
					}
					for kx := 0; kx < 3; kx++ {
						ix := ox + kx - 1
						if ix < 0 || ix >= 28 {
							continue
						}
						inp := int32(tc.input[iy*28+ix])
						w := int32(tc.conv1W[oc*9+ky*3+kx])
						acc += inp * w
					}
				}
				if acc < 0 {
					acc = 0
				}
				if acc > 127 {
					acc = 127
				}
				conv1Raw[(oy*28+ox)*conv1C+oc] = acc
			}
		}
	}

	for oy := 0; oy < feat1H; oy++ {
		for ox := 0; ox < feat1W; ox++ {
			for oc := 0; oc < conv1C; oc++ {
				sum := int32(0)
				for py := 0; py < 2; py++ {
					for px := 0; px < 2; px++ {
						iy := oy*2 + py
						ix := ox*2 + px
						sum += conv1Raw[(iy*28+ix)*conv1C+oc]
					}
				}
				avg := sum / 4
				if avg > 127 {
					avg = 127
				}
				tc.feat1[(oy*feat1W+ox)*conv1C+oc] = int8(avg)
			}
		}
	}

	conv2Raw := make([]int32, feat1H*feat1W*conv2C)
	for oy := 0; oy < feat1H; oy++ {
		for ox := 0; ox < feat1W; ox++ {
			for oc := 0; oc < conv2C; oc++ {
				acc := tc.conv2B[oc]
				for ic := 0; ic < conv1C; ic++ {
					for ky := 0; ky < 3; ky++ {
						iy := oy + ky - 1
						if iy < 0 || iy >= feat1H {
							continue
						}
						for kx := 0; kx < 3; kx++ {
							ix := ox + kx - 1
							if ix < 0 || ix >= feat1W {
								continue
							}
							inp := int32(tc.feat1[(iy*feat1W+ix)*conv1C+ic])
							wIndex := ((oc*conv1C+ic)*3+ky)*3 + kx
							acc += inp * int32(tc.conv2W[wIndex])
						}
					}
				}
				if acc < 0 {
					acc = 0
				}
				if acc > 127 {
					acc = 127
				}
				conv2Raw[(oy*feat1W+ox)*conv2C+oc] = acc
			}
		}
	}

	for oy := 0; oy < feat2H; oy++ {
		for ox := 0; ox < feat2W; ox++ {
			for oc := 0; oc < conv2C; oc++ {
				sum := int32(0)
				for py := 0; py < 2; py++ {
					for px := 0; px < 2; px++ {
						iy := oy*2 + py
						ix := ox*2 + px
						sum += conv2Raw[(iy*feat1W+ix)*conv2C+oc]
					}
				}
				avg := sum / 4
				if avg > 127 {
					avg = 127
				}
				tc.feat2[(oy*feat2W+ox)*conv2C+oc] = int8(avg)
			}
		}
	}

	for o := 0; o < fc1Out; o++ {
		acc := tc.fc1B[o]
		base := o * fc1In
		for i := 0; i < fc1In; i++ {
			acc += int32(tc.fc1W[base+i]) * int32(tc.feat2[i])
		}
		if acc < 0 {
			acc = 0
		}
		if acc > 127 {
			acc = 127
		}
		tc.fc1Act[o] = int8(acc)
	}

	for o := 0; o < logitsN; o++ {
		acc := tc.fc2B[o]
		base := o * fc1Out
		for i := 0; i < fc1Out; i++ {
			acc += int32(tc.fc2W[base+i]) * int32(tc.fc1Act[i])
		}
		tc.logits[o] = acc
	}
}

func runProgram(cfg modeConfig, archSpecPath string, tc testCase, testInstance string) ([]int32, string, error) {
	rt, err := runtimecfg.LoadRuntime(archSpecPath, testInstance)
	if err != nil {
		return nil, "", err
	}

	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		return nil, "", err
	}

	driver := rt.Driver
	device := rt.Device
	engine := rt.Engine
	program := loadProgram(cfg.programPath)

	mapped := make([][2]int, 0, len(program))
	for x := 0; x < rt.Config.Columns; x++ {
		for y := 0; y < rt.Config.Rows; y++ {
			key := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[key]; exists {
				driver.MapProgram(prog, [2]int{x, y})
				mapped = append(mapped, [2]int{x, y})
			}
		}
	}

	cfg.preload(driver, tc)
	scheduleMappedTiles(device, engine, mapped)
	driver.Run()

	logits := make([]int32, logitsN)
	for i := 0; i < logitsN; i++ {
		logits[i] = int32(driver.ReadMemory(cfg.outputXY[0], cfg.outputXY[1], uint32(logitsBase+i)))
	}

	if err := runtimecfg.CloseTraceLog(traceLog); err != nil {
		return nil, "", err
	}

	reportMismatch := 0
	passed := true
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &reportMismatch)
	if err != nil {
		return nil, "", err
	}
	return logits, reportPath, nil
}

func preloadPackedCase(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, tc testCase) {
	preloadCommonStage(driver, packedK1X, packedK1Y, tc, true, false, false)
	preloadCommonStage(driver, packedK2X, packedK2Y, tc, false, true, false)
	preloadCommonStage(driver, packedK3X, packedK3Y, tc, false, false, true)
}

func preloadSequentialCase(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, tc testCase) {
	preloadCommonStage(driver, seqX, seqY, tc, true, true, true)
}

func preloadCommonStage(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, x int, y int, tc testCase, withK1 bool, withK2 bool, withK3 bool) {
	if withK1 {
		writeInt8Slice(driver, x, y, inputBase, tc.input)
		writeInt8Slice(driver, x, y, conv1WBase, tc.conv1W)
		writeInt32Slice(driver, x, y, conv1BBase, tc.conv1B)
	}
	zeroMemory(driver, x, y, feat1Base, feat1Size)

	if withK2 {
		writeInt8Slice(driver, x, y, conv2WBase, tc.conv2W)
		writeInt32Slice(driver, x, y, conv2BBase, tc.conv2B)
	}
	zeroMemory(driver, x, y, feat2Base, feat2Size)
	zeroMemory(driver, x, y, conv2TmpBase, conv2TmpSize)

	if withK3 {
		writeInt8Slice(driver, x, y, fc1WBase, tc.fc1W)
		writeInt32Slice(driver, x, y, fc1BBase, tc.fc1B)
		writeInt8Slice(driver, x, y, fc2WBase, tc.fc2W)
		writeInt32Slice(driver, x, y, fc2BBase, tc.fc2B)
	}
	zeroMemory(driver, x, y, fc1ActBase, fc1Out)
	zeroMemory(driver, x, y, logitsBase, logitsN)
}

func zeroMemory(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, x int, y int, base int, count int) {
	for i := 0; i < count; i++ {
		driver.PreloadMemory(x, y, 0, uint32(base+i))
	}
}

func writeInt8Slice(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, x int, y int, base int, values []int8) {
	for i, v := range values {
		driver.PreloadMemory(x, y, uint32(int32(v)), uint32(base+i))
	}
}

func writeInt32Slice(driver interface {
	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
}, x int, y int, base int, values []int32) {
	for i, v := range values {
		driver.PreloadMemory(x, y, uint32(v), uint32(base+i))
	}
}

func scheduleMappedTiles(device cgra.Device, engine sim.Engine, mapped [][2]int) {
	sort.Slice(mapped, func(i, j int) bool {
		if mapped[i][1] != mapped[j][1] {
			return mapped[i][1] < mapped[j][1]
		}
		return mapped[i][0] < mapped[j][0]
	})

	for _, coord := range mapped {
		tile := device.GetTile(coord[0], coord[1])
		engine.Schedule(sim.MakeTickEvent(tile.GetTickingComponent(), 0))
	}
}

func compareLogits(label string, got []int32, expected []int32) int {
	mismatch := 0
	for i := 0; i < len(expected) && i < len(got); i++ {
		if got[i] != expected[i] {
			fmt.Printf("%s mismatch @logit[%d]: expected=%d got=%d\n", label, i, expected[i], got[i])
			mismatch++
		}
	}
	return mismatch
}

func resolveLocalPath(name string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve local path")
	}
	return filepath.Join(filepath.Dir(thisFile), name)
}

func envInt(name string, defaultValue int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func envInt64(name string, defaultValue int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func selectedModes(all []modeConfig) []modeConfig {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("ZEONICA_LENET_MODE")))
	if mode == "" || mode == "all" {
		return all
	}

	filtered := make([]modeConfig, 0, len(all))
	for _, cfg := range all {
		if cfg.name == mode {
			filtered = append(filtered, cfg)
		}
	}
	if len(filtered) == 0 {
		return all
	}
	return filtered
}

func main() {
	archSpecPath := resolveLocalPath("arch_spec.yaml")
	modes := []modeConfig{
		{
			name:        "packed",
			programPath: selectProgramPath("lenet_packed"),
			preload:     preloadPackedCase,
			outputXY:    [2]int{packedK3X, packedK3Y},
		},
		{
			name:        "sequential",
			programPath: selectProgramPath("lenet_seq"),
			preload:     preloadSequentialCase,
			outputXY:    [2]int{seqX, seqY},
		},
	}
	modes = selectedModes(modes)

	caseCount := envInt("ZEONICA_LENET_CASES", 1)
	baseSeed := envInt64("ZEONICA_RAND_SEED", 12345)
	totalMismatch := 0

	for caseIdx := 0; caseIdx < caseCount; caseIdx++ {
		seed := baseSeed + int64(caseIdx)
		tc := buildTestCase(seed)
		fmt.Printf("case %d seed=%d\n", caseIdx, seed)

		results := make(map[string][]int32, len(modes))
		for _, mode := range modes {
			testInstance := fmt.Sprintf("%s_%s_case%d", testName, mode.name, caseIdx)
			logits, reportPath, err := runProgram(mode, archSpecPath, tc, testInstance)
			if err != nil {
				panic(err)
			}
			fmt.Printf("%s report saved: %s\n", mode.name, reportPath)
			results[mode.name] = logits
			totalMismatch += compareLogits(mode.name, logits, tc.logits)
		}

		if _, okP := results["packed"]; okP {
			if _, okS := results["sequential"]; okS {
				for i := 0; i < logitsN; i++ {
					if results["packed"][i] != results["sequential"][i] {
						fmt.Printf(
							"packed/sequential mismatch @logit[%d]: expected=%d packed=%d seq=%d\n",
							i,
							tc.logits[i],
							results["packed"][i],
							results["sequential"][i],
						)
						totalMismatch++
					}
				}
			}
		}
	}

	if totalMismatch == 0 {
		fmt.Println("lenet packed and sequential programs match CPU reference")
		return
	}

	fmt.Printf("total mismatches: %d\n", totalMismatch)
	os.Exit(1)
}
