package report

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
)

type GenerateOptions struct {
	TestName      string
	LogPath       string
	GridWidth     int
	GridHeight    int
	TopN          int
	Passed        *bool
	MismatchCount *int
}

type Report struct {
	TestName        string       `json:"testName,omitempty"`
	LogPath         string       `json:"logPath"`
	Grid            GridInfo     `json:"grid"`
	TotalCycles     int64        `json:"totalCycles"`
	ActiveCycles    int64        `json:"activeCyclesGlobal"`
	IdleCycles      int64        `json:"idleCyclesGlobal"`
	Passed          *bool        `json:"passed,omitempty"`
	MismatchCount   *int         `json:"mismatchCount,omitempty"`
	InstCount       int64        `json:"instCount"`
	SendCount       int64        `json:"sendCount"`
	RecvCount       int64        `json:"recvCount"`
	MemoryCount     int64        `json:"memoryCount"`
	TotalEvents     int64        `json:"totalEvents"`
	ActiveTileCount int          `json:"activeTileCount"`
	Tiles           []TileStats  `json:"tiles"`
	TopHotTiles     []TopHotTile `json:"topHotTiles"`
}

type GridInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type TileStats struct {
	X              int     `json:"x"`
	Y              int     `json:"y"`
	Coord          string  `json:"coord"`
	ActiveCycles   int64   `json:"activeCycles"`
	UtilizationPct float64 `json:"utilizationPct"`
	InstCount      int64   `json:"instCount"`
	SendCount      int64   `json:"sendCount"`
	RecvCount      int64   `json:"recvCount"`
	MemoryCount    int64   `json:"memoryCount"`
	TotalEvents    int64   `json:"totalEvents"`
}

type TopHotTile struct {
	X              int     `json:"x"`
	Y              int     `json:"y"`
	Coord          string  `json:"coord"`
	UtilizationPct float64 `json:"utilizationPct"`
	ActiveCycles   int64   `json:"activeCycles"`
	TotalEvents    int64   `json:"totalEvents"`
}

type traceEvent struct {
	Timestamp string   `json:"time"`
	Msg       string   `json:"msg"`
	Behavior  string   `json:"Behavior"`
	Time      *float64 `json:"Time"`
	X         *int     `json:"X"`
	Y         *int     `json:"Y"`
	Src       string   `json:"Src"`
	Dst       string   `json:"Dst"`
	From      string   `json:"From"`
	To        string   `json:"To"`
}

type tileCoord struct {
	x int
	y int
}

type tileAccumulator struct {
	cycles      map[int64]struct{}
	instCount   int64
	sendCount   int64
	recvCount   int64
	memoryCount int64
	totalEvents int64
}

var tileEndpointPattern = regexp.MustCompile(`Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.`)

