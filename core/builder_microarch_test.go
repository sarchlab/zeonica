package core

import (
	"testing"

	"github.com/sarchlab/akita/v4/sim"
)

func TestCoreBuilderResourceSizing(t *testing.T) {
	engine := sim.NewSerialEngine()
	c := Builder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithEnableFIFOModel(true).
		WithRegisterCount(96).
		WithLocalMemoryWords(2048).
		WithPortBufferDepth(4, 6).
		Build("Core")

	if got := len(c.state.Registers); got != 96 {
		t.Fatalf("unexpected register count: got %d want 96", got)
	}
	if got := len(c.state.Memory); got != 2048 {
		t.Fatalf("unexpected local memory words: got %d want 2048", got)
	}
	if c.GetPortByName("North") == nil {
		t.Fatal("expected North port to be initialized")
	}
	if !c.state.EnableFIFOModel {
		t.Fatal("expected EnableFIFOModel to propagate to core state")
	}
}
