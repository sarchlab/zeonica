package verify

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sarchlab/zeonica/core"
)

// VerificationReport represents a complete verification report
type VerificationReport struct {
	ProgramCount  int
	LintIssues    []Issue
	StructIssues  []Issue
	TimingIssues  []Issue
	SimulationErr error
	SimulationOK  bool
	Arch          *ArchInfo
	Programs      map[string]core.Program
}

// GenerateReport runs both lint and functional simulation, returns a report
func GenerateReport(programs map[string]core.Program, arch *ArchInfo, maxSimSteps int) *VerificationReport {
	report := &VerificationReport{
		ProgramCount: len(programs),
		Arch:         arch,
		Programs:     programs,
	}

	// Run lint
	report.LintIssues = RunLint(programs, arch)

	// Categorize issues
	for _, issue := range report.LintIssues {
		if issue.Type == IssueStruct {
			report.StructIssues = append(report.StructIssues, issue)
		} else {
			report.TimingIssues = append(report.TimingIssues, issue)
		}
	}

	// Run functional simulation
	fs := NewFunctionalSimulator(programs, arch)
	report.SimulationErr = fs.Run(maxSimSteps)
	report.SimulationOK = report.SimulationErr == nil

	return report
}

// WriteReport writes a formatted report to a writer
func (r *VerificationReport) WriteReport(w io.Writer) {
	separator := strings.Repeat("=", 60)
	dash := strings.Repeat("-", 60)

	fmt.Fprintln(w, separator)
	fmt.Fprintln(w, "HISTOGRAM KERNEL VERIFICATION REPORT")
	fmt.Fprintln(w, separator)

	fmt.Fprintf(w, "\n✓ Loaded programs for %d PEs\n", r.ProgramCount)
	for coord := range r.Programs {
		fmt.Fprintf(w, "  - %s\n", coord)
	}

	// STAGE 1: LINT
	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, "STAGE 1: STATIC LINT CHECKS")
	fmt.Fprintln(w, separator)

	if len(r.LintIssues) == 0 {
		fmt.Fprintln(w, "✓ No lint issues found!")
	} else {
		fmt.Fprintf(w, "⚠ Found %d lint issues:\n\n", len(r.LintIssues))

		if len(r.StructIssues) > 0 {
			fmt.Fprintf(w, "\nSTRUCT ISSUES (%d):\n", len(r.StructIssues))
			fmt.Fprintln(w, dash)
			for _, issue := range r.StructIssues {
				fmt.Fprintf(w, "  [PE(%d,%d) t=%d op=%d] %s\n",
					issue.PEX, issue.PEY, issue.Time, issue.OpID, issue.Message)
			}
		}

		if len(r.TimingIssues) > 0 {
			fmt.Fprintf(w, "\nTIMING ISSUES (%d):\n", len(r.TimingIssues))
			fmt.Fprintln(w, dash)
			for i, issue := range r.TimingIssues {
				fmt.Fprintf(w, "  Issue %d: [PE(%d,%d) t=%d op=%d]\n",
					i+1, issue.PEX, issue.PEY, issue.Time, issue.OpID)
				fmt.Fprintf(w, "    Message: %s\n", issue.Message)
				if issue.Details != nil {
					if producer_t, ok := issue.Details["producer_t"]; ok {
						fmt.Fprintf(w, "    Producer writes at t=%v\n", producer_t)
					}
					if consumer_t, ok := issue.Details["consumer_t"]; ok {
						fmt.Fprintf(w, "    Consumer reads at t=%v\n", consumer_t)
					}
					if required_lat, ok := issue.Details["required_latency"]; ok {
						fmt.Fprintf(w, "    Required latency: %v cycles\n", required_lat)
					}
					if actual_lat, ok := issue.Details["actual_latency"]; ok {
						fmt.Fprintf(w, "    Actual latency: %v cycles\n", actual_lat)
					}
				}
				fmt.Fprintln(w)
			}
		}
	}

	// STAGE 2: FUNCTIONAL SIMULATION
	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, "STAGE 2: FUNCTIONAL SIMULATION")
	fmt.Fprintln(w, separator)

	if r.SimulationOK {
		fmt.Fprintln(w, "✓ Simulation completed successfully")
	} else {
		fmt.Fprintf(w, "⚠ Simulation error: %v\n", r.SimulationErr)
	}

	// STAGE 3: SUMMARY
	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, "VERIFICATION SUMMARY")
	fmt.Fprintln(w, separator)

	fmt.Fprintf(w, "Program Structure: %d PEs deployed\n", r.ProgramCount)
	fmt.Fprintf(w, "Lint Result: %d issues detected (%d STRUCT, %d TIMING)\n",
		len(r.LintIssues), len(r.StructIssues), len(r.TimingIssues))
	simStatus := "SUCCESS"
	if !r.SimulationOK {
		simStatus = "FAILED: " + r.SimulationErr.Error()
	}
	fmt.Fprintf(w, "Simulation Result: %s\n", simStatus)

	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, "RECOMMENDATION")
	fmt.Fprintln(w, separator)

	if len(r.TimingIssues) > 0 {
		fmt.Fprintln(w, "⚠ TIMING VIOLATIONS DETECTED")
		fmt.Fprintln(w, "The histogram kernel has cross-PE communication constraints")
		fmt.Fprintln(w, "that are not satisfied. Consider:")
		fmt.Fprintln(w, "  1. Adjusting operation timesteps to allow latency")
		fmt.Fprintln(w, "  2. Modifying the scheduling to respect network delays")
		fmt.Fprintln(w, "  3. Using buffering or pipelining strategies")
	} else {
		fmt.Fprintln(w, "✓ KERNEL PASSED ALL CHECKS")
		fmt.Fprintln(w, "The histogram kernel is ready for simulation.")
	}

	fmt.Fprintln(w)
}

// SaveReportToFile saves the report to a file and returns the filename
func (r *VerificationReport) SaveReportToFile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer file.Close()

	r.WriteReport(file)
	return nil
}
