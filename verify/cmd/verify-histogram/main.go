package main

import (
	"log"
	"os"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

// main runs lint and functional simulation on histogram kernel
func main() {
	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "test/Zeonica_Testbench/kernel/histogram/histogram-instructions.yaml"
	}
	programs := core.LoadProgramFileFromYAML(programPath)
	if len(programs) == 0 {
		log.Fatal("Failed to load histogram.yaml")
	}

	// Create ArchInfo from arch_spec.yaml
	arch := &verify.ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	report := verify.GenerateReport(programs, arch, 100)
	report.WriteReport(os.Stdout)
	if len(report.LintIssues) > 0 {
		log.Fatalf("Histogram verification failed with %d lint issues", len(report.LintIssues))
	}
	if !report.SimulationOK {
		log.Fatalf("Histogram simulation failed: %v", report.SimulationErr)
	}
}
