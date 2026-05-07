//nolint:funlen,gocyclo,lll,unused,gosimple
package core

import (
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// OpMode controls how instruction groups are scheduled.
type OpMode int

const (
	ExitDelay = 100
)

const (
	SyncOp OpMode = iota
	// AsyncOp executes operations asynchronously with reservation tracking.
	AsyncOp
)

const (
	ExecutionPolicyStrictTimed      = "strict_timed"
	ExecutionPolicyElasticScheduled = "elastic_scheduled"
	ExecutionPolicyInOrderDataflow  = "in_order_dataflow"
)

const (
	StallReasonScheduleBubble = "schedule_bubble"
	StallReasonOperandWait    = "operand_wait"
	StallReasonOutputBlocked  = "output_blocked"
)

type routingRule struct {
	src   cgra.Side
	dst   cgra.Side
	color string
}

// Trigger describes routing trigger metadata.
type Trigger struct {
	src    [12]bool
	color  int
	branch string
}

// ReservationState tracks pending operations and operand usage.
type ReservationState struct {
	ReservationMap  map[int]bool // to show whether each operation of a instruction group is finished.
	OpToExec        int
	RefCountRuntime map[string]int // to record the left times to be used of each source opearand. deep copied from RefCount
}

// DecrementRefCount decrements runtime operand use count and reports if still in use.
func (r *ReservationState) DecrementRefCount(opr Operand, state *coreState) bool {
	key := opr.Impl + opr.Color
	if _, ok := r.RefCountRuntime[key]; ok {
		if r.RefCountRuntime[key] == 0 {
			panic("ref count is 0 in DecrementRefCount before decrement " + key + "@(" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ")")
		}
		r.RefCountRuntime[key]--
		if r.RefCountRuntime[key] == 0 {
			return false
		}
		return true
	}
	// something wrong, raise error
	panic("invalid operand in DecrementRefCount")
}

// SetRefCount initializes runtime operand reference counters for an instruction group.
func (r *ReservationState) SetRefCount(ig InstructionGroup, state *coreState) {
	for _, op := range ig.Operations {
		for _, opr := range op.SrcOperands.Operands {
			if state.Directions[opr.Impl] {
				key := opr.Impl + opr.Color
				r.RefCountRuntime[key]++
			}
		}
		// only port in the src is needed to be considered
	}
}

// SetReservationMap marks operations in the instruction group as pending.
func (r *ReservationState) SetReservationMap(ig InstructionGroup, state *coreState) {
	for i := 0; i < len(ig.Operations); i++ {
		r.ReservationMap[i] = true
	}
	r.OpToExec = len(ig.Operations)
	// print("SetReservationMap: ", r.OpToExec, "\n")
}

type coreState struct {
	exit                 *bool // the signal is shared between cores
	requestExitTimestamp *float64
	retVal               *uint32 // the value is shared between cores
	SelectedBlock        *EntryBlock
	Directions           map[string]bool
	PCInBlock            int32
	NextPCInBlock        int32
	TileX, TileY         uint32
	Registers            []cgra.Data
	States               map[string]interface{} // This is to store core states, such as Phiconst, CmpFlags
	// still consider using outside block to control pc
	//Code         [][]string
	Memory               []uint32
	Code                 Program
	CurrReservationState ReservationState

	Mode OpMode

	RecvBufHead            [][]cgra.Data //[Color][Direction]
	RecvBufHeadReady       [][]bool
	SendBufHead            [][]cgra.Data
	SendBufHeadBusy        [][]bool
	RecvBufQueue           [][][]cgra.Data // [Color][Direction]FIFO
	SendBufQueue           [][][]cgra.Data // [Color][Direction]FIFO
	RecvQueueCapacity      int
	SendQueueCapacity      int
	EnableFIFOModel        bool
	EnableVectorPE         bool
	VectorLanes            int
	EnableQueueWatches     bool
	ConfiguredQueueWatches []resolvedQueueWatch
	WatchedQueues          []resolvedQueueWatch
	OpInputReadCache       map[string]cgra.Data
	AddrBuf                uint32 // buffer for the address of the memory
	IsToWriteMemory        bool
	BlockingMemoryOps      bool
	PendingMemoryOp        *pendingMemoryOp

	routingRules          []*routingRule
	triggers              []*Trigger
	CurrentTime           float64 // current simulation time for logging
	CurrentCycle          int64
	OpTimingCursor        map[int]int
	OpTimingLate          map[int]bool
	OpTimingRollCycle     map[int]int64
	OpIssueCount          map[int]int
	PendingSyncGroup      *pendingSyncGroup
	ReadyHeldTraceEnabled bool
	ReadyHeldRunMode      string
	TimingWaitBlocked     bool
	StallReason           string
	StallOpID             int
	StallOpCode           string
}

type pendingMemoryOp struct {
	OpCode      string
	Address     uint32
	Value       uint32
	Dst         []Operand
	Pred        bool
	IsWrite     bool
	RequestID   string
	RequestSent bool
	DataReady   *cgra.Data
	WriteDone   bool
}

type instEmulator struct {
	CareFlags             bool
	ExecutionPolicy       string
	StrictMaxSlip         int64
	StrictFailOnViolation bool
}

type issueReadiness struct {
	OperandsReady        bool
	PredicateReadyOrTrue bool
	ResourcesAvailable   bool
	Ready                bool
	WaitReason           string
}

type issueDecision struct {
	AnnotatedTimeT       *int64
	OperandsReady        bool
	PredicateReadyOrTrue bool
	ResourcesAvailable   bool
	TimingGateSatisfied  bool
	FireableExceptTime   bool
	BlockedByLowerBound  bool
	CanIssue             bool
	WaitReason           string
	TimingWaitBlocked    bool
}

type readyHeldObservation struct {
	RunMode              string
	Cycle                int64
	X                    int
	Y                    int
	OpID                 int
	OccurrenceIndex      int
	OpCode               string
	AnnotatedTimeT       *int64
	OperandsReady        bool
	PredicateReadyOrTrue bool
	ResourcesAvailable   bool
	TimingGateSatisfied  bool
	FireableExceptTime   bool
	BlockedByLowerBound  bool
	IssuedThisCycle      bool
}

type pendingSyncGroup struct {
	RemainingCycles   int
	BufferedResults   map[Operand]cgra.Data
	InvalidDecrements []int
	RepresentativeID  int
	RepresentativeOp  string
}

func (s *coreState) recvFIFOEnabled() bool {
	return s.EnableFIFOModel &&
		len(s.RecvBufQueue) == 4 &&
		len(s.RecvBufQueue[0]) > int(cgra.Router)
}

func (s *coreState) sendFIFOEnabled() bool {
	return s.EnableFIFOModel &&
		len(s.SendBufQueue) == 4 &&
		len(s.SendBufQueue[0]) > int(cgra.Router)
}

func (s *coreState) recvQueueCap() int {
	if s.RecvQueueCapacity > 0 {
		return s.RecvQueueCapacity
	}
	return 1
}

func (s *coreState) sendQueueCap(color, direction int) int {
	// Keep router-red as single outstanding request to preserve existing
	// address/req-state coupling semantics.
	if color == 0 && direction == int(cgra.Router) {
		return 1
	}
	if s.SendQueueCapacity > 0 {
		return s.SendQueueCapacity
	}
	return 1
}

func (s *coreState) syncRecvHead(color, direction int) {
	if len(s.RecvBufHead) <= color || len(s.RecvBufHeadReady) <= color {
		return
	}
	if len(s.RecvBufHead[color]) <= direction || len(s.RecvBufHeadReady[color]) <= direction {
		return
	}
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction && len(s.RecvBufQueue[color][direction]) > 0 {
		s.RecvBufHead[color][direction] = s.RecvBufQueue[color][direction][0]
		s.RecvBufHeadReady[color][direction] = true
		return
	}
	s.RecvBufHeadReady[color][direction] = false
}

func (s *coreState) syncSendHead(color, direction int) {
	if len(s.SendBufHead) <= color || len(s.SendBufHeadBusy) <= color {
		return
	}
	if len(s.SendBufHead[color]) <= direction || len(s.SendBufHeadBusy[color]) <= direction {
		return
	}
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction && len(s.SendBufQueue[color][direction]) > 0 {
		s.SendBufHead[color][direction] = s.SendBufQueue[color][direction][0]
		s.SendBufHeadBusy[color][direction] = true
		return
	}
	s.SendBufHeadBusy[color][direction] = false
}

func (s *coreState) recvQueueLen(color, direction int) int {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction {
		return len(s.RecvBufQueue[color][direction])
	}
	if s.RecvBufHeadReady[color][direction] {
		return 1
	}
	return 0
}

func (s *coreState) sendQueueLen(color, direction int) int {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		return len(s.SendBufQueue[color][direction])
	}
	if s.SendBufHeadBusy[color][direction] {
		return 1
	}
	return 0
}

func (s *coreState) recvQueueIsFull(color, direction int) bool {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction {
		return len(s.RecvBufQueue[color][direction]) >= s.recvQueueCap()
	}
	return s.RecvBufHeadReady[color][direction]
}

func (s *coreState) recvQueuePush(color, direction int, data cgra.Data) bool {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction {
		if len(s.RecvBufQueue[color][direction]) >= s.recvQueueCap() {
			return false
		}
		s.RecvBufQueue[color][direction] = append(s.RecvBufQueue[color][direction], data)
		s.syncRecvHead(color, direction)
		return true
	}
	if s.RecvBufHeadReady[color][direction] {
		return false
	}
	s.RecvBufHead[color][direction] = data
	s.RecvBufHeadReady[color][direction] = true
	return true
}

func (s *coreState) recvQueuePeek(color, direction int) (cgra.Data, bool) {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction {
		if len(s.RecvBufQueue[color][direction]) == 0 {
			return cgra.Data{}, false
		}
		return s.RecvBufQueue[color][direction][0], true
	}
	if !s.RecvBufHeadReady[color][direction] {
		return cgra.Data{}, false
	}
	return s.RecvBufHead[color][direction], true
}

func (s *coreState) recvQueueConsume(color, direction int) (cgra.Data, bool) {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction {
		if len(s.RecvBufQueue[color][direction]) == 0 {
			return cgra.Data{}, false
		}
		value := s.RecvBufQueue[color][direction][0]
		s.RecvBufQueue[color][direction] = s.RecvBufQueue[color][direction][1:]
		s.syncRecvHead(color, direction)
		return value, true
	}
	if !s.RecvBufHeadReady[color][direction] {
		return cgra.Data{}, false
	}
	value := s.RecvBufHead[color][direction]
	s.RecvBufHeadReady[color][direction] = false
	return value, true
}

func (s *coreState) sendQueueHasData(color, direction int) bool {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		return len(s.SendBufQueue[color][direction]) > 0
	}
	return s.SendBufHeadBusy[color][direction]
}

func (s *coreState) sendQueueIsFull(color, direction int) bool {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		return len(s.SendBufQueue[color][direction]) >= s.sendQueueCap(color, direction)
	}
	return s.SendBufHeadBusy[color][direction]
}

func (s *coreState) sendQueuePush(color, direction int, data cgra.Data) bool {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		if len(s.SendBufQueue[color][direction]) >= s.sendQueueCap(color, direction) {
			return false
		}
		s.SendBufQueue[color][direction] = append(s.SendBufQueue[color][direction], data)
		s.syncSendHead(color, direction)
		return true
	}
	if s.SendBufHeadBusy[color][direction] {
		return false
	}
	s.SendBufHeadBusy[color][direction] = true
	s.SendBufHead[color][direction] = data
	return true
}

func (s *coreState) sendQueuePeek(color, direction int) (cgra.Data, bool) {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		if len(s.SendBufQueue[color][direction]) == 0 {
			return cgra.Data{}, false
		}
		return s.SendBufQueue[color][direction][0], true
	}
	if !s.SendBufHeadBusy[color][direction] {
		return cgra.Data{}, false
	}
	return s.SendBufHead[color][direction], true
}

