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
		RecvBufHeadReady:  make([][]bool, 4),
		SendBufHeadBusy:   make([][]bool, 4),
		OpTimingCursor:    make(map[int]int),
		OpTimingLate:      make(map[int]bool),
		OpTimingRollCycle: make(map[int]int64),
		TimingWaitBlocked: false,
		StallReason:       "",
		StallOpID:         0,
		StallOpCode:       "",
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
	state.Code.CompiledII = 10 // must have schedule so elastic time-gating applies
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
	state.Code.CompiledII = 4           // must have schedule so strict time check runs
	state.CurrentCycle = 6              // step 2 (6%4); op was for step 1 → missed step, violation
	state.RecvBufHeadReady[0][0] = true // North dir slot 0 ready so CheckFlags passes
	op := Operation{
		OpCode:   "DATA_MOV",
		TimeStep: 1, // scheduled step 1; current step 2 > 1 → panic
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

func TestCanIssueStrictTimedWithDerivedTiming(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         4,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{
		7: []int64{5},
	}
	op := Operation{
		OpCode: "NOP",
		ID:     7,
	}

	state.CurrentCycle = 4
	if emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should block before derived cycle")
	}

	state.CurrentCycle = 5
	if !emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should allow exactly on derived cycle when ready")
	}

	state.CurrentCycle = 6
	if !emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should allow late issue after derived cycle when ready")
	}
}

func TestCanIssueElasticScheduledWithDerivedTiming(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{
		9: []int64{5},
	}
	op := Operation{
		OpCode: "NOP",
		ID:     9,
	}

	state.CurrentCycle = 4
	if emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should block before derived cycle")
	}

	state.CurrentCycle = 6
	if !emu.canIssue(op, &state) {
		t.Fatalf("elastic_scheduled should allow after derived cycle when ready")
	}
}

func TestRunInstructionGroupWithSyncOpsKeepsAliveOnDerivedTimingWait(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyStrictTimed,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{
		13: []int64{5},
	}
	state.CurrentCycle = 4

	group := InstructionGroup{
		Operations: []Operation{
			{
				OpCode: "NOP",
				ID:     13,
			},
		},
	}

	progress := emu.RunInstructionGroupWithSyncOps(group, &state, 0)
	if !progress {
		t.Fatalf("timing wait should keep core ticking until derived cycle is reached")
	}
	if !state.TimingWaitBlocked {
		t.Fatalf("expected timing wait marker to be set")
	}
}

func TestCanIssueStrictTimedDerivedTimingNotReady(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         4,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{
		11: []int64{5},
	}
	op := Operation{
		OpCode: "DATA_MOV",
		ID:     11,
		SrcOperands: OperandList{
			Operands: []Operand{
				{Impl: "North", Color: "R"},
			},
		},
	}

	state.CurrentCycle = 5
	if emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should stall when operand is not ready on derived cycle")
	}

	state.RecvBufHeadReady[0][0] = true // North-R becomes ready
	state.CurrentCycle = 6
	if !emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should allow late issue after derived-cycle stall")
	}
}

func TestCanIssueStrictTimedDerivedTimingWindowViolationSoft(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         1,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		17: []int64{5},
	}
	op := Operation{
		OpCode: "NOP",
		ID:     17,
	}

	state.CurrentCycle = 7 // lateness=2 > max slip=1, should roll to next II window
	if emu.canIssue(op, &state) {
		t.Fatalf("strict_timed soft mode should stall after window violation")
	}
	if !state.TimingWaitBlocked {
		t.Fatalf("expected timing wait after strict window violation")
	}
	if state.OpTimingRollCycle[op.ID] != 9 {
		t.Fatalf("expected rolled cycle 9, got %d", state.OpTimingRollCycle[op.ID])
	}

	state.CurrentCycle = 8
	if emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should keep waiting before rolled cycle")
	}

	state.CurrentCycle = 9
	if !emu.canIssue(op, &state) {
		t.Fatalf("strict_timed should issue at rolled cycle when ready")
	}
}

func TestCanIssueStrictTimedDerivedTimingSetsScheduleBubbleReason(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         1,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		21: []int64{5},
	}
	op := Operation{
		OpCode: "NOP",
		ID:     21,
	}

	state.CurrentCycle = 7
	if emu.canIssue(op, &state) {
		t.Fatalf("expected strict_timed violation to block issue")
	}
	if state.StallReason != StallReasonScheduleBubble {
		t.Fatalf("expected schedule bubble stall reason, got %q", state.StallReason)
	}
}

