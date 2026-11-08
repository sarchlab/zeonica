package core

// Circular buffers (TT-Metal CBs) on top of the existing port receive queues.
//
// A CB is the FIFO receive queue of one (direction, color) port of the
// consumer PE, with its own capacity (SetRecvQueueCapacity). Producers push
// tiles with ordinary port writes and block on the existing output/backpressure
// path when the CB is full. Consumers use two block-level instructions and an
// indexed operand on the same queue:
//
//	CB_WAIT  West, #n   wait until n tiles are in the queue (cb_wait_front)
//	West@i              read tile i without removing it (block reuse)
//	CB_POP   West, #n   remove n tiles, freeing capacity (cb_pop_front);
//	                    destination operands receive n credits for the producer
//
// The producer side uses credits, the way TT CBs use the consumer's ack counter:
//
//	CB_RESERVE East, [NorthWest,] #n   wait until n credits arrived on the given
//	                                   credit ports, then consume them (cb_reserve_back)
//
// Credits are scalar tokens (the count) on ordinary ports; the host injects the
// initial free space. A CB shared by two consumers (a pool) lists both ports.

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// SetRecvQueueCapacity overrides the receive-queue depth of one port/color,
// e.g. to size the queue as a circular buffer.
func (c *Core) SetRecvQueueCapacity(side cgra.Side, color string, capacity int) {
	if c.state.RecvQueueCaps == nil {
		c.state.RecvQueueCaps = map[int]int{}
	}
	c.state.RecvQueueCaps[recvCapKey(c.emu.getColorIndex(color), int(side))] = capacity
}

func recvCapKey(color, direction int) int { return color*64 + direction }

// parseIndexedPort splits "West@3" into ("West", 3).
func (i instEmulator) parseIndexedPort(impl string, state *coreState) (string, int, bool) {
	at := strings.IndexByte(impl, '@')
	if at < 0 {
		return "", 0, false
	}
	dir := i.normalizeDirection(impl[:at])
	if !state.Directions[dir] {
		return "", 0, false
	}
	idx, err := strconv.Atoi(impl[at+1:])
	if err != nil {
		panic(fmt.Sprintf("invalid indexed port operand %q", impl))
	}
	return dir, idx, true
}

func (s *coreState) recvQueueAt(color, direction, idx int) (cgra.Data, bool) {
	if s.recvFIFOEnabled() && len(s.RecvBufQueue[color]) > direction && idx < len(s.RecvBufQueue[color][direction]) {
		return s.RecvBufQueue[color][direction][idx], true
	}
	return cgra.Data{}, false
}

func isCBBlockOp(op string) bool { return op == "CB_WAIT" || op == "CB_POP" }

// creditPorts returns the (color, direction) queues of a CB_RESERVE and the
// number of credits it needs.
func (i instEmulator) creditPorts(inst Operation) ([][2]int, int) {
	srcs := inst.SrcOperands.Operands
	ports := make([][2]int, 0, len(srcs)-1)
	for _, o := range srcs[:len(srcs)-1] {
		ports = append(ports, [2]int{i.getColorIndex(o.Color), i.getDirecIndex(i.normalizeDirection(o.Impl))})
	}
	return ports, ttImmediate(srcs[len(srcs)-1])
}

func (s *coreState) creditsAvailable(ports [][2]int) int {
	total := 0
	for _, p := range ports {
		for k := 0; k < s.recvQueueLen(p[0], p[1]); k++ {
			v, _ := s.recvQueueAt(p[0], p[1], k)
			total += int(v.First())
		}
	}
	return total
}