func (s *coreState) sendQueueConsume(color, direction int) (cgra.Data, bool) {
	if s.sendFIFOEnabled() && len(s.SendBufQueue[color]) > direction {
		if len(s.SendBufQueue[color][direction]) == 0 {
			return cgra.Data{}, false
		}
		value := s.SendBufQueue[color][direction][0]
		s.SendBufQueue[color][direction] = s.SendBufQueue[color][direction][1:]
		s.syncSendHead(color, direction)
		return value, true
	}
	if !s.SendBufHeadBusy[color][direction] {
		return cgra.Data{}, false
	}
	value := s.SendBufHead[color][direction]
	s.SendBufHeadBusy[color][direction] = false
	return value, true
}

func (s *coreState) resetPortQueues() {
	for color := range s.RecvBufHeadReady {
		for direction := range s.RecvBufHeadReady[color] {
			s.RecvBufHeadReady[color][direction] = false
		}
	}
	for color := range s.SendBufHeadBusy {
		for direction := range s.SendBufHeadBusy[color] {
			s.SendBufHeadBusy[color][direction] = false
		}
	}
	if s.recvFIFOEnabled() {
		for color := range s.RecvBufQueue {
			for direction := range s.RecvBufQueue[color] {
				s.RecvBufQueue[color][direction] = s.RecvBufQueue[color][direction][:0]
				s.syncRecvHead(color, direction)
			}
		}
	}
	if s.sendFIFOEnabled() {
		for color := range s.SendBufQueue {
			for direction := range s.SendBufQueue[color] {
				s.SendBufQueue[color][direction] = s.SendBufQueue[color][direction][:0]
				s.syncSendHead(color, direction)
			}
		}
	}
}

func clone2DData(input [][]cgra.Data) [][]cgra.Data {
	if input == nil {
		return nil
	}
	out := make([][]cgra.Data, len(input))
	for i := range input {
		if input[i] == nil {
			continue
		}
		out[i] = append([]cgra.Data(nil), input[i]...)
	}
	return out
}

func clone2DBool(input [][]bool) [][]bool {
	if input == nil {
		return nil
	}
	out := make([][]bool, len(input))
	for i := range input {
		if input[i] == nil {
			continue
		}
		out[i] = append([]bool(nil), input[i]...)
	}
	return out
}

func clone3DData(input [][][]cgra.Data) [][][]cgra.Data {
	if input == nil {
		return nil
	}
	out := make([][][]cgra.Data, len(input))
	for i := range input {
		if input[i] == nil {
			continue
		}
		out[i] = make([][]cgra.Data, len(input[i]))
		for j := range input[i] {
			if input[i][j] == nil {
				continue
			}
			out[i][j] = append([]cgra.Data(nil), input[i][j]...)
		}
	}
	return out
}

