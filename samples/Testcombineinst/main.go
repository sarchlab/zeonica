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

//go:embed mac.cgraasm
var macKernel string

func macCompute(driver api.Driver) {

	// Set up the CGRA configuration with the FADD kernel
	for x := 0; x < 1; x++ {
		for y := 0; y < 1; y++ {
			driver.MapProgram(macKernel, [2]int{x, y})
		}
	}

	// Run the CGRA simulation for the input layer
	inputData := make([]uint32, 1)
	outputData := make([]uint32, 1)
	inputData[0] = uint32(1)
	driver.FeedIn(inputData, cgra.South, [2]int{0, 1}, 1, "R")
	driver.FeedIn(inputData, cgra.East, [2]int{0, 1}, 1, "R")
	driver.FeedIn(inputData, cgra.West, [2]int{0, 1}, 1, "R")
	driver.Run()
	driver.FeedIn(inputData, cgra.South, [2]int{0, 1}, 1, "R")
	driver.Collect(outputData, cgra.North, [2]int{0, 1}, 1, "R")
	driver.Run()
	fmt.Printf("MAC Result: %d\n", outputData)
}

func main() {
	// Open the log file for writing
	logFile, err := os.OpenFile("test_mac.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
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
		WithWidth(1).
		WithHeight(1).
		WithMonitor(monitor).
		Build("Device")

	driver.RegisterDevice(device)

	monitor.StartServer()

	// Run the FADD layer simulation
	macCompute(driver)

	// Keep the simulation alive for viewing results
	//time.Sleep(100 * time.Hour)
	atexit.Exit(0)
}
