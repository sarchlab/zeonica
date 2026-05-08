//nolint:funlen,whitespace
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
	// Set test parameters
	width := 2
	height := 2
	length := 10

	// Create test data
	src := []uint32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}
	dst := make([]uint32, length)

	// Create simulation engine
	engine := sim.NewSerialEngine()

	// Create driver
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")

	// Create device
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		Build("Device")

	driver.RegisterDevice(device)

	// Load program
	program := core.LoadProgramFileFromYAML("./test_loadstore.yaml")
	if len(program) == 0 {
		t.Fatal("Failed to load program")
	}

	// Set data flow - input from west, output to east
	driver.FeedIn(src, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.West, [2]int{1, 2}, 1, "R")

	// Map program to all cores
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	// Run simulation
	driver.Run()

	// Convert results and verify
	srcI := make([]int32, length*2)
	dstI := make([]int32, length*2)
	for i := 0; i < length; i++ {
		srcI[i] = *(*int32)(unsafe.Pointer(&src[i]))
		dstI[i] = *(*int32)(unsafe.Pointer(&dst[i]))
	}

	expected := []int32{1, 2, 9, 9, 0, 0, 3, 5, 6, 7}
	// Verify results: output should be input+2
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

	src := []uint32{114, 514, 19, 19, 810}
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

	src := []uint32{114, 514, 19, 19, 810}
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

func TestSharedBankedMemoryBlockingLoadWithoutLDW(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 1, 1, 2, 3)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_blocking_ld.yaml", 1, 1)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(42), 0)

	dst := make([]uint32, 1)
	driver.FeedIn([]uint32{1}, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Run()

	if dst[0] != 42 {
		t.Fatalf("blocking LD without LDW got %d, want 42", dst[0])
	}
}

func TestSharedBankedMemoryBlockingLoadCanWriteRegister(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 1, 1, 2, 3)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_blocking_ld_to_reg.yaml", 1, 1)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(123), 0)

	dst := make([]uint32, 1)
	driver.FeedIn([]uint32{1}, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Run()

	if dst[0] != 123 {
		t.Fatalf("blocking LD to register got %d, want 123", dst[0])
	}
}

func TestSharedBankedMemoryBlockingStoreWithoutSTW(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 1, 1, 2, 3)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_blocking_st_ld.yaml", 1, 1)

	dst := make([]uint32, 1)
	driver.FeedIn([]uint32{1}, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")
	driver.Run()

	if dst[0] != 99 {
		t.Fatalf("blocking ST/LD without STW/LDW got %d, want 99", dst[0])
	}
}

func TestSharedBankedMemoryConflictSerializesSameBank(t *testing.T) {
	sameDst, sameTime := runSharedBankedConflictProgram(t, "./test_shared_blocking_conflict_same.yaml")
	diffDst, diffTime := runSharedBankedConflictProgram(t, "./test_shared_blocking_conflict_diff.yaml")
	assertSameElements(t, sameDst, []uint32{11, 33})
	assertSameElements(t, diffDst, []uint32{11, 22})
	if sameTime <= diffTime {
		t.Fatalf("same-bank conflict should take longer than different banks: same=%g diff=%g", sameTime, diffTime)
	}
}

func TestSharedBankedMemoryGroupsDoNotConflictWithEachOther(t *testing.T) {
	isolatedDst, isolatedTime := runSharedBankedGroupIsolationProgram(t)
	sameDst, sameTime := runSharedBankedConflictProgram(t, "./test_shared_blocking_conflict_same.yaml")
	assertSameElements(t, isolatedDst, []uint32{44, 55})
	assertSameElements(t, sameDst, []uint32{11, 33})
	if isolatedTime >= sameTime {
		t.Fatalf(
			"separate shared-memory groups should avoid same-bank serialization: isolated=%g same-bank-single-group=%g",
			isolatedTime,
			sameTime,
		)
	}
}

func TestSharedBankedMemory4x4BoundaryAccess(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 4, 4, 2, 5)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_banked_4x4_boundary_access.yaml", 4, 4)
	preloadSharedBanked4x4Memory(driver)

	westDst := make([]uint32, 4)
	southDst := make([]uint32, 4)
	driver.FeedIn([]uint32{1, 1, 1, 1}, cgra.West, [2]int{0, 4}, 4, "R")
	driver.FeedIn([]uint32{1, 1, 1}, cgra.South, [2]int{1, 4}, 3, "R")
	driver.Collect(westDst, cgra.West, [2]int{0, 4}, 4, "R")
	driver.Collect(southDst, cgra.South, [2]int{0, 4}, 4, "R")
	driver.Run()

	assertSameElements(t, westDst, []uint32{11, 33, 44, 11})
	assertSameElements(t, southDst, []uint32{22, 22, 33, 44})
}

func TestSharedBankedMemory4x4MixedLDSTConflict(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 4, 4, 2, 5)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_banked_4x4_mixed_ld_st.yaml", 4, 4)
	preloadSharedBanked4x4Memory(driver)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(55), 4)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(66), 5)

	westDst := make([]uint32, 3)
	southDst := make([]uint32, 3)
	driver.FeedIn([]uint32{1, 1, 1}, cgra.West, [2]int{0, 3}, 3, "R")
	driver.FeedIn([]uint32{1, 1, 1}, cgra.South, [2]int{1, 4}, 3, "R")
	driver.Collect(westDst, cgra.West, [2]int{0, 3}, 3, "R")
	driver.Collect(southDst, cgra.South, [2]int{1, 4}, 3, "R")
	driver.Run()

	assertSameElements(t, westDst, []uint32{11, 99, 55})
	assertSameElements(t, southDst, []uint32{22, 77, 66})
}