func cloneStringBoolMap(input map[string]bool) map[string]bool {
	if input == nil {
		return nil
	}
	out := make(map[string]bool, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneStringIntMap(input map[string]int) map[string]int {
	if input == nil {
		return nil
	}
	out := make(map[string]int, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneIntBoolMap(input map[int]bool) map[int]bool {
	if input == nil {
		return nil
	}
	out := make(map[int]bool, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneIntIntMap(input map[int]int) map[int]int {
	if input == nil {
		return nil
	}
	out := make(map[int]int, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneIntInt64Map(input map[int]int64) map[int]int64 {
	if input == nil {
		return nil
	}
	out := make(map[int]int64, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneOperandDataMap(input map[Operand]cgra.Data) map[Operand]cgra.Data {
	if input == nil {
		return nil
	}
	out := make(map[Operand]cgra.Data, len(input))
	for operand, value := range input {
		out[operand] = value
	}
	return out
}

func cloneIntSlice(input []int) []int {
	if input == nil {
		return nil
	}
	return append([]int(nil), input...)
}

func clonePendingSyncGroup(input *pendingSyncGroup) *pendingSyncGroup {
	if input == nil {
		return nil
	}
	return &pendingSyncGroup{
		RemainingCycles:   input.RemainingCycles,
		BufferedResults:   cloneOperandDataMap(input.BufferedResults),
		InvalidDecrements: cloneIntSlice(input.InvalidDecrements),
		RepresentativeID:  input.RepresentativeID,
		RepresentativeOp:  input.RepresentativeOp,
	}
}

func clonePendingMemoryOp(input *pendingMemoryOp) *pendingMemoryOp {
	if input == nil {
		return nil
	}
	clone := *input
	clone.Dst = append([]Operand(nil), input.Dst...)
	if input.DataReady != nil {
		value := *input.DataReady
		clone.DataReady = &value
	}
	return &clone
}

func cloneIntAnyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func (s *coreState) cloneForEval() *coreState {
	clone := *s
	clone.Registers = append([]cgra.Data(nil), s.Registers...)
	clone.Memory = append([]uint32(nil), s.Memory...)
	clone.States = cloneIntAnyMap(s.States)
	clone.Directions = cloneStringBoolMap(s.Directions)
	clone.RecvBufHead = clone2DData(s.RecvBufHead)
	clone.RecvBufHeadReady = clone2DBool(s.RecvBufHeadReady)
	clone.SendBufHead = clone2DData(s.SendBufHead)
	clone.SendBufHeadBusy = clone2DBool(s.SendBufHeadBusy)
	clone.RecvBufQueue = clone3DData(s.RecvBufQueue)
	clone.SendBufQueue = clone3DData(s.SendBufQueue)
	clone.ConfiguredQueueWatches = cloneQueueWatches(s.ConfiguredQueueWatches)
	clone.WatchedQueues = cloneQueueWatches(s.WatchedQueues)
	clone.OpInputReadCache = make(map[string]cgra.Data)
	clone.OpTimingCursor = cloneIntIntMap(s.OpTimingCursor)
	clone.OpTimingLate = cloneIntBoolMap(s.OpTimingLate)
	clone.OpTimingRollCycle = cloneIntInt64Map(s.OpTimingRollCycle)
	clone.OpIssueCount = cloneIntIntMap(s.OpIssueCount)
	clone.PendingSyncGroup = clonePendingSyncGroup(s.PendingSyncGroup)
	clone.PendingMemoryOp = clonePendingMemoryOp(s.PendingMemoryOp)
	clone.CurrReservationState = ReservationState{
		ReservationMap:  cloneIntBoolMap(s.CurrReservationState.ReservationMap),
		OpToExec:        s.CurrReservationState.OpToExec,
		RefCountRuntime: cloneStringIntMap(s.CurrReservationState.RefCountRuntime),
	}
	return &clone
}

func (s *coreState) observeWatchedQueues(timeValue float64) {
	if s == nil || len(s.WatchedQueues) == 0 {
		return
	}

	for _, watch := range s.WatchedQueues {
		occupancy := 0
		var capacity int
		switch watch.Kind {
		case "recv":
			occupancy = s.recvQueueLen(watch.ColorIdx, watch.DirectionIdx)
			capacity = s.recvQueueCap()
		case "send":
			occupancy = s.sendQueueLen(watch.ColorIdx, watch.DirectionIdx)
			capacity = s.sendQueueCap(watch.ColorIdx, watch.DirectionIdx)
		default:
			continue
		}

		ObserveQueue(
			watch.Label,
			watch.Kind,
			timeValue,
			int(s.TileX),
			int(s.TileY),
			watch.Direction,
			watch.Color,
			occupancy,
			capacity,
		)
	}
}

func normalizeExecutionPolicyString(policy string) string {
	text := strings.ToLower(strings.TrimSpace(policy))
	switch text {
	case ExecutionPolicyStrictTimed, "strict-timed", "static":
		return ExecutionPolicyStrictTimed
	case ExecutionPolicyElasticScheduled, "elastic-scheduled", "hybrid":
		return ExecutionPolicyElasticScheduled
	case "", ExecutionPolicyInOrderDataflow, "in-order-dataflow", "dynamic":
		return ExecutionPolicyInOrderDataflow
	default:
		// Fall back to in-order dataflow for backward compatibility.
		return ExecutionPolicyInOrderDataflow
	}
}

func isStrictControlSensitiveOp(opCode string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(opCode))
	switch {
	case normalized == "SEL",
		normalized == "JMP",
		normalized == "RET",
		normalized == "CTRL_MOV",
		normalized == "CMP_EXPORT",
		normalized == "LT_EX":
		return true
	case strings.HasPrefix(normalized, "PHI"),
		strings.HasPrefix(normalized, "GRANT"),
		strings.HasPrefix(normalized, "ICMP"),
		strings.HasPrefix(normalized, "RETURN"),
		strings.HasPrefix(normalized, "B"):
		return true
	default:
		return false
	}
}

func (i instEmulator) panicSynchronizationViolation(operation Operation, state *coreState, reason string) {
	currentStep, targetStep, ii := i.resolveScheduleStep(operation, state)
	panic(fmt.Sprintf(
		"synchronization violation under %s: op=%s id=%d cycle=%d schedule_step=%d target_step=%d ii=%d raw_timestep=%d tile=(%d,%d): %s",
		normalizeExecutionPolicyString(i.ExecutionPolicy),
		operation.OpCode,
		operation.ID,
		state.CurrentCycle,
		currentStep,
		targetStep,
		ii,
		operation.TimeStep,
		state.TileX,
		state.TileY,
		reason,
	))
}

func (i instEmulator) resolveScheduleStep(operation Operation, state *coreState) (currentStep int64, targetStep int64, ii int64) {
	ii = int64(state.Code.CompiledII)
	if ii <= 0 {
		return state.CurrentCycle, int64(operation.TimeStep), 0
	}

	currentStep = state.CurrentCycle % ii
	if currentStep < 0 {
		currentStep += ii
	}

	targetStep = int64(operation.TimeStep)
	if targetStep < 0 {
		panic(fmt.Sprintf(
			"invalid time_step=%d for compiled_ii=%d at op=%s id=%d tile=(%d,%d)",
			operation.TimeStep,
			state.Code.CompiledII,
			operation.OpCode,
			operation.ID,
			state.TileX,
			state.TileY,
		))
	}
	// Normalize to phase within II: compiler may emit time_step >= ii (e.g. 4 when ii=4 → step 0).
	if targetStep >= ii {
		targetStep = targetStep % ii
	}

	return currentStep, targetStep, ii
}

func (i instEmulator) resolveDerivedSchedule(operation Operation, state *coreState) ([]int64, int, bool) {
	if state == nil || state.Code.DerivedTiming == nil {
		return nil, 0, false
	}

	schedule, exists := state.Code.DerivedTiming[operation.ID]
	if !exists || len(schedule) == 0 {
		return nil, 0, false
	}

	cursor := state.OpTimingCursor[operation.ID]
	return schedule, cursor, true
}

func (i instEmulator) advanceDerivedTimingCursor(operation Operation, state *coreState) {
	if state == nil || state.Code.DerivedTiming == nil {
		return
	}
	if _, exists := state.Code.DerivedTiming[operation.ID]; !exists {
		return
	}
	state.OpTimingCursor[operation.ID] = state.OpTimingCursor[operation.ID] + 1
	delete(state.OpTimingLate, operation.ID)
	delete(state.OpTimingRollCycle, operation.ID)
}

func (i instEmulator) setStallReason(state *coreState, operation Operation, reason string) {
	if state == nil || reason == "" {
		return
	}
	state.StallReason = reason
	state.StallOpID = operation.ID
	state.StallOpCode = operation.OpCode
}

func (i instEmulator) rollStrictExpectedCycle(expectedCycle, currentCycle int64, compiledII int) int64 {
	ii := int64(compiledII)
	if ii <= 0 {
		ii = 1
	}
	// Move to the next window start strictly after the current cycle.
	nextExpected := expectedCycle + ii
	if nextExpected > currentCycle {
		return nextExpected
	}
	delta := currentCycle - expectedCycle
	steps := delta/ii + 1
	return expectedCycle + steps*ii
}

func (s *coreState) readyHeldTraceActive() bool {
	return s != nil && s.ReadyHeldTraceEnabled && strings.TrimSpace(s.ReadyHeldRunMode) != ""
}

func (s *coreState) nextOpOccurrenceIndex(opID int) int {
	if s == nil || s.OpIssueCount == nil {
		return 0
	}
	return s.OpIssueCount[opID]
}

func (s *coreState) advanceOpOccurrenceIndex(opID int) {
	if s == nil {
		return
	}
	if s.OpIssueCount == nil {
		s.OpIssueCount = make(map[int]int)
	}
	s.OpIssueCount[opID] = s.OpIssueCount[opID] + 1
}

func (i instEmulator) applyIssueDecision(operation Operation, state *coreState, decision issueDecision) {
	if state == nil {
		return
	}
	if decision.TimingWaitBlocked {
		state.TimingWaitBlocked = true
	}
	if decision.WaitReason != "" {
		i.setStallReason(state, operation, decision.WaitReason)
	}
}

func (i instEmulator) readyHeldObservationFor(
	operation Operation,
	state *coreState,
	decision issueDecision,
	occurrenceIndex int,
	issuedThisCycle bool,
) (readyHeldObservation, bool) {
	if state == nil || !state.readyHeldTraceActive() {
		return readyHeldObservation{}, false
	}
	if !decision.FireableExceptTime && !decision.BlockedByLowerBound && !issuedThisCycle {
		return readyHeldObservation{}, false
	}
	return readyHeldObservation{
		RunMode:              state.ReadyHeldRunMode,
		Cycle:                state.CurrentCycle,
		X:                    int(state.TileX),
		Y:                    int(state.TileY),
		OpID:                 operation.ID,
		OccurrenceIndex:      occurrenceIndex,
		OpCode:               operation.OpCode,
		AnnotatedTimeT:       decision.AnnotatedTimeT,
		OperandsReady:        decision.OperandsReady,
		PredicateReadyOrTrue: decision.PredicateReadyOrTrue,
		ResourcesAvailable:   decision.ResourcesAvailable,
		TimingGateSatisfied:  decision.TimingGateSatisfied,
		FireableExceptTime:   decision.FireableExceptTime,
		BlockedByLowerBound:  decision.BlockedByLowerBound,
		IssuedThisCycle:      issuedThisCycle,
	}, true
}

func (i instEmulator) emitReadyHeldObservation(observation readyHeldObservation) {
	var annotated any
	if observation.AnnotatedTimeT != nil {
		annotated = *observation.AnnotatedTimeT
	}
	Trace(
		"ReadyHeld",
		"run_mode", observation.RunMode,
		"cycle", observation.Cycle,
		"X", observation.X,
		"Y", observation.Y,
		"ID", observation.OpID,
		"occurrence_index", observation.OccurrenceIndex,
		"OpCode", observation.OpCode,
		"annotated_time_t", annotated,
		"operands_ready", observation.OperandsReady,
		"predicate_ready_or_true", observation.PredicateReadyOrTrue,
		"resources_available", observation.ResourcesAvailable,
		"timing_gate_satisfied", observation.TimingGateSatisfied,
		"fireable_except_time", observation.FireableExceptTime,
		"blocked_by_lower_bound", observation.BlockedByLowerBound,
		"issued_this_cycle", observation.IssuedThisCycle,
	)
}

func (i instEmulator) issueDecision(operation Operation, state *coreState) issueDecision {
	decision := issueDecision{
		OperandsReady:        true,
		PredicateReadyOrTrue: true,
		ResourcesAvailable:   true,
		TimingGateSatisfied:  true,
		CanIssue:             true,
	}

	if !i.CareFlags || operation.InvalidIterations > 0 {
		decision.FireableExceptTime = true
		return decision
	}

	readiness := i.checkIssueReadinessDetails(operation, state)
	decision.OperandsReady = readiness.OperandsReady
	decision.PredicateReadyOrTrue = readiness.PredicateReadyOrTrue
	decision.ResourcesAvailable = readiness.ResourcesAvailable
	decision.FireableExceptTime = readiness.Ready

	policy := normalizeExecutionPolicyString(i.ExecutionPolicy)
	if schedule, cursor, hasDerived := i.resolveDerivedSchedule(operation, state); hasDerived &&
		(policy == ExecutionPolicyStrictTimed || policy == ExecutionPolicyElasticScheduled) {
		if cursor >= len(schedule) {
			decision.TimingGateSatisfied = false
			decision.FireableExceptTime = false
			decision.CanIssue = false
			return decision
		}

		annotatedTime := schedule[cursor]
		decision.AnnotatedTimeT = int64Ptr(annotatedTime)
		expectedCycle := annotatedTime

		switch policy {
		case ExecutionPolicyStrictTimed:
			if rolledCycle, exists := state.OpTimingRollCycle[operation.ID]; exists {
				expectedCycle = rolledCycle
			}
			decision.TimingGateSatisfied = state.CurrentCycle >= expectedCycle

			if isStrictControlSensitiveOp(operation.OpCode) {
				if state.CurrentCycle < expectedCycle {
					decision.CanIssue = false
					decision.WaitReason = StallReasonScheduleBubble
					decision.TimingWaitBlocked = true
					return decision
				}
				if readiness.Ready {
					if state.CurrentCycle > annotatedTime {
						state.OpTimingLate[operation.ID] = true
					}
					decision.CanIssue = true
					return decision
				}
				if state.CurrentCycle > annotatedTime {
					state.OpTimingLate[operation.ID] = true
				}
				decision.CanIssue = false
				decision.WaitReason = readiness.WaitReason
				return decision
			}

			if state.CurrentCycle < expectedCycle {
				decision.CanIssue = false
				decision.WaitReason = StallReasonScheduleBubble
				decision.TimingWaitBlocked = true
				return decision
			}

			lateness := state.CurrentCycle - expectedCycle
			if lateness > 0 && i.StrictMaxSlip >= 0 && lateness > i.StrictMaxSlip {
				reason := fmt.Sprintf(
					"strict slip window violation: lateness=%d exceeds max_slip=%d (expected=%d current=%d)",
					lateness,
					i.StrictMaxSlip,
					expectedCycle,
					state.CurrentCycle,
				)
				if i.StrictFailOnViolation {
					i.panicSynchronizationViolation(operation, state, reason)
				}

				nextExpected := i.rollStrictExpectedCycle(expectedCycle, state.CurrentCycle, state.Code.CompiledII)
				state.OpTimingRollCycle[operation.ID] = nextExpected
				Trace(
					"TimingViolation",
					"Policy", policy,
					"OpCode", operation.OpCode,
					"ID", operation.ID,
					"X", state.TileX,
					"Y", state.TileY,
					"ExpectedCycle", expectedCycle,
					"NextExpectedCycle", nextExpected,
					"CurrentCycle", state.CurrentCycle,
					"Lateness", lateness,
					"MaxSlip", i.StrictMaxSlip,
				)
				decision.CanIssue = false
				decision.WaitReason = StallReasonScheduleBubble
				decision.TimingWaitBlocked = true
				return decision
			}

			if !readiness.Ready {
				if state.CurrentCycle > annotatedTime {
					state.OpTimingLate[operation.ID] = true
				}
				decision.CanIssue = false
				decision.WaitReason = readiness.WaitReason
				return decision
			}

			if state.CurrentCycle > annotatedTime {
				state.OpTimingLate[operation.ID] = true
			}
			decision.CanIssue = true
			return decision
		case ExecutionPolicyElasticScheduled:
			decision.TimingGateSatisfied = state.CurrentCycle >= expectedCycle
			decision.BlockedByLowerBound = readiness.Ready && !decision.TimingGateSatisfied
			if !decision.TimingGateSatisfied {
				decision.CanIssue = false
				decision.WaitReason = StallReasonScheduleBubble
				decision.TimingWaitBlocked = true
				return decision
			}
			if readiness.Ready {
				decision.CanIssue = true
				return decision
			}
			decision.CanIssue = false
			decision.WaitReason = readiness.WaitReason
			return decision
		}
	}

	currentStep, targetStep, ii := i.resolveScheduleStep(operation, state)

	// No schedule (compiled_ii missing or 0): ignore time gating so existing workloads
	// (e.g. histogram) that do not use II-based scheduling still run like in-order.
	if ii <= 0 {
		decision.CanIssue = readiness.Ready
		decision.WaitReason = readiness.WaitReason
		return decision
	}

	switch policy {
	case ExecutionPolicyStrictTimed:
		decision.TimingGateSatisfied = currentStep >= targetStep
		if currentStep < targetStep {
			decision.CanIssue = false
			decision.WaitReason = StallReasonScheduleBubble
			decision.TimingWaitBlocked = true
			return decision
		}
		if currentStep == targetStep {
			if readiness.Ready {
				decision.CanIssue = true
				return decision
			}
			i.panicSynchronizationViolation(operation, state, "operand/credit not ready at scheduled step")
		}
		i.panicSynchronizationViolation(operation, state, "operation missed its exact scheduled step")
		return decision
	case ExecutionPolicyElasticScheduled:
		decision.TimingGateSatisfied = currentStep >= targetStep
		if currentStep < targetStep {
			decision.CanIssue = false
			decision.WaitReason = StallReasonScheduleBubble
			decision.TimingWaitBlocked = true
			return decision
		}
		decision.CanIssue = readiness.Ready
		decision.WaitReason = readiness.WaitReason
		return decision
	case ExecutionPolicyInOrderDataflow:
		decision.CanIssue = readiness.Ready
		decision.WaitReason = readiness.WaitReason
		return decision
	default:
		decision.CanIssue = readiness.Ready
		decision.WaitReason = readiness.WaitReason
		return decision
	}
}

func (i instEmulator) canIssue(operation Operation, state *coreState) bool {
	decision := i.issueDecision(operation, state)
	i.applyIssueDecision(operation, state, decision)
	return decision.CanIssue
}

// set up the necessary state for the instruction group
func (i instEmulator) SetUpInstructionGroup(index int32, state *coreState) {
	iGroup := state.SelectedBlock.InstructionGroups[index]

	state.CurrReservationState = ReservationState{
		ReservationMap:  make(map[int]bool),
		OpToExec:        0,
		RefCountRuntime: make(map[string]int),
	}
	state.CurrReservationState.SetReservationMap(iGroup, state)
	state.CurrReservationState.SetRefCount(iGroup, state)
}

func supportsDeferredLatency(opCode string) bool {
	switch normalizeLatencyOpcode(opCode) {
	case "LOAD", "STORE", "LDD", "STD", "LD", "LDW", "ST", "STW",
		"TRIGGER", "JMP", "BEQ", "BNE", "BLT",
		"RETURN_VALUE", "RETURN_VOID", "RET",
		"PHI", "PHI_CONST", "PHI_START", "GRANT_PREDICATE", "GRANT_ONCE":
		return false
	default:
		return true
	}
}

func (i instEmulator) deferredSyncGroupLatency(cinst InstructionGroup, state *coreState) (int, int, string, bool) {
	if state == nil {
		return 1, 0, "", false
	}

	type sendKey struct {
		color     int
		direction int
	}

	maxLatency := 1
	representativeID := 0
	representativeOp := ""
	requiredSends := make(map[sendKey]int)
	executedOps := 0

	for _, operation := range cinst.Operations {
		if operation.InvalidIterations > 0 {
			continue
		}
		executedOps++
		if !supportsDeferredLatency(operation.OpCode) {
			return 1, 0, "", false
		}

		latency := state.Code.OperationLatency(operation.OpCode)
		if latency > maxLatency {
			maxLatency = latency
			representativeID = operation.ID
			representativeOp = operation.OpCode
		} else if representativeOp == "" {
			representativeID = operation.ID
			representativeOp = operation.OpCode
		}

		for _, dst := range operation.DstOperands.Operands {
			normalized := i.normalizeDirection(dst.Impl)
			if !state.Directions[normalized] {
				continue
			}
			key := sendKey{
				color:     i.getColorIndex(dst.Color),
				direction: i.getDirecIndex(normalized),
			}
			requiredSends[key]++
			if requiredSends[key] > state.sendQueueCap(key.color, key.direction) {
				return 1, 0, "", false
			}
		}
	}

	if executedOps == 0 || maxLatency <= 1 {
		return 1, 0, "", false
	}

	return maxLatency, representativeID, representativeOp, true
}

func (i instEmulator) canCommitPendingSyncGroup(state *coreState, pending *pendingSyncGroup) bool {
	if state == nil || pending == nil {
		return true
	}

	type sendKey struct {
		color     int
		direction int
	}

	requiredSends := make(map[sendKey]int)
	for operand := range pending.BufferedResults {
		normalized := i.normalizeDirection(operand.Impl)
		if !state.Directions[normalized] {
			continue
		}
		key := sendKey{
			color:     i.getColorIndex(operand.Color),
			direction: i.getDirecIndex(normalized),
		}
		requiredSends[key]++
	}

	for key, required := range requiredSends {
		free := state.sendQueueCap(key.color, key.direction) - state.sendQueueLen(key.color, key.direction)
		if free < required {
			return false
		}
	}
	return true
}

func (i instEmulator) advancePendingSyncGroup(state *coreState) bool {
	if state == nil || state.PendingSyncGroup == nil {
		return false
	}

	pending := state.PendingSyncGroup
	if pending.RemainingCycles > 1 {
		pending.RemainingCycles--
		state.TimingWaitBlocked = true
		return true
	}

	if pending.RemainingCycles == 1 {
		pending.RemainingCycles = 0
	}

	if !i.canCommitPendingSyncGroup(state, pending) {
		state.TimingWaitBlocked = true
		state.StallReason = StallReasonOutputBlocked
		state.StallOpID = pending.RepresentativeID
		state.StallOpCode = pending.RepresentativeOp
		return true
	}

	for operand, value := range pending.BufferedResults {
		i.writeOperand(operand, value, state)
	}
	i.applyInvalidIterationDecrements(state, pending.InvalidDecrements)
	state.PendingSyncGroup = nil
	return true
}

func (i instEmulator) bufferDeferredResult(
	operand Operand,
	value cgra.Data,
	workState *coreState,
	bufferedResults map[Operand]cgra.Data,
) {
	bufferedResults[operand] = value
	if workState == nil || !strings.HasPrefix(operand.Impl, "$") {
		return
	}
	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
	if err != nil {
		panic(fmt.Sprintf("invalid register index in deferred result buffering: %v", operand))
	}
	if registerIndex < 0 || registerIndex >= len(workState.Registers) {
		panic(fmt.Sprintf("register index %d out of range in deferred result buffering", registerIndex))
	}
	workState.Registers[registerIndex] = value
}

func (i instEmulator) applyDeferredSyncIssueState(state *coreState, workState *coreState) {
	if state == nil || workState == nil {
		return
	}

	state.RecvBufHead = clone2DData(workState.RecvBufHead)
	state.RecvBufHeadReady = clone2DBool(workState.RecvBufHeadReady)
	state.RecvBufQueue = clone3DData(workState.RecvBufQueue)
	state.OpTimingCursor = cloneIntIntMap(workState.OpTimingCursor)
	state.OpTimingLate = cloneIntBoolMap(workState.OpTimingLate)
	state.OpTimingRollCycle = cloneIntInt64Map(workState.OpTimingRollCycle)
	state.OpIssueCount = cloneIntIntMap(workState.OpIssueCount)
	state.CurrentTime = workState.CurrentTime
}

func (i instEmulator) RunInstructionGroup(cinst InstructionGroup, state *coreState, time float64) bool {
	// check the Return signal
	if *state.exit && time > *state.requestExitTimestamp {
		if DebugEnabled() {
			slog.Debug("ExitSignal", "requestedAt", *state.requestExitTimestamp, "time", time)
		}
		return false
	}
	prevPC := state.PCInBlock
	prevCount := state.CurrReservationState.OpToExec
	progressSync := false
	if state.Mode == SyncOp {
		progressSync = i.RunInstructionGroupWithSyncOps(cinst, state, time)
	} else if state.Mode == AsyncOp {
		i.RunInstructionGroupWithAsyncOps(cinst, state, time)
	} else {
		panic("invalid mode")
	}

	nowCount := state.CurrReservationState.OpToExec

	// find the nextPC
	if state.Mode == AsyncOp {
		if state.CurrReservationState.OpToExec == 0 { // this instruction group is finished
			if state.NextPCInBlock == -1 { // nobody elect PC other than +4
				state.PCInBlock++
			} else { //  there is a jump
				state.PCInBlock = state.NextPCInBlock
				// not set block, in case of index out of range
			}

			if state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
				state.PCInBlock = -1
				state.SelectedBlock = nil
				slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
			} else {
				// set up for the new instruction group
				i.SetUpInstructionGroup(state.PCInBlock, state)
			}
			state.NextPCInBlock = -1
		} // else, this group is not finished, PC stays the same
	} else if state.Mode == SyncOp {
		if progressSync {
			// Timing wait means "advance cycle but keep the same instruction group",
			// otherwise later groups may observe stale local registers.
			if !state.TimingWaitBlocked {
				if state.NextPCInBlock == -1 {
					// print("PC+4 for PC=", state.PCInBlock, " X:", state.TileX, " Y:", state.TileY, "\n")
					// print("Instruction at PC=", state.PCInBlock, " is ", state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[0].OpCode, "\n")
					state.PCInBlock++
				} else {
					// print("PC+Jump to ", state.NextPCInBlock, " X:", state.TileX, " Y:", state.TileY, "\n")
					state.PCInBlock = state.NextPCInBlock
				}
			}
		}
		if state.SelectedBlock != nil && state.PCInBlock >= int32(len(state.SelectedBlock.InstructionGroups)) {
			state.PCInBlock = -1
			state.SelectedBlock = nil
			// print("PCInBlock = -1 at (", state.TileX, ",", state.TileY, ")\n")
			slog.Info("Flow", "PCInBlock", "-1", "X", state.TileX, "Y", state.TileY)
		}
		state.NextPCInBlock = -1
	} else {
		panic("invalid mode")
	}

	nowPC := state.PCInBlock

	if state.Mode == AsyncOp {
		if prevPC == nowPC && prevCount == nowCount {
			//print("Kernel (", state.TileX, ",", state.TileY, ") want to sleep, ", prevPC, " = ", nowPC, " ", prevCount, " = ", nowCount, " ", time, "\n")
			return false
		}
	} else if state.Mode == SyncOp {
		return progressSync
	} else {
		panic("invalid mode")
	}
	return true
}

func (i instEmulator) RunInstructionGroupWithSyncOps(cinst InstructionGroup, state *coreState, time float64) bool {
	state.TimingWaitBlocked = false
	state.StallReason = ""
	state.StallOpID = 0
	state.StallOpCode = ""
	state.OpInputReadCache = make(map[string]cgra.Data)
	if state.PendingMemoryOp != nil {
		return i.advancePendingMemoryOp(state)
	}
	if state.PendingSyncGroup != nil {
		return i.advancePendingSyncGroup(state)
	}
	if state.EnableFIFOModel {
		return i.runInstructionGroupWithSyncOpsTwoPhase(cinst, state, time)
	}
	return i.runInstructionGroupWithSyncOpsLegacy(cinst, state, time)
}

func (i instEmulator) advancePendingMemoryOp(state *coreState) bool {
	pending := state.PendingMemoryOp
	if pending == nil {
		return false
	}

	if pending.IsWrite {
		if !pending.WriteDone {
			state.TimingWaitBlocked = true
			state.StallReason = StallReasonOperandWait
			return true
		}
		state.PendingMemoryOp = nil
		return true
	}

	if pending.DataReady == nil {
		state.TimingWaitBlocked = true
		state.StallReason = StallReasonOperandWait
		return true
	}
	for _, dst := range pending.Dst {
		dstImpl := i.normalizeDirection(dst.Impl)
		if state.Directions[dstImpl] &&
			state.sendQueueIsFull(i.getColorIndex(dst.Color), i.getDirecIndex(dstImpl)) {
			state.TimingWaitBlocked = true
			state.StallReason = StallReasonOutputBlocked
			return true
		}
	}
	for _, dst := range pending.Dst {
		i.writeOperand(dst, *pending.DataReady, state)
	}
	state.PendingMemoryOp = nil
	return true
}

func (i instEmulator) runInstructionGroupWithSyncOpsLegacy(cinst InstructionGroup, state *coreState, time float64) bool {
	run := true
	type evaluatedDecision struct {
		operation       Operation
		decision        issueDecision
		occurrenceIndex int
	}
	evaluated := make([]evaluatedDecision, 0, len(cinst.Operations))
	for _, operation := range cinst.Operations {
		decision := i.issueDecision(operation, state)
		i.applyIssueDecision(operation, state, decision)
		evaluated = append(evaluated, evaluatedDecision{
			operation:       operation,
			decision:        decision,
			occurrenceIndex: state.nextOpOccurrenceIndex(operation.ID),
		})
		if decision.CanIssue {
			continue
		}
		run = false
		break
	}
	if run {
		deferredLatency, representativeID, representativeOp, deferGroup := i.deferredSyncGroupLatency(cinst, state)
		allResults := make(map[Operand]cgra.Data)
		invalidDecrements := make([]int, 0)
		for index := range cinst.Operations {
			operation := &state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[index]
			if operation.InvalidIterations > 0 {
				if deferGroup {
					invalidDecrements = append(invalidDecrements, index)
				} else {
					operation.InvalidIterations--
				}
				continue
			}
			occurrenceIndex := state.nextOpOccurrenceIndex(operation.ID)
			decision := evaluated[index].decision
			if observation, ok := i.readyHeldObservationFor(*operation, state, decision, occurrenceIndex, true); ok {
				i.emitReadyHeldObservation(observation)
			}
			results := i.RunOperation(*operation, state, time)
			state.advanceOpOccurrenceIndex(operation.ID)
			i.advanceDerivedTimingCursor(*operation, state)
			for operand, value := range results {
				allResults[operand] = value
			}
		}
		if deferGroup {
			state.PendingSyncGroup = &pendingSyncGroup{
				RemainingCycles:   deferredLatency - 1,
				BufferedResults:   allResults,
				InvalidDecrements: invalidDecrements,
				RepresentativeID:  representativeID,
				RepresentativeOp:  representativeOp,
			}
			state.TimingWaitBlocked = true
			return true
		}
		for operand, value := range allResults {
			i.writeOperand(operand, value, state)
		}
	} else {
		for _, eval := range evaluated {
			if observation, ok := i.readyHeldObservationFor(eval.operation, state, eval.decision, eval.occurrenceIndex, false); ok {
				i.emitReadyHeldObservation(observation)
			}
		}
	}
	if state.TimingWaitBlocked {
		if !run && state.StallReason != "" {
			Trace(
				"Stall",
				"Behavior", state.StallReason,
				"Policy", normalizeExecutionPolicyString(i.ExecutionPolicy),
				"Time", float64(state.CurrentCycle),
				"X", state.TileX,
				"Y", state.TileY,
				"ID", state.StallOpID,
				"OpCode", state.StallOpCode,
			)
		}
		return true
	}
	if !run && state.StallReason != "" {
		Trace(
			"Stall",
			"Behavior", state.StallReason,
			"Policy", normalizeExecutionPolicyString(i.ExecutionPolicy),
			"Time", float64(state.CurrentCycle),
			"X", state.TileX,
			"Y", state.TileY,
			"ID", state.StallOpID,
			"OpCode", state.StallOpCode,
		)
	}
	return run
}

func (i instEmulator) RunInstructionGroupWithAsyncOps(cinst InstructionGroup, state *coreState, time float64) {
	if state.EnableFIFOModel {
		i.runInstructionGroupWithAsyncOpsTwoPhase(cinst, state, time)
		return
	}
	i.runInstructionGroupWithAsyncOpsLegacy(cinst, state, time)
}

func (i instEmulator) runInstructionGroupWithAsyncOpsLegacy(cinst InstructionGroup, state *coreState, time float64) {
	// Collect all results first
	allResults := make(map[Operand]cgra.Data)
	for index := range cinst.Operations {
		// check all the operations in the instruction group and if any is ready, then run
		if !state.CurrReservationState.ReservationMap[index] {
			continue
		}
		// Get reference to the original operation in state.SelectedBlock
		operation := &state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations[index]
		if i.canIssue(*operation, state) { // can also only choose one (another pattern)
			state.CurrReservationState.ReservationMap[index] = false
			state.CurrReservationState.OpToExec--
			// Decrement InvalidIterations before running if needed
			if operation.InvalidIterations > 0 {
				// print("Invalid iteration for ", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")\n")
				operation.InvalidIterations--
				continue
			}
			results := i.RunOperation(*operation, state, time)
			state.advanceOpOccurrenceIndex(operation.ID)
			i.advanceDerivedTimingCursor(*operation, state)
			// Merge results into allResults
			for operand, value := range results {
				allResults[operand] = value
			}
			//print("RunOperation", operation.OpCode, "@(", state.TileX, ",", state.TileY, ")", time, ":", "YES", "\n")
		} else {
			//print("CheckFlags (", state.TileX, ",", state.TileY, ")", time, ":", "NO", "\n")
		}
	}
	// Write all results at once
	for operand, value := range allResults {
		i.writeOperand(operand, value, state)
	}
}

func (i instEmulator) runInstructionGroupWithSyncOpsTwoPhase(cinst InstructionGroup, state *coreState, time float64) bool {
	workState := state.cloneForEval()
	run := true
	type evaluatedDecision struct {
		operation       Operation
		decision        issueDecision
		occurrenceIndex int
	}
	evaluated := make([]evaluatedDecision, 0, len(cinst.Operations))
	for _, operation := range cinst.Operations {
		decision := i.issueDecision(operation, workState)
		i.applyIssueDecision(operation, workState, decision)
		evaluated = append(evaluated, evaluatedDecision{
			operation:       operation,
			decision:        decision,
			occurrenceIndex: workState.nextOpOccurrenceIndex(operation.ID),
		})
		if decision.CanIssue {
			continue
		}
		run = false
		break
	}

	if !run {
		for _, eval := range evaluated {
			if observation, ok := i.readyHeldObservationFor(eval.operation, workState, eval.decision, eval.occurrenceIndex, false); ok {
				i.emitReadyHeldObservation(observation)
			}
		}
		state.TimingWaitBlocked = workState.TimingWaitBlocked
		state.StallReason = workState.StallReason
		state.StallOpID = workState.StallOpID
		state.StallOpCode = workState.StallOpCode
		if state.TimingWaitBlocked {
			if state.StallReason != "" {
				Trace(
					"Stall",
					"Behavior", state.StallReason,
					"Policy", normalizeExecutionPolicyString(i.ExecutionPolicy),
					"Time", float64(state.CurrentCycle),
					"X", state.TileX,
					"Y", state.TileY,
					"ID", state.StallOpID,
					"OpCode", state.StallOpCode,
				)
			}
			return true
		}
		if state.StallReason != "" {
			Trace(
				"Stall",
				"Behavior", state.StallReason,
				"Policy", normalizeExecutionPolicyString(i.ExecutionPolicy),
				"Time", float64(state.CurrentCycle),
				"X", state.TileX,
				"Y", state.TileY,
				"ID", state.StallOpID,
				"OpCode", state.StallOpCode,
			)
		}
		return false
	}

	deferredLatency, representativeID, representativeOp, deferGroup := i.deferredSyncGroupLatency(cinst, state)
	invalidDecrements := make([]int, 0)
	issuedObservations := make([]readyHeldObservation, 0, len(cinst.Operations))
	bufferedResults := make(map[Operand]cgra.Data)
	for index, operation := range cinst.Operations {
		if operation.InvalidIterations > 0 {
			invalidDecrements = append(invalidDecrements, index)
			continue
		}
		occurrenceIndex := workState.nextOpOccurrenceIndex(operation.ID)
		decision := evaluated[index].decision
		if observation, ok := i.readyHeldObservationFor(operation, workState, decision, occurrenceIndex, true); ok {
			issuedObservations = append(issuedObservations, observation)
		}
		results := i.RunOperation(operation, workState, time)
		workState.advanceOpOccurrenceIndex(operation.ID)
		i.advanceDerivedTimingCursor(operation, workState)
		for operand, value := range results {
			if deferGroup {
				i.bufferDeferredResult(operand, value, workState, bufferedResults)
				continue
			}
			i.writeOperand(operand, value, workState)
		}
	}
	if deferGroup {
		i.applyDeferredSyncIssueState(state, workState)
		for _, observation := range issuedObservations {
			i.emitReadyHeldObservation(observation)
		}
		state.PendingSyncGroup = &pendingSyncGroup{
			RemainingCycles:   deferredLatency - 1,
			BufferedResults:   bufferedResults,
			InvalidDecrements: invalidDecrements,
			RepresentativeID:  representativeID,
			RepresentativeOp:  representativeOp,
		}
		state.TimingWaitBlocked = true
		return true
	}
	*state = *workState
	for _, observation := range issuedObservations {
		i.emitReadyHeldObservation(observation)
	}
	i.applyInvalidIterationDecrements(state, invalidDecrements)
	return true
}

func (i instEmulator) runInstructionGroupWithAsyncOpsTwoPhase(cinst InstructionGroup, state *coreState, time float64) {
	workState := state.cloneForEval()
	allResults := make(map[Operand]cgra.Data)
	invalidDecrements := make([]int, 0)
	for index, operation := range cinst.Operations {
		if !workState.CurrReservationState.ReservationMap[index] {
			continue
		}
		if i.canIssue(operation, workState) {
			workState.CurrReservationState.ReservationMap[index] = false
			workState.CurrReservationState.OpToExec--
			if operation.InvalidIterations > 0 {
				invalidDecrements = append(invalidDecrements, index)
				continue
			}
			results := i.RunOperation(operation, workState, time)
			workState.advanceOpOccurrenceIndex(operation.ID)
			i.advanceDerivedTimingCursor(operation, workState)
			for operand, value := range results {
				allResults[operand] = value
			}
		}
	}
	for operand, value := range allResults {
		i.writeOperand(operand, value, workState)
	}
	*state = *workState
	i.applyInvalidIterationDecrements(state, invalidDecrements)
}

func (i instEmulator) applyInvalidIterationDecrements(state *coreState, indices []int) {
	if len(indices) == 0 || state == nil || state.SelectedBlock == nil {
		return
	}
	if state.PCInBlock < 0 || int(state.PCInBlock) >= len(state.SelectedBlock.InstructionGroups) {
		return
	}
	operations := state.SelectedBlock.InstructionGroups[state.PCInBlock].Operations
	for _, idx := range indices {
		if idx < 0 || idx >= len(operations) {
			continue
		}
		if operations[idx].InvalidIterations > 0 {
			operations[idx].InvalidIterations--
		}
	}
}

func (i instEmulator) normalizeDirection(s string) string {
	u := strings.ToUpper(s)
	switch u {
	case "NORTH":
		return "North"
	case "SOUTH":
		return "South"
	case "EAST":
		return "East"
	case "WEST":
		return "West"
	case "NORTHEAST":
		return "NorthEast"
	case "NORTHWEST":
		return "NorthWest"
	case "SOUTHEAST":
		return "SouthEast"
	case "SOUTHWEST":
		return "SouthWest"
	case "ROUTER":
		return "Router"
	default:
		return s
	}
}

func (i instEmulator) checkIssueReadinessDetails(inst Operation, state *coreState) issueReadiness {
	readiness := issueReadiness{
		OperandsReady:        true,
		PredicateReadyOrTrue: true,
		ResourcesAvailable:   true,
		Ready:                true,
	}

	for index, src := range inst.SrcOperands.Operands {
		if index == 1 {
			if inst.OpCode == "PHI_CONST" || inst.OpCode == "PHI_START" {
				var stateKey string
				if inst.OpCode == "PHI_CONST" {
					stateKey = fmt.Sprintf("PhiConst_%d", inst.ID)
				} else if inst.OpCode == "PHI_START" {
					stateKey = fmt.Sprintf("PhiStart_%d", inst.ID)
				}
				if state.States[stateKey] == nil || state.States[stateKey] == false {
					if len(inst.SrcOperands.Operands) > 1 {
						continue
					}
					panic("PHI_CONST or PHI_START must have two sources")
				}
			}
		}
		srcImpl := i.normalizeDirection(src.Impl)
		if state.Directions[srcImpl] {
			if state.recvQueueLen(i.getColorIndex(src.Color), i.getDirecIndex(srcImpl)) == 0 {
				readiness.OperandsReady = false
				readiness.Ready = false
				readiness.WaitReason = StallReasonOperandWait
				return readiness
			}
		}
	}

	for _, dst := range inst.DstOperands.Operands {
		dstImpl := i.normalizeDirection(dst.Impl)
		if state.Directions[dstImpl] {
			if state.sendQueueIsFull(i.getColorIndex(dst.Color), i.getDirecIndex(dstImpl)) {
				Trace(
					"Backpressure",
					"Time", float64(state.CurrentCycle),
					"X", state.TileX,
					"Y", state.TileY,
					"OpCode", inst.OpCode,
					"ID", inst.ID,
					"Reason", "SendBufBusy",
					"DstDir", dstImpl,
					"Color", dst.Color,
					"Policy", normalizeExecutionPolicyString(i.ExecutionPolicy),
				)
				readiness.ResourcesAvailable = false
				readiness.Ready = false
				readiness.WaitReason = StallReasonOutputBlocked
				return readiness
			}
		}
	}

	if state.BlockingMemoryOps && isBlockingMemoryShape(inst, state) {
		routerColor := i.getColorIndex("R")
		routerDir := int(cgra.Router)
		if state.sendQueueIsFull(routerColor, routerDir) {
			readiness.ResourcesAvailable = false
			readiness.Ready = false
			readiness.WaitReason = StallReasonOutputBlocked
			return readiness
		}
	}

	return readiness
}

func (i instEmulator) checkIssueReadiness(inst Operation, state *coreState) (bool, string) {
	readiness := i.checkIssueReadinessDetails(inst, state)
	return readiness.Ready, readiness.WaitReason
}

func isBlockingMemoryShape(inst Operation, state *coreState) bool {
	if state == nil || !state.BlockingMemoryOps {
		return false
	}
	switch inst.OpCode {
	case "LD":
		return len(inst.DstOperands.Operands) > 0 && inst.DstOperands.Operands[0].Impl != "Router"
	case "ST":
		return len(inst.DstOperands.Operands) == 0
	default:
		return false
	}
}

func (i instEmulator) CheckFlags(inst Operation, state *coreState) bool {
	ready, _ := i.checkIssueReadiness(inst, state)
	return ready
}

func (i instEmulator) RunOperation(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	state.CurrentTime = time

	// Note: InvalidIterations is now handled in RunInstructionGroupWithSyncOps
	// before calling RunOperation, so we don't need to check it here

	instName := inst.OpCode
	/*
		if strings.Contains(instName, "CMP") {
			instName = "CMP"
		}*/
	// alwaysflag := false
	// if strings.HasPrefix(instName, "@") && !alwaysflag {
	// 	instName = "SENDREC"
	// 	alwaysflag = true
	// 	i.runAlwaysSendRec(tokens, state)
	// }

	instFuncs := map[string]func(Operation, *coreState) map[Operand]cgra.Data{
		"ADD":       i.runAdd, // ADD, ADDI, INC, SUB, DEC
		"SUB":       i.runSub,
		"SHL":       i.runSHL,
		"LRS":       i.runLRS,
		"MUL":       i.runMul,    // MULI
		"MUL_ADD":   i.runMulAdd, // dst = src0 * src1 + src2
		"DIV":       i.runDiv,
		"OR":        i.runOR,
		"XOR":       i.runXOR, // XOR XORI
		"AND":       i.runAND,
		"MOV":       i.runMov,
		"JMP":       i.runJmp,
		"BNE":       i.runBne,
		"BEQ":       i.runBeq, // BEQI
		"BLT":       i.runBlt,
		"PHI_CONST": i.runPhiConst, // backward compatibility
		"SEXT":      i.runMov,      // identity operation by now
		"ZEXT":      i.runMov,      // identity operation by now

		"FADD": i.runFAdd, // FADDI
		"FSUB": i.runFSub,
		"FMUL": i.runFMul,
		"NOP":  i.runNOP,

		"PHI":             i.runPhi,
		"PHI_START":       i.runPhiStart,
		"GRANT_PREDICATE": i.runGrantPred,
		"GRANT_ONCE":      i.runGrantOnce,
		"SEL":             i.runSel,

		// comparisons
		"ICMP_EQ":  i.runCmpExport,
		"ICMP_SLT": i.runLTExport,
		"ICMP_SGT": i.runGTExport,
		"ICMP_SGE": i.runSGEExport,
		"ICMP_ULT": i.runULTExport,

		// do not distinguish between data_mov and control mov
		"DATA_MOV": i.runMov,
		"CTRL_MOV": i.runMov,
		"CONSTANT": i.runMov,

		"GEP": i.runGep,

		"CMP_EXPORT": i.runCmpExport,

		"LT_EX": i.runLTExport,

		"LOAD":  i.runLoadDirect,
		"STORE": i.runStoreDirect,
		"LDD":   i.runLoadDirect,  // backward compatibility
		"STD":   i.runStoreDirect, // backward compatibility

		"LD":  i.runLoadDRAM,
		"LDW": i.runLoadWaitDRAM,

		"ST":  i.runStoreDRAM,
		"STW": i.runStoreWaitDRAM,

		"TRIGGER": i.runTrigger,

		"NOT": i.runNot,
	}

	vectorFuncs := map[string]func(Operation, *coreState) map[Operand]cgra.Data{
		"VBROADCAST":        i.runVBroadcast,
		"VADD":              i.runVAdd,
		"VMUL":              i.runVMul,
		"VECTOR.REDUCE.ADD": i.runVectorReduceAdd,
		"VEXTRACT":          i.runVExtract,
		"VLOAD_CONTIG":      i.runVLoadContig,
		"VSTORE_CONTIG":     i.runVStoreContig,
	}

	retFuncs := map[string]func(Operation, *coreState, float64) map[Operand]cgra.Data{
		"RETURN_VALUE": i.runRetImm,
		"RETURN_VOID":  i.runRetDelay,
		"RET":          i.runRetImm, // backward compatibility
	}

	if instFunc, ok := vectorFuncs[instName]; ok {
		return instFunc(inst, state)
	} else if instFunc, ok := instFuncs[instName]; ok {
		i.rejectVectorSourcesForScalarOp(inst, state)
		return instFunc(inst, state)
	} else if retFunc, ok := retFuncs[instName]; ok {
		i.rejectVectorSourcesForScalarOp(inst, state)
		return retFunc(inst, state, time)
	} else {
		panic(fmt.Sprintf("unknown instruction '%s' at PC %d", instName, state.PCInBlock))
	}
}

func (i instEmulator) readOperand(operand Operand, state *coreState) (value cgra.Data) {
	if strings.HasPrefix(operand.Impl, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in readOperand: %v", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in readOperand", registerIndex))
		}

		value = state.Registers[registerIndex]
		//fmt.Println("[readOperand] read ", value, "from register", registerIndex, ":", value, "@(", state.TileX, ",", state.TileY, ")")
	} else {
		normalizedImpl := i.normalizeDirection(operand.Impl)
		if state.Directions[normalizedImpl] {
			//fmt.Println("operand.Impl", operand.Impl)
			// must first check it is ready
			color, direction := i.getColorIndex(operand.Color), i.getDirecIndex(normalizedImpl)
			cacheKey := fmt.Sprintf("%d:%d", color, direction)
			if state.Mode == SyncOp {
				if cached, ok := state.OpInputReadCache[cacheKey]; ok {
					return cached
				}
			}
			peek, ok := state.recvQueuePeek(color, direction)
			if !ok {
				if state.Mode == SyncOp {
					// In sync mode, all ops in the same instruction group share one
					// snapshot of input heads. If a previous op consumed this queue
					// head earlier in the same tick, keep returning the snapshot.
					fallback := state.RecvBufHead[color][direction]
					state.OpInputReadCache[cacheKey] = fallback
					return fallback
				}
				panic(fmt.Sprintf("operand queue unexpectedly empty in async mode: %v", operand))
			}
			value = peek
			// consume queue head according to existing sync/async rules
			if state.Mode == SyncOp {
				consumed, ok := state.recvQueueConsume(color, direction)
				if !ok {
					panic(fmt.Sprintf("operand queue consume failed in sync mode: %v", operand))
				}
				value = consumed
				state.OpInputReadCache[cacheKey] = value
			} else {
				if !state.CurrReservationState.DecrementRefCount(operand, state) {
					// no longer used, pop queue head
					if _, ok := state.recvQueueConsume(color, direction); !ok {
						panic(fmt.Sprintf("operand queue consume failed in async mode: %v", operand))
					}
					//fmt.Println("Reduce {", operand.Impl, "} to zero")
				} else {
					//fmt.Println("Reduce {", operand.Impl, "} to ", state.CurrReservationState.RefCountRuntime[operand.Impl], "@(", state.TileX, ",", state.TileY, ")")
				}
			}
			//fmt.Println("state.RecvBufHead[", i.getColorIndex(operand.Color), "][", i.getDirecIndex(operand.Impl), "]:", value)
			//fmt.Println("[ReadOperand] read", value, "from port", operand.Impl, ":", value, "@(", state.TileX, ",", state.TileY, ")")
		} else {
			// try to convert into int
			operand.Impl = strings.TrimPrefix(operand.Impl, "#")
			num, err := strconv.Atoi(operand.Impl)
			if err == nil {
				value = cgra.NewScalar(uint32(num))
			} else {
				if immediate, err := strconv.ParseUint(operand.Impl, 0, 32); err == nil {
					value = cgra.NewScalar(uint32(immediate))
				} else {
					panic(fmt.Sprintf("Invalid operand %v in readOperand; at PC %d, (%d, %d)", operand, state.PCInBlock, state.TileX, state.TileY))
				}
			}
		}
	}
	return value
}

func (i instEmulator) writeOperand(operand Operand, value cgra.Data, state *coreState) {
	// if strings.HasPrefix(operand, "$") {
	// 	registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
	// 	if err != nil {
	// 		panic("invalid register index")
	// 	}

	// 	state.Registers[registerIndex] = value
	// }
	if strings.HasPrefix(operand.Impl, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil {
			panic(fmt.Sprintf("invalid register index in writeOperand: %v", operand))
		}

		if registerIndex < 0 || registerIndex >= len(state.Registers) {
			panic(fmt.Sprintf("register index %d out of range in writeOperand", registerIndex))
		}

		state.Registers[registerIndex] = value
		//fmt.Printf("Updated register $%d to value %d at PC %d\n", registerIndex, value, state.PC)
	} else {
		normalizedImpl := i.normalizeDirection(operand.Impl)
		if state.Directions[normalizedImpl] {
			color := i.getColorIndex(operand.Color)
			direction := i.getDirecIndex(normalizedImpl)
			if state.sendQueueIsFull(color, direction) {
				//fmt.Printf("sendbufhead busy\n")
				return
			}
			state.sendQueuePush(color, direction, value)
		} else {
			panic(fmt.Sprintf("Invalid operand %v in writeOperand; expected register", operand))
		}
	}
}

func (i instEmulator) rejectVectorSourcesForScalarOp(inst Operation, state *coreState) {
	for _, src := range inst.SrcOperands.Operands {
		value := i.peekScalarGuardOperand(src, state)
		if value.LaneCount() > 1 {
			panic(fmt.Sprintf("scalar opcode %s cannot consume vector src operand", inst.OpCode))
		}
	}
}

func (i instEmulator) peekScalarGuardOperand(operand Operand, state *coreState) cgra.Data {
	if strings.HasPrefix(operand.Impl, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand.Impl, "$"))
		if err != nil || registerIndex < 0 || registerIndex >= len(state.Registers) {
			return cgra.Data{}
		}
		return state.Registers[registerIndex]
	}
	normalizedImpl := i.normalizeDirection(operand.Impl)
	if state.Directions[normalizedImpl] {
		value, _ := state.recvQueuePeek(i.getColorIndex(operand.Color), i.getDirecIndex(normalizedImpl))
		return value
	}
	return cgra.NewScalar(0)
}

func (i instEmulator) vectorLanes(state *coreState) int {
	if state.VectorLanes <= 0 {
		return 1
	}
	return state.VectorLanes
}

func (i instEmulator) requireVectorEnabled(state *coreState) int {
	lanes := i.vectorLanes(state)
	if !state.EnableVectorPE || lanes <= 1 {
		panic("vector opcode requires simulator.device.enable_vector_pe=true")
	}
	return lanes
}

func (i instEmulator) requireVectorOperand(opCode string, value cgra.Data, lanes int) cgra.Data {
	if value.LaneCount() != lanes {
		panic(fmt.Sprintf("%s expects %d-lane vector", opCode, lanes))
	}
	return value
}

func (i instEmulator) runVBroadcast(inst Operation, state *coreState) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	src := i.readOperand(inst.SrcOperands.Operands[0], state)
	out := make([]uint32, lanes)
	for idx := range out {
		out[idx] = src.First()
	}
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.FromSlice(out, src.Pred)
	}
	return results
}

func (i instEmulator) runVectorBinary(inst Operation, state *coreState, fn func(uint32, uint32) uint32) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	src1 := i.requireVectorOperand(inst.OpCode, i.readOperand(inst.SrcOperands.Operands[0], state), lanes)
	src2 := i.requireVectorOperand(inst.OpCode, i.readOperand(inst.SrcOperands.Operands[1], state), lanes)
	out := make([]uint32, lanes)
	for idx := range out {
		out[idx] = fn(src1.Data[idx], src2.Data[idx])
	}
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.FromSlice(out, src1.Pred && src2.Pred)
	}
	return results
}

