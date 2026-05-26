package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sarchlab/zeonica/core"
)

//nolint:funlen
func TestObserverBuildMatchesGenerateFromLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "trace.json.log")
	ts0 := time.Date(2026, 3, 6, 0, 0, 0, 0, time.UTC)
	ts1 := ts0.Add(10 * time.Millisecond)
	ts2 := ts1.Add(10 * time.Millisecond)
	ts3 := ts2.Add(10 * time.Millisecond)

	event0 := traceEvent{
		Timestamp: ts0.Format(time.RFC3339Nano),
		Msg:       "DataFlow",
		Behavior:  "FeedIn",
		Time:      testFloat64Ptr(0),
		To:        "Device.Tile[0][0].Core.West",
	}
	event1 := traceEvent{
		Timestamp: ts1.Format(time.RFC3339Nano),
		Msg:       "Inst",
		Time:      testFloat64Ptr(0),
		X:         testIntPtr(0),
		Y:         testIntPtr(0),
	}
	event2 := traceEvent{
		Timestamp: ts2.Format(time.RFC3339Nano),
		Msg:       "Backpressure",
		Time:      testFloat64Ptr(0),
		X:         testIntPtr(0),
		Y:         testIntPtr(0),
	}
	event3 := traceEvent{
		Timestamp: ts3.Format(time.RFC3339Nano),
		Msg:       "Stall",
		Behavior:  "schedule_bubble",
		Time:      testFloat64Ptr(1),
		X:         testIntPtr(0),
		Y:         testIntPtr(0),
	}

	file, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	for _, event := range []traceEvent{event0, event1, event2, event3} {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("Marshal returned error: %v", err)
		}
		if _, err := file.Write(append(payload, '\n')); err != nil {
			t.Fatalf("Write returned error: %v", err)
		}
	}
	_ = file.Close()

	opts := GenerateOptions{
		TestName:   "observer-test",
		LogPath:    logPath,
		GridWidth:  1,
		GridHeight: 1,
		TopN:       5,
	}

	fromLog, err := GenerateFromLog(opts)
	if err != nil {
		t.Fatalf("GenerateFromLog returned error: %v", err)
	}

	observer := NewObserver()
	observer.Observe(core.TraceObservation{
		WallTime: ts0,
		Msg:      "DataFlow",
		Behavior: "FeedIn",
		Time:     testFloat64Ptr(0),
		To:       "Device.Tile[0][0].Core.West",
	})
	observer.Observe(core.TraceObservation{
		WallTime: ts1,
		Msg:      "Inst",
		Time:     testFloat64Ptr(0),
		X:        testIntPtr(0),
		Y:        testIntPtr(0),
	})
	observer.Observe(core.TraceObservation{
		WallTime: ts2,
		Msg:      "Backpressure",
		Time:     testFloat64Ptr(0),
		X:        testIntPtr(0),
		Y:        testIntPtr(0),
	})
	observer.Observe(core.TraceObservation{
		WallTime: ts3,
		Msg:      "Stall",
		Behavior: "schedule_bubble",
		Time:     testFloat64Ptr(1),
		X:        testIntPtr(0),
		Y:        testIntPtr(0),
	})

	fromObserver := observer.Build(opts)
	if !reflect.DeepEqual(fromLog, fromObserver) {
		t.Fatalf("expected observer report to match log report\nfrom log: %#v\nfrom observer: %#v", fromLog, fromObserver)
	}
}

func testIntPtr(v int) *int {
	return &v
}

func testFloat64Ptr(v float64) *float64 {
	return &v
}
