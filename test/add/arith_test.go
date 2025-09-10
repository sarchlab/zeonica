package main

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

func TestAddOperationWithRandomData(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 16

	// 创建测试数据
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// 生成随机测试数据
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFile("./test_add.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从西边输入，东边输出
	driver.FeedIn(src, cgra.West, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.East, [2]int{0, height}, height, "R")

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
	srcI := make([]int32, length)
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// 验证结果：输出应该是输入+2
	t.Log("=== ADD Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] + 2
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ ADD tests passed!")
	} else {
		t.Fatal("❌ ADD tests failed!")
	}
}

func TestSubOperationWithRandomData(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 16

	// 创建测试数据
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// 生成随机测试数据
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFile("./test_sub.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从北边输入，南边输出
	driver.FeedIn(src, cgra.North, [2]int{0, width}, width, "R")
	driver.Collect(dst, cgra.South, [2]int{0, width}, width, "R")

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
	srcI := make([]int32, length)
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// 验证结果：输出应该是输入-2
	t.Log("=== SUB Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] - 2
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ SUB tests passed!")
	} else {
		t.Fatal("❌ SUB tests failed!")
	}
}

func TestMulOperationWithRandomData(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 16

	// 创建测试数据
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// 生成随机测试数据
	rand.Seed(time.Now().UnixNano())
	minI := int32(-10)
	maxI := int32(10)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFile("./test_mul.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从东边输入，西边输出
	driver.FeedIn(src, cgra.East, [2]int{0, height}, height, "R")
	driver.Collect(dst, cgra.West, [2]int{0, height}, height, "R")

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
	srcI := make([]int32, length)
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// 验证结果：输出应该是输入*2
	t.Log("=== MUL Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] * 4
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ MUL tests passed!")
	} else {
		t.Fatal("❌ MUL tests failed!")
	}
}

func TestDivOperationWithRandomData(t *testing.T) {
	// 设置测试参数
	width := 2
	height := 2
	length := 16

	// 创建测试数据
	src := make([]uint32, length)
	dst := make([]uint32, length)

	// 生成随机测试数据（避免除零）
	rand.Seed(time.Now().UnixNano())
	minI := int32(-20)
	maxI := int32(20)
	for i := 0; i < length; i++ {
		INum := minI + rand.Int31n(maxI-minI+1)
		// 确保数据是4的倍数，避免除法精度问题
		if INum%4 != 0 {
			INum = INum - INum%4 + 4
		}
		src[i] = *(*uint32)(unsafe.Pointer(&INum))
	}

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
	program := core.LoadProgramFile("./test_div.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// 设置数据流 - 从南边输入，北边输出
	driver.FeedIn(src, cgra.South, [2]int{0, width}, width, "R")
	driver.Collect(dst, cgra.North, [2]int{0, width}, width, "R")

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
	srcI := make([]int32, length)
	dstI := make([]int32, length)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	// 验证结果：输出应该是输入/2
	t.Log("=== DIV Test Results ===")
	allPassed := true
	for i := 0; i < length; i++ {
		expected := srcI[i] / 4
		actual := dstI[i]

		if actual != expected {
			t.Errorf("Index %d: Input=%d, Expected=%d, Actual=%d",
				i, srcI[i], expected, actual)
			allPassed = false
		} else {
			t.Logf("Index %d: Input=%d, Output=%d ✓", i, srcI[i], actual)
		}
	}

	if allPassed {
		t.Log("✅ DIV tests passed!")
	} else {
		t.Fatal("❌ DIV tests failed!")
	}
}
