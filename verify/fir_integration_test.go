package verify

import (
	"testing"

	"github.com/sarchlab/zeonica/core"
)

// This test ensures verify can load and process the latest FIR kernel format from
// Zeonica_Testbench (explicit timesteps, opcode aliases like DATA_MOV/GRANT_PREDICATE/RETURN).
func TestFIRKernelFromTestbench(t *testing.T) {
	programs := core.LoadProgramFileFromYAML("../test/Zeonica_Testbench/kernel/fir/fir4x4.yaml")
	if len(programs) == 0 {
		t.Fatalf("failed to load FIR programs")
	}

	arch := &ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	report := GenerateReport(programs, arch, 1000)
	if report == nil {
		t.Fatalf("expected non-nil report")
	}
	if report.ProgramCount == 0 {
		t.Fatalf("expected non-zero ProgramCount")
	}
	// We don't assert on LintIssues/SimulationOK here: FIR kernels may evolve, and this test is
	// focused on format/compatibility rather than kernel quality.
}

