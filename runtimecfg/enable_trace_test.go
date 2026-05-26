package runtimecfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/report"
)

func TestResolveEnableTraceDefaultsFalse(t *testing.T) {
	cfg, err := Resolve(ArchSpec{}, "enable-trace-default")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.EnableTrace {
		t.Fatal("expected enableTrace default to be false")
	}
}

func TestResolveEnableTraceTrue(t *testing.T) {
	enabled := true
	cfg, err := Resolve(ArchSpec{
		Simulator: Simulator{
			Logging: SimulatorLogging{
				EnableTrace: &enabled,
			},
		},
	}, "enable-trace-true")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !cfg.EnableTrace {
		t.Fatal("expected enableTrace to be true")
	}
}

func TestInitTraceLoggerDisableTraceCreatesEmptyLogAndReport(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "trace.log")
	rt := &Runtime{
		Config: ResolvedConfig{
			TestName:       "disable-trace",
			Rows:           1,
			Columns:        1,
			LoggingEnabled: true,
			EnableTrace:    false,
			LogPath:        logPath,
		},
		Observer: report.NewObserver(),
	}

	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		t.Fatalf("InitTraceLogger returned error: %v", err)
	}

	core.Trace("Inst", "Time", float64(0), "X", 0, "Y", 0)

	if err := CloseTraceLog(traceLog); err != nil {
		t.Fatalf("CloseTraceLog returned error: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected empty trace log when enableTrace=false, got %d bytes", info.Size())
	}

	passed := true
	mismatch := 0
	reportPath, err := rt.GenerateSaveAndPrintReport(5, &passed, &mismatch)
	if err != nil {
		t.Fatalf("GenerateSaveAndPrintReport returned error: %v", err)
	}
	expectedReportPath := filepath.Join(filepath.Dir(logPath), "disable-trace.report.json")
	if reportPath != expectedReportPath {
		t.Fatalf("expected report path %q, got %q", expectedReportPath, reportPath)
	}

	content, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(content), `"logPath": "`+logPath+`"`) {
		t.Fatalf("expected report to preserve logPath %q, got %s", logPath, string(content))
	}
}

func TestInitTraceLoggerEnableTraceWritesEvents(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "trace.log")
	rt := &Runtime{
		Config: ResolvedConfig{
			TestName:       "enable-trace",
			Rows:           1,
			Columns:        1,
			LoggingEnabled: true,
			EnableTrace:    true,
			LogPath:        logPath,
		},
		Observer: report.NewObserver(),
	}

	traceLog, err := rt.InitTraceLogger(core.LevelTrace)
	if err != nil {
		t.Fatalf("InitTraceLogger returned error: %v", err)
	}

	core.Trace("Inst", "Time", float64(1), "X", 0, "Y", 0)
	time.Sleep(5 * time.Millisecond)

	if err := CloseTraceLog(traceLog); err != nil {
		t.Fatalf("CloseTraceLog returned error: %v", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("Stat returned error: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected non-empty trace log when enableTrace=true")
	}
}
