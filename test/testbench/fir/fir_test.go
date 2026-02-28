package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

func TestFir(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "fir.json.log")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer f.Close()

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: core.LevelTrace,
	})
	slog.SetDefault(slog.New(handler))

	archSpecPath, err := resolveArchSpecPath()
	if err != nil {
		t.Fatalf("resolve arch spec: %v", err)
	}
	rt, err := runtimecfg.LoadRuntime(archSpecPath, "fir")
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}

	mismatch := Fir(rt)
	if mismatch != 0 {
		t.Fatalf("fir mismatch count: got=%d expected=0", mismatch)
	}
}
