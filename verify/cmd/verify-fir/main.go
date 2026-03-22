package main

import (
	"log"
	"os"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

func main() {
	// Load FIR kernel from YAML
	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "test/Zeonica_Testbench/kernel/fir/fir-instructions.yaml"
	}
	programs := core.LoadProgramFileFromYAML(programPath)
	if len(programs) == 0 {
		log.Fatalf("Failed to load FIR program from %s", programPath)
	}

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
			"FIR verification failed with %d blocking lint issues (%d warnings)",
			report.BlockingLintIssueCount(),
			report.WarningLintIssueCount(),
		)
	}
	if report.WarningLintIssueCount() > 0 {
		log.Printf("FIR verification has %d non-blocking warnings", report.WarningLintIssueCount())
	}
	if !report.SimulationOK {
		log.Fatalf("FIR simulation failed: %v", report.SimulationErr)
	}
}
