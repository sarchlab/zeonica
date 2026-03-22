package runtimecfg

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/zeonica/core"
)

func TestResolveExecutionPolicyDefaultsToInOrder(t *testing.T) {
	cfg, err := Resolve(ArchSpec{}, "policy-default")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.ExecutionPolicy != "in_order_dataflow" {
		t.Fatalf("unexpected default execution policy: %q", cfg.ExecutionPolicy)
	}
}

func TestResolveExecutionPolicyAlias(t *testing.T) {
	spec := ArchSpec{
		Simulator: Simulator{
			ExecutionPolicy: "hybrid",
		},
	}
	cfg, err := Resolve(spec, "policy-alias")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.ExecutionPolicy != "elastic_scheduled" {
		t.Fatalf("unexpected normalized policy: %q", cfg.ExecutionPolicy)
	}
}

func TestResolveExecutionPolicyInvalid(t *testing.T) {
	spec := ArchSpec{
		Simulator: Simulator{
			ExecutionPolicy: "unknown_mode",
		},
	}
	_, err := Resolve(spec, "policy-invalid")
	if err == nil {
		t.Fatal("expected error for invalid policy, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported execution_policy") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveStrictDefaults(t *testing.T) {
	cfg, err := Resolve(ArchSpec{}, "strict-default")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.StrictMaxSlip != 4 {
		t.Fatalf("unexpected strict max slip: got %d want 4", cfg.StrictMaxSlip)
	}
	if cfg.StrictFailOnViolation {
		t.Fatalf("unexpected strict fail flag: got true want false")
	}
}

func TestResolveStrictEnvOverrides(t *testing.T) {
	t.Setenv("ZEONICA_STRICT_MAX_SLIP", "8")
	t.Setenv("ZEONICA_STRICT_FAIL_ON_VIOLATION", "true")

	cfg, err := Resolve(ArchSpec{
		Simulator: Simulator{
			StrictMaxSlip:         int64Ptr(2),
			StrictFailOnViolation: boolPtr(false),
		},
	}, "strict-env")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if cfg.StrictMaxSlip != 8 {
		t.Fatalf("unexpected strict max slip from env: got %d want 8", cfg.StrictMaxSlip)
	}
	if !cfg.StrictFailOnViolation {
		t.Fatalf("unexpected strict fail flag from env: got false want true")
	}
}

func TestResolveStrictInvalidEnv(t *testing.T) {
	t.Setenv("ZEONICA_STRICT_MAX_SLIP", "bad")
	_, err := Resolve(ArchSpec{}, "strict-invalid-env")
	if err == nil {
		t.Fatal("expected error for invalid strict env")
	}
	if !strings.Contains(err.Error(), "ZEONICA_STRICT_MAX_SLIP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveMicroarchitectureDefaults(t *testing.T) {
	cfg, err := Resolve(ArchSpec{}, "microarch-defaults")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.DriverPortIncomingBufferDepth != 1 || cfg.DriverPortOutgoingBufferDepth != 1 {
		t.Fatalf(
			"unexpected driver port depth defaults: in=%d out=%d",
			cfg.DriverPortIncomingBufferDepth,
			cfg.DriverPortOutgoingBufferDepth,
		)
	}
	if cfg.CorePortIncomingBufferDepth != 1 || cfg.CorePortOutgoingBufferDepth != 1 {
		t.Fatalf(
			"unexpected core port depth defaults: in=%d out=%d",
			cfg.CorePortIncomingBufferDepth,
			cfg.CorePortOutgoingBufferDepth,
		)
	}
	if cfg.NumRegisters != 64 || cfg.LocalMemoryWords != 1024 {
		t.Fatalf("unexpected tile defaults: regs=%d mem=%d", cfg.NumRegisters, cfg.LocalMemoryWords)
	}
	if cfg.MemoryMode != "simple" {
		t.Fatalf("unexpected memory mode default: %q", cfg.MemoryMode)
	}
	if cfg.LinkLatency != 1 || cfg.LinkBandwidth != 32 {
		t.Fatalf("unexpected link defaults: latency=%d bandwidth=%d", cfg.LinkLatency, cfg.LinkBandwidth)
	}
	if cfg.LinkTimingModel != "parse_only" {
		t.Fatalf("unexpected link timing model: %q", cfg.LinkTimingModel)
	}
	if cfg.EnableFIFOModel {
		t.Fatalf("unexpected fifo model default: got true want false")
	}
	if cfg.ProgramYAML != "" || cfg.ReportName != "" || len(cfg.QueueWatches) != 0 || len(cfg.BufferSweepDepths) != 0 {
		t.Fatalf(
			"unexpected experiment defaults: program=%q report=%q watches=%d depths=%d",
			cfg.ProgramYAML,
			cfg.ReportName,
			len(cfg.QueueWatches),
			len(cfg.BufferSweepDepths),
		)
	}
}

func TestResolveMicroarchitectureOverrides(t *testing.T) {
	spec := ArchSpec{
		CGRADefaults: CGRADefaults{Rows: 2, Columns: 2},
		TileDefaults: TileDefaults{NumRegisters: 96, LocalMemoryWords: 2048},
		LinkDefaults: LinkDefaults{Latency: intPtr(3), Bandwidth: intPtr(128)},
		Simulator: Simulator{
			EnableFIFOModel: boolPtr(true),
			Driver: NamedComponent{
				PortIncomingBufferDepth: intPtr(4),
				PortOutgoingBufferDepth: intPtr(5),
			},
			Device: DeviceComponent{
				MemoryMode:              "shared",
				PortIncomingBufferDepth: intPtr(6),
				PortOutgoingBufferDepth: intPtr(7),
				MemoryShare:             []MemoryShareEntry{{TileX: 1, TileY: 1, Group: 9}},
			},
		},
	}

	cfg, err := Resolve(spec, "microarch-overrides")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if cfg.DriverPortIncomingBufferDepth != 4 || cfg.DriverPortOutgoingBufferDepth != 5 {
		t.Fatalf(
			"driver buffer depth override failed: in=%d out=%d",
			cfg.DriverPortIncomingBufferDepth,
			cfg.DriverPortOutgoingBufferDepth,
		)
	}
	if cfg.CorePortIncomingBufferDepth != 6 || cfg.CorePortOutgoingBufferDepth != 7 {
		t.Fatalf(
			"core buffer depth override failed: in=%d out=%d",
			cfg.CorePortIncomingBufferDepth,
			cfg.CorePortOutgoingBufferDepth,
		)
	}
	if cfg.NumRegisters != 96 || cfg.LocalMemoryWords != 2048 {
		t.Fatalf("tile override failed: regs=%d mem=%d", cfg.NumRegisters, cfg.LocalMemoryWords)
	}
	if cfg.MemoryMode != "shared" {
		t.Fatalf("memory mode override failed: %q", cfg.MemoryMode)
	}
	if len(cfg.MemoryShare) != 4 {
		t.Fatalf("shared mode should materialize full 2x2 map, got %d", len(cfg.MemoryShare))
	}
	if got := cfg.MemoryShare[[2]int{1, 1}]; got != 9 {
		t.Fatalf("memory_share override for (1,1) failed: got %d want 9", got)
	}
	if cfg.LinkLatency != 3 || cfg.LinkBandwidth != 128 {
		t.Fatalf("link override failed: latency=%d bandwidth=%d", cfg.LinkLatency, cfg.LinkBandwidth)
	}
	if !cfg.EnableFIFOModel {
		t.Fatalf("fifo model override failed: got false want true")
	}
}

func TestResolveMicroarchitectureInvalidDepth(t *testing.T) {
	spec := ArchSpec{
		Simulator: Simulator{
			Driver: NamedComponent{PortIncomingBufferDepth: intPtr(0)},
		},
	}
	_, err := Resolve(spec, "microarch-invalid-depth")
	if err == nil {
		t.Fatal("expected invalid depth error")
	}
	if !strings.Contains(err.Error(), "port_incoming_buffer_depth") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveInvalidMemoryMode(t *testing.T) {
	spec := ArchSpec{Simulator: Simulator{Device: DeviceComponent{MemoryMode: "foo"}}}
	_, err := Resolve(spec, "memory-mode-invalid")
	if err == nil {
		t.Fatal("expected invalid memory mode error")
	}
	if !strings.Contains(err.Error(), "unsupported memory_mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveInvalidLinkLatency(t *testing.T) {
	spec := ArchSpec{LinkDefaults: LinkDefaults{Latency: intPtr(-1)}}
	_, err := Resolve(spec, "link-latency-invalid")
	if err == nil {
		t.Fatal("expected invalid link latency error")
	}
	if !strings.Contains(err.Error(), "link_defaults.latency") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveInvalidMemoryShareCoordinate(t *testing.T) {
	spec := ArchSpec{
		CGRADefaults: CGRADefaults{Rows: 2, Columns: 2},
		Simulator: Simulator{
			Device: DeviceComponent{
				MemoryMode:  "shared",
				MemoryShare: []MemoryShareEntry{{TileX: 3, TileY: 0, Group: 0}},
			},
		},
	}
	_, err := Resolve(spec, "memory-share-invalid")
	if err == nil {
		t.Fatal("expected invalid memory share coordinate error")
	}
	if !strings.Contains(err.Error(), "out-of-range tile") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveWithSpecPathExperimentConfig(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "base_arch_spec.yaml")
	spec := ArchSpec{
		Simulator: Simulator{
			ProgramYAML:       "fir+histogram/tmp-generated-instructions.yaml",
			ReportName:        "fir_histogram",
			BufferSweepDepths: []int{1, 2, 4, 8, 16},
			QueueWatches: []core.QueueWatchSpec{
				{Label: "hist_upstream", X: 1, Y: 1, Kind: "recv", Direction: "West", Color: "RED"},
				{Label: "hist_downstream", X: 2, Y: 1, Kind: "recv", Direction: "West", Color: "RED"},
			},
		},
	}

	cfg, err := ResolveWithSpecPath(spec, specPath, "")
	if err != nil {
		t.Fatalf("ResolveWithSpecPath returned error: %v", err)
	}

	expectedProgram := filepath.Join(filepath.Dir(specPath), "fir+histogram", "tmp-generated-instructions.yaml")
	if cfg.ProgramYAML != expectedProgram {
		t.Fatalf("unexpected resolved program path: got %q want %q", cfg.ProgramYAML, expectedProgram)
	}
	if cfg.ReportName != "fir_histogram" {
		t.Fatalf("unexpected report name: %q", cfg.ReportName)
	}
	if cfg.TestName != "fir_histogram" {
		t.Fatalf("expected report_name to seed test name, got %q", cfg.TestName)
	}
	if len(cfg.QueueWatches) != 2 {
		t.Fatalf("unexpected queue watch count: %d", len(cfg.QueueWatches))
	}
	if len(cfg.BufferSweepDepths) != 5 {
		t.Fatalf("unexpected buffer sweep depth count: %d", len(cfg.BufferSweepDepths))
	}
}

func TestResolveWithSpecPathRejectsInvalidQueueWatch(t *testing.T) {
	spec := ArchSpec{
		Simulator: Simulator{
			QueueWatches: []core.QueueWatchSpec{
				{Label: "bad", X: 0, Y: 0, Kind: "recv", Direction: "Bogus", Color: "RED"},
			},
		},
	}

	_, err := ResolveWithSpecPath(spec, "", "invalid-watch")
	if err == nil {
		t.Fatal("expected invalid queue watch error")
	}
	if !strings.Contains(err.Error(), "simulator.queue_watches") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func int64Ptr(v int64) *int64 { return &v }

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }
