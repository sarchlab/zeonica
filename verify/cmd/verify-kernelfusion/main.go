package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/verify"
)

func main() {
	programPath := os.Getenv("ZEONICA_PROGRAM_YAML")
	if programPath == "" {
		programPath = "test/testbench/kernelfusion/tmp-generated-instructions.yaml"
	}

	rows := getEnvInt("VERIFY_ROWS", 8)
	cols := getEnvInt("VERIFY_COLS", 8)
	maxPrint := getEnvInt("VERIFY_MAX_PRINT", 50)
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

	programs := core.LoadProgramFileFromYAML(programPath)
	if len(programs) == 0 {
		fmt.Printf("failed to load program: %s\n", programPath)
		os.Exit(2)
	}

	arch := &verify.ArchInfo{
		Rows:         rows,
		Columns:      cols,
		Topology:     "mesh",
		HopLatency:   1,
		MemCapacity:  2048,
		CtrlMemItems: 20,
	}

	issues := verify.RunLint(programs, arch, lintOpts)
	structCnt := 0
	timingCnt := 0
	predicateCnt := 0
	for _, it := range issues {
		switch it.Type {
		case verify.IssueStruct:
			structCnt++
		case verify.IssueTiming:
			timingCnt++
		case verify.IssuePredicate:
			predicateCnt++
		}
	}

	fmt.Printf("program: %s\n", programPath)
	fmt.Printf("arch: %dx%d\n", cols, rows)
	fmt.Printf(
		"predicate_lint: prologueAware=%t warmupCap=%d steadyPasses=%d\n",
		lintOpts.EnablePrologueAwarePredicate,
		lintOpts.PredicateWarmupPassCap,
		lintOpts.PredicateSteadyStatePasses,
	)
	fmt.Printf(
		"total issues: %d (STRUCT=%d, TIMING=%d, PREDICATE=%d)\n",
		len(issues),
		structCnt,
		timingCnt,
		predicateCnt,
	)
	report := &verify.VerificationReport{LintIssues: issues}
	fmt.Printf(
		"severity: blocking=%d, warning=%d\n",
		report.BlockingLintIssueCount(),
		report.WarningLintIssueCount(),
	)

	if len(issues) == 0 {
		fmt.Println("verify lint passed: no dependency/timing issues found")
		return
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Type != issues[j].Type {
			return issues[i].Type < issues[j].Type
		}
		if issues[i].PEY != issues[j].PEY {
			return issues[i].PEY < issues[j].PEY
		}
		if issues[i].PEX != issues[j].PEX {
			return issues[i].PEX < issues[j].PEX
		}
		if issues[i].Time != issues[j].Time {
			return issues[i].Time < issues[j].Time
		}
		return issues[i].OpID < issues[j].OpID
	})

	limit := len(issues)
	if maxPrint > 0 && limit > maxPrint {
		limit = maxPrint
	}
	fmt.Printf("\nshowing first %d issue(s):\n", limit)
	for i := 0; i < limit; i++ {
		it := issues[i]
		fmt.Printf(
			"[%03d] %-6s PE(%d,%d) t=%d op=%d | %s\n",
			i+1, it.Type, it.PEX, it.PEY, it.Time, it.OpID, it.Message,
		)
	}

	if limit < len(issues) {
		fmt.Printf("... %d more issue(s) not shown\n", len(issues)-limit)
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