func (i instEmulator) runVAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	return i.runVectorBinary(inst, state, func(a, b uint32) uint32 { return a + b })
}

func (i instEmulator) runVMul(inst Operation, state *coreState) map[Operand]cgra.Data {
	return i.runVectorBinary(inst, state, func(a, b uint32) uint32 { return a * b })
}

func (i instEmulator) runVectorReduceAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	src := i.requireVectorOperand(inst.OpCode, i.readOperand(inst.SrcOperands.Operands[0], state), lanes)
	var sum uint32
	for _, lane := range src.Data {
		sum += lane
	}
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(sum, src.Pred)
	}
	return results
}

func (i instEmulator) runVExtract(inst Operation, state *coreState) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	src := i.requireVectorOperand(inst.OpCode, i.readOperand(inst.SrcOperands.Operands[0], state), lanes)
	index := int(i.readOperand(inst.SrcOperands.Operands[1], state).First())
	if index < 0 || index >= lanes {
		panic(fmt.Sprintf("vector extract index out of bounds: %d", index))
	}
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(src.Data[index], src.Pred)
	}
	return results
}

func (i instEmulator) runVLoadContig(inst Operation, state *coreState) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	base := int(i.readOperand(inst.SrcOperands.Operands[0], state).First())
	if base < 0 || base+lanes > len(state.Memory) {
		panic("vector load out of bounds")
	}
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.FromSlice(state.Memory[base:base+lanes], true)
	}
	return results
}

