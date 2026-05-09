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

	EnergyActionInstPrefix     = "pe.inst."
	EnergyActionDataflowPrefix = "pe.dataflow."
	EnergyActionMemoryPrefix   = "pe.memory."

	EnergyActionPredicateSuppressed = EnergyActionInstPrefix + "predicate_suppressed"
	EnergyActionInstUnknown         = EnergyActionInstPrefix + "<unknown>"

	EnergyActionDataflowSend    = EnergyActionDataflowPrefix + "send"
	EnergyActionDataflowRecv    = EnergyActionDataflowPrefix + "recv"
	EnergyActionDataflowFeedIn  = EnergyActionDataflowPrefix + "feedin"
	EnergyActionDataflowCollect = EnergyActionDataflowPrefix + "collect"
	EnergyActionDataflowUnknown = EnergyActionDataflowPrefix + "<unknown>"

	EnergyActionMemoryLocalRead     = EnergyActionMemoryPrefix + "local_read"
	EnergyActionMemoryLocalWrite    = EnergyActionMemoryPrefix + "local_write"
	EnergyActionMemoryRequestLoad   = EnergyActionMemoryPrefix + "request_load"
	EnergyActionMemoryRequestStore  = EnergyActionMemoryPrefix + "request_store"
	EnergyActionMemoryResponseLoad  = EnergyActionMemoryPrefix + "response_load"
	EnergyActionMemoryResponseStore = EnergyActionMemoryPrefix + "response_store"
	EnergyActionMemoryUnknown       = EnergyActionMemoryPrefix + "<unknown>"

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

