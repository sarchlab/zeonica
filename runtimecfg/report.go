package runtimecfg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarchlab/zeonica/report"
)

const defaultTopN = 5

// BuildReportOptions builds report options from resolved runtime configuration.
func (r *Runtime) BuildReportOptions(topN int, passed *bool, mismatchCount *int) (report.GenerateOptions, error) {
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
	reportName := fmt.Sprintf("%s.report.json", r.Config.TestName)
	if r.Config.LogPath == "" {
		return reportName
	}

	dir := filepath.Dir(r.Config.LogPath)
	if dir == "." || dir == "" {
		return reportName
	}

	return filepath.Join(dir, reportName)
}

// GenerateAndSaveReport generates report data and persists it as JSON.
func (r *Runtime) GenerateAndSaveReport(topN int, passed *bool, mismatchCount *int) (report.Report, string, error) {
	opts, err := r.BuildReportOptions(topN, passed, mismatchCount)
	if err != nil {
		return report.Report{}, "", err
	}

	var result report.Report
	if r.Observer != nil {
		result = r.Observer.Build(opts)
	} else {
		result, err = report.GenerateFromLog(opts)
		if err != nil {
			return report.Report{}, "", fmt.Errorf("generate report from log: %w", err)
		}
	}

	reportPath := r.DefaultReportPath()
	if dir := filepath.Dir(reportPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return report.Report{}, "", fmt.Errorf("create report directory: %w", err)
		}
	}
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