func (i instEmulator) runVStoreContig(inst Operation, state *coreState) map[Operand]cgra.Data {
	lanes := i.requireVectorEnabled(state)
	src := i.requireVectorOperand(inst.OpCode, i.readOperand(inst.SrcOperands.Operands[0], state), lanes)
	base := int(i.readOperand(inst.SrcOperands.Operands[1], state).First())
	if base < 0 || base+lanes > len(state.Memory) {
		panic("vector store out of bounds")
	}
	copy(state.Memory[base:base+lanes], src.Data)
	return nil
}

func (i instEmulator) getDirecIndex(side string) int {
	var srcIndex int

	switch side {
	case "North":
		srcIndex = int(cgra.North)
	case "West":
		srcIndex = int(cgra.West)
	case "South":
		srcIndex = int(cgra.South)
	case "East":
		srcIndex = int(cgra.East)
	case "NorthEast":
		srcIndex = int(cgra.NorthEast)
	case "NorthWest":
		srcIndex = int(cgra.NorthWest)
	case "SouthEast":
		srcIndex = int(cgra.SouthEast)
	case "SouthWest":
		srcIndex = int(cgra.SouthWest)
	case "Router":
		srcIndex = int(cgra.Router)
	default:
		panic("invalid side")
	}

	return srcIndex
}

