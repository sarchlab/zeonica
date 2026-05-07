package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type opKey struct {
	x    int
	y    int
	opID int
}

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

func main() {
	logPath := flag.String("log", "", "path to elastic trace log (JSONL)")
	outPath := flag.String("out", "", "path to output timing sidecar JSON")
	flag.Parse()

	if *logPath == "" || *outPath == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/mlp-timing-derive -log <elastic.json.log> -out <nominal_timing.json>")
		os.Exit(2)
	}

	scheduleMap, totalFirings, err := deriveFromLog(*logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "derive timing from %s: %v\n", *logPath, err)
		os.Exit(1)
	}

	keys := make([]opKey, 0, len(scheduleMap))
	for key := range scheduleMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].y != keys[j].y {
			return keys[i].y < keys[j].y
		}
		if keys[i].x != keys[j].x {
			return keys[i].x < keys[j].x
		}
		return keys[i].opID < keys[j].opID
	})

	ops := make([]timingOpSchedule, 0, len(keys))
	for _, key := range keys {
		cycles := scheduleMap[key]
		copied := make([]int64, len(cycles))
		copy(copied, cycles)
		ops = append(ops, timingOpSchedule{
			X:      key.x,
			Y:      key.y,
			OpID:   key.opID,
			Cycles: copied,
		})
	}

	sidecar := timingSidecar{
		SourceLog: *logPath,
		DerivedAt: time.Now().Format(time.RFC3339Nano),
		Ops:       ops,
	}

	content, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal sidecar json: %v\n", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "create output dir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, content, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write sidecar file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("derived timing sidecar written: %s\n", *outPath)
	fmt.Printf("source log: %s\n", *logPath)
	fmt.Printf("ops: %d\n", len(ops))
	fmt.Printf("total firings: %d\n", totalFirings)
}

func deriveFromLog(logPath string) (map[opKey][]int64, int64, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		_ = file.Close()
	}()

	scheduleMap := make(map[opKey][]int64)
	var totalFirings int64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		msg, _ := event["msg"].(string)
		if msg != "Inst" {
			continue
		}

		timeValue, ok := toFloat(event["Time"])
		if !ok {
			continue
		}
		x, ok := toInt(event["X"])
		if !ok {
			continue
		}
		y, ok := toInt(event["Y"])
		if !ok {
			continue
		}
		opID, ok := toInt(event["ID"])
		if !ok {
			continue
		}

		cycle := int64(math.Round(timeValue))
		key := opKey{
			x:    x,
			y:    y,
			opID: opID,
		}
		scheduleMap[key] = append(scheduleMap[key], cycle)
		totalFirings++
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}
	if totalFirings == 0 {
		return nil, 0, fmt.Errorf("no Inst events found in log")
	}
	return scheduleMap, totalFirings, nil
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case int32:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}
