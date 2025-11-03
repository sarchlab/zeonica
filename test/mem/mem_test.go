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
	program := core.LoadProgramFileFromYAML("./test_loadstore.yaml")
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

func makeBytesFromUint32(data uint32) []byte {
	return []byte{byte(data >> 24), byte(data >> 16), byte(data >> 8), byte(data)}
}

func TestLoadWaitDRAMOperation(t *testing.T) {
	width := 2
	height := 2

	src1 := make([]uint32, 1)
	src1[0] = 114
	src2 := make([]uint32, 1)
	src2[0] = 514
	dst1 := make([]uint32, 1)
	dst2 := make([]uint32, 1)

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
		WithMemoryMode("local").
		Build("Device")

	driver.RegisterDevice(device)

	program := core.LoadProgramFileFromYAML("./test_lw.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	driver.FeedIn(src1, cgra.West, [2]int{0, 1}, 1, "R")
	driver.FeedIn(src2, cgra.West, [2]int{1, 2}, 1, "R")
	driver.Collect(dst1, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Collect(dst2, cgra.East, [2]int{1, 2}, 1, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(3), 0)
	driver.PreloadSharedMemory(0, 1, makeBytesFromUint32(1), 0)

	driver.Run()

	srcI1 := make([]int32, 1)
	srcI2 := make([]int32, 1)
	dstI1 := make([]int32, 1)
	dstI2 := make([]int32, 1)

	for i := 0; i < 1; i++ {
		srcI1[i] = *(*int32)(unsafe.Pointer(&src1[i]))
		srcI2[i] = *(*int32)(unsafe.Pointer(&src2[i]))
		dstI1[i] = *(*int32)(unsafe.Pointer(&dst1[i]))
		dstI2[i] = *(*int32)(unsafe.Pointer(&dst2[i]))
	}

	expected1 := []int32{3}
	expected2 := []int32{1}

	t.Logf("=== LoadWaitDRAM Test Results ===")
	allPassed := true
	if dstI1[0] != expected1[0] {
		t.Errorf("Index 0: Input=%d, Expected=%d, Actual=%d",
			srcI1[0], expected1[0], dstI1[0])
		allPassed = false
	} else {
		t.Logf("Index 0: Input=%d, Output=%d ✓", srcI1[0], dstI1[0])
	}
	if dstI2[0] != expected2[0] {
		t.Errorf("Index 0: Input=%d, Expected=%d, Actual=%d",
			srcI2[0], expected2[0], dstI2[0])
		allPassed = false
	} else {
		t.Logf("Index 0: Input=%d, Output=%d ✓", srcI2[0], dstI2[0])
	}
	if allPassed {
		t.Log("✅ LoadWaitDRAM tests passed!")
	} else {
		t.Fatal("❌ LoadWaitDRAM tests failed!")
	}

}

func TestGoAround(t *testing.T) {
	width := 2
	height := 2

	src := make([]uint32, 5)
	src = []uint32{114, 514, 19, 19, 810}
	dst := make([]uint32, 5)
	srcI := make([]int32, 5)
	dstI := make([]int32, 5)

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
		WithMemoryMode("local").
		Build("Device")

	driver.RegisterDevice(device)

	program := core.LoadProgramFileFromYAML("./test_lwsw-go-a-round.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.West, [2]int{1, 2}, 1, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	driver.Run()

	for i := 0; i < 5; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	expected := []int32{114, 514, 19, 19, 810}

	t.Logf("=== GoAround Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		if dstI[i] != expected[i] {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected[i], dstI[i])
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], dstI[i])
		}
	}
	if allPassed {
		t.Log("✅ GoAround tests passed!")
	} else {
		t.Fatal("❌ GoAround tests failed!")
	}

}

func TestSharedMemory(t *testing.T) {
	width := 2
	height := 2

	src := make([]uint32, 5)
	src = []uint32{114, 514, 19, 19, 810}
	dst1 := make([]uint32, 5)
	dst2 := make([]uint32, 5)
	dst3 := make([]uint32, 5)
	srcI := make([]int32, 5)
	dstI1 := make([]int32, 5)
	dstI2 := make([]int32, 5)
	dstI3 := make([]int32, 5)

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
		WithMemoryMode("shared").
		WithMemoryShare(map[[2]int]int{
			{0, 0}: 0,
			{0, 1}: 0,
			{1, 0}: 0,
			{1, 1}: 0,
		}).
		Build("Device")

	driver.RegisterDevice(device)

	program := core.LoadProgramFileFromYAML("./test_all-shared-mem.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst1, cgra.West, [2]int{1, 2}, 1, "R")
	driver.Collect(dst2, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Collect(dst3, cgra.East, [2]int{1, 2}, 1, "R")

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	driver.Run()

	for i := 0; i < 5; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI1[i] = *(*int32)(unsafe.Pointer(&dst1[i]))
		dstI2[i] = *(*int32)(unsafe.Pointer(&dst2[i]))
		dstI3[i] = *(*int32)(unsafe.Pointer(&dst3[i]))
	}

	expected := []int32{114, 514, 19, 19, 810}

	t.Logf("=== SharedMemory Test Results ===")
	allPassed := true
	for i := 0; i < 5; i++ {
		if dstI1[i] != expected[i] || dstI2[i] != expected[i] || dstI3[i] != expected[i] {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual1=%d, Actual2=%d, Actual3=%d",
				i, srcI[i], expected[i], dstI1[i], dstI2[i], dstI3[i])
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output1=%d, Output2=%d, Output3=%d ✓", i, srcI[i], dstI1[i], dstI2[i], dstI3[i])
		}
	}
	if allPassed {
		t.Log("✅ SharedMemory tests passed!")
	} else {
		t.Fatal("❌ SharedMemory tests failed!")
	}

}
