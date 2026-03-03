package core

import (
	"os"
	"strings"
	"testing"
)

func newPolicyTestState() coreState {
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
		RecvBufHeadReady: make([][]bool, 4),
		SendBufHeadBusy:  make([][]bool, 4),
	}

	for i := 0; i < 4; i++ {
		state.RecvBufHeadReady[i] = make([]bool, 12)
		state.SendBufHeadBusy[i] = make([]bool, 12)
	}

	return state
}

func TestCanIssueInOrderIgnoresTimeStep(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyInOrderDataflow,
	}
	state := newPolicyTestState()
	state.CurrentCycle = 0
	op := Operation{
		OpCode:   "NOP",
		TimeStep: 10,
	}

	if !emu.canIssue(op, &state) {
		t.Fatalf("in_order_dataflow should ignore timestep and allow ready op")
	}
}

func TestCanIssueElasticScheduled(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	op := Operation{
		OpCode:   "NOP",
		TimeStep: 5,
	}

	state.CurrentCycle = 4
	if emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should block before timestep")
	}

	state.CurrentCycle = 5
	if !emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should allow at timestep when ready")
	}
}

func TestCanIssueElasticScheduledWithCompiledIIConversion(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 4
	op := Operation{
		OpCode:   "NOP",
		TimeStep: 1,
	}

	state.CurrentCycle = 0 // step 0
	if emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should block before converted step")
	}

	state.CurrentCycle = 2 // step 2
	if !emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should allow when converted step >= time_step")
	}

	state.CurrentCycle = 5 // step 1 (5 %% 4)
	if !emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should allow on converted matching step")
	}
}

func TestCanIssueStrictTimedViolation(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyStrictTimed,
	}
	state := newPolicyTestState()
	state.CurrentCycle = 3
	op := Operation{
		OpCode:   "DATA_MOV",
		TimeStep: 3,
		SrcOperands: OperandList{
			Operands: []Operand{
				{Impl: "North", Color: "R"},
			},
		},
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected strict_timed synchronization violation panic")
		}
		if !strings.Contains(recovered.(string), "synchronization violation") {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()

	_ = emu.canIssue(op, &state)
}

func TestCanIssueStrictTimedWithCompiledIIConversion(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyStrictTimed,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 4
	op := Operation{
		OpCode:   "NOP",
		TimeStep: 1,
	}

	state.CurrentCycle = 5 // step 1
	if !emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should allow when converted step equals time_step")
	}

	state.CurrentCycle = 6 // step 2: missed exact step
	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected strict_timed missed-step synchronization violation")
		}
		if !strings.Contains(recovered.(string), "missed its exact scheduled step") {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()
	_ = emu.canIssue(op, &state)
}

func TestLoadProgramFileFromYAMLPreservesTimeStep(t *testing.T) {
	filePath := "../test/testbench/stonneGEMM8x8/gemm.yaml"
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Skipf("test file does not exist: %s", filePath)
	}

	programMap := LoadProgramFileFromYAML(filePath)
	program, ok := programMap["(0,0)"]
	if !ok {
		t.Fatalf("core (0,0) not found in parsed program")
	}
	if len(program.EntryBlocks) == 0 || len(program.EntryBlocks[0].InstructionGroups) < 2 {
		t.Fatalf("unexpected program structure for core (0,0)")
	}

	group0 := program.EntryBlocks[0].InstructionGroups[0]
	if len(group0.Operations) == 0 {
		t.Fatalf("group0 has no operations")
	}
	if group0.Operations[0].TimeStep != 0 {
		t.Fatalf("unexpected timestep for first op: got %d want 0", group0.Operations[0].TimeStep)
	}

	group1 := program.EntryBlocks[0].InstructionGroups[1]
	if len(group1.Operations) == 0 {
		t.Fatalf("group1 has no operations")
	}
	if group1.Operations[0].TimeStep != 1 {
		t.Fatalf("unexpected timestep for second group first op: got %d want 1", group1.Operations[0].TimeStep)
	}
}
