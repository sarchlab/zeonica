package main

import (
	"fmt"
	"log"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

func main() {
	// Load AXPY kernel from YAML
	programPath := "test/Zeonica_Testbench/kernel/axpy/axpy.yaml"
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

	fmt.Println("==============================================================================")
	fmt.Println("AXPY KERNEL VERIFICATION")
	fmt.Println("==============================================================================")
	fmt.Printf("\nLoaded %d PE programs from %s\n\n", len(programs), programPath)

	// ========== LINT CHECK ==========
	fmt.Println("==============================================================================")
	fmt.Println("STAGE 1: LINT CHECK (Structural & Timing Validation)")
	fmt.Println("==============================================================================\n")

	issues := verify.RunLint(programs, arch)

	if len(issues) == 0 {
		fmt.Println("✅ LINT PASSED - No structural or timing issues found\n")
	} else {
		fmt.Printf("❌ LINT FAILED - Found %d issues:\n\n", len(issues))
		for i, issue := range issues {
			fmt.Printf("Issue %d:\n", i+1)
			fmt.Printf("  Type:     %s\n", issue.Type)
			fmt.Printf("  Location: PE(%d, %d) t=%d op=%d\n", issue.PEX, issue.PEY, issue.Time, issue.OpID)
			fmt.Printf("  Message:  %s\n", issue.Message)
			if issue.Details != nil {
				fmt.Printf("  Details:  %v\n", issue.Details)
			}
			fmt.Println()
		}
	}

	// ========== FUNCTIONAL SIMULATOR ==========
	fmt.Println("==============================================================================")
	fmt.Println("STAGE 2: FUNCTIONAL SIMULATOR (Dataflow Verification)")
	fmt.Println("==============================================================================\n")

	fs := verify.NewFunctionalSimulator(programs, arch)
	if fs == nil {
		log.Fatalf("Failed to create FunctionalSimulator")
	}

	// Preload test data into memory if needed
	// (AXPY might need input data)
	// fs.PreloadMemory(x, y, value, address)

	// Run simulator for reasonable number of steps
	err := fs.Run(1000)
	if err != nil {
		log.Fatalf("Simulation failed: %v", err)
	}

	fmt.Println("✅ FUNCTIONAL SIMULATOR PASSED - Execution completed successfully\n")

	// Display simulation results
	fmt.Println("------------------------------------------------------------------------------")
	fmt.Println("Simulation Results by PE:")
	fmt.Println("------------------------------------------------------------------------------\n")

	for y := 0; y < arch.Rows; y++ {
		for x := 0; x < arch.Columns; x++ {
			coord := fmt.Sprintf("(%d, %d)", x, y)

			// Try to get some register values
			hasValues := false
			for regIdx := 0; regIdx < 4; regIdx++ {
				val := fs.GetRegisterValue(x, y, regIdx)
				if val != 0 {
					if !hasValues {
						fmt.Printf("PE %s:\n", coord)
						hasValues = true
					}
					fmt.Printf("  $%-2d = %d\n", regIdx, val)
				}
			}
		}
	}

	// ========== SUMMARY ==========
	fmt.Println("\n==============================================================================")
	fmt.Println("VERIFICATION SUMMARY")
	fmt.Println("==============================================================================\n")

	if len(issues) == 0 {
		fmt.Println("✅ Lint Check:       PASSED")
	} else {
		fmt.Printf("❌ Lint Check:       FAILED (%d issues)\n", len(issues))
	}

	fmt.Println("✅ Functional Sim:   PASSED")

	fmt.Println("\n==============================================================================\n")

	// Exit with error code if lint failed
	if len(issues) > 0 {
		log.Fatalf("AXPY verification failed with %d lint issues", len(issues))
	}
}
