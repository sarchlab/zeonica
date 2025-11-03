package core

import (
	"fmt"
	"os"
	"testing"
)

func TestLoadProgramFileFromASM_PEFormat(t *testing.T) {
	// Test with fir4x4.asm format
	filePath := "../test/Zeonica_Testbench/kernel/fir/fir4x4.asm"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("Test file does not exist: %s", filePath)
	}

	// Load the program file
	programMap := LoadProgramFileFromASM(filePath)

	// Verify that we loaded some programs
	if len(programMap) == 0 {
		t.Error("No programs were loaded from the ASM file")
	}

	// Print the loaded programs
	fmt.Println("=== Loaded Programs (PE Format) ===")
	for coord, program := range programMap {
		fmt.Printf("\n--- Core at %s ---\n", coord)
		PrintProgram(program)
	}

	// Print summary
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total cores loaded: %d\n", len(programMap))

	// Verify specific core exists
	if _, exists := programMap["(0,1)"]; !exists {
		t.Error("Expected core at (0,1) not found")
	}

	// Verify core has entry blocks
	for coord, program := range programMap {
		if len(program.EntryBlocks) == 0 {
			t.Errorf("Core at %s has no entry blocks", coord)
		}
		for _, entryBlock := range program.EntryBlocks {
			if len(entryBlock.InstructionGroups) == 0 {
				t.Errorf("Core at %s has no instruction groups", coord)
			}
		}
	}
}

func TestLoadProgramFileFromASM_CoreFormat(t *testing.T) {
	// Test with fir.asm format
	filePath := "../test/fir/fir.asm"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("Test file does not exist: %s", filePath)
	}

	// Load the program file
	programMap := LoadProgramFileFromASM(filePath)

	// Verify that we loaded some programs
	if len(programMap) == 0 {
		t.Error("No programs were loaded from the ASM file")
	}

	// Print the loaded programs
	fmt.Println("=== Loaded Programs (Core Format) ===")
	for coord, program := range programMap {
		fmt.Printf("\n--- Core at %s ---\n", coord)
		PrintProgram(program)
	}

	// Print summary
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total cores loaded: %d\n", len(programMap))

	// Verify specific core exists
	if _, exists := programMap["(0,0)"]; !exists {
		t.Error("Expected core at (0,0) not found")
	}

	// Verify core has entry blocks
	for coord, program := range programMap {
		if len(program.EntryBlocks) == 0 {
			t.Errorf("Core at %s has no entry blocks", coord)
		}
		for _, entryBlock := range program.EntryBlocks {
			if len(entryBlock.InstructionGroups) == 0 {
				t.Errorf("Core at %s has no instruction groups", coord)
			}
		}
	}
}

func TestParseASMOperand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Operand
	}{
		{
			name:  "Direction and color in brackets",
			input: "[NORTH, RED]",
			expected: Operand{
				Flag:  false,
				Color: "R",
				Impl:  "North",
			},
		},
		{
			name:  "Register in brackets",
			input: "[$0]",
			expected: Operand{
				Flag:  false,
				Color: "",
				Impl:  "$0",
			},
		},
		{
			name:  "Immediate in brackets",
			input: "[#0]",
			expected: Operand{
				Flag:  false,
				Color: "",
				Impl:  "0",
			},
		},
		{
			name:  "Register without brackets",
			input: "$0",
			expected: Operand{
				Flag:  false,
				Color: "",
				Impl:  "$0",
			},
		},
		{
			name:  "Direction without brackets",
			input: "North",
			expected: Operand{
				Flag:  false,
				Color: "",
				Impl:  "North",
			},
		},
		{
			name:  "Immediate number",
			input: "114",
			expected: Operand{
				Flag:  false,
				Color: "",
				Impl:  "114",
			},
		},
		{
			name:  "Yellow color",
			input: "[WEST, YELLOW]",
			expected: Operand{
				Flag:  false,
				Color: "Y",
				Impl:  "West",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseASMOperand(tt.input)
			if result.Flag != tt.expected.Flag {
				t.Errorf("Flag: got %v, want %v", result.Flag, tt.expected.Flag)
			}
			if result.Color != tt.expected.Color {
				t.Errorf("Color: got %v, want %v", result.Color, tt.expected.Color)
			}
			if result.Impl != tt.expected.Impl {
				t.Errorf("Impl: got %v, want %v", result.Impl, tt.expected.Impl)
			}
		})
	}
}

func TestParseASMInstruction(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectedOp   string
		expectedSrcs int
		expectedDsts int
	}{
		{
			name:         "Comma-separated format with brackets",
			input:        "GRANT_ONCE, [#0] -> [$0]",
			expectedOp:   "GRANT_ONCE",
			expectedSrcs: 1,
			expectedDsts: 1,
		},
		{
			name:         "Space-separated format with immediate",
			input:        "PHI_CONST 0 East -> East South $0",
			expectedOp:   "PHI_CONST",
			expectedSrcs: 2,
			expectedDsts: 3,
		},
		{
			name:         "Multiple source operands",
			input:        "ADD, [NORTH, RED], [$0] -> [WEST, RED], [SOUTH, RED]",
			expectedOp:   "ADD",
			expectedSrcs: 2,
			expectedDsts: 2,
		},
		{
			name:         "No destination operands",
			input:        "RETURN, [WEST, RED]",
			expectedOp:   "RETURN",
			expectedSrcs: 1,
			expectedDsts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opcode, srcOps, dstOps := parseASMInstruction(tt.input)
			if opcode != tt.expectedOp {
				t.Errorf("Opcode: got %v, want %v", opcode, tt.expectedOp)
			}
			if len(srcOps) != tt.expectedSrcs {
				t.Errorf("Source operands: got %d, want %d", len(srcOps), tt.expectedSrcs)
			}
			if len(dstOps) != tt.expectedDsts {
				t.Errorf("Destination operands: got %d, want %d", len(dstOps), tt.expectedDsts)
			}
		})
	}
}