func (i instEmulator) RouterSrcMustBeDirection(src string) {
	arr := []string{"NORTH", "SOUTH", "WEST", "EAST", "NORTHEAST", "NORTHWEST", "SOUTHEAST", "SOUTHWEST", "ROUTER"}
	res := false
	for _, s := range arr {
		if s == src {
			res = true
			break
		}
	}

	if res {
		panic("the source of a ROUTER_FORWARD instruction must be directions")
	}
}

func (i instEmulator) getColorIndex(color string) int {
	switch strings.ToUpper(color) {
	case "R", "RED":
		return 0
	case "Y", "YELLOW":
		return 1
	case "B", "BLUE":
		return 2
	default:
		panic("Wrong Color: [" + color + "]")
	}
}

// float32 to uint32
func float2Uint(f float32) uint32 {
	return math.Float32bits(f)
}

// uint32 to float32
func uint2Float(u uint32) float32 {
	return math.Float32frombits(u)
}

// runMov handles the MOV instruction for both immediate values and register-to-register moves.
// Prototype for moving an immediate: MOV, DstReg, Immediate
// Prototype for register to register: MOV, DstReg, SrcReg
func (i instEmulator) runMov(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	oprStruct := i.readOperand(src, state)
	opr := oprStruct.First()
	finalPred := oprStruct.Pred
	// Collect results for destination operands
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(opr, finalPred)
	}

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runGep(inst Operation, state *coreState) map[Operand]cgra.Data {
	results := make(map[Operand]cgra.Data)
	if len(inst.SrcOperands.Operands) == 0 {
		return results
	}

	src1 := inst.SrcOperands.Operands[0]
	src1Struct := i.readOperand(src1, state)
	src1Val := src1Struct.First()
	finalPred := src1Struct.Pred
	dstVal := src1Val

	if len(inst.SrcOperands.Operands) > 1 {
		src2 := inst.SrcOperands.Operands[1]
		src2Struct := i.readOperand(src2, state)
		src2Val := src2Struct.First()
		dstVal = src1Val + src2Val
		finalPred = src1Struct.Pred && src2Struct.Pred

		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	}

	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))

	return results
}

