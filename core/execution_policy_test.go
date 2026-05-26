package core

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

type readyHeldLog struct {
	RunMode              string `json:"run_mode"`
	Cycle                int64  `json:"cycle"`
	X                    int    `json:"X"`
	Y                    int    `json:"Y"`
	ID                   int    `json:"ID"`
	OccurrenceIndex      int    `json:"occurrence_index"`
	OpCode               string `json:"OpCode"`
	AnnotatedTimeT       *int64 `json:"annotated_time_t"`
	OperandsReady        bool   `json:"operands_ready"`
	PredicateReadyOrTrue bool   `json:"predicate_ready_or_true"`
	ResourcesAvailable   bool   `json:"resources_available"`
	TimingGateSatisfied  bool   `json:"timing_gate_satisfied"`
	FireableExceptTime   bool   `json:"fireable_except_time"`
	BlockedByLowerBound  bool   `json:"blocked_by_lower_bound"`
	IssuedThisCycle      bool   `json:"issued_this_cycle"`
}

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
		OpIssueCount:      make(map[int]int),
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

func captureReadyHeldLogs(t *testing.T, fn func()) []readyHeldLog {
	t.Helper()

	var buffer bytes.Buffer
	oldLogger := slog.Default()
	oldTraceEnabled := TraceEnabled()
	oldObserver := traceObserver

	handler := slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: LevelTrace})
	slog.SetDefault(slog.New(handler))
	SetTraceEnabled(true)
	traceObserver = nil
	defer func() {
		traceObserver = oldObserver
		SetTraceEnabled(oldTraceEnabled)
		slog.SetDefault(oldLogger)
	}()

	fn()

	logs := make([]readyHeldLog, 0)
	for _, line := range strings.Split(strings.TrimSpace(buffer.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry struct {
			Msg string `json:"msg"`
			readyHeldLog
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal trace line: %v", err)
		}
		if entry.Msg != "ReadyHeld" {
			continue
		}
		logs = append(logs, entry.readyHeldLog)
	}
	return logs
}

func newSyncTraceState(group InstructionGroup) coreState {
	state := newPolicyTestState()
	state.SelectedBlock = &EntryBlock{InstructionGroups: []InstructionGroup{group}}
	state.PCInBlock = 0
	state.ReadyHeldTraceEnabled = true
	state.ReadyHeldRunMode = "lower_bound"
	return state
}

func TestIssueDecisionElasticScheduledDerivedTimingReadyButHeld(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{9: {5}}
	state.CurrentCycle = 4
	operation := Operation{OpCode: "NOP", ID: 9}

	decision := emu.issueDecision(operation, &state)
	if decision.AnnotatedTimeT == nil || *decision.AnnotatedTimeT != 5 {
		t.Fatalf("annotated_time_t = %v, want 5", decision.AnnotatedTimeT)
	}
	if !decision.OperandsReady || !decision.PredicateReadyOrTrue || !decision.ResourcesAvailable {
		t.Fatalf("expected all non-timing readiness gates to pass: %+v", decision)
	}
	if decision.TimingGateSatisfied {
		t.Fatalf("expected timing gate to be unsatisfied before annotated cycle")
	}
	if !decision.FireableExceptTime {
		t.Fatalf("expected fireable_except_time=true when only lower-bound timing blocks issue")
	}
	if !decision.BlockedByLowerBound {
		t.Fatalf("expected blocked_by_lower_bound=true")
	}
	if decision.CanIssue {
		t.Fatalf("expected can_issue=false before annotated cycle")
	}

	emu.applyIssueDecision(operation, &state, decision)
	if !state.TimingWaitBlocked {
		t.Fatalf("expected timing wait marker after applying decision")
	}
	if state.StallReason != StallReasonScheduleBubble {
		t.Fatalf("stall reason = %q, want %q", state.StallReason, StallReasonScheduleBubble)
	}
}

func TestIssueDecisionElasticScheduledDerivedTimingOutputBlocked(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{23: {5}}
	state.CurrentCycle = 5
	state.SendBufHeadBusy[0][emu.getDirecIndex("East")] = true
	operation := Operation{
		OpCode:      "DATA_MOV",
		ID:          23,
		DstOperands: OperandList{Operands: []Operand{{Impl: "East", Color: "R"}}},
	}

	decision := emu.issueDecision(operation, &state)
	if decision.ResourcesAvailable {
		t.Fatalf("expected resources_available=false")
	}
	if decision.FireableExceptTime {
		t.Fatalf("expected fireable_except_time=false when output credit is missing")
	}
	if decision.BlockedByLowerBound {
		t.Fatalf("expected blocked_by_lower_bound=false when non-timing checks already fail")
	}
	if decision.CanIssue {
		t.Fatalf("expected can_issue=false when output is blocked")
	}
}

