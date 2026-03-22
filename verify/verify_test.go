//nolint:funlen
package verify

import (
	"strings"
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

func TestRunLintPredicatePhiStartFirstSourceRisk(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// Force $0 predicate=false, then PHI_START reads $0 as first source.
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "GRANT_PREDICATE",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "#1", Color: "RED"},
										{Impl: "#0", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0", Color: "RED"},
									},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "PHI_START",
								ID:     145,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0", Color: "RED"},
										{Impl: "$1", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$2", Color: "RED"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := RunLint(map[string]core.Program{"(0, 0)": prog}, arch)

	found := false
	for _, issue := range issues {
		if issue.Type == IssuePredicate && strings.Contains(issue.Message, "PHI_START") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a PREDICATE issue for PHI_START first-source risk, got %v", issues)
	}
}

func TestRunLintPredicatePhiBothTrueRisk(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// $0 and $1 are both definitely true before PHI.
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#1", Color: "RED"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#2", Color: "RED"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$1", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "PHI",
								ID:     111,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0", Color: "RED"},
										{Impl: "$1", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$2", Color: "RED"}},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := RunLint(map[string]core.Program{"(0, 0)": prog}, arch)

	found := false
	for _, issue := range issues {
		if issue.Type == IssuePredicate && strings.Contains(issue.Message, "PHI") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected a PREDICATE issue for PHI both-true risk, got %v", issues)
	}
}

func TestVerificationReportBlockingAndWarningCounts(t *testing.T) {
	report := &VerificationReport{
		LintIssues: []Issue{
			{Type: IssueStruct, Message: "struct issue"},
			{Type: IssueTiming, Message: "timing issue"},
			{
				Type:    IssuePredicate,
				Message: "predicate possible",
				Details: map[string]interface{}{"certainty": "possible"},
			},
			{
				Type:    IssuePredicate,
				Message: "predicate definite",
				Details: map[string]interface{}{"certainty": "definite"},
			},
			{
				Type:    IssuePredicate,
				Message: "predicate without certainty",
			},
		},
	}

	if got := report.WarningLintIssueCount(); got != 1 {
		t.Fatalf("expected 1 warning issue, got %d", got)
	}
	if got := report.BlockingLintIssueCount(); got != 4 {
		t.Fatalf("expected 4 blocking issues, got %d", got)
	}
}

func TestRunLintPredicatePhiStartInvalidIterationsPrologueSafe(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// The first PHI_START execution should see $0=true.
	// A later GPRED overwrites $0 predicate to false, but only after one invalid iteration.
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode: "MOV",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#1", Color: "RED"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "GRANT_PREDICATE",
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "#1", Color: "RED"},
										{Impl: "#0", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$2", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode:            "GRANT_PREDICATE",
								InvalidIterations: 1,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "#1", Color: "RED"},
										{Impl: "#0", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode: "PHI_START",
								ID:     210,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0", Color: "RED"},
										{Impl: "$2", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$3", Color: "RED"}},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := RunLint(map[string]core.Program{"(0, 0)": prog}, arch)
	for _, issue := range issues {
		if issue.Type == IssuePredicate && strings.Contains(issue.Message, "first source") {
			t.Fatalf("unexpected PHI_START first-source issue with prologue protection: %+v", issue)
		}
	}
}

func TestRunLintPredicatePhiSteadyStateAfterWarmupStillDetected(t *testing.T) {
	arch := &ArchInfo{
		Rows:         2,
		Columns:      2,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  1024,
		CtrlMemItems: 256,
	}

	// All three ops become executable after warmup; PHI should still be flagged.
	prog := core.Program{
		EntryBlocks: []core.EntryBlock{
			{
				InstructionGroups: []core.InstructionGroup{
					{
						Operations: []core.Operation{
							{
								OpCode:            "MOV",
								InvalidIterations: 1,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#1", Color: "RED"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$0", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode:            "MOV",
								InvalidIterations: 1,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "#2", Color: "RED"}},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$1", Color: "RED"}},
								},
							},
						},
					},
					{
						Operations: []core.Operation{
							{
								OpCode:            "PHI",
								ID:                211,
								InvalidIterations: 1,
								SrcOperands: core.OperandList{
									Operands: []core.Operand{
										{Impl: "$0", Color: "RED"},
										{Impl: "$1", Color: "RED"},
									},
								},
								DstOperands: core.OperandList{
									Operands: []core.Operand{{Impl: "$2", Color: "RED"}},
								},
							},
						},
					},
				},
			},
		},
	}

	issues := RunLint(map[string]core.Program{"(0, 0)": prog}, arch)
	foundDefinitePhi := false
	for _, issue := range issues {
		if issue.Type == IssuePredicate && strings.Contains(issue.Message, "PHI id=211") {
			if certainty, ok := issue.Details["certainty"].(string); ok && certainty == "definite" {
				foundDefinitePhi = true
			}
		}
	}
	if !foundDefinitePhi {
		t.Fatalf("expected definite PHI predicate issue after warmup, got: %+v", issues)
	}
}
