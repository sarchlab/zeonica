package core

import (
	"fmt"
	"os"
	"testing"
)

func TestLoadProgramFileFromYAML(t *testing.T) {
	// Check if file exists
	filePath := "../test/Zeonica_Testbench/kernel/fir/fir4x4.yaml"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("File does not exist: %s", filePath)
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

	// Basic sanity checks for explicit timestep materialization:
	// PE(0,1) has its first instruction at t=3 in the FIR sample.
	if prog, ok := programMap["(0,1)"]; ok {
		if len(prog.EntryBlocks) == 0 || len(prog.EntryBlocks[0].InstructionGroups) < 4 {
			t.Fatalf("expected PE(0,1) to have >= 4 instruction groups, got %d",
				len(prog.EntryBlocks[0].InstructionGroups))
		}
		if len(prog.EntryBlocks[0].InstructionGroups[0].Operations) != 0 {
			t.Fatalf("expected PE(0,1) timestep 0 to be empty (NOP), got %d ops",
				len(prog.EntryBlocks[0].InstructionGroups[0].Operations))
		}
		if len(prog.EntryBlocks[0].InstructionGroups[3].Operations) == 0 {
			t.Fatalf("expected PE(0,1) timestep 3 to have ops, got 0")
		}
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
