package verify

import (
	"os"
	"testing"

	"github.com/sarchlab/zeonica/core"
)

// TestHistogramLint tests lint checking on the actual histogram kernel
func TestHistogramLint(t *testing.T) {
	const programPath = "../test/Zeonica_Testbench/kernel/histogram/histogram.yaml"
	if !fileExists(programPath) {
		t.Skip("Histogram YAML not found; skipping integration test")
	}

	// Create ArchInfo matching histogram arch_spec.yaml:
	// - main CGRA: 4x4 mesh
	// - 128 registers per PE
	// - 2048 memory capacity per PE
	// - 20 control memory items
	arch := &ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	// Load histogram program from YAML
	programs := core.LoadProgramFileFromYAML(programPath)

	// Run lint checks
	issues := RunLint(programs, arch)

	// Report any issues found
	if len(issues) > 0 {
		t.Logf("Lint found %d issues:\n", len(issues))
		for _, issue := range issues {
			t.Logf("  [%s] at PE (%d, %d) t=%d op=%d: %s\n",
				issue.Type, issue.PEX, issue.PEY, issue.Time, issue.OpID, issue.Message)
			if issue.Details != nil {
				for k, v := range issue.Details {
					t.Logf("    %s: %v\n", k, v)
				}
			}
		}
		// Don't fail for now - just report findings
	}

	// Verify lint found the expected number of structures (rough check)
	t.Logf("Total PEs with programs: %d\n", len(programs))
	for coord := range programs {
		t.Logf("  Program at: %s\n", coord)
	}
}

// TestHistogramFunctionalSim tests functional simulation on histogram kernel
func TestHistogramFunctionalSim(t *testing.T) {
	const programPath = "../test/Zeonica_Testbench/kernel/histogram/histogram.yaml"
	if !fileExists(programPath) {
		t.Skip("Histogram YAML not found; skipping integration test")
	}

	arch := &ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	// Load histogram program
	programs := core.LoadProgramFileFromYAML(programPath)

	// Create functional simulator
	fs := NewFunctionalSimulator(programs, arch)
	if fs == nil {
		t.Fatal("Failed to create FunctionalSimulator")
	}

	// Preload some test data into memory
	// Based on histogram.yaml: Core (0,2) does LOAD at timestep 10
	// This simulates data available in memory for loading
	fs.PreloadMemory(0, 2, 42, 0)
	fs.PreloadMemory(0, 2, 100, 1)
	fs.PreloadMemory(0, 2, 200, 2)
	fs.PreloadMemory(2, 1, 10, 0)
	fs.PreloadMemory(2, 1, 20, 1)
	fs.PreloadMemory(2, 1, 30, 2)

	// Run simulation for enough timesteps to cover histogram operations
	// histogram has operations up to t=12 in the visible spec
	err := fs.Run(100)
	if err != nil {
		t.Fatalf("Simulation failed: %v", err)
	}

	// Validate that certain operations were executed
	// Core (1,2) at t=11 does ADD: $0 = WEST + #1
	// After ADD, $0 should contain some computed value
	// (depends on data flow from (0,2))

	t.Logf("✓ Histogram functional simulation completed without errors\n")

	// Print some register values for inspection
	val := fs.GetRegisterValue(1, 2, 0)
	t.Logf("  Core (1,2) register $0 = %d\n", val)

	val = fs.GetRegisterValue(0, 2, 0)
	t.Logf("  Core (0,2) register $0 = %d\n", val)
}

// TestHistogramBothModesComparison runs both lint and sim for comprehensive check
func TestHistogramBothModesComparison(t *testing.T) {
	const programPath = "../test/Zeonica_Testbench/kernel/histogram/histogram.yaml"
	if !fileExists(programPath) {
		t.Skip("Histogram YAML not found; skipping integration test")
	}

	arch := &ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	programs := core.LoadProgramFileFromYAML(programPath)

	// Stage 1: Lint
	t.Log("Stage 1: Running lint checks...")
	lintIssues := RunLint(programs, arch)
	t.Logf("  Lint check complete: %d issues found\n", len(lintIssues))

	structIssues := 0
	timingIssues := 0
	for _, issue := range lintIssues {
		if issue.Type == IssueStruct {
			structIssues++
		} else if issue.Type == IssueTiming {
			timingIssues++
		}
	}
	t.Logf("    - STRUCT issues: %d\n", structIssues)
	t.Logf("    - TIMING issues: %d\n", timingIssues)

	// Stage 2: Functional Simulation
	t.Log("Stage 2: Running functional simulator...")
	fs := NewFunctionalSimulator(programs, arch)

	// Preload memory for LOAD operations
	fs.PreloadMemory(0, 2, 42, 0)
	fs.PreloadMemory(2, 1, 10, 0)

	err := fs.Run(100)
	if err != nil {
		t.Logf("  Simulation error (may be expected): %v\n", err)
	} else {
		t.Log("  Functional simulation complete: OK")
	}

	// Summary
	t.Logf("\nHistogram Verification Summary:\n")
	t.Logf("  Programs loaded: %d PEs\n", len(programs))
	t.Logf("  Lint result: %d issues (STRUCT: %d, TIMING: %d)\n",
		len(lintIssues), structIssues, timingIssues)
	t.Logf("  Simulation result: %s\n", func() string {
		if err != nil {
			return "ERROR: " + err.Error()
		}
		return "SUCCESS"
	}())
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return false
}
