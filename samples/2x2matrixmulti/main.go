package main

import (
	_ "embed"
	"fmt"
	"os"
	//"time"

	"github.com/sarchlab/akita/v3/monitoring"
	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
)

var width = 3
var height = 3

//go:embed matrixmulti.cgraasm
var matrixMultiKernal string
//var pe(0,0)
//var pe(0,1)
// var pe(0,2)
// var pe(1,0)
// var pe(1,1)
// var pe(1,2)
// var pe(2,0)
// var pe(2,1)
// var pe(2,2)

//go:embed output.cgraasm
var output string

func matrixMulti(driver api.Driver) {

	//1 3
	//2 4
	src1 := []uint32{1, 0, 3, 2, 0, 4}
	//2 6
	//4 8
	src2 := []uint32{2, 0, 6, 4, 0, 8}
	dst := make([]uint32, 6)

	driver.FeedIn(src1[:], cgra.West, [2]int{0, height}, height, "R")
	driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width, "R")
	// for x := 0; x < width; x++ {
	// 	for y := 0; y < height; y++ {
	// 		driver.MapProgram(matrixMultiKernal, [2]int{x, y})
	// 	}
	// }
	//change to mannual mapping
	//each pe has different instruction
	driver.Run()
	driver.FeedIn(src2[:], cgra.North, [2]int{0, width}, width, "B") //for output signal
	driver.Collect(dst, cgra.South, [2]int{0, height}, height, "B")  //for output
	driver.Run()
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
