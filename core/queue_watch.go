package core

import (
	"fmt"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

// QueueWatchSpec declares one queue to sample for occupancy reporting.
type QueueWatchSpec struct {
	Label     string `json:"label" yaml:"label"`
	X         int    `json:"x" yaml:"x"`
	Y         int    `json:"y" yaml:"y"`
	Kind      string `json:"kind" yaml:"kind"`
	Direction string `json:"direction" yaml:"direction"`
	Color     string `json:"color" yaml:"color"`
}

type resolvedQueueWatch struct {
	Label        string
	X            int
	Y            int
	Kind         string
	Direction    string
	DirectionIdx int
	Color        string
	ColorIdx     int
}

// ValidateQueueWatchSpecs checks queue watch definitions before runtime build.
func ValidateQueueWatchSpecs(specs []QueueWatchSpec) error {
	_, err := resolveQueueWatchSpecs(specs)
	return err
}

func resolveQueueWatchSpecs(specs []QueueWatchSpec) ([]resolvedQueueWatch, error) {
	if len(specs) == 0 {
		return nil, nil
	}

	resolved := make([]resolvedQueueWatch, 0, len(specs))
	for idx, spec := range specs {
		watch, err := resolveQueueWatchSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("queue watch[%d]: %w", idx, err)
		}
		resolved = append(resolved, watch)
	}
	return resolved, nil
}

func matchingQueueWatchesForTile(enabled bool, queueWatches []resolvedQueueWatch, x, y int) []resolvedQueueWatch {
	if !enabled || len(queueWatches) == 0 {
		return nil
	}

	var matched []resolvedQueueWatch
	for _, watch := range queueWatches {
		if watch.X == x && watch.Y == y {
			matched = append(matched, watch)
		}
	}
	return matched
}

func cloneQueueWatches(input []resolvedQueueWatch) []resolvedQueueWatch {
	if len(input) == 0 {
		return nil
	}
	out := make([]resolvedQueueWatch, len(input))
	copy(out, input)
	return out
}

func resolveQueueWatchSpec(spec QueueWatchSpec) (resolvedQueueWatch, error) {
	kind := strings.ToLower(strings.TrimSpace(spec.Kind))
	if kind != "recv" && kind != "send" {
		return resolvedQueueWatch{}, fmt.Errorf("invalid kind %q", spec.Kind)
	}

	directionIdx, directionName, err := resolveQueueWatchDirection(spec.Direction)
	if err != nil {
		return resolvedQueueWatch{}, err
	}

	colorIdx, colorName, err := resolveQueueWatchColor(spec.Color)
	if err != nil {
		return resolvedQueueWatch{}, err
	}

	label := strings.TrimSpace(spec.Label)
	if label == "" {
		label = fmt.Sprintf("%s(%d,%d).%s.%s", kind, spec.X, spec.Y, directionName, colorName)
	}

	return resolvedQueueWatch{
		Label:        label,
		X:            spec.X,
		Y:            spec.Y,
		Kind:         kind,
		Direction:    directionName,
		DirectionIdx: directionIdx,
		Color:        colorName,
		ColorIdx:     colorIdx,
	}, nil
}

func resolveQueueWatchDirection(raw string) (int, string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "north":
		return int(cgra.North), cgra.North.Name(), nil
	case "east":
		return int(cgra.East), cgra.East.Name(), nil
	case "south":
		return int(cgra.South), cgra.South.Name(), nil
	case "west":
		return int(cgra.West), cgra.West.Name(), nil
	case "northeast":
		return int(cgra.NorthEast), cgra.NorthEast.Name(), nil
	case "northwest":
		return int(cgra.NorthWest), cgra.NorthWest.Name(), nil
	case "southeast":
		return int(cgra.SouthEast), cgra.SouthEast.Name(), nil
	case "southwest":
		return int(cgra.SouthWest), cgra.SouthWest.Name(), nil
	case "router":
		return int(cgra.Router), cgra.Router.Name(), nil
	default:
		return 0, "", fmt.Errorf("invalid direction %q", raw)
	}
}

func resolveQueueWatchColor(raw string) (int, string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "R", "RED":
		return 0, "RED", nil
	case "Y", "YELLOW":
		return 1, "YELLOW", nil
	case "B", "BLUE":
		return 2, "BLUE", nil
	default:
		return 0, "", fmt.Errorf("invalid color %q", raw)
	}
}
