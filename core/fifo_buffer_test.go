package core

import (
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func newFIFOTestState(recvCap, sendCap int) coreState {
	state := coreState{
		Directions: map[string]bool{
			"North":     true,
			"East":      true,
			"South":     true,
			"West":      true,
			"NorthEast": true,
			"SouthEast": true,
			"SouthWest": true,
			"NorthWest": true,
			"Router":    true,
		},
		Mode:              SyncOp,
		RecvQueueCapacity: recvCap,
		SendQueueCapacity: sendCap,
		RecvBufHead:       make([][]cgra.Data, 4),
		RecvBufHeadReady:  make([][]bool, 4),
		SendBufHead:       make([][]cgra.Data, 4),
		SendBufHeadBusy:   make([][]bool, 4),
		RecvBufQueue:      make([][][]cgra.Data, 4),
		SendBufQueue:      make([][][]cgra.Data, 4),
		OpInputReadCache:  make(map[string]cgra.Data),
	}
	for c := 0; c < 4; c++ {
		state.RecvBufHead[c] = make([]cgra.Data, 12)
		state.RecvBufHeadReady[c] = make([]bool, 12)
		state.SendBufHead[c] = make([]cgra.Data, 12)
		state.SendBufHeadBusy[c] = make([]bool, 12)
		state.RecvBufQueue[c] = make([][]cgra.Data, 12)
		state.SendBufQueue[c] = make([][]cgra.Data, 12)
		for d := 0; d < 12; d++ {
			state.RecvBufQueue[c][d] = make([]cgra.Data, 0, recvCap)
			state.SendBufQueue[c][d] = make([]cgra.Data, 0, sendCap)
		}
	}
	return state
}

func TestRecvFIFOOrderAndCapacity(t *testing.T) {
	state := newFIFOTestState(2, 2)
	emu := instEmulator{}
	north := emu.getDirecIndex("North")

	if !state.recvQueuePush(0, north, cgra.NewScalar(11)) {
		t.Fatal("expected first recv enqueue to succeed")
	}
	if !state.recvQueuePush(0, north, cgra.NewScalar(22)) {
		t.Fatal("expected second recv enqueue to succeed")
	}
	if state.recvQueuePush(0, north, cgra.NewScalar(33)) {
		t.Fatal("expected recv queue to report full at capacity")
	}

	state.OpInputReadCache = make(map[string]cgra.Data)
	v1 := emu.readOperand(Operand{Impl: "North", Color: "R"}, &state)
	state.OpInputReadCache = make(map[string]cgra.Data)
	v2 := emu.readOperand(Operand{Impl: "North", Color: "R"}, &state)
	if v1.First() != 11 || v2.First() != 22 {
		t.Fatalf("unexpected FIFO order: got (%d,%d), want (11,22)", v1.First(), v2.First())
	}
	if state.recvQueueLen(0, north) != 0 {
		t.Fatalf("expected recv queue empty after two consumes, got %d", state.recvQueueLen(0, north))
	}
}

func TestSyncModeDuplicatePortReadConsumesOnce(t *testing.T) {
	state := newFIFOTestState(4, 2)
	emu := instEmulator{}
	north := emu.getDirecIndex("North")

	if !state.recvQueuePush(0, north, cgra.NewScalar(101)) {
		t.Fatal("expected first recv enqueue to succeed")
	}
	if !state.recvQueuePush(0, north, cgra.NewScalar(202)) {
		t.Fatal("expected second recv enqueue to succeed")
	}

	state.OpInputReadCache = make(map[string]cgra.Data)
	v1 := emu.readOperand(Operand{Impl: "North", Color: "R"}, &state)
	v2 := emu.readOperand(Operand{Impl: "North", Color: "R"}, &state)

	if v1.First() != 101 || v2.First() != 101 {
		t.Fatalf("expected duplicate reads to reuse same token, got (%d,%d)", v1.First(), v2.First())
	}
	if state.recvQueueLen(0, north) != 1 {
		t.Fatalf("expected queue length 1 after duplicate read consume-once, got %d", state.recvQueueLen(0, north))
	}
}

func TestSendQueueBlocksOnlyWhenFull(t *testing.T) {
	state := newFIFOTestState(2, 2)
	emu := instEmulator{}
	east := emu.getDirecIndex("East")

	if !state.sendQueuePush(0, east, cgra.NewScalar(1)) {
		t.Fatal("expected first send enqueue to succeed")
	}

	op := Operation{
		OpCode: "MOV",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "#1", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "East", Color: "R"},
		}},
	}

	ready, reason := emu.checkIssueReadiness(op, &state)
	if !ready {
		t.Fatalf("expected issue ready when queue has room, got reason=%s", reason)
	}

	if !state.sendQueuePush(0, east, cgra.NewScalar(2)) {
		t.Fatal("expected second send enqueue to succeed")
	}
	ready, reason = emu.checkIssueReadiness(op, &state)
	if ready || reason != StallReasonOutputBlocked {
		t.Fatalf("expected output blocked when queue full, got ready=%v reason=%s", ready, reason)
	}
}

func TestRouterRedKeepsSingleOutstanding(t *testing.T) {
	state := newFIFOTestState(2, 8)
	router := int(cgra.Router)

	if !state.sendQueuePush(0, router, cgra.NewScalar(7)) {
		t.Fatal("expected first router-red enqueue to succeed")
	}
	if state.sendQueuePush(0, router, cgra.NewScalar(8)) {
		t.Fatal("expected router-red second enqueue to fail (single outstanding)")
	}
}