func (i instEmulator) runNOP(inst Operation, state *coreState) map[Operand]cgra.Data {
	// do nothing
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", false)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runNot(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	srcPred := srcStruct.Pred

	result := uint32(0)
	if srcVal == 0 {
		result = 1
	} else {
		result = 0
	}
	finalPred := srcPred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runLoadDirect(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	finalPred := addrStruct.Pred
	results := make(map[Operand]cgra.Data)

	// Predicated-off load should not touch memory or trigger bounds checks.
	if !finalPred {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, false)
		}
		return results
	}

	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds, addr: " + strconv.Itoa(int(addr)) + ", len(state.Memory): " + strconv.Itoa(len(state.Memory)))
	}
	value := state.Memory[addr]
	slog.Warn("Memory",
		"Time", state.CurrentTime,
		"Behavior", "LoadDirect",
		"ID", inst.ID,
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	//Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runLoadDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	dst := inst.DstOperands.Operands[0]
	if state.BlockingMemoryOps && i.normalizeDirection(dst.Impl) != "Router" {
		finalPred := addrStruct.Pred
		state.PendingMemoryOp = &pendingMemoryOp{
			OpCode:  inst.OpCode,
			Address: addr,
			Dst:     append([]Operand(nil), inst.DstOperands.Operands...),
			Pred:    finalPred,
			IsWrite: false,
		}
		state.AddrBuf = addr
		state.IsToWriteMemory = false
		state.TimingWaitBlocked = true
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
		return map[Operand]cgra.Data{
			{Impl: "Router", Color: "R"}: cgra.NewScalarWithPred(addr, finalPred),
		}
	}
	if dst.Impl != "Router" {
		panic("the destination of a LOAD_DRAM instruction must be Router")
	}

	slog.Warn("DRAM",
		"Behavior", "LoadDRAM",
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	finalPred := addrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(addr, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	state.AddrBuf = addr
	state.IsToWriteMemory = false // not for write memory
	return results
}

func (i instEmulator) runLoadWaitDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a LOAD_WAIT_DRAM instruction must be Router")
	}
	valueStruct := i.readOperand(src, state)
	value := valueStruct.First()
	finalPred := valueStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runStoreDirect(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	valueStruct := i.readOperand(src1, state)
	value := valueStruct.First()
	src2 := inst.SrcOperands.Operands[1]
	addrStruct := i.readOperand(src2, state)
	addr := addrStruct.First()
	finalPred := addrStruct.Pred && valueStruct.Pred
	if !finalPred {
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
		return make(map[Operand]cgra.Data)
	}
	if addr >= uint32(len(state.Memory)) {
		panic("memory address out of bounds, addr: " + strconv.Itoa(int(addr)) + ", len(state.Memory): " + strconv.Itoa(len(state.Memory)))
	}
	slog.Warn("Memory",
		"Behavior", "StoreDirect",
		"Value", value,
		"Addr", addr,
		"X", state.TileX,
		"Y", state.TileY,
	)
	state.Memory[addr] = value
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runStoreDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	if state.BlockingMemoryOps && len(inst.DstOperands.Operands) == 0 {
		valueStruct := i.readOperand(inst.SrcOperands.Operands[0], state)
		value := valueStruct.First()
		addrStruct := i.readOperand(inst.SrcOperands.Operands[1], state)
		addr := addrStruct.First()
		finalPred := addrStruct.Pred && valueStruct.Pred
		state.PendingMemoryOp = &pendingMemoryOp{
			OpCode:  inst.OpCode,
			Address: addr,
			Value:   value,
			Pred:    finalPred,
			IsWrite: true,
		}
		state.AddrBuf = addr
		state.IsToWriteMemory = true
		state.TimingWaitBlocked = true
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
		return map[Operand]cgra.Data{
			{Impl: "Router", Color: "R"}: cgra.NewScalarWithPred(value, finalPred),
		}
	}

	src1 := inst.SrcOperands.Operands[0]
	addrStruct := i.readOperand(src1, state)
	addr := addrStruct.First()
	src2 := inst.SrcOperands.Operands[1]
	valueStruct := i.readOperand(src2, state)
	value := valueStruct.First()
	dst := inst.DstOperands.Operands[0]
	if dst.Impl != "Router" {
		panic("the destination of a STORE_DRAM instruction must be Router")
	}

	slog.Warn("DRAM",
		"Behavior", "StoreDRAM",
		"Addr", addr,
		"Value", value,
		"X", state.TileX,
		"Y", state.TileY,
	)
	finalPred := addrStruct.Pred && valueStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(value, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	state.AddrBuf = addr
	state.IsToWriteMemory = true // for write memory
	return results
}

func (i instEmulator) runStoreWaitDRAM(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	if src.Impl != "Router" {
		panic("the source of a STORE_WAIT_DRAM instruction must be Router")
	}
	srcStruct := i.readOperand(src, state) // do nothing, only get the write done
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runTrigger(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	// just consume a operand and do nothing
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	// elect no next PC
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) parseAndCompareI(inst Operation, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state).First()
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst.SrcOperands.Operands[1].Impl, 10, 32)
	if err != nil {
		panic("invalid compare number")
	}

	immeI32 := int32(uint32(imme))
	srcValI := int32(srcVal)

	conditionFuncs := map[string]func(int32, int32) bool{
		"EQ": func(a, b int32) bool { return a == b },
		"NE": func(a, b int32) bool { return a != b },
		"LE": func(a, b int32) bool { return a <= b },
		"LT": func(a, b int32) bool { return a < b },
		"GT": func(a, b int32) bool { return a > b },
		"GE": func(a, b int32) bool { return a >= b },
	}

	for key, function := range conditionFuncs {
		if strings.Contains(instruction, key) && function(srcValI, immeI32) {
			dstVal = 1
		}
	}
	i.writeOperand(dst, cgra.NewScalar(dstVal), state)
	// elect no next PC
}

func (i instEmulator) parseAndCompareF32(inst Operation, state *coreState) {
	instruction := inst.OpCode
	dst := inst.DstOperands.Operands[0]
	src := inst.SrcOperands.Operands[0]

	srcVal := i.readOperand(src, state).First()
	dstVal := uint32(0)
	imme, err := strconv.ParseUint(inst.SrcOperands.Operands[1].Impl, 10, 32)
	if err != nil {
		panic("invalid compare number")
	}

	conditionFuncsF := map[string]func(float32, float32) bool{
		"EQ": func(a, b float32) bool { return a == b },
		"NE": func(a, b float32) bool { return a != b },
		"LT": func(a, b float32) bool { return a < b },
		"LE": func(a, b float32) bool { return a <= b },
		"GT": func(a, b float32) bool { return a > b },
		"GE": func(a, b float32) bool { return a >= b },
	}

	immeF32 := math.Float32frombits(uint32(imme))
	srcValF := math.Float32frombits(srcVal)

	for key, function := range conditionFuncsF {
		if strings.Contains(instruction, key) && function(srcValF, immeF32) {
			dstVal = 1
		}
	}
	i.writeOperand(dst, cgra.NewScalar(dstVal), state)
	// elect no next PC
}

func (i instEmulator) runAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed + src2Signed
	dstVal := uint32(dstValSigned)
	finalPred := src1Struct.Pred && src2Struct.Pred
	//fmt.Printf("IADD: Adding %d (src1) + %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runSub(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed - src2Signed
	dstVal := uint32(dstValSigned)

	// fmt.Printf("ISUB: Subtracting %d (src1) - %d (src2) = %d\n", src1Signed, src2Signed, dstValSigned)
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runMul(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	// convert to signed integer for multiplication
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)
	dstValSigned := src1Signed * src2Signed
	dstVal := uint32(dstValSigned)
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Struct.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Struct.Pred), "Result", fmt.Sprintf("%d(%t)", dstVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runMulAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	// MUL_ADD: dst = src0 * src1 + src2 (systolic MAC: psum += activation * weight)
	src0 := inst.SrcOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[1]
	src2 := inst.SrcOperands.Operands[2]

	s0 := i.readOperand(src0, state)
	s1 := i.readOperand(src1, state)
	s2 := i.readOperand(src2, state)

	s0Val := int32(s0.First())
	s1Val := int32(s1.First())
	s2Val := int32(s2.First())
	dstValSigned := s0Val*s1Val + s2Val
	dstVal := uint32(dstValSigned)
	//finalPred := s0.Pred && s1.Pred && s2.Pred
	//Only for systolic array currently. if need for other cases, please modify the finalPred calculation.
	finalPred := s0.Pred && s1.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runDiv(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	// convert to signed integer for division
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	// avoid division by zero
	if src2Signed == 0 {
		panic("Division by zero at " + strconv.Itoa(int(state.PCInBlock)) + "@(" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ")")
	}

	dstValSigned := src1Signed / src2Signed
	dstVal := uint32(dstValSigned)
	finalPred := src1Struct.Pred && src2Struct.Pred

	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(dstVal, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// fmt.Printf("DIV Instruction, Data are %d and %d, Res is %d\n", src1Signed, src2Signed, dstValSigned)
	// elect no next PC
	return results
}

func (i instEmulator) runSHL(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal << shiftStrVal
	finalPred := srcStruct.Pred && shiftStrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("LLS: %s = %s << %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
	return results
}

func (i instEmulator) runLRS(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	shiftStr := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	shiftStrStruct := i.readOperand(shiftStr, state)
	shiftStrVal := shiftStrStruct.First()

	result := srcVal >> shiftStrVal
	finalPred := srcStruct.Pred && shiftStrStruct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("LRS: %s = %s >> %d => Result: %d\n", dst, src, shift, result)
	// elect no next PC
	return results
}

func (i instEmulator) runOR(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 | srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("OR: %s = %s | %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) runXOR(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 ^ srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("XOR: %s = %s ^ %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) runAND(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	srcVal1 := src1Struct.First()
	src2Struct := i.readOperand(src2, state)
	srcVal2 := src2Struct.First()
	result := srcVal1 & srcVal2
	finalPred := src1Struct.Pred && src2Struct.Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(result, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	//fmt.Printf("AND: %s = %s & %s => Result: %d\n", dst, src1, src2, result)
	// elect no next PC
	return results
}

func (i instEmulator) Jump(dst uint32, state *coreState) {
	state.NextPCInBlock = int32(dst)
}

func (i instEmulator) runJmp(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	i.Jump(srcVal, state)
	Trace("Inst", "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", srcStruct.Pred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBeq(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal == immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBne(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal != immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runBlt(inst Operation, state *coreState) map[Operand]cgra.Data {
	// not safe in new scenario
	src := inst.SrcOperands.Operands[0]
	imme := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	srcVal := srcStruct.First()
	immeStruct := i.readOperand(imme, state)
	immeVal := immeStruct.First()
	finalPred := srcStruct.Pred && immeStruct.Pred

	if srcVal < immeVal {
		i.Jump(srcVal, state)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runRetImm(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	var finalPred bool
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		srcStruct := i.readOperand(src, state)
		srcVal := srcStruct.First()
		srcPred := srcStruct.Pred
		finalPred = srcPred
		if srcPred {
			slog.Info("Control: Cond",
				"X", state.TileX,
				"Y", state.TileY,
				"SrcVal", srcVal,
			)
			*state.retVal = srcVal
			*state.exit = true
			*state.requestExitTimestamp = time
			// 	fmt.Println("++++++++++++ RETURN executed", srcVal, "T=", time)
			// } else {
			// 	fmt.Println("++++++++++++ RETURN bypassed")
		}
	} else {
		panic("RETURN_VALUE requires a source operand")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runRetDelay(inst Operation, state *coreState, time float64) map[Operand]cgra.Data {
	var finalPred bool
	if len(inst.SrcOperands.Operands) > 0 {
		src := inst.SrcOperands.Operands[0]
		srcStruct := i.readOperand(src, state)
		srcVal := srcStruct.First()
		srcPred := srcStruct.Pred
		finalPred = srcPred
		if srcPred {
			slog.Info("Control: Cond",
				"X", state.TileX,
				"Y", state.TileY,
				"SrcVal", srcVal,
			)
			*state.retVal = 0
			*state.exit = true
			*state.requestExitTimestamp = time + ExitDelay
			// 	fmt.Println("++++++++++++ RETURN executed", srcVal, "T=", time)
			// } else {
			// 	fmt.Println("++++++++++++ RETURN bypassed")
		}
	} else {
		panic("RETURN_VOID requires a source operand")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return make(map[Operand]cgra.Data)
}

func (i instEmulator) runFAdd(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float + src2Float

	resultUint := float2Uint(resultFloat)
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runFSub(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float - src2Float

	resultUint := float2Uint(resultFloat)
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runFMul(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Uint := src1Struct.First()
	src2Uint := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	src1Float := uint2Float(src1Uint)
	src2Float := uint2Float(src2Uint)

	resultFloat := src1Float * src2Float

	resultUint := float2Uint(resultFloat)
	finalPred := src1Pred && src2Pred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(resultUint, finalPred)
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runCmpExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Val := i.readOperand(src1, state)
	src2Val := i.readOperand(src2, state)

	resultVal := 0

	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if src1Val.First() == src2Val.First() && src1Val.Pred == src2Val.Pred {
		finalPred = src1Val.Pred
		resultVal = 1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
		// fmt.Println(">>>>>>>>>>>>>>> ICMP_EQ: ", src1Val.First(), src2Val.First(), "Yes")
	} else {
		finalPred = src1Val.Pred
		resultVal = 0
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
		// fmt.Println(">>>>>>>>>>>>>>> ICMP_EQ: ", src1Val.First(), src2Val.First(), "No")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val.First(), src1Val.Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val.First(), src2Val.Pred), "Result", fmt.Sprintf("%d(%t)", resultVal, finalPred))
	return results
}

func (i instEmulator) runSgtExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred

	//o
	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	results := make(map[Operand]cgra.Data)
	if src1Signed > src2Signed {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runLTExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred
	results := make(map[Operand]cgra.Data)
	if src1Val < src2Val {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runGTExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	finalPred := resultPred
	results := make(map[Operand]cgra.Data)
	if src1Val > src2Val {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, finalPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runSGEExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	src1Signed := int32(src1Val)
	src2Signed := int32(src2Val)

	resultVal := 0

	results := make(map[Operand]cgra.Data)
	if src1Signed >= src2Signed {
		resultVal = 1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, resultPred)
		}
	} else {
		resultVal = 0
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, resultPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Result", fmt.Sprintf("%d(%t)", resultVal, resultPred))
	return results
}

func (i instEmulator) runULTExport(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred
	resultPred := src1Pred && src2Pred

	resultVal := uint32(0)
	results := make(map[Operand]cgra.Data)
	if src1Val < src2Val {
		resultVal = 1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(1, resultPred)
		}
	} else {
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, resultPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Result", fmt.Sprintf("%d(%t)", resultVal, resultPred))
	return results
}

func (i instEmulator) runPhi(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if src1Struct.Pred && !src2Struct.Pred {
		finalPred = src1Struct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src1Struct.First(), finalPred)
		}
	} else if !src1Struct.Pred && src2Struct.Pred {
		finalPred = src2Struct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src2Struct.First(), finalPred)
		}
	} else if !src1Struct.Pred && !src2Struct.Pred {
		finalPred = false
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(0, finalPred)
		}
	} else {
		panic("Phi operation: both sources have the same predicate")
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runPhiConst(inst Operation, state *coreState) map[Operand]cgra.Data { // Possibly wrong
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]

	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	// Track PHI_CONST per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("PhiConst_%d", inst.ID)
	var result uint32
	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if state.States[stateKey] == nil || state.States[stateKey] == false {
		result = src1Val
		finalPred = src1Pred
		state.States[stateKey] = true
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
	} else {
		result = src2Val
		finalPred = src2Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	// elect no next PC
	return results
}

func (i instEmulator) runPhiStart(inst Operation, state *coreState) map[Operand]cgra.Data {
	src1 := inst.SrcOperands.Operands[0]
	src2 := inst.SrcOperands.Operands[1]
	src1Struct := i.readOperand(src1, state)
	src1Val := src1Struct.First()
	src1Pred := src1Struct.Pred

	// Track PHI_START per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("PhiStart_%d", inst.ID)
	var result uint32
	var finalPred bool
	results := make(map[Operand]cgra.Data)

	if state.States[stateKey] == nil || state.States[stateKey] == false { // first execution
		// if !src1Pred {
		// 	panic("Predicate of first time PHI_START must be true at (" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ") instruction " + strconv.Itoa(inst.ID))
		// }
		result = src1Val
		finalPred = src1Pred
		state.States[stateKey] = true
		// fmt.Println("set state.States[", stateKey, "] to true")
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src0(FirstTime)", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Result", fmt.Sprintf("%d(%t)", result, finalPred))
	} else {
		src2Struct := i.readOperand(src2, state) // only in normal path will consume src2
		src2Val := src2Struct.First()
		src2Pred := src2Struct.Pred
		if src1Pred && src2Pred {
			panic("Only one of the predicates of PHI_START can be true at (" + strconv.Itoa(int(state.TileX)) + "," + strconv.Itoa(int(state.TileY)) + ") instruction " + strconv.Itoa(inst.ID))
		}
		if src1Pred {
			result = src1Val
			finalPred = src1Pred
		} else { // src2Pred is true or both are false(arbitrary)
			result = src2Val
			finalPred = src2Pred
		}
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(result, finalPred)
		}
		Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Src0", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src1", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Result", fmt.Sprintf("%d(%t)", result, finalPred))
	}

	// elect no next PC
	return results
}

func (i instEmulator) runGrantPred(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]
	pred := inst.SrcOperands.Operands[1]

	srcStruct := i.readOperand(src, state)
	predStruct := i.readOperand(pred, state)
	srcVal := srcStruct.First()
	predVal := predStruct.First()
	srcPred := srcStruct.Pred
	predPred := predStruct.Pred

	resultPred := false

	if predVal == 0 {
		resultPred = false
	} else {
		resultPred = true
	}

	//fmt.Printf("GRANTPRED: srcVal = %d, predVal = %t at (%d, %d)\n", srcVal, predVal, state.TileX, state.TileY)
	finalPred := resultPred && predPred && srcPred
	results := make(map[Operand]cgra.Data)
	for _, dst := range inst.DstOperands.Operands {
		results[dst] = cgra.NewScalarWithPred(srcVal, finalPred)
	}

	// fmt.Println("<<<<<<<<<<<<<< GRANTPRED: ", srcVal, predVal, finalPred)

	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "SrcOperand", fmt.Sprintf("%d(%t)", srcVal, srcStruct.Pred), "PredOperand", fmt.Sprintf("%d(%t)", predVal, predStruct.Pred), "Pred", finalPred, "Result", fmt.Sprintf("%d(%t)", srcVal, finalPred))
	// elect no next PC
	return results
}

func (i instEmulator) runGrantOnce(inst Operation, state *coreState) map[Operand]cgra.Data {
	src := inst.SrcOperands.Operands[0]

	srcStruct := i.readOperand(src, state)
	// Track GRANT_ONCE per instruction to avoid cross-interference.
	stateKey := fmt.Sprintf("GrantOnce_%d", inst.ID)
	var finalPred bool
	results := make(map[Operand]cgra.Data)
	if state.States[stateKey] == nil || state.States[stateKey] == false {
		state.States[stateKey] = true
		finalPred = srcStruct.Pred
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(srcStruct.First(), finalPred)
		}
	} else {
		finalPred = false
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(srcStruct.First(), finalPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Pred", finalPred)
	return results
}

func (i instEmulator) runSel(inst Operation, state *coreState) map[Operand]cgra.Data {
	sel := inst.SrcOperands.Operands[0]
	src1 := inst.SrcOperands.Operands[1]
	src2 := inst.SrcOperands.Operands[2]

	selStruct := i.readOperand(sel, state)
	src1Struct := i.readOperand(src1, state)
	src2Struct := i.readOperand(src2, state)

	selVal := selStruct.First()
	src1Val := src1Struct.First()
	src2Val := src2Struct.First()

	selPred := selStruct.Pred
	src1Pred := src1Struct.Pred
	src2Pred := src2Struct.Pred

	resultPred := selPred && src1Pred && src2Pred

	if selVal != 0 && selVal != 1 {
		panic("Sel must be 0 or 1")
	}

	results := make(map[Operand]cgra.Data)
	if selVal == 0 { // if sel is 0, select src2
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src2Val, resultPred)
		}
	} else { // if sel is 1, select src1
		for _, dst := range inst.DstOperands.Operands {
			results[dst] = cgra.NewScalarWithPred(src1Val, resultPred)
		}
	}
	Trace("Inst", "Time", state.CurrentTime, "OpCode", inst.OpCode, "ID", inst.ID, "X", state.TileX, "Y", state.TileY, "Sel", fmt.Sprintf("%d(%t)", selVal, selPred), "Src1", fmt.Sprintf("%d(%t)", src1Val, src1Pred), "Src2", fmt.Sprintf("%d(%t)", src2Val, src2Pred), "Sel", fmt.Sprintf("%d(%t)", selVal, selPred))
	return results
}
