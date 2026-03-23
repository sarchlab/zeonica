package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/zeonica/cgra"
)

func TestLoadOperationLatencyProfileFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "latency.yaml")
	content := []byte("default_latency: 3\nopcodes:\n  mul: 2\n  FMUL: 4\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write latency file: %v", err)
	}

	t.Setenv(operationLatencyFileEnv, path)

	opcodes, defaultLatency, err := loadOperationLatencyProfileFromEnv()
	if err != nil {
		t.Fatalf("load latency profile: %v", err)
	}
	if defaultLatency != 3 {
		t.Fatalf("unexpected default latency: got %d want 3", defaultLatency)
	}
	if opcodes["MUL"] != 2 {
		t.Fatalf("expected normalized MUL latency 2, got %d", opcodes["MUL"])
	}
	if opcodes["FMUL"] != 4 {
		t.Fatalf("expected FMUL latency 4, got %d", opcodes["FMUL"])
	}
}

func TestLoadOperationLatencyProfileFromEnvDefaultsToOneWhenUnset(t *testing.T) {
	t.Setenv(operationLatencyFileEnv, "")

	opcodes, defaultLatency, err := loadOperationLatencyProfileFromEnv()
	if err != nil {
		t.Fatalf("load empty latency profile: %v", err)
	}
	if len(opcodes) != 0 {
		t.Fatalf("expected no opcode latencies, got %d", len(opcodes))
	}
	if defaultLatency != 1 {
		t.Fatalf("unexpected default latency: got %d want 1", defaultLatency)
	}
}

func TestLoadOperationLatencyProfileRejectsInvalidLatency(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "latency.yaml")
	content := []byte("default_latency: 1\nopcodes:\n  MUL: 0\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write latency file: %v", err)
	}

	t.Setenv(operationLatencyFileEnv, path)

	if _, _, err := loadOperationLatencyProfileFromEnv(); err == nil {
		t.Fatal("expected invalid latency error")
	}
}

func newLatencyTestState(recvCap, sendCap int, enableFIFO bool) coreState {
	state := newFIFOTestState(recvCap, sendCap)
	state.EnableFIFOModel = enableFIFO
	state.Registers = make([]cgra.Data, 8)
	state.OpTimingCursor = make(map[int]int)
	state.OpTimingLate = make(map[int]bool)
	state.OpTimingRollCycle = make(map[int]int64)
	state.OpIssueCount = make(map[int]int)
	state.Code = Program{DefaultOperationLatency: 1}
	state.SelectedBlock = &EntryBlock{}
	state.PCInBlock = 0
	return state
}

func TestSyncOpcodeLatencyDelaysRegisterWriteback(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newLatencyTestState(4, 4, false)
	state.Registers[0] = cgra.NewScalar(5)
	state.Code.OperationLatencies = map[string]int{"MUL": 2}

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "MUL",
				ID:     1,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
					{Impl: "#3", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
			},
		},
	}
	state.SelectedBlock.InstructionGroups = []InstructionGroup{ig}

	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 0) {
		t.Fatal("expected first issue cycle to make progress")
	}
	if got := state.Registers[1].First(); got != 0 {
		t.Fatalf("unexpected early writeback: got %d want 0", got)
	}
	if state.PendingSyncGroup == nil {
		t.Fatal("expected pending sync group after first issue")
	}

	state.CurrentCycle = 1
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 1) {
		t.Fatal("expected completion cycle to make progress")
	}
	if got := state.Registers[1].First(); got != 15 {
		t.Fatalf("unexpected delayed writeback result: got %d want 15", got)
	}
	if state.PendingSyncGroup != nil {
		t.Fatal("expected pending sync group cleared after commit")
	}
}