func GenerateFromLog(opts GenerateOptions) (Report, error) {
	if opts.LogPath == "" {
		return Report{}, fmt.Errorf("log path is required")
	}

	topN := opts.TopN
	if topN <= 0 {
		topN = 5
	}

	file, err := os.Open(opts.LogPath)
	if err != nil {
		return Report{}, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	tileData := make(map[tileCoord]*tileAccumulator)
	globalCycleSet := make(map[int64]struct{})

	var maxCycle int64 = -1
	maxX, maxY := -1, -1

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event traceEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		coord, ok := resolveTileCoord(event)
		if !ok {
			continue
		}

		if coord.x > maxX {
			maxX = coord.x
		}
		if coord.y > maxY {
			maxY = coord.y
		}

		acc, exists := tileData[coord]
		if !exists {
			acc = &tileAccumulator{
				cycles: make(map[int64]struct{}),
			}
			tileData[coord] = acc
		}

		cycle, hasCycle := parseCycle(event.Time)
		if hasCycle {
			acc.cycles[cycle] = struct{}{}
			globalCycleSet[cycle] = struct{}{}
			if cycle > maxCycle {
				maxCycle = cycle
			}
		}

		classifyAndCount(event, acc)
	}

	if err := scanner.Err(); err != nil {
		return Report{}, fmt.Errorf("scan log file: %w", err)
	}

	totalCycles := int64(0)
	if maxCycle >= 0 {
		totalCycles = maxCycle + 1
	}

	activeCycles := int64(len(globalCycleSet))
	idleCycles := totalCycles - activeCycles
	if idleCycles < 0 {
		idleCycles = 0
	}

	width := opts.GridWidth
	if width <= 0 {
		width = maxX + 1
	}
	height := opts.GridHeight
	if height <= 0 {
		height = maxY + 1
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	tiles := make([]TileStats, 0, len(tileData))
	for coord, acc := range tileData {
		activeTileCycles := int64(len(acc.cycles))
		util := 0.0
		if totalCycles > 0 {
			util = float64(activeTileCycles) * 100.0 / float64(totalCycles)
		}

		tiles = append(tiles, TileStats{
			X:              coord.x,
			Y:              coord.y,
			Coord:          formatCoord(coord.x, coord.y),
			ActiveCycles:   activeTileCycles,
			UtilizationPct: util,
			InstCount:      acc.instCount,
			SendCount:      acc.sendCount,
			RecvCount:      acc.recvCount,
			MemoryCount:    acc.memoryCount,
			TotalEvents:    acc.totalEvents,
		})
	}

	sort.Slice(tiles, func(i, j int) bool {
		if tiles[i].Y != tiles[j].Y {
			return tiles[i].Y < tiles[j].Y
		}
		return tiles[i].X < tiles[j].X
	})

	var instTotal int64
	var sendTotal int64
	var recvTotal int64
	var memoryTotal int64
	var eventTotal int64

	for _, tile := range tiles {
		instTotal += tile.InstCount
		sendTotal += tile.SendCount
		recvTotal += tile.RecvCount
		memoryTotal += tile.MemoryCount
		eventTotal += tile.TotalEvents
	}

	topHotTiles := buildTopHotTiles(tiles, topN)

	report := Report{
		TestName:        opts.TestName,
		LogPath:         opts.LogPath,
		Grid:            GridInfo{Width: width, Height: height},
		TotalCycles:     totalCycles,
		ActiveCycles:    activeCycles,
		IdleCycles:      idleCycles,
		Passed:          opts.Passed,
		MismatchCount:   opts.MismatchCount,
		InstCount:       instTotal,
		SendCount:       sendTotal,
		RecvCount:       recvTotal,
		MemoryCount:     memoryTotal,
		TotalEvents:     eventTotal,
		ActiveTileCount: len(tiles),
		Tiles:           tiles,
		TopHotTiles:     topHotTiles,
	}

	return report, nil
}

func SaveJSON(report Report, path string) error {
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write report file: %w", err)
	}

	return nil
}

func PrintSummary(report Report) {
	PrintSummaryToWriter(report, os.Stdout)
}

func PrintSummaryToWriter(report Report, w io.Writer) {
	fmt.Fprintln(w, "========================")
	fmt.Fprintln(w, "Zeonica Report Summary")
	fmt.Fprintln(w, "========================")
	fmt.Fprintf(w, "test: %s\n", report.TestName)
	fmt.Fprintf(w, "log: %s\n", report.LogPath)
	fmt.Fprintf(w, "grid: %dx%d\n", report.Grid.Width, report.Grid.Height)
	fmt.Fprintf(w, "cycles: total=%d active=%d idle=%d\n", report.TotalCycles, report.ActiveCycles, report.IdleCycles)
	fmt.Fprintf(w, "events: total=%d inst=%d send=%d recv=%d memory=%d\n",
		report.TotalEvents, report.InstCount, report.SendCount, report.RecvCount, report.MemoryCount)
	fmt.Fprintf(w, "active tiles: %d\n", report.ActiveTileCount)
	if report.Passed != nil {
		fmt.Fprintf(w, "passed: %t\n", *report.Passed)
	}
	if report.MismatchCount != nil {
		fmt.Fprintf(w, "mismatch count: %d\n", *report.MismatchCount)
	}

	if len(report.TopHotTiles) > 0 {
		fmt.Fprintln(w, "top hot tiles:")
		for idx, tile := range report.TopHotTiles {
			fmt.Fprintf(w, "  %d) %s util=%.2f%% activeCycles=%d totalEvents=%d\n",
				idx+1, tile.Coord, tile.UtilizationPct, tile.ActiveCycles, tile.TotalEvents)
		}
	}
}

