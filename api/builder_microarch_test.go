package api

import (
	"testing"

	"github.com/sarchlab/akita/v4/sim"
)

func TestDriverBuilderWithPortBufferDepth(t *testing.T) {
	engine := sim.NewSerialEngine()
	driver := DriverBuilder{}.
		WithEngine(engine).
		WithFreq(1*sim.GHz).
		WithPortBufferDepth(3, 5).
		Build("Driver")

	impl, ok := driver.(*driverImpl)
	if !ok {
		t.Fatalf("expected *driverImpl, got %T", driver)
	}

	factory, ok := impl.portFactory.(defaultPortFactory)
	if !ok {
		t.Fatalf("expected defaultPortFactory, got %T", impl.portFactory)
	}
	if factory.incomingBufCap != 3 || factory.outgoingBufCap != 5 {
		t.Fatalf("unexpected driver port caps: in=%d out=%d", factory.incomingBufCap, factory.outgoingBufCap)
	}
}
