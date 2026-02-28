package runtimecfg

import (
	"fmt"

	"github.com/sarchlab/zeonica/report"
)

const defaultTopN = 5

// BuildReportOptions builds report options from resolved runtime configuration.
func (r *Runtime) BuildReportOptions(topN int, passed *bool, mismatchCount *int) (report.GenerateOptions, error) {
	if !r.Config.LoggingEnabled {
		return report.GenerateOptions{}, fmt.Errorf("logging is disabled, cannot build report options from trace log")
	}
	if r.Config.LogPath == "" {
		return report.GenerateOptions{}, fmt.Errorf("log path is empty, cannot build report options")
	}
	if topN <= 0 {
		topN = defaultTopN
	}

	return report.GenerateOptions{
		TestName:      r.Config.TestName,
		LogPath:       r.Config.LogPath,
		GridWidth:     r.Config.Columns,
		GridHeight:    r.Config.Rows,
		TopN:          topN,
		Passed:        passed,
		MismatchCount: mismatchCount,
	}, nil
}

// DefaultReportPath returns the default output path for report JSON.
func (r *Runtime) DefaultReportPath() string {
	return fmt.Sprintf("%s.report.json", r.Config.TestName)
}

// GenerateAndSaveReport generates report data and persists it as JSON.
func (r *Runtime) GenerateAndSaveReport(topN int, passed *bool, mismatchCount *int) (report.Report, string, error) {
	opts, err := r.BuildReportOptions(topN, passed, mismatchCount)
	if err != nil {
		return report.Report{}, "", err
	}

	result, err := report.GenerateFromLog(opts)
	if err != nil {
		return report.Report{}, "", fmt.Errorf("generate report from log: %w", err)
	}

	reportPath := r.DefaultReportPath()
	if err := report.SaveJSON(result, reportPath); err != nil {
		return report.Report{}, "", fmt.Errorf("save report json: %w", err)
	}

	return result, reportPath, nil
}

// GenerateSaveAndPrintReport generates, saves, and prints summary in one call.
func (r *Runtime) GenerateSaveAndPrintReport(topN int, passed *bool, mismatchCount *int) (string, error) {
	result, reportPath, err := r.GenerateAndSaveReport(topN, passed, mismatchCount)
	if err != nil {
		return "", err
	}
	report.PrintSummary(result)
	return reportPath, nil
}