func classifyAndCount(event traceEvent, acc *tileAccumulator) {
	switch event.Msg {
	case "Inst":
		acc.instCount++
		acc.totalEvents++
	case "Memory":
		acc.memoryCount++
		acc.totalEvents++
	case "DataFlow":
		switch event.Behavior {
		case "Send", "Collect":
			acc.sendCount++
		case "Recv", "FeedIn":
			acc.recvCount++
		}
		acc.totalEvents++
	}
}

func resolveTileCoord(event traceEvent) (tileCoord, bool) {
	if event.X != nil && event.Y != nil {
		return tileCoord{x: *event.X, y: *event.Y}, true
	}

	endpoints := endpointCandidates(event)
	for _, endpoint := range endpoints {
		coord, ok := parseTileFromEndpoint(endpoint)
		if ok {
			return coord, true
		}
	}

	return tileCoord{}, false
}

func endpointCandidates(event traceEvent) []string {
	if event.Msg != "DataFlow" {
		return []string{event.Src, event.Dst, event.From, event.To}
	}

	switch event.Behavior {
	case "Send":
		return []string{event.Src, event.From, event.Dst, event.To}
	case "Recv":
		return []string{event.Dst, event.To, event.Src, event.From}
	case "FeedIn":
		return []string{event.To, event.Dst, event.Src, event.From}
	case "Collect":
		return []string{event.From, event.Src, event.Dst, event.To}
	default:
		return []string{event.Src, event.Dst, event.From, event.To}
	}
}

func parseTileFromEndpoint(endpoint string) (tileCoord, bool) {
	if endpoint == "" {
		return tileCoord{}, false
	}

	matches := tileEndpointPattern.FindStringSubmatch(endpoint)
	if len(matches) != 3 {
		return tileCoord{}, false
	}

	var x int
	var y int
	if _, err := fmt.Sscanf(matches[0], "Device.Tile[%d][%d].Core.", &x, &y); err != nil {
		return tileCoord{}, false
	}

	return tileCoord{x: x, y: y}, true
}

func parseCycle(timeValue *float64) (int64, bool) {
	if timeValue == nil {
		return 0, false
	}
	if *timeValue < 0 {
		return 0, false
	}
	return int64(math.Round(*timeValue)), true
}

func buildTopHotTiles(tiles []TileStats, topN int) []TopHotTile {
	if len(tiles) == 0 || topN <= 0 {
		return nil
	}

	tmp := make([]TileStats, len(tiles))
	copy(tmp, tiles)

	sort.Slice(tmp, func(i, j int) bool {
		if tmp[i].UtilizationPct != tmp[j].UtilizationPct {
			return tmp[i].UtilizationPct > tmp[j].UtilizationPct
		}
		if tmp[i].TotalEvents != tmp[j].TotalEvents {
			return tmp[i].TotalEvents > tmp[j].TotalEvents
		}
		if tmp[i].Y != tmp[j].Y {
			return tmp[i].Y < tmp[j].Y
		}
		return tmp[i].X < tmp[j].X
	})

	if topN > len(tmp) {
		topN = len(tmp)
	}

	out := make([]TopHotTile, 0, topN)
	for i := 0; i < topN; i++ {
		out = append(out, TopHotTile{
			X:              tmp[i].X,
			Y:              tmp[i].Y,
			Coord:          tmp[i].Coord,
			UtilizationPct: tmp[i].UtilizationPct,
			ActiveCycles:   tmp[i].ActiveCycles,
			TotalEvents:    tmp[i].TotalEvents,
		})
	}

	return out
}

func formatCoord(x, y int) string {
	return fmt.Sprintf("(%d,%d)", x, y)
}
