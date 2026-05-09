package report

import "testing"

func TestBuildEnergyReportCountsPredicateAndLayeredLoadStore(t *testing.T) {
	falsePred := false
	truePred := true
	model := &EnergyModel{
		Enabled:             true,
		Units:               "pJ",
		UnknownActionPolicy: EnergyUnknownActionError,
		Actions: map[string]float64{
			EnergyActionInstPrefix + "ADD":   1,
			EnergyActionPredicateSuppressed:  0.25,
			EnergyActionInstPrefix + "LOAD":  2,
			EnergyActionInstPrefix + "STORE": 3,
			EnergyActionMemoryRequestLoad:    4,
			EnergyActionMemoryResponseLoad:   5,
			EnergyActionMemoryRequestStore:   6,
			EnergyActionMemoryResponseStore:  7,
			EnergyActionDataflowSend:         8,
			EnergyActionDataflowRecv:         9,
		},
	}
	events := []energyEvent{
		{event: traceEvent{Msg: "Inst", OpCode: "ADD", Pred: &truePred}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Inst", OpCode: "ADD", Pred: &falsePred}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Inst", OpCode: "LOAD", Pred: &truePred}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Memory", Behavior: "Send", OpCode: "LOAD"}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Memory", Behavior: "Recv", OpCode: "LOAD"}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Inst", OpCode: "STORE", Pred: &truePred}, coord: tileCoord{x: 1, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Memory", Behavior: "Send", OpCode: "STORE"}, coord: tileCoord{x: 1, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "Memory", Behavior: "Recv", OpCode: "STORE"}, coord: tileCoord{x: 1, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "DataFlow", Behavior: "Send"}, coord: tileCoord{x: 1, y: 0}, hasCoord: true},
		{event: traceEvent{Msg: "DataFlow", Behavior: "Recv"}, coord: tileCoord{x: 1, y: 0}, hasCoord: true},
	}

	result := BuildEnergyReport(model, events, nil, 10, 2, 1)
	if result == nil {
		t.Fatal("expected energy report")
	}
	if result.DynamicEnergyPJ != 45.25 {
		t.Fatalf("dynamic energy = %v, want 45.25", result.DynamicEnergyPJ)
	}
	if got := actionCount(result.ActionCounts, EnergyActionPredicateSuppressed); got != 1 {
		t.Fatalf("predicate_suppressed count = %d, want 1", got)
	}
	if got := actionCount(result.ActionCounts, EnergyActionMemoryRequestLoad); got != 1 {
		t.Fatalf("request_load count = %d, want 1", got)
	}
	if got := actionCount(result.ActionCounts, EnergyActionMemoryResponseStore); got != 1 {
		t.Fatalf("response_store count = %d, want 1", got)
	}
}

func TestBuildEnergyReportUnknownPolicies(t *testing.T) {
	model := &EnergyModel{
		Enabled:             true,
		Units:               "pJ",
		UnknownActionPolicy: EnergyUnknownActionZero,
		Actions:             map[string]float64{},
	}
	events := []energyEvent{
		{event: traceEvent{Msg: "Inst", OpCode: "ADD"}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
	}
	result := BuildEnergyReport(model, events, nil, 1, 1, 1)
	if len(result.UnknownActions) != 1 {
		t.Fatalf("unknown actions = %d, want 1", len(result.UnknownActions))
	}
	if result.DynamicEnergyPJ != 0 {
		t.Fatalf("dynamic energy = %v, want 0", result.DynamicEnergyPJ)
	}
	if got := actionCount(result.ActionCounts, "pe.inst.ADD"); got != 1 {
		t.Fatalf("ADD count = %d, want 1", got)
	}
}

func TestBuildEnergyReportUnknownErrorMarksEstimationFailed(t *testing.T) {
	model := &EnergyModel{
		Enabled:             true,
		Units:               "pJ",
		UnknownActionPolicy: EnergyUnknownActionError,
		Actions:             map[string]float64{},
	}
	events := []energyEvent{
		{event: traceEvent{Msg: "Inst", OpCode: "ADD"}, coord: tileCoord{x: 0, y: 0}, hasCoord: true},
	}
	result := BuildEnergyReport(model, events, nil, 1, 1, 1)
	if result.EstimationOK {
		t.Fatal("expected strict unknown action to mark estimation failed")
	}
	if len(result.UnknownActions) != 1 {
		t.Fatalf("unknown actions = %d, want 1", len(result.UnknownActions))
	}
}

func TestBuildEnergyReportStaticScope(t *testing.T) {
	model := &EnergyModel{
		Enabled:             true,
		Units:               "pJ",
		UnknownActionPolicy: EnergyUnknownActionError,
		Actions:             map[string]float64{},
		Static: EnergyStaticModel{
			Enabled:               true,
			Scope:                 EnergyStaticScopeInstantiatedTilesTotalCycles,
			TileLeakagePJPerCycle: 0.5,
		},
	}
	result := BuildEnergyReport(model, nil, nil, 10, 2, 3)
	if result.StaticEnergyPJ != 30 {
		t.Fatalf("static energy = %v, want 30", result.StaticEnergyPJ)
	}
	if result.TotalEnergyPJ != 30 {
		t.Fatalf("total energy = %v, want 30", result.TotalEnergyPJ)
	}
	if got := sumTileEnergy(result.ByTile); got != result.TotalEnergyPJ {
		t.Fatalf("sum tile energy = %v, want total %v", got, result.TotalEnergyPJ)
	}
	if len(result.ByTile) != 6 {
		t.Fatalf("tile breakdown entries = %d, want 6", len(result.ByTile))
	}
}

func TestBuildEnergyReportActiveStaticScopeIncludesActiveTiles(t *testing.T) {
	model := &EnergyModel{
		Enabled:             true,
		Units:               "pJ",
		UnknownActionPolicy: EnergyUnknownActionError,
		Actions:             map[string]float64{},
		Static: EnergyStaticModel{
			Enabled:               true,
			Scope:                 EnergyStaticScopeActiveTilesActiveCycles,
			TileLeakagePJPerCycle: 0.5,
		},
	}
	tiles := []TileStats{
		{X: 0, Y: 0, Coord: "(0,0)", ActiveCycles: 3},
		{X: 1, Y: 0, Coord: "(1,0)", ActiveCycles: 5},
	}
	result := BuildEnergyReport(model, nil, tiles, 10, 2, 1)
	if result.StaticEnergyPJ != 4 {
		t.Fatalf("static energy = %v, want 4", result.StaticEnergyPJ)
	}
	if got := sumTileEnergy(result.ByTile); got != result.TotalEnergyPJ {
		t.Fatalf("sum tile energy = %v, want total %v", got, result.TotalEnergyPJ)
	}
	if len(result.ByTile) != 2 {
		t.Fatalf("tile breakdown entries = %d, want 2", len(result.ByTile))
	}
}

func actionCount(actions []EnergyActionCount, name string) int64 {
	for _, action := range actions {
		if action.Action == name {
			return action.Count
		}
	}
	return 0
}

func sumTileEnergy(tiles []EnergyTileBreakdown) float64 {
	var total float64
	for _, tile := range tiles {
		total += tile.TotalEnergyPJ
	}
	return total
}