func TestIssueDecisionElasticScheduledDerivedTimingOperandWait(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	state := newPolicyTestState()
	state.Code.DerivedTiming = map[int][]int64{11: {5}}
	state.CurrentCycle = 5
	operation := Operation{
		OpCode:      "DATA_MOV",
		ID:          11,
		SrcOperands: OperandList{Operands: []Operand{{Impl: "North", Color: "R"}}},
	}

	decision := emu.issueDecision(operation, &state)
	if decision.OperandsReady {
		t.Fatalf("expected operands_ready=false")
	}
	if decision.FireableExceptTime {
		t.Fatalf("expected fireable_except_time=false when operands are not ready")
	}
	if decision.BlockedByLowerBound {
		t.Fatalf("expected blocked_by_lower_bound=false when operand wait is the real blocker")
	}
}

func TestRunInstructionGroupWithSyncOpsEmitsBlockedThenIssuedReadyHeld(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	group := InstructionGroup{Operations: []Operation{{OpCode: "NOP", ID: 41}}}
	state := newSyncTraceState(group)
	state.Code.DerivedTiming = map[int][]int64{41: {5}}

	logs := captureReadyHeldLogs(t, func() {
		state.CurrentCycle = 4
		if !emu.RunInstructionGroupWithSyncOps(group, &state, 4) {
			t.Fatalf("expected timing wait to keep sync core alive")
		}
		state.CurrentCycle = 5
		if !emu.RunInstructionGroupWithSyncOps(group, &state, 5) {
			t.Fatalf("expected issued cycle to report progress")
		}
	})

	if len(logs) != 2 {
		t.Fatalf("expected 2 ReadyHeld logs, got %d: %+v", len(logs), logs)
	}
	if logs[0].OccurrenceIndex != 0 || logs[1].OccurrenceIndex != 0 {
		t.Fatalf("expected same occurrence index for blocked/issued pair, got %+v", logs)
	}
	if !logs[0].BlockedByLowerBound || logs[0].IssuedThisCycle {
		t.Fatalf("unexpected blocked log: %+v", logs[0])
	}
	if logs[1].BlockedByLowerBound || !logs[1].IssuedThisCycle {
		t.Fatalf("unexpected issued log: %+v", logs[1])
	}
	if state.OpIssueCount[41] != 1 {
		t.Fatalf("expected issued occurrence count to advance to 1, got %d", state.OpIssueCount[41])
	}
}

func TestRunInstructionGroupWithSyncOpsReadyHeldOccurrenceIndexIncrements(t *testing.T) {
	emu := instEmulator{
		CareFlags:       true,
		ExecutionPolicy: ExecutionPolicyElasticScheduled,
	}
	group := InstructionGroup{Operations: []Operation{{OpCode: "NOP", ID: 42}}}
	state := newSyncTraceState(group)
	state.Code.DerivedTiming = map[int][]int64{42: {5, 6}}

	logs := captureReadyHeldLogs(t, func() {
		state.CurrentCycle = 5
		if !emu.RunInstructionGroupWithSyncOps(group, &state, 5) {
			t.Fatalf("expected first issue to make progress")
		}
		state.CurrentCycle = 6
		if !emu.RunInstructionGroupWithSyncOps(group, &state, 6) {
			t.Fatalf("expected second issue to make progress")
		}
	})

	if len(logs) != 2 {
		t.Fatalf("expected 2 issued ReadyHeld logs, got %d: %+v", len(logs), logs)
	}
	if logs[0].OccurrenceIndex != 0 || logs[1].OccurrenceIndex != 1 {
		t.Fatalf("expected occurrence indexes [0 1], got [%d %d]", logs[0].OccurrenceIndex, logs[1].OccurrenceIndex)
	}
	if !logs[0].IssuedThisCycle || !logs[1].IssuedThisCycle {
		t.Fatalf("expected issued_this_cycle=true for both logs: %+v", logs)
	}
	if state.OpIssueCount[42] != 2 {
		t.Fatalf("expected occurrence count to reach 2, got %d", state.OpIssueCount[42])
	}
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
