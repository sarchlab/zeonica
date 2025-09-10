package main

import (
	"fmt"
	"testing"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestLoadStoreOperation(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 10

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
	program := core.LoadProgramFile("./test_loadstore.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从西边输入，东边输出
	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")
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
	srcI := make([]int32, length*2)
	dstI := make([]int32, length*2)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	expected := []int32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}
	// 验证结果：输出应该是输入+2
	t.Log("=== LoadStore Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		actual := dstI[i]

		if actual != expected[i] {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ LoadStore tests passed!")
	} else {
		t.Fatal("❌ LoadStore tests failed!")
	}
}
