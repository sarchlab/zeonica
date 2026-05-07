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

func TestBankedSharedMemoryAddressMapping(t *testing.T) {
	engine := sim.NewSerialEngine()
	controller := newBankedSharedMemoryController(
		"BankedSharedMemory",
		engine,
		1*sim.GHz,
		BankedSharedMemoryConfig{
			Banks:           4,
			BaseLatency:     3,
			InterleaveBytes: 4,
			Capacity:        1024,
		},
	)

	cases := map[uint64]int{
		0:  0,
		4:  1,
		8:  2,
		12: 3,
		16: 0,
	}
	for addr, want := range cases {
		if got := controller.BankForAddress(addr); got != want {
			t.Fatalf("BankForAddress(%d) got %d, want %d", addr, got, want)
		}
	}
}

func TestBankedSharedMemorySerializesSameBankOnly(t *testing.T) {
	engine := sim.NewSerialEngine()
	controller := newBankedSharedMemoryController(
		"BankedSharedMemory",
		engine,
		1*sim.GHz,
		BankedSharedMemoryConfig{
			Banks:           2,
			BaseLatency:     3,
			InterleaveBytes: 4,
			Capacity:        1024,
		},
	)

	firstBank0 := controller.scheduleCycleForAddress(0)
	secondBank0 := controller.scheduleCycleForAddress(8)
	firstBank1 := controller.scheduleCycleForAddress(4)

	if firstBank0 != 3 {
		t.Fatalf("first bank-0 request completed at cycle %d, want 3", firstBank0)
	}
	if secondBank0 != 6 {
		t.Fatalf("second same-bank request completed at cycle %d, want 6", secondBank0)
	}
	if firstBank1 != 3 {
		t.Fatalf("different-bank request completed at cycle %d, want 3", firstBank1)
	}
}
