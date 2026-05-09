package report

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	EnergyUnknownActionError = "error"
	EnergyUnknownActionWarn  = "warn"
	EnergyUnknownActionZero  = "zero"

	EnergyStaticScopeInstantiatedTilesTotalCycles = "instantiated_tiles_total_cycles"
	EnergyStaticScopeActiveTilesActiveCycles      = "active_tiles_active_cycles"
)

// EnergyModel configures action-based energy estimation in picojoules.
type EnergyModel struct {
	Enabled             bool
	Units               string
	ModelFile           string
	Actions             map[string]float64
	UnknownActionPolicy string
	Static              EnergyStaticModel
}

// EnergyStaticModel configures optional leakage/static energy estimation.
type EnergyStaticModel struct {
	Enabled               bool    `yaml:"enabled"`
	Scope                 string  `yaml:"scope"`
	TileLeakagePJPerCycle float64 `yaml:"tile_leakage_pj_per_cycle"`
}

// EnergyReport is the additive energy section in the execution report.
type EnergyReport struct {
	Units            string                 `json:"units"`
	EstimationOK     bool                   `json:"estimationOK"`
	FailureReason    string                 `json:"failureReason,omitempty"`
	UnknownPolicy    string                 `json:"unknownActionPolicy"`
	DynamicEnergyPJ  float64                `json:"dynamicEnergyPJ"`
	StaticEnergyPJ   float64                `json:"staticEnergyPJ,omitempty"`
	TotalEnergyPJ    float64                `json:"totalEnergyPJ"`
	ActionCounts     []EnergyActionCount    `json:"actionCounts"`
	ByLayer          []EnergyBreakdownEntry `json:"byLayer,omitempty"`
	ByOpcode         []EnergyBreakdownEntry `json:"byOpcode,omitempty"`
	ByDataflowAction []EnergyBreakdownEntry `json:"byDataflowAction,omitempty"`
	ByMemoryAction   []EnergyBreakdownEntry `json:"byMemoryAction,omitempty"`
	ByTile           []EnergyTileBreakdown  `json:"byTile,omitempty"`
	UnknownActions   []EnergyIssue          `json:"unknownActions,omitempty"`
	UnresolvedEvents []EnergyIssue          `json:"unresolvedEvents,omitempty"`
	ModelFile        string                 `json:"modelFile,omitempty"`
	StaticScope      string                 `json:"staticScope,omitempty"`
}

// EnergyActionCount records one normalized action count and energy.
type EnergyActionCount struct {
	Action      string  `json:"action"`
	Count       int64   `json:"count"`
	EnergyPerPJ float64 `json:"energyPerActionPJ"`
	EnergyPJ    float64 `json:"energyPJ"`
}

// EnergyBreakdownEntry records an aggregate energy bucket.
type EnergyBreakdownEntry struct {
	Name     string  `json:"name"`
	Count    int64   `json:"count"`
	EnergyPJ float64 `json:"energyPJ"`
}

// EnergyTileBreakdown records per-tile dynamic and static energy.
type EnergyTileBreakdown struct {
	X               int     `json:"x"`
	Y               int     `json:"y"`
	Coord           string  `json:"coord"`
	DynamicEnergyPJ float64 `json:"dynamicEnergyPJ"`
	StaticEnergyPJ  float64 `json:"staticEnergyPJ,omitempty"`
	TotalEnergyPJ   float64 `json:"totalEnergyPJ"`
}

// EnergyIssue records an event that could not be priced or located.
type EnergyIssue struct {
	Action string `json:"action,omitempty"`
	Msg    string `json:"msg"`
	Detail string `json:"detail,omitempty"`
	Coord  string `json:"coord,omitempty"`
}

type energyAccumulator struct {
	count  int64
	energy float64
}

// NormalizeEnergyModel fills defaults for an energy model.
func NormalizeEnergyModel(model *EnergyModel) *EnergyModel {
	if model == nil || !model.Enabled {
		return nil
	}
	next := *model
	if strings.TrimSpace(next.Units) == "" {
		next.Units = "pJ"
	}
	if strings.TrimSpace(next.UnknownActionPolicy) == "" {
		next.UnknownActionPolicy = EnergyUnknownActionError
	}
	if next.Actions == nil {
		next.Actions = map[string]float64{}
	}
	return &next
}

