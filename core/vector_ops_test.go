package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func newVectorTestState(lanes int) coreState {
	state := newFIFOTestState(4, 4)
	state.Registers = make([]cgra.Data, 16)
	state.Memory = make([]uint32, 128)
	state.EnableVectorPE = lanes > 1
	state.VectorLanes = lanes
	state.Code = Program{DefaultOperationLatency: 1}
	return state
}

func mustPanicContains(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		got := fmt.Sprint(r)
		if want != "" && !strings.Contains(got, want) {
			t.Fatalf("panic mismatch: got %q want substring %q", got, want)
		}
	}()
	fn()
}

func TestVectorBroadcastAndArithmetic(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(4)
	state.Registers[0] = cgra.NewScalar(3)
	state.Registers[1] = cgra.FromSlice([]uint32{1, 2, 3, 4}, true)

	vb := Operation{
		OpCode: "VBROADCAST",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$2", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(vb, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[2].Data; len(got) != 4 || got[0] != 3 || got[3] != 3 {
		t.Fatalf("unexpected VBROADCAST result: %v", got)
	}

	vmul := Operation{
		OpCode: "VMUL",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$1", Color: "R"},
			{Impl: "$2", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$3", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(vmul, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[3].Data; got[0] != 3 || got[1] != 6 || got[2] != 9 || got[3] != 12 {
		t.Fatalf("unexpected VMUL result: %v", got)
	}

	vadd := Operation{
		OpCode: "VADD",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$3", Color: "R"},
			{Impl: "$1", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$4", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(vadd, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[4].Data; got[0] != 4 || got[1] != 8 || got[2] != 12 || got[3] != 16 {
		t.Fatalf("unexpected VADD result: %v", got)
	}

	reduce := Operation{
		OpCode: "VECTOR.REDUCE.ADD",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$4", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$5", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(reduce, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[5].First(); got != 40 {
		t.Fatalf("unexpected VECTOR.REDUCE.ADD result: got %d want 40", got)
	}

	extract := Operation{
		OpCode: "VEXTRACT",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$4", Color: "R"},
			{Impl: "#2", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$6", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(extract, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[6].First(); got != 12 {
		t.Fatalf("unexpected VEXTRACT result: got %d want 12", got)
	}
}

func TestVectorLoadAndStoreContiguous(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(4)
	copy(state.Memory[8:12], []uint32{11, 22, 33, 44})

	load := Operation{
		OpCode: "VLOAD_CONTIG",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "#8", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
		}},
	}
	for dst, value := range emu.RunOperation(load, &state, 0) {
		emu.writeOperand(dst, value, &state)
	}
	if got := state.Registers[0].Data; got[0] != 11 || got[3] != 44 {
		t.Fatalf("unexpected VLOAD_CONTIG result: %v", got)
	}

	store := Operation{
		OpCode: "VSTORE_CONTIG",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
			{Impl: "#20", Color: "R"},
		}},
	}
	emu.RunOperation(store, &state, 0)
	if got := state.Memory[20:24]; got[0] != 11 || got[3] != 44 {
		t.Fatalf("unexpected VSTORE_CONTIG memory contents: %v", got)
	}
}

func TestVectorOpcodeRequiresEnabledMode(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(1)
	state.EnableVectorPE = false

	op := Operation{
		OpCode: "VBROADCAST",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "#7", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
		}},
	}
	mustPanicContains(t, "requires simulator.device.enable_vector_pe=true", func() {
		emu.RunOperation(op, &state, 0)
	})
}

func TestVectorRejectsLaneMismatch(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(4)
	state.Registers[0] = cgra.FromSlice([]uint32{1, 2, 3, 4}, true)
	state.Registers[1] = cgra.FromSlice([]uint32{9, 9}, true)

	op := Operation{
		OpCode: "VADD",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
			{Impl: "$1", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$2", Color: "R"},
		}},
	}
	mustPanicContains(t, "expects 4-lane vector", func() {
		emu.RunOperation(op, &state, 0)
	})
}

func TestScalarOpcodeRejectsVectorOperand(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(4)
	state.Registers[0] = cgra.FromSlice([]uint32{1, 2, 3, 4}, true)

	op := Operation{
		OpCode: "ADD",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
			{Impl: "#1", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$1", Color: "R"},
		}},
	}
	mustPanicContains(t, "scalar opcode ADD cannot consume vector src operand", func() {
		emu.RunOperation(op, &state, 0)
	})
}

func TestVectorLoadStoreBoundsChecks(t *testing.T) {
	emu := instEmulator{}
	state := newVectorTestState(4)
	state.Memory = make([]uint32, 10)
	state.Registers[0] = cgra.FromSlice([]uint32{1, 2, 3, 4}, true)

	load := Operation{
		OpCode: "VLOAD_CONTIG",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "#8", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{
			{Impl: "$1", Color: "R"},
		}},
	}
	mustPanicContains(t, "vector load out of bounds", func() {
		emu.RunOperation(load, &state, 0)
	})

	store := Operation{
		OpCode: "VSTORE_CONTIG",
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
			{Impl: "#8", Color: "R"},
		}},
	}
	mustPanicContains(t, "vector store out of bounds", func() {
		emu.RunOperation(store, &state, 0)
	})
}

func TestDeferredLatencyScalesVectorOpsByLaneCount(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newVectorTestState(8)
	state.Code.OperationLatencies = map[string]int{
		"VMUL": 8,
		"ADD":  2,
	}

	ig := InstructionGroup{
		Operations: []Operation{
			{OpCode: "VMUL", ID: 1},
			{OpCode: "ADD", ID: 2},
		},
	}

	latency, representativeID, representativeOp, deferred := emu.deferredSyncGroupLatency(ig, &state)
	if !deferred {
		t.Fatal("expected vector arithmetic group to support deferred latency")
	}
	if latency != 8 {
		t.Fatalf("unexpected latency: got %d want 8", latency)
	}
	if representativeID != 1 || representativeOp != "VMUL" {
		t.Fatalf("unexpected representative op: id=%d op=%s", representativeID, representativeOp)
	}
}
