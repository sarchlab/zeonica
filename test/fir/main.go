package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func Fir() {
	// 设置测试参数
	width := 3
	height := 3

	// 创建模拟引擎
	engine := sim.NewSerialEngine()

	// 创建driver
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")

	// 创建设备
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		Build("Device")

	driver.RegisterDevice(device)

	// 加载程序
	program := core.LoadProgramFileFromYAML("./fir.yaml")
	if len(program) == 0 {
		panic("Failed to load program")
	}

	// 映射程序到所有core
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	driver.PreloadMemory(0, 0, 3, 0)
	driver.PreloadMemory(0, 0, 1, 1)
	driver.PreloadMemory(0, 1, 2, 2)
	driver.PreloadMemory(0, 1, 4, 3) // addr has ERRORS !!!!!!

	triggerDataPacket0 := []uint32{1}
	triggerDataPacket1 := []uint32{1}

	driver.FeedIn(triggerDataPacket0, cgra.North, [2]int{1, 2}, 1, "R")
	driver.FeedIn(triggerDataPacket1, cgra.West, [2]int{2, 3}, 1, "R")

	driver.Run()

	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println("========================")
	fmt.Println(driver.ReadMemory(0, 2, 0))
}

func main() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))

	slog.Debug("This Debug message will not be displayed")
	slog.Info("This is an Info message")
	Fir()
}
