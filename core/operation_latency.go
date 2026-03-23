package core

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const operationLatencyFileEnv = "ZEONICA_OPERATION_LATENCY_FILE"

type operationLatencySidecar struct {
	DefaultLatency int            `yaml:"default_latency"`
	Opcodes        map[string]int `yaml:"opcodes"`
}

func loadOperationLatencyProfileFromEnv() (map[string]int, int, error) {
	path := strings.TrimSpace(os.Getenv(operationLatencyFileEnv))
	if path == "" {
		return nil, 1, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s (%s): %w", operationLatencyFileEnv, path, err)
	}

	var sidecar operationLatencySidecar
	if err := yaml.Unmarshal(data, &sidecar); err != nil {
		return nil, 0, fmt.Errorf("parse %s (%s): %w", operationLatencyFileEnv, path, err)
	}

	defaultLatency := sidecar.DefaultLatency
	if defaultLatency == 0 {
		defaultLatency = 1
	}
	if defaultLatency <= 0 {
		return nil, 0, fmt.Errorf("default_latency must be > 0, got %d", sidecar.DefaultLatency)
	}

	normalized := make(map[string]int, len(sidecar.Opcodes))
	for opcode, latency := range sidecar.Opcodes {
		key := normalizeLatencyOpcode(opcode)
		if key == "" {
			return nil, 0, fmt.Errorf("opcode latency entry has empty opcode key")
		}
		if latency <= 0 {
			return nil, 0, fmt.Errorf("opcode %s latency must be > 0, got %d", key, latency)
		}
		normalized[key] = latency
	}

	return normalized, defaultLatency, nil
}

func normalizeLatencyOpcode(opCode string) string {
	return strings.ToUpper(strings.TrimSpace(opCode))
}

func cloneOperationLatencyMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(src))
	for opcode, latency := range src {
		cloned[opcode] = latency
	}
	return cloned
}
