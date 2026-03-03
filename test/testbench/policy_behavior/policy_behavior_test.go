package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/core"
	"github.com/sarchlab/zeonica/runtimecfg"
)

type runResult struct {
	panicMsg string
	memValue uint32
	retValue uint32
	endNS    int64
}

func resolveScenarioPath(t *testing.T, filename string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("cannot resolve current test file path")
	}

	path := filepath.Clean(filepath.Join(filepath.Dir(thisFile), filename))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scenario file %s not found: %v", path, err)
	}
	return path
}

func writePolicyArchSpec(t *testing.T, policy string) string {
	t.Helper()

	spec := fmt.Sprintf(`cgra_defaults:
  rows: 1
  columns: 2
simulator:
  execution_model: "serial"
  execution_policy: "%s"
  logging:
    enabled: false
  driver:
    name: "Driver"
    frequency: "1GHz"
  device:
    name: "Device"
    frequency: "1GHz"
    bind_to_architecture: true
`, policy)

	specPath := filepath.Join(t.TempDir(), "arch_spec.yaml")
	if err := os.WriteFile(specPath, []byte(spec), 0o600); err != nil {
		t.Fatalf("write arch spec: %v", err)
	}
	return specPath
}

func runWorkloadWithPolicy(t *testing.T, policy, scenarioPath string) (result runResult) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			result.panicMsg = fmt.Sprint(recovered)
		}
	}()

	specPath := writePolicyArchSpec(t, policy)
	rt, err := runtimecfg.LoadRuntime(specPath, "policy_behavior_"+policy)
	if err != nil {
		t.Fatalf("load runtime: %v", err)
	}

	program := core.LoadProgramFileFromYAML(scenarioPath)
	if len(program) == 0 {
		t.Fatalf("empty program map from %s", scenarioPath)
	}

	width := rt.Config.Columns
	height := rt.Config.Rows
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			coord := fmt.Sprintf("(%d,%d)", x, y)
			if prog, exists := program[coord]; exists {
				rt.Driver.MapProgram(prog, [2]int{x, y})
			}
		}
	}

	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			tile := rt.Device.GetTile(x, y)
			rt.Engine.Schedule(sim.MakeTickEvent(tile.GetTickingComponent(), 0))
		}
	}

	rt.Driver.FeedIn([]uint32{42}, cgra.West, [2]int{0, 1}, 1, "R")
	rt.Driver.Run()

	result.memValue = rt.Driver.ReadMemory(1, 0, 0)
	result.retValue = rt.Device.GetTile(1, 0).GetRetVal()
	result.endNS = int64(rt.Engine.CurrentTime() * 1e9)

	return result
}

func TestPolicyBehaviorLateArrival(t *testing.T) {
	scenarioPath := resolveScenarioPath(t, "late_arrival.yaml")

	strict := runWorkloadWithPolicy(t, "strict_timed", scenarioPath)
	if !strings.Contains(strict.panicMsg, "synchronization violation") {
		t.Fatalf("strict_timed should report synchronization violation, got: %q", strict.panicMsg)
	}

	elastic := runWorkloadWithPolicy(t, "elastic_scheduled", scenarioPath)
	if elastic.panicMsg != "" {
		t.Fatalf("elastic_scheduled should tolerate late arrival, got panic: %s", elastic.panicMsg)
	}
	if elastic.memValue != 42 || elastic.retValue != 1 {
		t.Fatalf("elastic_scheduled wrong result: mem=%d ret=%d want mem=42 ret=1", elastic.memValue, elastic.retValue)
	}

	inOrder := runWorkloadWithPolicy(t, "in_order_dataflow", scenarioPath)
	if inOrder.panicMsg != "" {
		t.Fatalf("in_order_dataflow should tolerate late arrival, got panic: %s", inOrder.panicMsg)
	}
	if inOrder.memValue != 42 || inOrder.retValue != 1 {
		t.Fatalf("in_order_dataflow wrong result: mem=%d ret=%d want mem=42 ret=1", inOrder.memValue, inOrder.retValue)
	}
}
