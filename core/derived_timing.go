package core

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const timingSidecarEnv = "ZEONICA_TIMING_SIDECAR"

type timingSidecar struct {
	SourceLog string             `json:"source_log"`
	DerivedAt string             `json:"derived_at"`
	Ops       []timingOpSchedule `json:"ops"`
}

type timingOpSchedule struct {
	X      int     `json:"x"`
	Y      int     `json:"y"`
	OpID   int     `json:"op_id"`
	Cycles []int64 `json:"cycles"`
}

func loadDerivedTimingFromEnv() (map[string]map[int][]int64, error) {
	path := strings.TrimSpace(os.Getenv(timingSidecarEnv))
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s (%s): %w", timingSidecarEnv, path, err)
	}

	var sidecar timingSidecar
	if err := json.Unmarshal(data, &sidecar); err != nil {
		return nil, fmt.Errorf("parse timing sidecar %s: %w", path, err)
	}

	result := make(map[string]map[int][]int64)
	for _, op := range sidecar.Ops {
		if len(op.Cycles) == 0 {
			continue
		}
		coordKey := fmt.Sprintf("(%d,%d)", op.X, op.Y)
		if _, exists := result[coordKey]; !exists {
			result[coordKey] = make(map[int][]int64)
		}
		result[coordKey][op.OpID] = append(result[coordKey][op.OpID], op.Cycles...)
	}

	return result, nil
}

func cloneDerivedTimingMap(src map[int][]int64) map[int][]int64 {
	if len(src) == 0 {
		return nil
	}

	cloned := make(map[int][]int64, len(src))
	for opID, cycles := range src {
		copied := make([]int64, len(cycles))
		copy(copied, cycles)
		cloned[opID] = copied
	}
	return cloned
}
