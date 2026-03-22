package main

import (
	"log"
	"os"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

func main() {
	// Load AXPY kernel from YAML
	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "test/Zeonica_Testbench/kernel/axpy/axpy-instructions.yaml"
	}
	programs := core.LoadProgramFileFromYAML(programPath)

	if len(programs) == 0 {
		log.Fatalf("Failed to load AXPY program from %s", programPath)
	}

	// Create architecture info (AXPY uses 4x4 CGRA)
	arch := &verify.ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	report := verify.GenerateReport(programs, arch, 1000)
	report.WriteReport(os.Stdout)
	if report.BlockingLintIssueCount() > 0 {
		log.Fatalf(
			"AXPY verification failed with %d blocking lint issues (%d warnings)",
			report.BlockingLintIssueCount(),
			report.WarningLintIssueCount(),
		)
	}
	if report.WarningLintIssueCount() > 0 {
		log.Printf("AXPY verification has %d non-blocking warnings", report.WarningLintIssueCount())
	}
	if !report.SimulationOK {
		log.Fatalf("AXPY simulation failed: %v", report.SimulationErr)
	}
}
