package core

import (
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func TestSyncTwoPhaseNoPartialCommitOnStall(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newFIFOTestState(4, 4)
	state.Mode = SyncOp
	state.EnableFIFOModel = true
	state.Registers = make([]cgra.Data, 8)
	state.Registers[0] = cgra.NewScalar(9)

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "MOV",
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "#1", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
				}},
			},
			{
				OpCode: "MOV",
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "North", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
			},
		},
	}

	run := emu.RunInstructionGroupWithSyncOps(ig, &state, 0)
	if run {
		t.Fatal("expected instruction group to stall on missing North operand")
	}
	if got := state.Registers[0].First(); got != 9 {
		t.Fatalf("expected no partial commit on stall, got register0=%d want 9", got)
	}
}

func TestSyncTwoPhaseCommitOnSuccess(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newFIFOTestState(4, 4)
	state.Mode = SyncOp
	state.EnableFIFOModel = true
	state.Registers = make([]cgra.Data, 8)

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "MOV",
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "#7", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
				}},
			},
		},
	}

	run := emu.RunInstructionGroupWithSyncOps(ig, &state, 0)
	if !run {
		t.Fatal("expected instruction group to run successfully")
	}
	if got := state.Registers[0].First(); got != 7 {
		t.Fatalf("unexpected committed register value: got %d want 7", got)
	}
}