func TestCanIssueStrictTimedDerivedTimingWindowViolationOverridesReadinessReason(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         1,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		31: []int64{5},
	}
	op := Operation{
		OpCode: "DATA_MOV",
		ID:     31,
		SrcOperands: OperandList{
			Operands: []Operand{
				{Impl: "North", Color: "R"},
			},
		},
	}

	state.CurrentCycle = 7 // lateness=2 > max slip, and operand not ready
	if emu.canIssue(op, &state) {
		t.Fatalf("expected strict_timed to block on window violation")
	}
	if state.StallReason != StallReasonScheduleBubble {
		t.Fatalf("expected schedule bubble to override readiness reason, got %q", state.StallReason)
	}
	if !state.TimingWaitBlocked {
		t.Fatalf("expected timing wait after strict window violation")
	}
	if state.OpTimingRollCycle[op.ID] != 9 {
		t.Fatalf("expected rolled cycle 9, got %d", state.OpTimingRollCycle[op.ID])
	}
}

func TestCanIssueStrictTimedDerivedTimingRollDoesNotDependOnReadiness(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         1,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		32: []int64{5},
	}
	op := Operation{
		OpCode: "DATA_MOV",
		ID:     32,
		SrcOperands: OperandList{
			Operands: []Operand{
				{Impl: "North", Color: "R"},
			},
		},
	}

	state.CurrentCycle = 7 // first observe violation while not ready
	if emu.canIssue(op, &state) {
		t.Fatalf("expected violation cycle to block")
	}
	if state.OpTimingRollCycle[op.ID] != 9 {
		t.Fatalf("expected rolled cycle 9, got %d", state.OpTimingRollCycle[op.ID])
	}

	state.CurrentCycle = 8
	if emu.canIssue(op, &state) {
		t.Fatalf("expected strict_timed to wait before rolled cycle")
	}

	state.RecvBufHeadReady[0][0] = true // ready at rolled cycle
	state.CurrentCycle = 9
	if !emu.canIssue(op, &state) {
		t.Fatalf("expected strict_timed to issue at first legal rolled cycle when ready")
	}
}

func TestIsStrictControlSensitiveOpCoversAliasesAndFamilies(t *testing.T) {
	cases := []struct {
		opCode string
		want   bool
	}{
		{opCode: "PHI_START", want: true},
		{opCode: "grant_once", want: true},
		{opCode: "ICMP_SGE", want: true},
		{opCode: "CMP_EXPORT", want: true},
		{opCode: "lt_ex", want: true},
		{opCode: "RETURN_VALUE", want: true},
		{opCode: "BNE", want: true},
		{opCode: "CTRL_MOV", want: true},
		{opCode: "ADD", want: false},
		{opCode: "DATA_MOV", want: false},
		{opCode: "NOP", want: false},
	}

	for _, tc := range cases {
		got := isStrictControlSensitiveOp(tc.opCode)
		if got != tc.want {
			t.Fatalf("isStrictControlSensitiveOp(%q) = %t, want %t", tc.opCode, got, tc.want)
		}
	}
}

func TestCanIssueStrictTimedDerivedTimingControlAliasSkipsWindowPenalty(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         0,
		StrictFailOnViolation: false,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		33: []int64{5},
	}
	op := Operation{
		OpCode: "CMP_EXPORT",
		ID:     33,
	}

	state.CurrentCycle = 7 // lateness=2, but control-sensitive alias should skip penalty
	if !emu.canIssue(op, &state) {
		t.Fatalf("expected control-sensitive alias to skip finite-W replay penalty")
	}
	if _, exists := state.OpTimingRollCycle[op.ID]; exists {
		t.Fatalf("did not expect roll cycle for control-sensitive op")
	}
}

func TestCanIssueGuidedDerivedTimingSetsOutputBlockedReason(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{
		23: []int64{5},
	}
	op := Operation{
		OpCode: "DATA_MOV",
		ID:     23,
		DstOperands: OperandList{
			Operands: []Operand{
				{Impl: "East", Color: "R"},
			},
		},
	}
	state.CurrentCycle = 5
	state.SendBufHeadBusy[0][emu.getDirecIndex("East")] = true // East-R blocked
	if emu.canIssue(op, &state) {
		t.Fatalf("expected guided mode to block when output is busy")
	}
	if state.StallReason != StallReasonOutputBlocked {
		t.Fatalf("expected output blocked stall reason, got %q", state.StallReason)
	}
}

func TestCanIssueStrictTimedDerivedTimingWindowViolationHard(t *testing.T) {
	emu := instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       ExecutionPolicyStrictTimed,
		StrictMaxSlip:         0,
		StrictFailOnViolation: true,
	}
	state := newPolicyTestState()
	state.Code.CompiledII = 2
	state.Code.DerivedTiming = map[int][]int64{
		19: []int64{5},
	}
	op := Operation{
		OpCode: "NOP",
		ID:     19,
	}

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatalf("expected strict_timed hard mode panic on window violation")
		}
		if !strings.Contains(recovered.(string), "strict slip window violation") {
			t.Fatalf("unexpected panic: %v", recovered)
		}
	}()

	state.CurrentCycle = 6 // lateness=1 > max slip=0
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