func TestSyncOpcodeLatencyDelaysGroupCommitWithDataMov(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newLatencyTestState(4, 4, true)
	state.Registers[0] = cgra.NewScalar(5)
	state.Code.OperationLatencies = map[string]int{"MUL": 2}

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "MUL",
				ID:     11,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
					{Impl: "#3", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
			},
			{
				OpCode: "DATA_MOV",
				ID:     12,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "East", Color: "R"},
				}},
			},
		},
	}
	state.SelectedBlock.InstructionGroups = []InstructionGroup{ig}

	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 0) {
		t.Fatal("expected first issue cycle to make progress")
	}
	east := emu.getDirecIndex("East")
	if state.sendQueueLen(0, east) != 0 {
		t.Fatalf("expected no outgoing data before commit, got send queue len %d", state.sendQueueLen(0, east))
	}
	if got := state.Registers[1].First(); got != 0 {
		t.Fatalf("unexpected early register writeback: got %d want 0", got)
	}

	state.CurrentCycle = 1
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 1) {
		t.Fatal("expected completion cycle to make progress")
	}
	if got := state.Registers[1].First(); got != 15 {
		t.Fatalf("unexpected committed register value: got %d want 15", got)
	}
	if state.sendQueueLen(0, east) != 1 {
		t.Fatalf("expected delayed DATA_MOV to enqueue once, got len %d", state.sendQueueLen(0, east))
	}
	head, ok := state.sendQueuePeek(0, east)
	if !ok || head.First() != 15 {
		t.Fatalf("unexpected delayed DATA_MOV payload: ok=%v value=%d", ok, head.First())
	}
}

func TestSyncOpcodeLatencyUsesMaxAcrossGroup(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newLatencyTestState(4, 4, true)
	state.Registers[0] = cgra.NewScalar(5)
	state.Code.OperationLatencies = map[string]int{
		"ADD": 1,
		"MUL": 2,
	}

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "ADD",
				ID:     21,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
					{Impl: "#1", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
			},
			{
				OpCode: "MUL",
				ID:     22,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
					{Impl: "#3", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$2", Color: "R"},
				}},
			},
		},
	}
	state.SelectedBlock.InstructionGroups = []InstructionGroup{ig}

	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 0) {
		t.Fatal("expected first issue cycle to make progress")
	}
	if got := state.Registers[2].First(); got != 0 {
		t.Fatalf("unexpected early result writeback: got %d want 0", got)
	}

	state.CurrentCycle = 1
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 1) {
		t.Fatal("expected completion cycle to make progress")
	}
	if got := state.Registers[1].First(); got != 6 {
		t.Fatalf("unexpected committed ADD result: got %d want 6", got)
	}
	if got := state.Registers[2].First(); got != 18 {
		t.Fatalf("unexpected committed MUL result: got %d want 18", got)
	}
}

func TestSyncOpcodeLatencyRespectsDerivedTimingIssueCycle(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyStrictTimed,
	}
	state := newLatencyTestState(4, 4, true)
	state.Registers[0] = cgra.NewScalar(7)
	state.Code.OperationLatencies = map[string]int{"MUL": 2}
	state.Code.DerivedTiming = map[int][]int64{
		31: []int64{5},
	}

	ig := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "MUL",
				ID:     31,
				SrcOperands: OperandList{Operands: []Operand{
					{Impl: "$0", Color: "R"},
					{Impl: "#2", Color: "R"},
				}},
				DstOperands: OperandList{Operands: []Operand{
					{Impl: "$1", Color: "R"},
				}},
			},
		},
	}
	state.SelectedBlock.InstructionGroups = []InstructionGroup{ig}

	state.CurrentCycle = 4
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 4) {
		t.Fatal("expected pre-issue timing wait to keep core alive")
	}
	if state.PendingSyncGroup != nil {
		t.Fatal("did not expect pending sync group before legal issue cycle")
	}
	if got := state.Registers[1].First(); got != 0 {
		t.Fatalf("unexpected result before legal issue cycle: got %d want 0", got)
	}

	state.CurrentCycle = 5
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 5) {
		t.Fatal("expected legal issue cycle to make progress")
	}
	if state.PendingSyncGroup == nil {
		t.Fatal("expected pending sync group on legal issue cycle")
	}
	if got := state.Registers[1].First(); got != 0 {
		t.Fatalf("unexpected result on issue cycle before commit: got %d want 0", got)
	}

	state.CurrentCycle = 6
	if !emu.RunInstructionGroupWithSyncOps(ig, &state, 6) {
		t.Fatal("expected completion cycle to make progress")
	}
	if got := state.Registers[1].First(); got != 14 {
		t.Fatalf("unexpected delayed result after derived-timing issue: got %d want 14", got)
	}
}