type energyCountingResult struct {
	actionCounts map[string]int64
	byTile       map[tileCoord]float64
	unknown      []EnergyIssue
	unresolved   []EnergyIssue
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
func BuildEnergyReport(
	model *EnergyModel,
	events []energyEvent,
	tiles []TileStats,
	totalCycles int64,
	width int,
	height int,
) *EnergyReport {
	model = NormalizeEnergyModel(model)
	if model == nil {
		return nil
	}

	counts := countEnergyActions(model, events)

	actionBreakdown := buildActionCountBreakdown(counts.actionCounts, model.Actions)
	dynamic := sumActionEnergy(actionBreakdown)
	tileBreakdown := buildTileEnergyBreakdown(counts.byTile)
	tileBreakdown = ensureStaticTiles(model.Static, tileBreakdown, tiles, width, height)
	staticEnergy := applyStaticEnergy(model.Static, tileBreakdown, tiles, totalCycles, width, height)
	total := dynamic + staticEnergy
	estimationOK := true
	failureReason := ""
	if model.UnknownActionPolicy == EnergyUnknownActionError && (len(counts.unknown) > 0 || len(counts.unresolved) > 0) {
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
		ByOpcode:         buildPrefixBreakdown(actionBreakdown, EnergyActionInstPrefix, true),
		ByDataflowAction: buildPrefixBreakdown(actionBreakdown, EnergyActionDataflowPrefix, false),
		ByMemoryAction:   buildPrefixBreakdown(actionBreakdown, EnergyActionMemoryPrefix, false),
		ByTile:           tileBreakdown,
		UnknownActions:   counts.unknown,
		UnresolvedEvents: counts.unresolved,
		ModelFile:        modelFile,
		StaticScope:      model.Static.Scope,
	}
}

func countEnergyActions(model *EnergyModel, events []energyEvent) energyCountingResult {
	result := energyCountingResult{
		actionCounts: map[string]int64{},
		byTile:       map[tileCoord]float64{},
	}
	for _, item := range events {
		countEnergyEvent(model, item, &result)
	}
	return result
}

func countEnergyEvent(model *EnergyModel, item energyEvent, result *energyCountingResult) {
	action, ok := normalizeEnergyAction(item.event)
	if !ok {
		return
	}
	if !item.hasCoord {
		result.unresolved = append(result.unresolved, unresolvedEnergyIssue(action, item.event))
		if model.UnknownActionPolicy == EnergyUnknownActionError {
			return
		}
	}
	energyPerAction, known := model.Actions[action]
	if !known {
		result.unknown = append(result.unknown, unknownEnergyIssue(action, item))
		if model.UnknownActionPolicy == EnergyUnknownActionError {
			return
		}
		energyPerAction = 0
	}
	result.actionCounts[action]++
	if item.hasCoord {
		result.byTile[item.coord] += energyPerAction
	}
}

func unresolvedEnergyIssue(action string, event traceEvent) EnergyIssue {
	return EnergyIssue{Action: action, Msg: event.Msg, Detail: "missing tile coordinate"}
}

func unknownEnergyIssue(action string, item energyEvent) EnergyIssue {
	return EnergyIssue{
		Action: action,
		Msg:    item.event.Msg,
		Detail: "missing energy action value",
		Coord:  issueCoord(item),
	}
}

func normalizeEnergyAction(event traceEvent) (string, bool) {
	switch event.Msg {
	case "Inst":
		return normalizeInstEnergyAction(event), true
	case "DataFlow":
		return normalizeDataflowEnergyAction(event), true
	case "Memory":
		return normalizeMemoryEnergyAction(event), true
	default:
		return "", false
	}
}

func normalizeInstEnergyAction(event traceEvent) string {
	if event.Pred != nil && !*event.Pred {
		return EnergyActionPredicateSuppressed
	}
	opcode := strings.ToUpper(strings.TrimSpace(event.OpCode))
	if opcode == "" {
		return EnergyActionInstUnknown
	}
	return EnergyActionInstPrefix + opcode
}

func normalizeDataflowEnergyAction(event traceEvent) string {
	actions := map[string]string{
		"send":    EnergyActionDataflowSend,
		"recv":    EnergyActionDataflowRecv,
		"feedin":  EnergyActionDataflowFeedIn,
		"collect": EnergyActionDataflowCollect,
	}
	if action, ok := actions[strings.ToLower(strings.TrimSpace(event.Behavior))]; ok {
		return action
	}
	return EnergyActionDataflowUnknown
}

func normalizeMemoryEnergyAction(event traceEvent) string {
	behavior := strings.ToLower(strings.TrimSpace(event.Behavior))
	switch behavior {
	case "writememory":
		return EnergyActionMemoryLocalWrite
	case "readmemory":
		return EnergyActionMemoryLocalRead
	case "send":
		return memoryTransferAction("request", event.OpCode)
	case "recv":
		return memoryTransferAction("response", event.OpCode)
	default:
		return EnergyActionMemoryUnknown
	}
}

func memoryTransferAction(prefix, opcode string) string {
	switch memoryOpcodeClass(opcode) {
	case "store":
		if prefix == "request" {
			return EnergyActionMemoryRequestStore
		}
		return EnergyActionMemoryResponseStore
	case "load":
		if prefix == "request" {
			return EnergyActionMemoryRequestLoad
		}
		return EnergyActionMemoryResponseLoad
	default:
		return EnergyActionMemoryPrefix + prefix + "_unknown"
	}
}

func memoryOpcodeClass(opcode string) string {
	classes := map[string]string{
		"STORE": "store",
		"ST":    "store",
		"STW":   "store",
		"LOAD":  "load",
		"LD":    "load",
		"LDW":   "load",
	}
	return classes[strings.ToUpper(strings.TrimSpace(opcode))]
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
	sortEnergyTileBreakdown(out)
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

func ensureStaticTiles(
	model EnergyStaticModel,
	byTile []EnergyTileBreakdown,
	tiles []TileStats,
	width int,
	height int,
) []EnergyTileBreakdown {
	if !model.Enabled || model.TileLeakagePJPerCycle == 0 {
		return byTile
	}
	seen := map[string]struct{}{}
	for _, tile := range byTile {
		seen[tile.Coord] = struct{}{}
	}
	switch model.Scope {
	case EnergyStaticScopeInstantiatedTilesTotalCycles:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				byTile = appendStaticTileIfMissing(byTile, seen, x, y)
			}
		}
	case EnergyStaticScopeActiveTilesActiveCycles:
		for _, tile := range tiles {
			byTile = appendStaticTileIfMissing(byTile, seen, tile.X, tile.Y)
		}
	}
	sortEnergyTileBreakdown(byTile)
	return byTile
}

func appendStaticTileIfMissing(
	byTile []EnergyTileBreakdown,
	seen map[string]struct{},
	x int,
	y int,
) []EnergyTileBreakdown {
	coord := formatCoord(x, y)
	if _, ok := seen[coord]; ok {
		return byTile
	}
	seen[coord] = struct{}{}
	return append(byTile, EnergyTileBreakdown{X: x, Y: y, Coord: coord})
}

func sortEnergyTileBreakdown(tiles []EnergyTileBreakdown) {
	sort.Slice(tiles, func(i, j int) bool {
		if tiles[i].Y != tiles[j].Y {
			return tiles[i].Y < tiles[j].Y
		}
		return tiles[i].X < tiles[j].X
	})
}

func issueCoord(item energyEvent) string {
	if !item.hasCoord {
		return ""
	}
	return formatCoord(item.coord.x, item.coord.y)
}
