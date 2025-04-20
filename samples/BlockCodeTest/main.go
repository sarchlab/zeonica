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

var width = 1
var height = 1

//go:embed DoMul.cgraasm
var doMul string

func matrixMulti(driver api.Driver) {

	src1 := []uint32{1}
	src2 := []uint32{2}
	dst := make([]uint32, 1)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			driver.MapProgram(doMul, [2]int{x, y})
		}
	}
	driver.FeedIn(src1[:], cgra.South, [2]int{0, 1}, 1, "R")
	driver.FeedIn(src2[:], cgra.West, [2]int{0, 1}, 1, "R")
	driver.Run()
	driver.FeedIn(src2[:], cgra.West, [2]int{0, 1}, 1, "R") //for output signal
	driver.Collect(dst, cgra.East, [2]int{0, 1}, 1, "R")    //for output
	driver.Run()
	fmt.Println(dst)
}

func main() {
	// Open the log file for writing
	logFile, err := os.OpenFile("test.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
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
