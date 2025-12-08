package verify

import (
	"testing"

	"github.com/sarchlab/zeonica/core"
)

// TestRunLintBasic validates the lint checks work on a simple program
func TestRunLintBasic(t *testing.T) {
	// Create a minimal ArchInfo
	arch := &ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// Create a simple valid program at (0,0)
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "#5"}, // Immediate
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0"}, // Register
									},
								},
							},
						},
					},
				},
			},
		},
	}

	programs := map[string]core.Program{
		"(0, 0)": prog,
	}

	issues := RunLint(programs, arch)
	if len(issues) > 0 {
		t.Errorf("Expected no lint issues for valid program, got %d", len(issues))
		for _, issue := range issues {
			t.Logf("  Issue: %s at (%d, %d): %s", issue.Type, issue.PEX, issue.PEY, issue.Message)
		}
	}
}

// TestRunLintInvalidCoordinate validates detection of out-of-bounds coordinates
func TestRunLintInvalidCoordinate(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode:      "NOP",
								SrcOperands: core.OperandList{Operands: []core.Operand{}},
								DstOperands: core.OperandList{Operands: []core.Operand{}},
							},
						},
					},
				},
			},
		},
	}

	programs := map[string]core.Program{
		"(5, 5)": prog, // Out of bounds
	}

	issues := RunLint(programs, arch)
	found := false
	for _, issue := range issues {
		if issue.Type == IssueStruct {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected STRUCT issue for out-of-bounds coordinate")
	}
}

// TestFunctionalSimulatorBasic tests basic operation execution
func TestFunctionalSimulatorBasic(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// Create a program with MOV and ADD
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#10"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "ADD",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0"},
										{Impl: "#5"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$1"}},
								},
							},
						},
					},
				},
			},
		},
	}

	programs := map[string]core.Program{
		"(0, 0)": prog,
	}

	fs := NewFunctionalSimulator(programs, arch)
	if fs == nil {
		t.Fatal("Failed to create FunctionalSimulator")
	}

	// Run simulator
	err := fs.Run(100)
	if err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}

	// Validate results
	val0 := fs.GetRegisterValue(0, 0, 0)
	if val0 != 10 {
		t.Errorf("Expected $0 = 10, got %d", val0)
	}

	val1 := fs.GetRegisterValue(0, 0, 1)
	if val1 != 15 {
		t.Errorf("Expected $1 = 15, got %d", val1)
	}
}

// TestFunctionalSimulatorMemory tests LOAD and STORE operations
func TestFunctionalSimulatorMemory(t *testing.T) {
	arch := &ArchInfo{
		Rows:         1,
		Columns:      1,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// Create program with STORE and LOAD
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#42"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "STORE",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0"},
										{Impl: "#0"},
									},
								},
								DstOperands: core.OperandList{Operands: []core.Operand{}},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "LOAD",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#0"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$1"}},
								},
							},
						},
					},
				},
			},
		},
	}

	programs := map[string]core.Program{
		"(0, 0)": prog,
	}

	fs := NewFunctionalSimulator(programs, arch)
	if fs == nil {
		t.Fatal("Failed to create FunctionalSimulator")
	}

	// Run simulator
	err := fs.Run(100)
	if err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}

	// Validate: $1 should contain 42 (loaded from memory)
	val1 := fs.GetRegisterValue(0, 0, 1)
	if val1 != 42 {
		t.Errorf("Expected $1 = 42 (from memory), got %d", val1)
	}
}
