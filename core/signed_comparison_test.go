package core

import (
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func TestSignedComparisonOpcodes(t *testing.T) {
	tests := []struct {
		name   string
		opcode string
		left   int32
		right  int32
		want   uint32
	}{
		{name: "sgt positive", opcode: "ICMP_SGT", left: 5, right: 3, want: 1},
		{name: "sgt negative and positive", opcode: "ICMP_SGT", left: -1, right: 0, want: 0},
		{name: "sgt two negatives", opcode: "ICMP_SGT", left: -1, right: -2, want: 1},
		{name: "slt positive", opcode: "ICMP_SLT", left: 3, right: 5, want: 1},
		{name: "slt positive and negative", opcode: "ICMP_SLT", left: 0, right: -1, want: 0},
		{name: "slt two negatives", opcode: "ICMP_SLT", left: -2, right: -1, want: 1},
		{name: "sge equality", opcode: "ICMP_SGE", left: 5, right: 5, want: 1},
		{name: "sge negative and positive", opcode: "ICMP_SGE", left: -1, right: 0, want: 0},
		{name: "sge minimum and maximum", opcode: "ICMP_SGE", left: -1 << 31, right: 1<<31 - 1, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := newVectorTestState(1)
			state.Registers[0] = cgra.NewScalar(uint32(tt.left))
			state.Registers[1] = cgra.NewScalar(uint32(tt.right))

			got := runComparisonOpcode(t, tt.opcode, &state)
			if got.First() != tt.want {
				t.Fatalf("%s(%d, %d) = %d, want %d", tt.opcode, tt.left, tt.right, got.First(), tt.want)
			}
			if !got.Pred {
				t.Fatal("comparison of valid operands returned an invalid predicate")
			}
		})
	}
}

func TestSignedComparisonCombinesPredicates(t *testing.T) {
	state := newVectorTestState(1)
	left := int32(-2)
	right := int32(-1)
	state.Registers[0] = cgra.NewScalarWithPred(uint32(left), true)
	state.Registers[1] = cgra.NewScalarWithPred(uint32(right), false)

	got := runComparisonOpcode(t, "ICMP_SLT", &state)
	if got.Pred {
		t.Fatal("comparison result must be invalid when either source predicate is false")
	}
}

func TestCastTruncUsesScalarDataPath(t *testing.T) {
	state := newVectorTestState(1)
	state.Registers[0] = cgra.NewScalar(0x89abcdef)
	dst := Operand{Impl: "$1", Color: "R"}
	op := Operation{
		OpCode:      "CAST_TRUNC",
		SrcOperands: OperandList{Operands: []Operand{{Impl: "$0", Color: "R"}}},
		DstOperands: OperandList{Operands: []Operand{dst}},
	}

	got := instEmulator{}.RunOperation(op, &state, 0)[dst]
	if got.First() != 0x89abcdef || !got.Pred {
		t.Fatalf("CAST_TRUNC result = %#v, want unchanged scalar", got)
	}
}

func runComparisonOpcode(t *testing.T, opcode string, state *coreState) cgra.Data {
	t.Helper()
	emu := instEmulator{}
	dst := Operand{Impl: "$2", Color: "R"}
	op := Operation{
		OpCode: opcode,
		SrcOperands: OperandList{Operands: []Operand{
			{Impl: "$0", Color: "R"},
			{Impl: "$1", Color: "R"},
		}},
		DstOperands: OperandList{Operands: []Operand{dst}},
	}

	results := emu.RunOperation(op, state, 0)
	result, ok := results[dst]
	if !ok {
		t.Fatalf("%s did not produce a result", opcode)
	}
	return result
}