// cbReadiness handles CB_WAIT/CB_POP and indexed reads. It returns handled=true
// when it decided readiness; otherwise the generic checks continue.
func (i instEmulator) cbReadiness(inst Operation, state *coreState) (bool, issueReadiness) {
	r := issueReadiness{OperandsReady: true, PredicateReadyOrTrue: true, ResourcesAvailable: true, Ready: true}
	waitOperand := func() (bool, issueReadiness) {
		r.OperandsReady, r.Ready, r.WaitReason = false, false, StallReasonOperandWait
		if (inst.OpCode == "CB_RESERVE" || isCBBlockOp(inst.OpCode)) && state.TT.WaitSince < 0 {
			state.TT.WaitSince = state.TT.Now
		}
		return true, r
	}
	if inst.OpCode == "CB_RESERVE" {
		ports, n := i.creditPorts(inst)
		if state.creditsAvailable(ports) < n {
			return waitOperand()
		}
		return true, r
	}
	if isCBBlockOp(inst.OpCode) {
		port := inst.SrcOperands.Operands[0]
		n := ttImmediate(inst.SrcOperands.Operands[1])
		dir := i.normalizeDirection(port.Impl)
		if state.recvQueueLen(i.getColorIndex(port.Color), i.getDirecIndex(dir)) < n {
			return waitOperand()
		}
		// Enough tiles: the generic checks handle the credit-return destinations.
		return false, r
	}
	for _, src := range inst.SrcOperands.Operands {
		if dir, idx, ok := i.parseIndexedPort(src.Impl, state); ok {
			if state.recvQueueLen(i.getColorIndex(src.Color), i.getDirecIndex(dir)) <= idx {
				return waitOperand()
			}
		}
	}
	return false, r
}

func (i instEmulator) runCBWait(inst Operation, state *coreState) map[Operand]cgra.Data {
	i.observeCB(inst, state, ttImmediate(inst.SrcOperands.Operands[1]))
	return nil
}

// ttRunState is per-core TT state that must survive the issue logic's
// evaluation clones (cloneForEval copies the pointer).
type ttRunState struct {
	Now             float64 // current cycle
	WaitSince       float64 // cycle a CB operation started waiting, -1 if not waiting
	LastWaitBlocked bool    // a CB wait/reserve blocked since the last CB_POP or TT_RESUME_WORK
	ResumeCharged   bool    // the pending TT_RESUME_WORK is paid
}

// TTCBEvent is one executed CB operation (instrumentation; see TTCBObserver).
type TTCBEvent struct {
	X, Y        int
	Op          string // CB_WAIT, CB_POP, CB_RESERVE
	Port, Color string // first port operand
	Tiles       int
	Start, End  float64 // Start < End when the operation had to wait
}

// TTCBObserver, when set, receives every executed CB operation.
var TTCBObserver func(TTCBEvent)

// observeCB reports a CB operation and records whether it had to wait (for
// TT_RESUME_WORK: a wait since the last release).
func (i instEmulator) observeCB(inst Operation, state *coreState, tiles int) {
	start := state.TT.Now
	if inst.OpCode == "CB_POP" {
		state.TT.LastWaitBlocked = false // a release ends the previous wait phase
	}
	if state.TT.WaitSince >= 0 {
		start = state.TT.WaitSince
		state.TT.LastWaitBlocked = true
	}
	state.TT.WaitSince = -1
	if TTCBObserver != nil {
		port := inst.SrcOperands.Operands[0]
		TTCBObserver(TTCBEvent{X: int(state.TileX), Y: int(state.TileY), Op: inst.OpCode, Port: i.normalizeDirection(port.Impl),
			Color: port.Color, Tiles: tiles, Start: start, End: state.TT.Now})
	}
}

func (i instEmulator) runCBPop(inst Operation, state *coreState) map[Operand]cgra.Data {
	port := inst.SrcOperands.Operands[0]
	color, dir := i.getColorIndex(port.Color), i.getDirecIndex(i.normalizeDirection(port.Impl))
	n := ttImmediate(inst.SrcOperands.Operands[1])
	i.observeCB(inst, state, n)
	for k := n; k > 0; k-- {
		if _, ok := state.recvQueueConsume(color, dir); !ok {
			panic(fmt.Sprintf("CB_POP on empty queue %s at (%d,%d)", port.Impl, state.TileX, state.TileY))
		}
	}
	return ttAllDst(inst, cgra.NewScalar(uint32(n)))
}

