package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
<<<<<<< HEAD
	// "github.com/sarchlab/zeonica/cgra"
=======
>>>>>>> origin/main
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func Histogram() {
	width := 4
	height := 4

	engine := sim.NewSerialEngine()

	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		Build("Device")

	driver.RegisterDevice(device)

	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		//programPath = "test/Zeonica_Testbench/kernel/histogram/histogram-instructions.yaml"
		programPath = "tmp-generated-instructions.yaml"
	}
	program := core.LoadProgramFileFromYAML(programPath)
	fmt.Println("program:", program)

	if len(program) == 0 {
		panic("Failed to load program")
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// preload input data from benchmark (DATA_LEN=20)
	inputData := []uint32{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		11, 12, 13, 14, 14, 14, 14, 14, 14, 15,
	}
	expected := computeHistogram(inputData, 5)

	// histogram tile (2,1): initialize histogram[0..4] to 0
	for addr := 0; addr < 5; addr++ {
		driver.PreloadMemory(2, 1, 0, uint32(addr))
	}
	// data tile (3,2): input_data[0..19]
	for addr, val := range inputData {
		driver.PreloadMemory(3, 2, val, uint32(addr))
	}

	// fire all the cores in the beginning
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := device.GetTile(x, y)
			tickingComponent := tile.GetTickingComponent()
			engine.Schedule(sim.MakeTickEvent(tickingComponent, 0))
		}
	}

	driver.Run()

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")

	// print output memory data
	outputTile := [2]int{2, 1}
	fmt.Printf("output memory @ tile (%d,%d):\n", outputTile[0], outputTile[1])
	scanLimit := 5
	outputData := make([]uint32, scanLimit)
	for addr := 0; addr < scanLimit; addr++ {
		val := driver.ReadMemory(outputTile[0], outputTile[1], uint32(addr))
		outputData[addr] = val
<<<<<<< HEAD
		if addr < len(inputData) {
			fmt.Printf("  addr %d -> %d\n", addr, val)
		}
=======
		fmt.Printf("  addr %d -> %d\n", addr, val)
>>>>>>> origin/main
	}

	fmt.Println("expected histogram (CPU):")
	histMismatch := 0
	for i, val := range expected {
		fmt.Printf("  addr %d -> %d\n", i, val)
		if i < len(outputData) && outputData[i] != val {
			histMismatch++
		}
	}
	if histMismatch == 0 {
		fmt.Println("✅ output matches expected histogram")
	} else {
		fmt.Printf("❌ output mismatches histogram: %d\n", histMismatch)
	}
}

func computeHistogram(input []uint32, bins int) []uint32 {
	// Match histogram_int.cpp semantics:
	// b = BUCKET_LEN * (input[i] - MIN) / (MAX - MIN)
	const minVal = 1
	const maxVal = 19
	const dataLen = 20
	delta := maxVal - minVal
	result := make([]uint32, bins)
	for i := 0; i < len(input) && i < dataLen; i++ {
		v := int(input[i])
		if v < minVal || v > maxVal {
			continue
		}
		b := bins * (v - minVal) / delta
		if b >= 0 && b < bins {
			result[b]++
		}
	}
	return result
}

func main() {
	f, err := os.Create("histogram.json.log")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))
	Histogram()
}
