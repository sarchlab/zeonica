package config

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

type bankedReadRespondEvent struct {
	*sim.EventBase
	req *mem.ReadReq
}

type bankedWriteRespondEvent struct {
	*sim.EventBase
	req *mem.WriteReq
}

type bankedSharedMemoryController struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort       sim.Port
	storage       *mem.Storage
	baseLatency   int
	banks         int
	interleave    uint64
	nextBankCycle []int64
}

type BankedSharedMemoryConfig struct {
	Banks           int
	BaseLatency     int
	InterleaveBytes uint64
	Capacity        uint64
}

func newBankedSharedMemoryController(
	name string,
	engine sim.Engine,
	freq sim.Freq,
	cfg BankedSharedMemoryConfig,
) *bankedSharedMemoryController {
	if cfg.Banks <= 0 {
		cfg.Banks = 1
	}
	if cfg.BaseLatency <= 0 {
		cfg.BaseLatency = 1
	}
	if cfg.InterleaveBytes == 0 {
		cfg.InterleaveBytes = 4
	}
	if cfg.Capacity == 0 {
		cfg.Capacity = 4 * mem.GB
	}

	c := &bankedSharedMemoryController{
		storage:       mem.NewStorage(cfg.Capacity),
		baseLatency:   cfg.BaseLatency,
		banks:         cfg.Banks,
		interleave:    cfg.InterleaveBytes,
		nextBankCycle: make([]int64, cfg.Banks),
	}
	c.TickingComponent = sim.NewTickingComponent(name, engine, freq, c)
	c.topPort = sim.NewPort(c, 16, 16, name+".TopPort")
	c.AddPort("Top", c.topPort)
	c.AddMiddleware(&bankedSharedMemoryMiddleware{bankedSharedMemoryController: c})
	return c
}

func (c *bankedSharedMemoryController) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

func (c *bankedSharedMemoryController) Handle(e sim.Event) error {
	switch e := e.(type) {
	case *bankedReadRespondEvent:
		return c.handleReadRespondEvent(e)
	case *bankedWriteRespondEvent:
		return c.handleWriteRespondEvent(e)
	case sim.TickEvent:
		return c.TickingComponent.Handle(e)
	default:
		log.Panicf("cannot handle event of %s", reflect.TypeOf(e))
	}
	return nil
}

func (c *bankedSharedMemoryController) WriteStorage(addr uint32, data []byte) error {
	return c.storage.Write(uint64(addr), data)
}

func (c *bankedSharedMemoryController) ReadStorage(addr uint32, size uint64) ([]byte, error) {
	return c.storage.Read(uint64(addr), size)
}

func (c *bankedSharedMemoryController) BankForAddress(addr uint64) int {
	return int((addr / c.interleave) % uint64(c.banks))
}

func (c *bankedSharedMemoryController) scheduleCycleForAddress(addr uint64) int64 {
	bank := c.BankForAddress(addr)
	nowCycle := int64(c.Freq.ThisTick(c.CurrentTime()) * sim.VTimeInSec(c.Freq))
	startCycle := nowCycle
	if c.nextBankCycle[bank] > startCycle {
		startCycle = c.nextBankCycle[bank]
	}
	doneCycle := startCycle + int64(c.baseLatency)
	c.nextBankCycle[bank] = doneCycle
	return doneCycle
}

func (c *bankedSharedMemoryController) timeForCycle(cycle int64) sim.VTimeInSec {
	return sim.VTimeInSec(float64(cycle) / float64(c.Freq))
}

func (c *bankedSharedMemoryController) CurrentTime() sim.VTimeInSec {
	return c.Engine.CurrentTime()
}

func (c *bankedSharedMemoryController) handleReadRespondEvent(e *bankedReadRespondEvent) error {
	req := e.req
	data, err := c.storage.Read(req.Address, req.AccessByteSize)
	if err != nil {
		log.Panic(err)
	}

	rsp := mem.DataReadyRspBuilder{}.
		WithSrc(c.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithData(data).
		Build()
	if sendErr := c.topPort.Send(rsp); sendErr != nil {
		c.Engine.Schedule(&bankedReadRespondEvent{EventBase: sim.NewEventBase(c.Freq.NextTick(e.Time()), c), req: req})
		return nil
	}
	tracing.TraceReqComplete(req, c)
	c.TickLater()
	return nil
}

func (c *bankedSharedMemoryController) handleWriteRespondEvent(e *bankedWriteRespondEvent) error {
	req := e.req
	if req.DirtyMask == nil {
		if err := c.storage.Write(req.Address, req.Data); err != nil {
			log.Panic(err)
		}
	} else {
		data, err := c.storage.Read(req.Address, uint64(len(req.Data)))
		if err != nil {
			log.Panic(err)
		}
		for i := range req.Data {
			if req.DirtyMask[i] {
				data[i] = req.Data[i]
			}
		}
		if err := c.storage.Write(req.Address, data); err != nil {
			log.Panic(err)
		}
	}

	rsp := mem.WriteDoneRspBuilder{}.
		WithSrc(c.topPort.AsRemote()).
		WithDst(req.Src).
		WithRspTo(req.ID).
		Build()
	if sendErr := c.topPort.Send(rsp); sendErr != nil {
		c.Engine.Schedule(&bankedWriteRespondEvent{EventBase: sim.NewEventBase(c.Freq.NextTick(e.Time()), c), req: req})
		return nil
	}
	tracing.TraceReqComplete(req, c)
	c.TickLater()
	return nil
}

type bankedSharedMemoryMiddleware struct {
	*bankedSharedMemoryController
}

func (m *bankedSharedMemoryMiddleware) Tick() bool {
	msg := m.topPort.RetrieveIncoming()
	if msg == nil {
		return false
	}
	tracing.TraceReqReceive(msg, m.bankedSharedMemoryController)

	switch msg := msg.(type) {
	case *mem.ReadReq:
		doneCycle := m.scheduleCycleForAddress(msg.Address)
		m.Engine.Schedule(&bankedReadRespondEvent{
			EventBase: sim.NewEventBase(m.timeForCycle(doneCycle), m.bankedSharedMemoryController),
			req:       msg,
		})
	case *mem.WriteReq:
		doneCycle := m.scheduleCycleForAddress(msg.Address)
		m.Engine.Schedule(&bankedWriteRespondEvent{
			EventBase: sim.NewEventBase(m.timeForCycle(doneCycle), m.bankedSharedMemoryController),
			req:       msg,
		})
	default:
		log.Panicf("cannot handle request of type %s", reflect.TypeOf(msg))
	}
	return true
}
