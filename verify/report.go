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
	ProgramCount    int
	LintIssues      []Issue
	StructIssues    []Issue
	TimingIssues    []Issue
	PredicateIssues []Issue
	SimulationErr   error
	SimulationOK    bool
	Arch            *ArchInfo
	Programs        map[string]core.Program
}

func predicateIssueIsPossible(issue Issue) bool {
	if issue.Type != IssuePredicate {
		return false
	}
	if issue.Details == nil {
		return false
	}
	certainty, ok := issue.Details["certainty"]
	if !ok {
		return false
	}
	certaintyStr, ok := certainty.(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(certaintyStr), "possible")
}

// BlockingLintIssues returns lint issues that should fail verification.
// Current policy:
// - STRUCT/TIMING issues are always blocking.
// - PREDICATE issues are blocking unless tagged as certainty=possible.
func (r *VerificationReport) BlockingLintIssues() []Issue {
	blocking := make([]Issue, 0, len(r.LintIssues))
	for _, issue := range r.LintIssues {
		if issue.Type == IssuePredicate && predicateIssueIsPossible(issue) {
			continue
		}
		blocking = append(blocking, issue)
	}
	return blocking
}

// WarningLintIssues returns non-blocking lint issues (currently predicate possible).
func (r *VerificationReport) WarningLintIssues() []Issue {
	warnings := make([]Issue, 0)
	for _, issue := range r.LintIssues {
		if issue.Type == IssuePredicate && predicateIssueIsPossible(issue) {
			warnings = append(warnings, issue)
		}
	}
	return warnings
}

// BlockingLintIssueCount returns number of blocking lint issues.
func (r *VerificationReport) BlockingLintIssueCount() int {
	return len(r.BlockingLintIssues())
}

// WarningLintIssueCount returns number of warning lint issues.
func (r *VerificationReport) WarningLintIssueCount() int {
	return len(r.WarningLintIssues())
}

// GenerateReport runs both lint and functional simulation, returns a report.
// Optional lint options can be provided to tune predicate analysis.
func GenerateReport(
	programs map[string]core.Program,
	arch *ArchInfo,
	maxSimSteps int,
	opts ...LintOptions,
) *VerificationReport {
	report := &VerificationReport{
		ProgramCount: len(programs),
		Arch:         arch,
		Programs:     programs,
	}

	// Run lint
	report.LintIssues = RunLint(programs, arch, opts...)

	// Categorize issues
	for _, issue := range report.LintIssues {
		switch issue.Type {
		case IssueStruct:
			report.StructIssues = append(report.StructIssues, issue)
		case IssueTiming:
			report.TimingIssues = append(report.TimingIssues, issue)
		case IssuePredicate:
			report.PredicateIssues = append(report.PredicateIssues, issue)
		default:
			report.TimingIssues = append(report.TimingIssues, issue)
		}
	}

	// Run functional simulation
	fs := NewFunctionalSimulator(programs, arch)
	report.SimulationErr = fs.Run(maxSimSteps)
	report.SimulationOK = report.SimulationErr == nil

	return report
}

// WriteReport writes a formatted report to a writer.
//
//nolint:gocyclo,funlen
func (r *VerificationReport) WriteReport(w io.Writer) {
	separator := strings.Repeat("=", 60)
	dash := strings.Repeat("-", 60)

	fmt.Fprintln(w, separator)
	fmt.Fprintln(w, "KERNEL VERIFICATION REPORT")
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
		fmt.Fprintf(
			w,
			"  Blocking: %d, Warning: %d\n",
			r.BlockingLintIssueCount(),
			r.WarningLintIssueCount(),
		)

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
					if producerT, ok := issue.Details["producer_t"]; ok {
						fmt.Fprintf(w, "    Producer writes at t=%v\n", producerT)
					}
					if consumerT, ok := issue.Details["consumer_t"]; ok {
						fmt.Fprintf(w, "    Consumer reads at t=%v\n", consumerT)
					}
					if requiredLat, ok := issue.Details["required_latency"]; ok {
						fmt.Fprintf(w, "    Required latency: %v cycles\n", requiredLat)
					}
					if actualLat, ok := issue.Details["actual_latency"]; ok {
						fmt.Fprintf(w, "    Actual latency: %v cycles\n", actualLat)
					}
				}
				fmt.Fprintln(w)
			}
		}

		if len(r.PredicateIssues) > 0 {
			fmt.Fprintf(w, "\nPREDICATE ISSUES (%d):\n", len(r.PredicateIssues))
			fmt.Fprintln(w, dash)
			for i, issue := range r.PredicateIssues {
				fmt.Fprintf(w, "  Issue %d: [PE(%d,%d) t=%d op=%d]\n",
					i+1, issue.PEX, issue.PEY, issue.Time, issue.OpID)
				fmt.Fprintf(w, "    Message: %s\n", issue.Message)
				if issue.Details != nil {
					if opCode, ok := issue.Details["opcode"]; ok {
						fmt.Fprintf(w, "    OpCode: %v\n", opCode)
					}
					if certainty, ok := issue.Details["certainty"]; ok {
						fmt.Fprintf(w, "    Certainty: %v\n", certainty)
					}
					if warmupHits, ok := issue.Details["warmup_hits"]; ok {
						fmt.Fprintf(w, "    Warmup hits: %v\n", warmupHits)
					}
					if steadyHits, ok := issue.Details["steady_hits"]; ok {
						fmt.Fprintf(w, "    Steady hits: %v\n", steadyHits)
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
	fmt.Fprintf(w, "Lint Result: %d issues detected (%d STRUCT, %d TIMING, %d PREDICATE)\n",
		len(r.LintIssues), len(r.StructIssues), len(r.TimingIssues), len(r.PredicateIssues))
	fmt.Fprintf(
		w,
		"Lint Severity: %d blocking, %d warning\n",
		r.BlockingLintIssueCount(),
		r.WarningLintIssueCount(),
	)
	simStatus := "SUCCESS"
	if !r.SimulationOK {
		simStatus = "FAILED: " + r.SimulationErr.Error()
	}
	fmt.Fprintf(w, "Simulation Result: %s\n", simStatus)

	fmt.Fprintln(w, "\n"+separator)
	fmt.Fprintln(w, "RECOMMENDATION")
	fmt.Fprintln(w, separator)

	if r.BlockingLintIssueCount() > 0 {
		fmt.Fprintln(w, "⚠ BLOCKING LINT ISSUES DETECTED")
		fmt.Fprintln(w, "This kernel still has structural/timing/definite-predicate issues.")
		fmt.Fprintln(w, "Fix blocking issues before trusting simulation results.")
	} else if r.WarningLintIssueCount() > 0 {
		fmt.Fprintln(w, "⚠ PREDICATE RISKS DETECTED")
		fmt.Fprintln(w, "Only non-blocking predicate warnings are present (certainty=possible).")
		fmt.Fprintln(w, "You may still review PHI/PHI_START/GPRED flows for robustness.")
	} else {
		fmt.Fprintln(w, "✓ KERNEL PASSED ALL CHECKS")
		fmt.Fprintln(w, "The kernel is ready for simulation.")
	}

	fmt.Fprintln(w)
}

// SaveReportToFile saves the report to a file and returns the filename
func (r *VerificationReport) SaveReportToFile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create report file: %w", err)
	}
	defer func() { _ = file.Close() }()

	r.WriteReport(file)
	return nil
}
