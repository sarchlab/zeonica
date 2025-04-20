package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/sarchlab/akita/v4/monitoring"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/api"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
)

// Generate random matrix of given size
func generateRandomMatrix(size int) [][]uint32 {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	matrix := make([][]uint32, size)
	for i := range matrix {
		matrix[i] = make([]uint32, size)
		for j := range matrix[i] {
			matrix[i][j] = uint32(r.Intn(10))
		}
	}
	return matrix
}

// Flatten a 2D matrix into a 1D slice
func flattenMatrix(matrix [][]uint32) []uint32 {
	size := len(matrix)
	flat := make([]uint32, size*size)
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			flat[i*size+j] = matrix[i][j]
		}
	}
	return flat
}

// This is use to generate the expected result
func nativeMatrixMultiply(a, b [][]uint32) [][]uint32 {
	size := len(a)
	result := make([][]uint32, size)
	for i := range result {
		result[i] = make([]uint32, size)
		for j := range result[i] {
			for k := 0; k < size; k++ {
				result[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return result
}

// ExampleMatrixMulti demonstrates how to use the CGRA for matrix multiplication
func ExampleMatrixMulti() {
	originalStdout := os.Stdout
	defer func() { os.Stdout = originalStdout }()
	logFile, err := os.OpenFile("matrix_multiplication_case.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Println("Failed to open log file:", err)
		return
	}
	defer logFile.Close()

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
		WithWidth(3).
		WithHeight(3).
		WithMonitor(monitor).
		Build("Device")

	driver.RegisterDevice(device)

	size := 2
	//generate random matrix A and B
	matrixA := generateRandomMatrix(size)
	matrixB := generateRandomMatrix(size)
	//Expected result
	expected := nativeMatrixMultiply(matrixA, matrixB)
	expectedFlat := flattenMatrix(expected)

	fmt.Println("Matrix A:")
	printMatrix(matrixA)
	fmt.Println("Matrix B:")
	printMatrix(matrixB)
	fmt.Println("Expected Result:")
	printMatrix(expected)

	flatB := flattenMatrix(matrixB)
	driver.PreloadMemory(0, 0, flatB[0], 0)
	driver.PreloadMemory(0, 1, flatB[1], 0)
	driver.PreloadMemory(1, 0, flatB[2], 0)
	driver.PreloadMemory(1, 1, flatB[3], 0)

	kernels := api.PerPEKernels{
		{0, 0}: mult2Kernel,
		{1, 0}: mac2Kernel,
		{2, 0}: storeDataKernel,
		{0, 1}: mult1Kernel,
		{1, 1}: mac1Kernel,
		{2, 1}: storeDataKernel,
		{0, 2}: loadDataKernel,
		{1, 2}: loadDataKernel,
		{2, 2}: doNothingKernel,
	}

	if err := driver.SetPerPEKernels(kernels); err != nil {
		fmt.Println("Failed to set kernels:", err)
		return
	}

	flatA := flattenMatrix(matrixA)
	driver.FeedIn(flatA[0:2], cgra.South, [2]int{0, 2}, 2, "R")
	driver.FeedIn(flatA[2:4], cgra.South, [2]int{0, 2}, 2, "B")

	driver.Run()

	result := make([]uint32, size*size)
	result[0] = driver.ReadMemory(2, 0, 0)
	result[1] = driver.ReadMemory(2, 1, 0)
	result[2] = driver.ReadMemory(2, 0, 1)
	result[3] = driver.ReadMemory(2, 1, 1)

	fmt.Println("CGRA Result:", result)
	fmt.Println("Expected Result:", expectedFlat)
	os.Stdout = originalStdout
	if fmt.Sprint(result) != fmt.Sprint(expectedFlat) {
		fmt.Println("Test failed: results do not match")
	} else {
		fmt.Println("Test passed: results match")
	}

	// Output:
	// Test passed: results match
}

func printMatrix(matrix [][]uint32) {
	for _, row := range matrix {
		fmt.Println(row)
	}
}
