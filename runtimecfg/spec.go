// Package runtimecfg loads and resolves simulator runtime settings from arch spec.
package runtimecfg

import (
	"fmt"
	"os"

	"github.com/sarchlab/zeonica/core"
	"gopkg.in/yaml.v3"
)

// ArchSpec captures the architecture and simulator settings from arch_spec.yaml.
// It intentionally keeps an inline map at each level to allow forward-compatible
// extension without changing callers.
type ArchSpec struct {
	CGRADefaults CGRADefaults   `yaml:"cgra_defaults"`
	TileDefaults TileDefaults   `yaml:"tile_defaults"`
	LinkDefaults LinkDefaults   `yaml:"link_defaults"`
	Simulator    Simulator      `yaml:"simulator"`
	Extra        map[string]any `yaml:",inline"`
}

// CGRADefaults contains default CGRA shape settings from arch spec.
type CGRADefaults struct {
	Rows    int            `yaml:"rows"`
	Columns int            `yaml:"columns"`
	Extra   map[string]any `yaml:",inline"`
}

// TileDefaults defines default per-tile microarchitecture parameters.
type TileDefaults struct {
	NumRegisters     int            `yaml:"num_registers"`
	LocalMemoryWords int            `yaml:"local_memory_words"`
	Extra            map[string]any `yaml:",inline"`
}

// LinkDefaults captures inter-tile link metadata. This release parses and validates
// these fields, but does not feed them into cycle-accurate link timing yet.
type LinkDefaults struct {
	Latency   *int           `yaml:"latency"`
	Bandwidth *int           `yaml:"bandwidth"`
	Extra     map[string]any `yaml:",inline"`
}

// Simulator contains simulator runtime settings from arch spec.
type Simulator struct {
	ExecutionModel        string                `yaml:"execution_model"`
	ExecutionPolicy       string                `yaml:"execution_policy"`
	EnableFIFOModel       *bool                 `yaml:"enable_fifo_model"`
	EnableQueueWatches    *bool                 `yaml:"enable_queue_watches"`
	ProgramYAML           string                `yaml:"program_yaml"`
	ReportName            string                `yaml:"report_name"`
	QueueWatches          []core.QueueWatchSpec `yaml:"queue_watches"`
	BufferSweepDepths     []int                 `yaml:"buffer_sweep_depths"`
	StrictMaxSlip         *int64                `yaml:"strict_max_slip"`
	StrictFailOnViolation *bool                 `yaml:"strict_fail_on_violation"`
	Logging               SimulatorLogging      `yaml:"logging"`
	Driver                NamedComponent        `yaml:"driver"`
	Device                DeviceComponent       `yaml:"device"`
	Extra                 map[string]any        `yaml:",inline"`
}

// SimulatorLogging configures trace logging behavior.
type SimulatorLogging struct {
	Enabled     *bool          `yaml:"enabled"`
	EnableTrace *bool          `yaml:"enableTrace"`
	File        string         `yaml:"file"`
	Extra       map[string]any `yaml:",inline"`
}

// NamedComponent contains shared component naming/frequency fields.
type NamedComponent struct {
	Name                    string         `yaml:"name"`
	Frequency               string         `yaml:"frequency"`
	PortIncomingBufferDepth *int           `yaml:"port_incoming_buffer_depth"`
	PortOutgoingBufferDepth *int           `yaml:"port_outgoing_buffer_depth"`
	Extra                   map[string]any `yaml:",inline"`
}

// MemoryShareEntry maps one tile coordinate to a shared-memory controller group.
type MemoryShareEntry struct {
	TileX int            `yaml:"tile_x"`
	TileY int            `yaml:"tile_y"`
	Group int            `yaml:"group"`
	Extra map[string]any `yaml:",inline"`
}

// DeviceComponent defines simulator device-specific settings.
type DeviceComponent struct {
	Name                    string             `yaml:"name"`
	Frequency               string             `yaml:"frequency"`
	BindToArchitecture      *bool              `yaml:"bind_to_architecture"`
	MemoryMode              string             `yaml:"memory_mode"`
	MemoryShare             []MemoryShareEntry `yaml:"memory_share"`
	PortIncomingBufferDepth *int               `yaml:"port_incoming_buffer_depth"`
	PortOutgoingBufferDepth *int               `yaml:"port_outgoing_buffer_depth"`
	Extra                   map[string]any     `yaml:",inline"`
}

// Load reads and parses an architecture spec YAML file.
func Load(path string) (ArchSpec, error) {
	if path == "" {
		return ArchSpec{}, fmt.Errorf("arch spec path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ArchSpec{}, fmt.Errorf("read arch spec: %w", err)
	}

	var spec ArchSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return ArchSpec{}, fmt.Errorf("parse arch spec: %w", err)
	}

	return spec, nil
}
