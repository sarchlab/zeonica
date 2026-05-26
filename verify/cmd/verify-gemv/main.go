package main

import (
	"log"
	"os"
	"strconv"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

// main runs lint and functional simulation on gemv kernel
func main() {
	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "test/testbench/gemv/tmp-generated-instructions.yaml"
	}
	programs := core.LoadProgramFileFromYAML(programPath)
	if len(programs) == 0 {
		log.Fatalf("Failed to load gemv program from %s", programPath)
	}
	lintOpts := verify.DefaultLintOptions()
	lintOpts.EnablePrologueAwarePredicate = getEnvBool(
		"VERIFY_PRED_PROLOGUE_AWARE",
		lintOpts.EnablePrologueAwarePredicate,
	)
	lintOpts.PredicateWarmupPassCap = getEnvInt(
		"VERIFY_PRED_WARMUP_CAP",
		lintOpts.PredicateWarmupPassCap,
	)
	lintOpts.PredicateSteadyStatePasses = getEnvInt(
		"VERIFY_PRED_STEADY_PASSES",
		lintOpts.PredicateSteadyStatePasses,
	)

	arch := &verify.ArchInfo{
		Rows:         4,
		Columns:      4,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	report := verify.GenerateReport(programs, arch, 1000, lintOpts)
	report.WriteReport(os.Stdout)
	if report.BlockingLintIssueCount() > 0 {
		log.Fatalf(
			"GEMV verification failed with %d blocking lint issues (%d warnings)",
			report.BlockingLintIssueCount(),
			report.WarningLintIssueCount(),
		)
	}
	if report.WarningLintIssueCount() > 0 {
		log.Printf("GEMV verification has %d non-blocking warnings", report.WarningLintIssueCount())
	}
	if !report.SimulationOK {
		log.Fatalf("GEMV simulation failed: %v", report.SimulationErr)
	}
}

func getEnvInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getEnvBool(name string, fallback bool) bool {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}
