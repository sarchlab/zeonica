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
			"pe.inst.ADD":                  1,
			"pe.inst.predicate_suppressed": 0.25,
			"pe.inst.LOAD":                 2,
			"pe.inst.STORE":                3,
			"pe.memory.request_load":       4,
			"pe.memory.response_load":      5,
			"pe.memory.request_store":      6,
			"pe.memory.response_store":     7,
			"pe.dataflow.send":             8,
			"pe.dataflow.recv":             9,
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
	if got := actionCount(result.ActionCounts, "pe.inst.predicate_suppressed"); got != 1 {
		t.Fatalf("predicate_suppressed count = %d, want 1", got)
	}
	if got := actionCount(result.ActionCounts, "pe.memory.request_load"); got != 1 {
		t.Fatalf("request_load count = %d, want 1", got)
	}
	if got := actionCount(result.ActionCounts, "pe.memory.response_store"); got != 1 {
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
}

func actionCount(actions []EnergyActionCount, name string) int64 {
	for _, action := range actions {
		if action.Action == name {
			return action.Count
		}
	}
	return 0
}
