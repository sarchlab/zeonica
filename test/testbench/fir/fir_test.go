package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/zeonica/core"
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

	retVal, expected := Fir()
	if retVal != expected {
		t.Fatalf("fir mismatch: got=%d expected=%d", retVal, expected)
	}
}
