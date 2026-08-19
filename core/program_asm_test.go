package core

import "testing"

func TestLoadProgramFileFromASM_PEFormat(t *testing.T) {
	path := writeTestFile(t, "program-pe.asm", `
# Compiled II: 2
PE(0,1):
{
  ADD, [EAST, RED], [#1] -> [$0] (t=0, inv_iters=0)
}
PE(1,1):
{
  DATA_MOV, [WEST, RED] -> [EAST, RED] (t=1, inv_iters=0)
}
`)

	assertLoadedASMProgram(t, LoadProgramFileFromASM(path), "(0,1)")
}

func TestLoadProgramFileFromASM_CoreFormat(t *testing.T) {
	path := writeTestFile(t, "program-core.asm", `
Core 0,0:
MOV [#1] -> [$0]
ADD [$0] [#2] -> [EAST, RED]
Core 1,0:
RETURN [WEST, RED]
`)

	assertLoadedASMProgram(t, LoadProgramFileFromASM(path), "(0,0)")
}

func assertLoadedASMProgram(t *testing.T, programs map[string]Program, expectedCoord string) {
	t.Helper()
	program, ok := programs[expectedCoord]
	if !ok {
		t.Fatalf("expected core %s, got coordinates %v", expectedCoord, programs)
	}
	if len(program.EntryBlocks) != 1 {
		t.Fatalf("core %s entry block count = %d, want 1", expectedCoord, len(program.EntryBlocks))
	}
	if len(program.EntryBlocks[0].InstructionGroups) == 0 {
		t.Fatalf("core %s has no instruction groups", expectedCoord)
	}
}

func TestParseASMOperand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected Operand
	}{
		{name: "Direction and color in brackets", input: "[NORTH, RED]", expected: Operand{Color: "R", Impl: "North"}},
		{name: "Register in brackets", input: "[$0]", expected: Operand{Impl: "$0"}},
		{name: "Immediate in brackets", input: "[#0]", expected: Operand{Impl: "0"}},
		{name: "Register without brackets", input: "$0", expected: Operand{Impl: "$0"}},
		{name: "Direction without brackets", input: "North", expected: Operand{Impl: "North"}},
		{name: "Immediate number", input: "114", expected: Operand{Impl: "114"}},
		{name: "Yellow color", input: "[WEST, YELLOW]", expected: Operand{Color: "Y", Impl: "West"}},
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