// ValidateEnergyModel checks whether the model is usable.
func ValidateEnergyModel(model *EnergyModel) error {
	model = NormalizeEnergyModel(model)
	if model == nil {
		return nil
	}
	if model.Units != "pJ" {
		return fmt.Errorf("energy.units must be pJ, got %q", model.Units)
	}
	switch model.UnknownActionPolicy {
	case EnergyUnknownActionError, EnergyUnknownActionWarn, EnergyUnknownActionZero:
	default:
		return fmt.Errorf("energy.unknown_action_policy must be error, warn, or zero, got %q", model.UnknownActionPolicy)
	}
	for action, value := range model.Actions {
		if strings.TrimSpace(action) == "" {
			return fmt.Errorf("energy.actions contains an empty action name")
		}
		if value < 0 {
			return fmt.Errorf("energy.actions[%s] must be >= 0", action)
		}
	}
	if model.Static.Enabled {
		switch model.Static.Scope {
		case EnergyStaticScopeInstantiatedTilesTotalCycles, EnergyStaticScopeActiveTilesActiveCycles:
		default:
			return fmt.Errorf("energy.static.scope must be %q or %q, got %q",
				EnergyStaticScopeInstantiatedTilesTotalCycles,
				EnergyStaticScopeActiveTilesActiveCycles,
				model.Static.Scope,
			)
		}
		if model.Static.TileLeakagePJPerCycle < 0 {
			return fmt.Errorf("energy.static.tile_leakage_pj_per_cycle must be >= 0")
		}
	}
	return nil
}

// BuildEnergyReport converts normalized action counts into energy totals.
func BuildEnergyReport(model *EnergyModel, events []energyEvent, tiles []TileStats, totalCycles int64, width, height int) *EnergyReport {
	model = NormalizeEnergyModel(model)
	if model == nil {
		return nil
	}

	actionCounts := map[string]int64{}
	byTile := map[tileCoord]float64{}
	var unknown []EnergyIssue
	var unresolved []EnergyIssue

	for _, item := range events {
		action, ok := normalizeEnergyAction(item.event)
		if !ok {
			continue
		}
		if !item.hasCoord {
			unresolved = append(unresolved, EnergyIssue{
				Action: action,
				Msg:    item.event.Msg,
				Detail: "missing tile coordinate",
			})
			if model.UnknownActionPolicy == EnergyUnknownActionError {
				continue
			}
		}
		energyPerAction, known := model.Actions[action]
		if !known {
			unknown = append(unknown, EnergyIssue{
				Action: action,
				Msg:    item.event.Msg,
				Detail: "missing energy action value",
				Coord:  issueCoord(item),
			})
			if model.UnknownActionPolicy == EnergyUnknownActionError {
				continue
			}
			energyPerAction = 0
		}
		actionCounts[action]++
		if item.hasCoord {
			byTile[item.coord] += energyPerAction
		}
	}

	actionBreakdown := buildActionCountBreakdown(actionCounts, model.Actions)
	dynamic := sumActionEnergy(actionBreakdown)
	tileBreakdown := buildTileEnergyBreakdown(byTile)
	staticEnergy := applyStaticEnergy(model.Static, tileBreakdown, tiles, totalCycles, width, height)
	total := dynamic + staticEnergy
	estimationOK := true
	failureReason := ""
	if model.UnknownActionPolicy == EnergyUnknownActionError && (len(unknown) > 0 || len(unresolved) > 0) {
		estimationOK = false
		failureReason = "unknown or unresolved energy actions encountered"
	}

	modelFile := strings.TrimSpace(model.ModelFile)
	if modelFile != "" {
		modelFile = filepath.Clean(modelFile)
	}
	return &EnergyReport{
		EstimationOK:     estimationOK,
		FailureReason:    failureReason,
		UnknownPolicy:    model.UnknownActionPolicy,
		Units:            "pJ",
		DynamicEnergyPJ:  dynamic,
		StaticEnergyPJ:   staticEnergy,
		TotalEnergyPJ:    total,
		ActionCounts:     actionBreakdown,
		ByLayer:          buildLayerBreakdown(actionBreakdown),
		ByOpcode:         buildPrefixBreakdown(actionBreakdown, "pe.inst.", true),
		ByDataflowAction: buildPrefixBreakdown(actionBreakdown, "pe.dataflow.", false),
		ByMemoryAction:   buildPrefixBreakdown(actionBreakdown, "pe.memory.", false),
		ByTile:           tileBreakdown,
		UnknownActions:   unknown,
		UnresolvedEvents: unresolved,
		ModelFile:        modelFile,
		StaticScope:      model.Static.Scope,
	}
}

func normalizeEnergyAction(event traceEvent) (string, bool) {
	switch event.Msg {
	case "Inst":
		if event.Pred != nil && !*event.Pred {
			return "pe.inst.predicate_suppressed", true
		}
		opcode := strings.ToUpper(strings.TrimSpace(event.OpCode))
		if opcode == "" {
			return "pe.inst.<unknown>", true
		}
		return "pe.inst." + opcode, true
	case "DataFlow":
		switch strings.ToLower(strings.TrimSpace(event.Behavior)) {
		case "send":
			return "pe.dataflow.send", true
		case "recv":
			return "pe.dataflow.recv", true
		case "feedin":
			return "pe.dataflow.feedin", true
		case "collect":
			return "pe.dataflow.collect", true
		default:
			return "pe.dataflow.<unknown>", true
		}
	case "Memory":
		return normalizeMemoryEnergyAction(event), true
	default:
		return "", false
	}
}

