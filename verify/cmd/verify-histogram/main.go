package main

import (
	"fmt"
	"log"
	"os"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

// main runs lint and functional simulation on histogram kernel
func main() {
	programs := core.LoadProgramFileFromYAML("test/Zeonica_Testbench/kernel/histogram/histogram.yaml")
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

	// Generate report
	report := verify.GenerateReport(programs, arch, 100)

	// Write to stdout
	report.WriteReport(os.Stdout)

	// Save to file
	err := report.SaveReportToFile("histogram_verification_report.txt")
	if err != nil {
		log.Fatalf("Failed to save report: %v", err)
	}

	fmt.Println("✓ Report saved to: histogram_verification_report.txt")
}
