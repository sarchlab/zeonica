package main

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestCmpExOperation(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 5

	// 创建测试数据
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// 生成随机测试数据
	src = []uint32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}

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
	program := core.LoadProgramFileFromYAML("./test_cmpex.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从西边输入，东边输出
	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")

	// 映射程序到所有core
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// 运行模拟
	driver.Run()

	// 转换结果并验证
	srcI := make([]int32, length*2)
	dstI := make([]int32, length*2)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	expected := []int32{0, 1, 1, 0, 0}
	// 验证结果：输出应该是输入+2
	t.Log("=== CmpEx Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		actual := dstI[i]

		if actual != expected[i] {
			t.Errorf("Index %d: Input=%d, %d, Expected=%d, Actual=%d",
				i, srcI[2*i], srcI[2*i+1], expected[i], actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, %d, Output=%d ✓", i, srcI[2*i], srcI[2*i+1], actual)
		}
	}

	if allPassed {
		t.Log("✅ CmpEx tests passed!")
	} else {
		t.Fatal("❌ CmpEx tests failed!")
	}
}

func TestGpredOperation(t *testing.T) {

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})

	slog.SetDefault(slog.New(handler))

	// 设置测试参数
	width := 2
	height := 2
	length := 10

	// 生成随机测试数据
	srcData := []uint32{8, 2, 9, 4, 12, 10, 3, 5, 6, 7}
	srcGrant1 := []uint32{1, 0, 1, 1, 0, 1, 1, 0, 1, 1}
	srcGrant2 := []uint32{1, 1, 0, 1, 1, 1, 0}
	srcMixedGrant := make([]uint32, 17)

	index1 := 0
	index2 := 0

	for i := 0; i < 17; i++ {
		if i%5 == 0 || i%5 == 2 || i%5 == 3 {
			srcMixedGrant[i] = srcGrant1[index1]
			index1++
		} else {
			srcMixedGrant[i] = srcGrant2[index2]
			index2++
		}
	}

	dst := make([]uint32, length)

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
	program := core.LoadProgramFileFromYAML("./test_gpred.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从西边输入，东边输出
	driver.FeedIn(srcData, cgra.West, [2]int{0, 1}, 1, "R")
	driver.FeedIn(srcMixedGrant, cgra.West, [2]int{1, 2}, 1, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")

	// 映射程序到所有core
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// 运行模拟
	driver.Run()

	// 转换结果并验证
	srcIData := make([]int32, length)
	srcIGrant := make([]int32, 17)
	dstI := make([]int32, length)

	for i := 0; i < length; i++ {
		srcIData[i] = *(*int32)(unsafe.Pointer(&srcData[i]))
	}

	for i := 0; i < 17; i++ {
		srcIGrant[i] = *(*int32)(unsafe.Pointer(&srcMixedGrant[i]))
	}

	expected := []int32{8, 9, 10, 3, 6}
	for i := 0; i < 5; i++ {
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	t.Log("=== Gpred Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		actual := dstI[i]
		if actual != expected[i] {
			t.Errorf("Index %d:, Expected=%d, Actual=%d",
				i, expected[i], actual)
			allPassed = false
		}
	}

	if allPassed {
		t.Log("✅ Gpred tests passed!")
	} else {
		t.Fatal("❌ Gpred tests failed!")
	}
}

func TestPhiOperation(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 5

	// 生成随机测试数据
	srcData := []uint32{5, 5, 5, 5, 5}
	dst := make([]uint32, length)

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
	program := core.LoadProgramFileFromYAML("./test_phiconst.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从西边输入，东边输出
	driver.FeedIn(srcData, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.West, [2]int{1, 2}, 1, "R")

	// 映射程序到所有core
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// 运行模拟
	driver.Run()

	// 转换结果并验证
	srcIData := make([]int32, length)
	dstI := make([]int32, length)

	for i := 0; i < length; i++ {
		srcIData[i] = *(*int32)(unsafe.Pointer(&srcData[i]))
	}

	expected := []int32{1, 5, 5, 5, 5}
	for i := 0; i < 5; i++ {
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	t.Log("=== Phi-Const Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		actual := dstI[i]
		if actual != expected[i] {
			t.Errorf("Index %d:, Expected=%d, Actual=%d",
				i, expected[i], actual)
			allPassed = false
		}
	}

	if allPassed {
		t.Log("✅ Phi-Const tests passed!")
	} else {
		t.Fatal("❌ Gpred tests failed!")
	}
}