func normalizeMemoryEnergyAction(event traceEvent) string {
	behavior := strings.ToLower(strings.TrimSpace(event.Behavior))
	opcode := strings.ToUpper(strings.TrimSpace(event.OpCode))
	switch behavior {
	case "writememory":
		return "pe.memory.local_write"
	case "readmemory":
		return "pe.memory.local_read"
	case "send":
		if opcode == "STORE" || opcode == "ST" || opcode == "STW" {
			return "pe.memory.request_store"
		}
		if opcode == "LOAD" || opcode == "LD" || opcode == "LDW" {
			return "pe.memory.request_load"
		}
		return "pe.memory.request_unknown"
	case "recv":
		if opcode == "STORE" || opcode == "ST" || opcode == "STW" {
			return "pe.memory.response_store"
		}
		if opcode == "LOAD" || opcode == "LD" || opcode == "LDW" {
			return "pe.memory.response_load"
		}
		return "pe.memory.response_unknown"
	default:
		return "pe.memory.<unknown>"
	}
}

func buildActionCountBreakdown(counts map[string]int64, values map[string]float64) []EnergyActionCount {
	out := make([]EnergyActionCount, 0, len(counts))
	for action, count := range counts {
		per := values[action]
		out = append(out, EnergyActionCount{
			Action:      action,
			Count:       count,
			EnergyPerPJ: per,
			EnergyPJ:    float64(count) * per,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Action < out[j].Action })
	return out
}

func sumActionEnergy(actions []EnergyActionCount) float64 {
	var total float64
	for _, action := range actions {
		total += action.EnergyPJ
	}
	return total
}

func buildLayerBreakdown(actions []EnergyActionCount) []EnergyBreakdownEntry {
	acc := map[string]energyAccumulator{}
	for _, action := range actions {
		layer := "unknown"
		parts := strings.Split(action.Action, ".")
		if len(parts) >= 2 {
			layer = strings.Join(parts[:2], ".")
		}
		current := acc[layer]
		current.count += action.Count
		current.energy += action.EnergyPJ
		acc[layer] = current
	}
	return sortedBreakdown(acc)
}

func buildPrefixBreakdown(actions []EnergyActionCount, prefix string, suppressPredicate bool) []EnergyBreakdownEntry {
	acc := map[string]energyAccumulator{}
	for _, action := range actions {
		if !strings.HasPrefix(action.Action, prefix) {
			continue
		}
		name := strings.TrimPrefix(action.Action, prefix)
		if suppressPredicate && name == "predicate_suppressed" {
			continue
		}
		current := acc[name]
		current.count += action.Count
		current.energy += action.EnergyPJ
		acc[name] = current
	}
	return sortedBreakdown(acc)
}

func sortedBreakdown(acc map[string]energyAccumulator) []EnergyBreakdownEntry {
	out := make([]EnergyBreakdownEntry, 0, len(acc))
	for name, value := range acc {
		out = append(out, EnergyBreakdownEntry{Name: name, Count: value.count, EnergyPJ: value.energy})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func buildTileEnergyBreakdown(dynamic map[tileCoord]float64) []EnergyTileBreakdown {
	out := make([]EnergyTileBreakdown, 0, len(dynamic))
	for coord, energy := range dynamic {
		out = append(out, EnergyTileBreakdown{
			X:               coord.x,
			Y:               coord.y,
			Coord:           formatCoord(coord.x, coord.y),
			DynamicEnergyPJ: energy,
			TotalEnergyPJ:   energy,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Y != out[j].Y {
			return out[i].Y < out[j].Y
		}
		return out[i].X < out[j].X
	})
	return out
}

func applyStaticEnergy(
	model EnergyStaticModel,
	byTile []EnergyTileBreakdown,
	tiles []TileStats,
	totalCycles int64,
	width int,
	height int,
) float64 {
	if !model.Enabled || model.TileLeakagePJPerCycle == 0 {
		return 0
	}
	var total float64
	switch model.Scope {
	case EnergyStaticScopeInstantiatedTilesTotalCycles:
		total = float64(width*height) * float64(totalCycles) * model.TileLeakagePJPerCycle
		perTile := float64(totalCycles) * model.TileLeakagePJPerCycle
		for idx := range byTile {
			byTile[idx].StaticEnergyPJ += perTile
			byTile[idx].TotalEnergyPJ += perTile
		}
	case EnergyStaticScopeActiveTilesActiveCycles:
		activeCyclesByCoord := map[string]int64{}
		for _, tile := range tiles {
			activeCyclesByCoord[tile.Coord] = tile.ActiveCycles
			total += float64(tile.ActiveCycles) * model.TileLeakagePJPerCycle
		}
		for idx := range byTile {
			static := float64(activeCyclesByCoord[byTile[idx].Coord]) * model.TileLeakagePJPerCycle
			byTile[idx].StaticEnergyPJ += static
			byTile[idx].TotalEnergyPJ += static
		}
	}
	return total
}

func issueCoord(item energyEvent) string {
	if !item.hasCoord {
		return ""
	}
	return formatCoord(item.coord.x, item.coord.y)
}
