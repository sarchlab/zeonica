package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

func TestFft(t *testing.T) {
	if _, err := resolveProgramPath(); err != nil {
		t.Skipf("skip FFT test because generated program is unavailable: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "fft.json.log")
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
	rt, err := runtimecfg.LoadRuntime(archSpecPath, "fft")
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}

	mismatch := Fft(rt)
	if mismatch != 0 {
		t.Fatalf("fft mismatch count: got=%d expected=0", mismatch)
	}
}
