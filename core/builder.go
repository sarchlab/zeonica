package core

import (
	"os"
	"strings"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

// Builder can create new cores.
type Builder struct {
	engine                sim.Engine
	freq                  sim.Freq
	exitAddr              *bool
	retValAddr            *uint32
	exitReqAddr           *float64
	executionPolicy       string
	strictMaxSlip         int64
	strictFailOnViolation bool
	portIncomingBufferCap int
	portOutgoingBufferCap int
	enableFIFOModel       bool
	enableQueueWatches    bool
	queueWatches          []QueueWatchSpec
	numRegisters          int
	localMemoryWords      int
	enableVectorPE        bool
	vectorLanes           int
	blockingMemoryOps     bool
	sharedMemoryBase      uint32
}

// WithEngine sets the engine.
func (b Builder) WithEngine(engine sim.Engine) Builder {
	b.engine = engine
	return b
}

// WithFreq sets the frequency of the core.
func (b Builder) WithFreq(freq sim.Freq) Builder {
	b.freq = freq
	return b
}

// WithSharedMemoryBase sets a word-address offset for shared-memory requests.
func (b Builder) WithSharedMemoryBase(base uint32) Builder {
	b.sharedMemoryBase = base
	return b
}

// WithExitAddr sets the shared exit flag address.
func (b Builder) WithExitAddr(exitAddr *bool) Builder {
	b.exitAddr = exitAddr
	return b
}

// WithRetValAddr sets the shared return value address.
func (b Builder) WithRetValAddr(retValAddr *uint32) Builder {
	b.retValAddr = retValAddr
	return b
}

func (b Builder) WithExitReqAddr(exitReqAddr *float64) Builder {
	b.exitReqAddr = exitReqAddr
	return b
}

// WithExecutionPolicy sets the execution policy for issue-time gating.
func (b Builder) WithExecutionPolicy(policy string) Builder {
	b.executionPolicy = policy
	return b
}

// WithStrictTimingConfig sets strict timing replay controls.
func (b Builder) WithStrictTimingConfig(maxSlip int64, failOnViolation bool) Builder {
	b.strictMaxSlip = maxSlip
	b.strictFailOnViolation = failOnViolation
	return b
}

// WithPortBufferDepth configures each core port incoming/outgoing capacity.
func (b Builder) WithPortBufferDepth(incoming, outgoing int) Builder {
	b.portIncomingBufferCap = incoming
	b.portOutgoingBufferCap = outgoing
	return b
}

// WithEnableFIFOModel toggles FIFO-based execution behavior.
func (b Builder) WithEnableFIFOModel(enabled bool) Builder {
	b.enableFIFOModel = enabled
	return b
}

// WithEnableQueueWatches toggles optional queue-occupancy instrumentation.
func (b Builder) WithEnableQueueWatches(enabled bool) Builder {
	b.enableQueueWatches = enabled
	return b
}

// WithQueueWatches sets optional queue watch definitions for occupancy instrumentation.
func (b Builder) WithQueueWatches(queueWatches []QueueWatchSpec) Builder {
	if len(queueWatches) == 0 {
		b.queueWatches = nil
		return b
	}
	b.queueWatches = append([]QueueWatchSpec(nil), queueWatches...)
	return b
}

// WithRegisterCount configures register-file size per core.
func (b Builder) WithRegisterCount(num int) Builder {
	b.numRegisters = num
	return b
}

// WithLocalMemoryWords configures local memory size (in words) per core.
func (b Builder) WithLocalMemoryWords(words int) Builder {
	b.localMemoryWords = words
	return b
}

// WithVectorConfig configures optional vector PE execution.
func (b Builder) WithVectorConfig(enabled bool, lanes int) Builder {
	b.enableVectorPE = enabled
	b.vectorLanes = lanes
	return b
}

func (b Builder) WithBlockingMemoryOps(enabled bool) Builder {
	b.blockingMemoryOps = enabled
	return b
}

func readyHeldTraceEnabledFromEnv() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("ZEONICA_TRACE_READY_HELD")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// Build creates a core.
//
//nolint:funlen
func (b Builder) Build(name string) *Core {
	c := &Core{}

	incomingBufCap := b.portIncomingBufferCap
	if incomingBufCap <= 0 {
		incomingBufCap = 1
	}
	outgoingBufCap := b.portOutgoingBufferCap
	if outgoingBufCap <= 0 {
		outgoingBufCap = 1
	}
	registerCount := b.numRegisters
	if registerCount <= 0 {
		registerCount = 64
	}
	localMemoryWords := b.localMemoryWords
	if localMemoryWords <= 0 {
		localMemoryWords = 1024
	}
	vectorLanes := b.vectorLanes
	if !b.enableVectorPE || vectorLanes <= 0 {
		vectorLanes = 1
	}
	resolvedQueueWatches, err := resolveQueueWatchSpecs(b.queueWatches)
	if err != nil {
		panic(err)
	}

	c.TickingComponent = sim.NewTickingComponent(name, b.engine, b.freq, c)
	c.emu = instEmulator{
		CareFlags:             true,
		ExecutionPolicy:       normalizeExecutionPolicyString(b.executionPolicy),
		StrictMaxSlip:         b.strictMaxSlip,
		StrictFailOnViolation: b.strictFailOnViolation,
	}
	c.state = coreState{
		exit:                 b.exitAddr,
		retVal:               b.retValAddr,
		requestExitTimestamp: b.exitReqAddr,
		SelectedBlock:        nil,
		PCInBlock:            -1,
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
		Registers:              make([]cgra.Data, registerCount),
		Memory:                 make([]uint32, localMemoryWords),
		RecvBufHead:            make([][]cgra.Data, 4),
		RecvBufHeadReady:       make([][]bool, 4),
		SendBufHead:            make([][]cgra.Data, 4),
		SendBufHeadBusy:        make([][]bool, 4),
		RecvBufQueue:           make([][][]cgra.Data, 4),
		SendBufQueue:           make([][][]cgra.Data, 4),
		HostDrainDirections:    make(map[int]bool),
		RecvQueueCapacity:      incomingBufCap,
		SendQueueCapacity:      outgoingBufCap,
		EnableFIFOModel:        b.enableFIFOModel,
		EnableVectorPE:         b.enableVectorPE,
		VectorLanes:            vectorLanes,
		EnableQueueWatches:     b.enableQueueWatches,
		ConfiguredQueueWatches: cloneQueueWatches(resolvedQueueWatches),
		OpInputReadCache:       make(map[string]cgra.Data),
		AddrBuf:                0,
		SharedMemoryBase:       b.sharedMemoryBase,
		IsToWriteMemory:        false,
		BlockingMemoryOps:      b.blockingMemoryOps,
		States:                 make(map[string]interface{}),
		Mode:                   SyncOp,
		CurrentCycle:           0,
		OpTimingCursor:         make(map[int]int),
		OpTimingLate:           make(map[int]bool),
		OpTimingRollCycle:      make(map[int]int64),
		OpIssueCount:           make(map[int]int),
		ReadyHeldTraceEnabled:  readyHeldTraceEnabledFromEnv(),
		ReadyHeldRunMode:       strings.TrimSpace(os.Getenv("ZEONICA_READY_HELD_RUN_MODE")),
		TimingWaitBlocked:      false,
		StallReason:            "",
		StallOpID:              0,
		StallOpCode:            "",
		CurrReservationState: ReservationState{
			ReservationMap:  make(map[int]bool),
			OpToExec:        0,
			RefCountRuntime: make(map[string]int),
		},
	}

	for i := 0; i < 4; i++ {
		c.state.RecvBufHead[i] = make([]cgra.Data, 12)
		c.state.RecvBufHeadReady[i] = make([]bool, 12)
		c.state.SendBufHead[i] = make([]cgra.Data, 12)
		c.state.SendBufHeadBusy[i] = make([]bool, 12)
		c.state.RecvBufQueue[i] = make([][]cgra.Data, 12)
		c.state.SendBufQueue[i] = make([][]cgra.Data, 12)
		for direction := 0; direction < 12; direction++ {
			c.state.RecvBufQueue[i][direction] = make([]cgra.Data, 0, incomingBufCap)
			c.state.SendBufQueue[i][direction] = make([]cgra.Data, 0, outgoingBufCap)
		}
	}

	c.ports = make(map[cgra.Side]*portPair)

	b.makePort(c, cgra.North, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.West, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.South, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.East, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.NorthEast, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.SouthEast, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.SouthWest, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.NorthWest, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.Router, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.Dummy1, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.Dummy2, incomingBufCap, outgoingBufCap)
	b.makePort(c, cgra.Dummy3, incomingBufCap, outgoingBufCap)

	return c
}

func (b *Builder) makePort(c *Core, side cgra.Side, incomingBufCap, outgoingBufCap int) {
	localPort := sim.NewPort(c, incomingBufCap, outgoingBufCap, c.Name()+"."+side.Name())
	c.ports[side] = &portPair{
		local: localPort,
	}
	c.AddPort(side.Name(), localPort)
}
