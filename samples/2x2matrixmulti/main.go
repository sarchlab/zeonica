package main

import (
	_ "embed"
	"fmt"
	"os"

	//"time"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 3
var height = 3

//go:embed Donothing.cgraasm
var doNothingKernel string

//go:embed LoadData.cgraasm
var loadDataKernel string

//go:embed StoreData.cgraasm
var storeDataKernel string

//go:embed MAC1.cgraasm
var mac1Kernel string

//go:embed MULT1.cgraasm
var mult1Kernel string

//go:embed MAC2.cgraasm
var mac2Kernel string

//go:embed MULT2.cgraasm
var mult2Kernel string

func matrixMulti(driver api.Driver) {

	//1 3
	//2 4
	src1 := []uint32{1, 2, 3, 4}
	//2 6
	//4 8
	src2 := []uint32{2, 4, 6, 8}
	dst := make([]uint32, 4)
	//write memory:    x, y, data, baseAddr
	driver.PreloadMemory(0, 0, src2[0], 0)
	driver.PreloadMemory(0, 1, src2[1], 0)
	driver.PreloadMemory(1, 0, src2[2], 0)
	driver.PreloadMemory(1, 1, src2[3], 0)

	//expected output:
	//14 20
	//30 44

	//create table of mapping kernel to PE
	kernels := api.PerPEKernels{
		{0, 0}: mult2Kernel,
		{0, 1}: mac2Kernel,
		{0, 2}: storeDataKernel,
		{1, 0}: mult1Kernel,
		{1, 1}: mac1Kernel,
		{1, 2}: storeDataKernel,
		{2, 0}: loadDataKernel,
		{2, 1}: loadDataKernel,
		{2, 2}: doNothingKernel,
	}

	// set the mapping
	if err := driver.SetPerPEKernels(kernels); err != nil {
		panic(err)
	}
	//send data to PE(2,0) and PE(2,1)
	//driver.FeedIn(src1[:], cgra.South, [2]int{0, 2}, 2, "R")
	driver.FeedIn(src1[0:2], cgra.South, [2]int{0, 2}, 2, "R")
	driver.FeedIn(src1[2:4], cgra.South, [2]int{0, 2}, 2, "B")
	driver.Run()
	//driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width, "B") //for output signal
	//driver.Collect(dst, cgra.South, [2]int{0, height}, height, "B")  //for output
	dst[0] = driver.ReadMemory(2, 2, 0)
	dst[1] = driver.ReadMemory(2, 2, 1)
	dst[2] = driver.ReadMemory(1, 2, 0)
	dst[3] = driver.ReadMemory(1, 2, 1)
	fmt.Println(dst)
}

func main() {
	// Open the log file for writing
	logFile, err := os.OpenFile("matrix_multiplication.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		return
	}
	defer logFile.Close()

	// Redirect stdout and stderr to the log file
	os.Stdout = logFile
	os.Stderr = logFile
	monitor := monitoring.NewMonitor()

	engine := sim.NewSerialEngine()
	monitor.RegisterEngine(engine)

	driver := api.DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		Build("Driver")
	monitor.RegisterComponent(driver)

	device := config.DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(width).
		WithHeight(height).
		WithMonitor(monitor).
		Build("Device")

	driver.RegisterDevice(device)

	monitor.StartServer()

	matrixMulti(driver)

	//time.Sleep(100 * time.Hour)
	atexit.Exit(0)
}