func TestSharedBankedMemory4x4SameBankSlowerThanDifferentBank(t *testing.T) {
	sameDst, sameTime := runSharedBanked4x4TimingProgram(t, "./test_shared_banked_4x4_conflict_same.yaml")
	diffDst, diffTime := runSharedBanked4x4TimingProgram(t, "./test_shared_banked_4x4_conflict_diff.yaml")

	assertSameElements(t, sameDst, []uint32{11, 33, 55, 77, 99, 111, 133})
	assertSameElements(t, diffDst, []uint32{11, 22, 33, 44, 55, 66, 77})
	if sameTime <= diffTime {
		t.Fatalf("4x4 same-bank conflict should take longer than mixed-bank access: same=%g diff=%g", sameTime, diffTime)
	}
}

func runSharedBankedConflictProgram(t *testing.T, programPath string) ([]uint32, float64) {
	t.Helper()
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 1, 2, 2, 3)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, programPath, 1, 2)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(11), 0)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(22), 1)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(33), 2)

	dst := make([]uint32, 2)
	driver.FeedIn([]uint32{1, 1}, cgra.West, [2]int{0, 2}, 2, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 2}, 2, "R")
	driver.Run()
	return dst, float64(engine.CurrentTime())
}

func runSharedBankedGroupIsolationProgram(t *testing.T) ([]uint32, float64) {
	t.Helper()
	engine := sim.NewSerialEngine()
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithWidth(1).
		WithHeight(2).
		WithMemoryMode("shared").
		WithMemoryShare(map[[2]int]int{
			{0, 0}: 0,
			{0, 1}: 1,
		}).
		WithSharedMemoryModel("banked").
		WithSharedMemoryBankConfig(2, 3, 4).
		Build("Device")
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, "./test_shared_blocking_group_isolation.yaml", 1, 2)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(44), 0)
	driver.PreloadSharedMemory(0, 1, makeBytesFromUint32(55), 0)

	dst := make([]uint32, 2)
	driver.FeedIn([]uint32{1, 1}, cgra.West, [2]int{0, 2}, 2, "R")
	driver.Collect(dst, cgra.East, [2]int{0, 2}, 2, "R")
	driver.Run()
	return dst, float64(engine.CurrentTime())
}

func runSharedBanked4x4TimingProgram(t *testing.T, programPath string) ([]uint32, float64) {
	t.Helper()
	engine := sim.NewSerialEngine()
	driver, device := newSharedBankedMemoryTestRig(engine, 4, 4, 2, 5)
	driver.RegisterDevice(device)
	mapProgramFile(t, driver, programPath, 4, 4)
	preloadSharedBanked4x4Memory(driver)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(55), 4)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(66), 5)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(77), 6)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(88), 7)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(99), 8)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(111), 10)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(133), 12)

	westDst := make([]uint32, 4)
	southDst := make([]uint32, 3)
	driver.FeedIn([]uint32{1, 1, 1, 1}, cgra.West, [2]int{0, 4}, 4, "R")
	driver.FeedIn([]uint32{1, 1, 1}, cgra.South, [2]int{1, 4}, 3, "R")
	driver.Collect(westDst, cgra.West, [2]int{0, 4}, 4, "R")
	driver.Collect(southDst, cgra.South, [2]int{1, 4}, 3, "R")
	driver.Run()

	dst := append([]uint32{}, westDst...)
	dst = append(dst, southDst...)
	return dst, float64(engine.CurrentTime())
}

func preloadSharedBanked4x4Memory(driver api.Driver) {
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(11), 0)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(22), 1)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(33), 2)
	driver.PreloadSharedMemory(0, 0, makeBytesFromUint32(44), 3)
}

func newSharedBankedMemoryTestRig(engine sim.Engine, width, height, banks, latency int) (api.Driver, cgra.Device) {
	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")
	share := make(map[[2]int]int)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			share[[2]int{x, y}] = 0
		}
	}
	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithWidth(width).
		WithHeight(height).
		WithMemoryMode("shared").
		WithMemoryShare(share).
		WithSharedMemoryModel("banked").
		WithSharedMemoryBankConfig(banks, latency, 4).
		Build("Device")
	return driver, device
}

func mapProgramFile(t *testing.T, driver api.Driver, path string, width, height int) {
	t.Helper()
	program := core.LoadProgramFileFromYAML(path)
	if len(program) == 0 {
		t.Fatalf("failed to load program %s", path)
	}
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}
}

func assertSameElements(t *testing.T, got, want []uint32) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v want %v", got, want)
	}
	remaining := make(map[uint32]int, len(want))
	for _, value := range want {
		remaining[value]++
	}
	for _, value := range got {
		remaining[value]--
	}
	for value, count := range remaining {
		if count != 0 {
			t.Fatalf("values mismatch: got %v want %v; value %d has count delta %d", got, want, value, count)
		}
	}
}
