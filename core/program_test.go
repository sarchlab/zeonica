package core

import (
	"fmt"
	"os"
	"testing"
)

func TestLoadProgramFileFromYAML(t *testing.T) {
	// Use histogram.yaml from Zeonica_Testbench as test file
	filePath := "../test/Zeonica_Testbench/kernel/histogram/histogram.yaml"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("Test file does not exist: %s (skipping test)", filePath)
	}

	// Read and print first few lines of the file for debugging
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	fmt.Printf("=== File content (first 500 chars) ===\n")
	if len(data) > 500 {
		fmt.Printf("%s...\n", string(data[:500]))
	} else {
		fmt.Printf("%s\n", string(data))
	}

	// Load the program file - adjust path from core/ directory
	programMap := LoadProgramFileFromYAML(filePath)

	// Print the loaded programs
	fmt.Println("=== Loaded Programs ===")
	for coord, program := range programMap {
		fmt.Printf("\n--- Core at %s ---\n", coord)
		PrintProgram(program)
	}

	// Verify that we loaded some programs
	if len(programMap) == 0 {
		t.Error("No programs were loaded from the file")
	}

	// Print summary
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total cores loaded: %d\n", len(programMap))

	// Print each core's coordinate
	fmt.Println("Core coordinates:")
	for coord := range programMap {
		fmt.Printf("  %s\n", coord)
	}
}
