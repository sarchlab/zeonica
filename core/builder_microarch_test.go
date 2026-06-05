package core

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
)

type testCoreExecutionModel struct {
	name string
}

var testCoreExecutionModelResetCount int
var testCoreExecutionModelTickCount int

func (m testCoreExecutionModel) Name() string {
	return m.name
}

func (m testCoreExecutionModel) Reset(c *Core) {
	testCoreExecutionModelResetCount++
}

func (m testCoreExecutionModel) Tick(c *Core) bool {
	testCoreExecutionModelTickCount++
	c.state.CurrentCycle++
	return true
}

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
	if c.CoreExecutionModelName() != LegacyCGRAPEExecutionModel {
		t.Fatalf("unexpected default core execution model: %q", c.CoreExecutionModelName())
	}
}

func TestCoreBuilderExplicitLegacyExecutionModel(t *testing.T) {
	engine := sim.NewSerialEngine()
	c := Builder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithCoreExecutionModel(LegacyCGRAPEExecutionModel).
		Build("Core")

	if c.CoreExecutionModelName() != LegacyCGRAPEExecutionModel {
		t.Fatalf("unexpected core execution model: %q", c.CoreExecutionModelName())
	}
}

func TestCoreTickDelegatesToExecutionModel(t *testing.T) {
	modelName := fmt.Sprintf("test_delegate_model_%s", t.Name())
	testCoreExecutionModelResetCount = 0
	testCoreExecutionModelTickCount = 0
	registerCoreExecutionModel(modelName, func() CoreExecutionModel {
		return testCoreExecutionModel{name: modelName}
	})

	engine := sim.NewSerialEngine()
	exit := false
	retVal := uint32(0)
	exitReq := float64(0)
	c := Builder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithExitAddr(&exit).
		WithRetValAddr(&retVal).
		WithExitReqAddr(&exitReq).
		WithCoreExecutionModel(modelName).
		Build("Core")

	c.MapProgram(Program{}, 0, 0)
	if testCoreExecutionModelResetCount != 1 {
		t.Fatalf("expected model Reset to be called once, got %d", testCoreExecutionModelResetCount)
	}
	if !c.Tick() {
		t.Fatal("expected delegated Tick to report progress")
	}
	if testCoreExecutionModelTickCount != 1 {
		t.Fatalf("expected model Tick to be called once, got %d", testCoreExecutionModelTickCount)
	}
	if c.state.CurrentCycle != 1 {
		t.Fatalf("expected model Tick to control cycle update, got %d", c.state.CurrentCycle)
	}
}
