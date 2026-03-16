// Package runtimecfg loads and resolves simulator runtime settings from arch spec.
package runtimecfg

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ArchSpec captures the architecture and simulator settings from arch_spec.yaml.
// It intentionally keeps an inline map at each level to allow forward-compatible
// extension without changing callers.
type ArchSpec struct {
	CGRADefaults CGRADefaults   `yaml:"cgra_defaults"`
	Simulator    Simulator      `yaml:"simulator"`
	Extra        map[string]any `yaml:",inline"`
}

// CGRADefaults contains default CGRA shape settings from arch spec.
type CGRADefaults struct {
	Rows    int            `yaml:"rows"`
	Columns int            `yaml:"columns"`
	Extra   map[string]any `yaml:",inline"`
}

// Simulator contains simulator runtime settings from arch spec.
type Simulator struct {
	ExecutionModel  string           `yaml:"execution_model"`
	ExecutionPolicy string           `yaml:"execution_policy"`
	Logging         SimulatorLogging `yaml:"logging"`
	Driver          NamedComponent   `yaml:"driver"`
	Device          DeviceComponent  `yaml:"device"`
	Extra           map[string]any   `yaml:",inline"`
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
	Name      string         `yaml:"name"`
	Frequency string         `yaml:"frequency"`
	Extra     map[string]any `yaml:",inline"`
}

// DeviceComponent defines simulator device-specific settings.
type DeviceComponent struct {
	Name               string         `yaml:"name"`
	Frequency          string         `yaml:"frequency"`
	BindToArchitecture *bool          `yaml:"bind_to_architecture"`
	Extra              map[string]any `yaml:",inline"`
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
