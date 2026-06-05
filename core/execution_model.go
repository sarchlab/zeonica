package core

import (
	"fmt"
	"strings"
)

const LegacyCGRAPEExecutionModel = "legacy_cgra_pe"

// CoreExecutionModel controls how one core advances during a simulator tick.
type CoreExecutionModel interface {
	Name() string
	Reset(c *Core)
	Tick(c *Core) bool
}

type coreExecutionModelFactory func() CoreExecutionModel

var coreExecutionModelRegistry = map[string]coreExecutionModelFactory{
	LegacyCGRAPEExecutionModel: func() CoreExecutionModel {
		return legacyCGRAPEExecutionModel{}
	},
}

func normalizeCoreExecutionModelName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return LegacyCGRAPEExecutionModel
	}
	return normalized
}

func registerCoreExecutionModel(name string, factory coreExecutionModelFactory) {
	normalized := normalizeCoreExecutionModelName(name)
	if factory == nil {
		panic("core execution model factory must not be nil")
	}
	coreExecutionModelRegistry[normalized] = factory
}

// NormalizeCoreExecutionModelName resolves and validates a core execution model name.
func NormalizeCoreExecutionModelName(name string) (string, error) {
	normalized := normalizeCoreExecutionModelName(name)
	if _, ok := coreExecutionModelRegistry[normalized]; !ok {
		return "", fmt.Errorf("unsupported core_execution_model %q (supported: %s)", name, LegacyCGRAPEExecutionModel)
	}
	return normalized, nil
}

func newCoreExecutionModel(name string) CoreExecutionModel {
	normalized, err := NormalizeCoreExecutionModelName(name)
	if err != nil {
		panic(err)
	}
	return coreExecutionModelRegistry[normalized]()
}

type legacyCGRAPEExecutionModel struct{}

func (legacyCGRAPEExecutionModel) Name() string {
	return LegacyCGRAPEExecutionModel
}

func (legacyCGRAPEExecutionModel) Reset(c *Core) {}

func (legacyCGRAPEExecutionModel) Tick(c *Core) bool {
	madeProgress := false
	madeProgress = c.doRecv() || madeProgress
	madeProgress = c.runProgram() || madeProgress
	madeProgress = c.doSend() || madeProgress
	c.state.observeWatchedQueues(float64(c.Engine.CurrentTime() * 1e9))
	c.state.CurrentCycle++
	return madeProgress
}
