package runtimecfg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/zeonica/report"
)

func TestResolveEnergyModelInline(t *testing.T) {
	enabled := true
	spec := ArchSpec{
		Energy: EnergySpec{
			Enabled:             &enabled,
			Units:               "pJ",
			UnknownActionPolicy: report.EnergyUnknownActionWarn,
			Actions: map[string]float64{
				"pe.inst.ADD": 1.5,
			},
		},
	}
	cfg, err := Resolve(spec, "energy-inline")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.EnergyModel == nil {
		t.Fatal("expected energy model")
	}
	if cfg.EnergyModel.Actions["pe.inst.ADD"] != 1.5 {
		t.Fatalf("ADD energy = %v, want 1.5", cfg.EnergyModel.Actions["pe.inst.ADD"])
	}
	if cfg.EnergyModel.UnknownActionPolicy != report.EnergyUnknownActionWarn {
		t.Fatalf("unknown policy = %q, want warn", cfg.EnergyModel.UnknownActionPolicy)
	}
}

func TestResolveEnergyModelFileAndInlineOverride(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "energy.yaml")
	if err := os.WriteFile(modelPath, []byte(`
units: pJ
unknown_action_policy: warn
actions:
  pe.inst.ADD: 1
  pe.inst.MUL: 2
`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	enabled := true
	spec := ArchSpec{
		Energy: EnergySpec{
			Enabled:   &enabled,
			ModelFile: modelPath,
			Actions: map[string]float64{
				"pe.inst.ADD": 3,
			},
		},
	}
	cfg, err := Resolve(spec, "energy-file")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.EnergyModel.Actions["pe.inst.ADD"] != 3 {
		t.Fatalf("ADD energy = %v, want inline override 3", cfg.EnergyModel.Actions["pe.inst.ADD"])
	}
	if cfg.EnergyModel.Actions["pe.inst.MUL"] != 2 {
		t.Fatalf("MUL energy = %v, want model file value 2", cfg.EnergyModel.Actions["pe.inst.MUL"])
	}
	if cfg.EnergyModel.UnknownActionPolicy != report.EnergyUnknownActionWarn {
		t.Fatalf("unknown policy = %q, want model file value warn", cfg.EnergyModel.UnknownActionPolicy)
	}
}

func TestResolveEnergyModelInlinePolicyOverridesModelFile(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "energy.yaml")
	if err := os.WriteFile(modelPath, []byte(`
units: pJ
unknown_action_policy: warn
actions:
  pe.inst.ADD: 1
`), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	enabled := true
	spec := ArchSpec{
		Energy: EnergySpec{
			Enabled:             &enabled,
			ModelFile:           modelPath,
			UnknownActionPolicy: report.EnergyUnknownActionZero,
		},
	}
	cfg, err := Resolve(spec, "energy-policy-override")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.EnergyModel.UnknownActionPolicy != report.EnergyUnknownActionZero {
		t.Fatalf("unknown policy = %q, want inline override zero", cfg.EnergyModel.UnknownActionPolicy)
	}
}

func TestResolveEnergyModelInvalidUnits(t *testing.T) {
	enabled := true
	spec := ArchSpec{
		Energy: EnergySpec{
			Enabled: &enabled,
			Units:   "nJ",
		},
	}
	if _, err := Resolve(spec, "energy-invalid"); err == nil {
		t.Fatal("expected invalid units error")
	}
}
