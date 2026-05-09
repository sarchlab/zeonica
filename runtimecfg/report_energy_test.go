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

func TestGenerateAndSaveReportFailsOnStrictEnergyUnknown(t *testing.T) {
	rt := newEnergyReportRuntime(t, report.EnergyUnknownActionError)
	rt.Observer.Observe(core.TraceObservation{
		WallTime: time.Now(),
		Msg:      "Inst",
		Time:     testFloat64Ptr(0),
		X:        testIntPtr(0),
		Y:        testIntPtr(0),
		OpCode:   "ADD",
	})

	_, _, err := rt.GenerateAndSaveReport(5, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "energy estimation failed") {
		t.Fatalf("expected strict energy estimation error, got %v", err)
	}
}

func TestGenerateAndSaveReportAllowsWarnEnergyUnknown(t *testing.T) {
	rt := newEnergyReportRuntime(t, report.EnergyUnknownActionWarn)
	rt.Observer.Observe(core.TraceObservation{
		WallTime: time.Now(),
		Msg:      "Inst",
		Time:     testFloat64Ptr(0),
		X:        testIntPtr(0),
		Y:        testIntPtr(0),
		OpCode:   "ADD",
	})

	result, path, err := rt.GenerateAndSaveReport(5, nil, nil)
	if err != nil {
		t.Fatalf("GenerateAndSaveReport returned error: %v", err)
	}
	if result.Energy == nil || len(result.Energy.UnknownActions) != 1 {
		t.Fatalf("expected one unknown energy action, got %#v", result.Energy)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected report to be saved: %v", err)
	}
}

func newEnergyReportRuntime(t *testing.T, policy string) *Runtime {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "trace.log")
	return &Runtime{
		Config: ResolvedConfig{
			TestName: "energy-report",
			LogPath:  logPath,
			Rows:     1,
			Columns:  1,
			EnergyModel: &report.EnergyModel{
				Enabled:             true,
				Units:               "pJ",
				UnknownActionPolicy: policy,
				Actions:             map[string]float64{},
			},
		},
		Observer: report.NewObserver(),
	}
}

func testIntPtr(v int) *int {
	return &v
}

func testFloat64Ptr(v float64) *float64 {
	return &v
}
