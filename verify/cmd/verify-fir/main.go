package main

import (
	"fmt"
	"log"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

func main() {
	// Load FIR kernel from YAML
	programPath := "test/Zeonica_Testbench/kernel/fir/fir4x4.yaml"
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

	fmt.Println("==============================================================================")
	fmt.Println("FIR KERNEL VERIFICATION")
	fmt.Println("==============================================================================")
	fmt.Printf("\nLoaded %d PE programs from %s\n\n", len(programs), programPath)

	fmt.Println("==============================================================================")
	fmt.Println("STAGE 1: LINT CHECK (Structural & Timing Validation)")
	fmt.Println("==============================================================================")
	fmt.Println()

	issues := verify.RunLint(programs, arch)
	if len(issues) == 0 {
		fmt.Println("✅ LINT PASSED - No structural or timing issues found")
		fmt.Println()
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

	fmt.Println("==============================================================================")
	fmt.Println("STAGE 2: FUNCTIONAL SIMULATOR (Dataflow Verification)")
	fmt.Println("==============================================================================")
	fmt.Println()

	fs := verify.NewFunctionalSimulator(programs, arch)
	if fs == nil {
		log.Fatalf("Failed to create FunctionalSimulator")
	}

	if err := fs.Run(1000); err != nil {
		log.Fatalf("Simulation failed: %v", err)
	}

	fmt.Println("✅ FUNCTIONAL SIMULATOR PASSED - Execution completed successfully")
	fmt.Println()

	fmt.Println("==============================================================================")
	fmt.Println("VERIFICATION SUMMARY")
	fmt.Println("==============================================================================")
	fmt.Println()

	if len(issues) == 0 {
		fmt.Println("✅ Lint Check:       PASSED")
	} else {
		fmt.Printf("❌ Lint Check:       FAILED (%d issues)\n", len(issues))
	}

	fmt.Println("✅ Functional Sim:   PASSED")
	fmt.Println()
	fmt.Println("==============================================================================")
	fmt.Println()

	if len(issues) > 0 {
		log.Fatalf("FIR verification failed with %d lint issues", len(issues))
	}
}
