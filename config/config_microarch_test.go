package config

import (
	"testing"

	"github.com/sarchlab/akita/v4/sim"
)

func TestDeviceBuilderLocalMemoryWordsPropagatesToTile(t *testing.T) {
	engine := sim.NewSerialEngine()
	dev := DeviceBuilder{}.
		WithEngine(engine).
		WithFreq(1 * sim.GHz).
		WithWidth(1).
		WithHeight(1).
		WithMemoryMode("simple").
		WithLocalMemoryWords(32).
		Build("Device")

	tile := dev.GetTile(0, 0)
	_ = tile.GetMemory(0, 0, 31)

	didPanic := false
	func() {
		defer func() {
			if recover() != nil {
				didPanic = true
			}
		}()
		_ = tile.GetMemory(0, 0, 32)
	}()
	if !didPanic {
		t.Fatal("expected out-of-range panic at address 32 with local_memory_words=32")
	}
}