func (i instEmulator) runCBReserve(inst Operation, state *coreState) map[Operand]cgra.Data {
	ports, need := i.creditPorts(inst)
	i.observeCB(inst, state, need)
	for _, p := range ports {
		for need > 0 && state.recvQueueLen(p[0], p[1]) > 0 {
			head := state.RecvBufQueue[p[0]][p[1]][0]
			if v := int(head.First()); v > need {
				state.RecvBufQueue[p[0]][p[1]][0] = cgra.NewScalar(uint32(v - need))
				need = 0
			} else {
				state.recvQueueConsume(p[0], p[1])
				need -= v
			}
		}
	}
	if need > 0 {
		panic(fmt.Sprintf("CB_RESERVE short of %d credits at (%d,%d)", need, state.TileX, state.TileY))
	}
	return nil
}

// Progress work (TT-Sim's progress clock). TT_PROGRESS_WORK is a deferred
// operation whose remaining cycles also shrink by the progress credits that
// arrive on the PE's progress ports while it is pending; credits arriving at
// any other time are dropped. TT_PROGRESS_CREDIT #steps sends such a credit.
func (c *Core) SetProgressCreditPort(side cgra.Side, color string) {
	c.state.ProgressCreditPorts = append(c.state.ProgressCreditPorts, [2]int{c.emu.getColorIndex(color), int(side)})
}

// TT_RESUME_WORK is deferred work that is only paid when a CB wait or reserve
// blocked since the last CB_POP (TT-Sim charges the compute kernel for resuming
// after it stalled on its reader); otherwise it completes in one cycle.
func (c *Core) applyProgressCredits() {
	s := &c.state
	s.TT.Now = float64(c.Engine.CurrentTime() * 1e9)
	if g := s.PendingSyncGroup; g != nil && g.RepresentativeOp == "TT_RESUME_WORK" && !s.TT.ResumeCharged {
		g.RemainingCycles = 1
	}
	for _, p := range s.ProgressCreditPorts {
		for s.recvQueueLen(p[0], p[1]) > 0 {
			v, _ := s.recvQueueConsume(p[0], p[1])
			g := s.PendingSyncGroup
			if g != nil && strings.HasPrefix(g.RepresentativeOp, "TT_PROGRESS_WORK") {
				g.RemainingCycles -= int(v.First())
				if g.RemainingCycles < 1 {
					g.RemainingCycles = 1
				}
			}
		}
	}
}

func (i instEmulator) runTTResume(inst Operation, state *coreState) map[Operand]cgra.Data {
	state.TT.ResumeCharged, state.TT.LastWaitBlocked = state.TT.LastWaitBlocked, false
	return nil
}

func (i instEmulator) runTTProgressCredit(inst Operation, state *coreState) map[Operand]cgra.Data {
	return ttAllDst(inst, cgra.NewScalar(uint32(ttImmediate(inst.SrcOperands.Operands[0]))))
}

func ttImmediate(o Operand) int {
	v, err := strconv.ParseInt(strings.TrimPrefix(o.Impl, "#"), 0, 64)
	if err != nil {
		panic(fmt.Sprintf("expected immediate, got %q", o.Impl))
	}
	return int(v)
}

// QueueSummary describes the PC, stall reason and non-empty receive/send
// queues of a core (for diagnosing a stalled program).
func (c *Core) QueueSummary() string {
	s := c.state
	out := fmt.Sprintf("pc=%d stall=%s/%s", s.PCInBlock, s.StallReason, s.StallOpCode)
	for color := range s.RecvBufQueue {
		for dir := range s.RecvBufQueue[color] {
			if n := len(s.RecvBufQueue[color][dir]); n > 0 {
				out += fmt.Sprintf(" recv[c%d,%s]=%d/%d", color, cgra.Side(dir).Name(), n, s.recvQueueCap(color, dir))
			}
		}
	}
	for color := range s.SendBufQueue {
		for dir := range s.SendBufQueue[color] {
			if n := len(s.SendBufQueue[color][dir]); n > 0 {
				out += fmt.Sprintf(" send[c%d,%s]=%d", color, cgra.Side(dir).Name(), n)
			}
		}
	}
	return out
}

// RecvQueueContents returns a copy of one receive queue (instrumentation).
func (c *Core) RecvQueueContents(side cgra.Side, color string) []cgra.Data {
	ci, d := c.emu.getColorIndex(color), int(side)
	if !c.state.recvFIFOEnabled() || len(c.state.RecvBufQueue[ci]) <= d {
		return nil
	}
	return append([]cgra.Data(nil), c.state.RecvBufQueue[ci][d]...)
}
