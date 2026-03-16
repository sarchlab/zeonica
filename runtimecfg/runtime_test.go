package runtimecfg

import (
	"strings"
	"testing"
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
