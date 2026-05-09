package runtimecfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sarchlab/zeonica/report"
	"gopkg.in/yaml.v3"
)

type energyModelFile struct {
	Energy              EnergySpec               `yaml:"energy"`
	Enabled             *bool                    `yaml:"enabled"`
	Units               string                   `yaml:"units"`
	Actions             map[string]float64       `yaml:"actions"`
	UnknownActionPolicy string                   `yaml:"unknown_action_policy"`
	Static              report.EnergyStaticModel `yaml:"static"`
}

func resolveEnergyModel(spec EnergySpec, specPath string) (*report.EnergyModel, error) {
	enabled := defaultOrBool(spec.Enabled, false)
	if !enabled && strings.TrimSpace(spec.ModelFile) == "" {
		return nil, nil
	}

	model := &report.EnergyModel{
		Enabled:             enabled,
		Units:               defaultOrString(spec.Units, "pJ"),
		ModelFile:           resolveSpecRelativePath(specPath, spec.ModelFile),
		Actions:             copyEnergyActions(spec.Actions),
		UnknownActionPolicy: defaultOrString(spec.UnknownActionPolicy, report.EnergyUnknownActionError),
		Static:              spec.Static,
	}

	if model.ModelFile != "" {
		fileModel, err := loadEnergyModelFile(model.ModelFile)
		if err != nil {
			return nil, err
		}
		model = mergeEnergyModels(fileModel, model)
		model.ModelFile = filepath.Clean(model.ModelFile)
	}

	model = report.NormalizeEnergyModel(model)
	if model == nil {
		return nil, nil
	}
	if err := report.ValidateEnergyModel(model); err != nil {
		return nil, err
	}
	return model, nil
}

func loadEnergyModelFile(path string) (*report.EnergyModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read energy model file: %w", err)
	}

	var file energyModelFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse energy model file: %w", err)
	}

	spec := file.Energy
	if spec.Enabled == nil {
		spec.Enabled = file.Enabled
	}
	if spec.Units == "" {
		spec.Units = file.Units
	}
	if len(spec.Actions) == 0 {
		spec.Actions = file.Actions
	}
	if spec.UnknownActionPolicy == "" {
		spec.UnknownActionPolicy = file.UnknownActionPolicy
	}
	if !spec.Static.Enabled && spec.Static.Scope == "" && spec.Static.TileLeakagePJPerCycle == 0 {
		spec.Static = file.Static
	}

	return &report.EnergyModel{
		Enabled:             defaultOrBool(spec.Enabled, true),
		Units:               defaultOrString(spec.Units, "pJ"),
		ModelFile:           path,
		Actions:             copyEnergyActions(spec.Actions),
		UnknownActionPolicy: defaultOrString(spec.UnknownActionPolicy, report.EnergyUnknownActionError),
		Static:              spec.Static,
	}, nil
}

func mergeEnergyModels(base, override *report.EnergyModel) *report.EnergyModel {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	merged := *base
	merged.Enabled = override.Enabled
	if strings.TrimSpace(override.Units) != "" {
		merged.Units = override.Units
	}
	if strings.TrimSpace(override.UnknownActionPolicy) != "" {
		merged.UnknownActionPolicy = override.UnknownActionPolicy
	}
	if strings.TrimSpace(override.ModelFile) != "" {
		merged.ModelFile = override.ModelFile
	}
	if override.Static.Enabled || override.Static.Scope != "" || override.Static.TileLeakagePJPerCycle != 0 {
		merged.Static = override.Static
	}
	merged.Actions = copyEnergyActions(base.Actions)
	for action, value := range override.Actions {
		merged.Actions[action] = value
	}
	return &merged
}

func copyEnergyActions(input map[string]float64) map[string]float64 {
	if len(input) == 0 {
		return map[string]float64{}
	}
	out := make(map[string]float64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
