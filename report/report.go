// Package report generates and prints execution summaries from trace logs.
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
	"time"

	"github.com/sarchlab/zeonica/core"
)

// GenerateOptions controls report generation behavior from a trace log.
type GenerateOptions struct {
	TestName      string
	LogPath       string
	GridWidth     int
	GridHeight    int
	TopN          int
	Passed        *bool
	MismatchCount *int
	EnergyModel   *EnergyModel
}

// Report is the aggregate execution summary derived from a trace log.
type Report struct {
	TestName                 string                `json:"testName,omitempty"`
	LogPath                  string                `json:"logPath"`
	Grid                     GridInfo              `json:"grid"`
	TotalCycles              int64                 `json:"totalCycles"`
	ActiveCycles             int64                 `json:"activeCyclesGlobal"`
	IdleCycles               int64                 `json:"idleCyclesGlobal"`
	Passed                   *bool                 `json:"passed,omitempty"`
	MismatchCount            *int                  `json:"mismatchCount,omitempty"`
	InstCount                int64                 `json:"instCount"`
	SendCount                int64                 `json:"sendCount"`
	RecvCount                int64                 `json:"recvCount"`
	MemoryCount              int64                 `json:"memoryCount"`
	TotalEvents              int64                 `json:"totalEvents"`
	WallClockDurationSec     float64               `json:"wallClockDurationSec"`
	InstThroughputPerCycle   float64               `json:"instThroughputPerCycle"`
	EventThroughputPerCycle  float64               `json:"eventThroughputPerCycle"`
	InstThroughputPerSec     float64               `json:"instThroughputPerSec"`
	BackpressureCount        int64                 `json:"backpressureCount"`
	BackpressureCycles       int64                 `json:"backpressureCycles"`
	ScheduleBubbleStallCount int64                 `json:"scheduleBubbleStallCount"`
	OperandWaitStallCount    int64                 `json:"operandWaitStallCount"`
	OutputBlockedStallCount  int64                 `json:"outputBlockedStallCount"`
	ActiveTileCount          int                   `json:"activeTileCount"`
	Tiles                    []TileStats           `json:"tiles"`
	TopHotTiles              []TopHotTile          `json:"topHotTiles"`
	TopBackpressureTiles     []TopBackpressureTile `json:"topBackpressureTiles"`
	WatchedQueues            []QueueStats          `json:"watchedQueues,omitempty"`
	Energy                   *EnergyReport         `json:"energy,omitempty"`
}

// GridInfo describes the grid size used by the workload.
type GridInfo struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TileStats stores per-tile metrics in the generated report.
type TileStats struct {
	X                        int     `json:"x"`
	Y                        int     `json:"y"`
	Coord                    string  `json:"coord"`
	ActiveCycles             int64   `json:"activeCycles"`
	UtilizationPct           float64 `json:"utilizationPct"`
	InstCount                int64   `json:"instCount"`
	SendCount                int64   `json:"sendCount"`
	RecvCount                int64   `json:"recvCount"`
	MemoryCount              int64   `json:"memoryCount"`
	TotalEvents              int64   `json:"totalEvents"`
	BackpressureCount        int64   `json:"backpressureCount"`
	ScheduleBubbleStallCount int64   `json:"scheduleBubbleStallCount"`
	OperandWaitStallCount    int64   `json:"operandWaitStallCount"`
	OutputBlockedStallCount  int64   `json:"outputBlockedStallCount"`
}

// TopHotTile is a ranked hot tile summary entry.
type TopHotTile struct {
	X              int     `json:"x"`
	Y              int     `json:"y"`
	Coord          string  `json:"coord"`
	UtilizationPct float64 `json:"utilizationPct"`
	ActiveCycles   int64   `json:"activeCycles"`
	TotalEvents    int64   `json:"totalEvents"`
}

// TopBackpressureTile is a ranked backpressure hot tile entry.
type TopBackpressureTile struct {
	X                 int    `json:"x"`
	Y                 int    `json:"y"`
	Coord             string `json:"coord"`
	BackpressureCount int64  `json:"backpressureCount"`
}

// QueueStats stores aggregated occupancy metrics for one watched queue.
type QueueStats struct {
	Label             string  `json:"label"`
	Kind              string  `json:"kind"`
	X                 int     `json:"x"`
	Y                 int     `json:"y"`
	Coord             string  `json:"coord"`
	Direction         string  `json:"direction"`
	Color             string  `json:"color"`
	Capacity          int     `json:"capacity"`
	SampleCount       int64   `json:"sampleCount"`
	AvgOccupancy      float64 `json:"avgOccupancy"`
	PeakOccupancy     int     `json:"peakOccupancy"`
	AvgUtilizationPct float64 `json:"avgUtilizationPct"`
}

type traceEvent struct {
	Timestamp string   `json:"time"`
	Msg       string   `json:"msg"`
	Behavior  string   `json:"Behavior"`
	Time      *float64 `json:"Time"`
	ID        *int     `json:"ID"`
	OpID      *int     `json:"OpID"`
	OpCode    string   `json:"OpCode"`
	Pred      *bool    `json:"Pred"`
	Addr      *uint64  `json:"Addr"`
	PhysAddr  *uint64  `json:"PhysAddr"`
	Data      any      `json:"Data,omitempty"`
	X         *int     `json:"X"`
	Y         *int     `json:"Y"`
	Src       string   `json:"Src"`
	Dst       string   `json:"Dst"`
	From      string   `json:"From"`
	To        string   `json:"To"`
	Label     string   `json:"Label"`
	Kind      string   `json:"Kind"`
	Direction string   `json:"Direction"`
	Color     string   `json:"Color"`
	Occupancy *int     `json:"Occupancy"`
	Capacity  *int     `json:"Capacity"`
}

type tileCoord struct {
	x int
	y int
}

type tileAccumulator struct {
	cycles                   map[int64]struct{}
	backpressureCycles       map[int64]struct{}
	instCount                int64
	sendCount                int64
	recvCount                int64
	memoryCount              int64
	totalEvents              int64
	backpressureCount        int64
	scheduleBubbleStallCount int64
	operandWaitStallCount    int64
	outputBlockedStallCount  int64
}

type queueKey struct {
	label     string
	x         int
	y         int
	kind      string
	direction string
	color     string
}

type queueAccumulator struct {
	capacity      int
	sampleCount   int64
	occupancySum  int64
	peakOccupancy int
}

type collector struct {
	tileData                 map[tileCoord]*tileAccumulator
	queueData                map[queueKey]*queueAccumulator
	energyEvents             []energyEvent
	globalCycleSet           map[int64]struct{}
	globalBackpressureCycles map[int64]struct{}
	maxCycle                 int64
	maxX                     int
	maxY                     int
	globalBackpressureCount  int64
	minWallTS                *time.Time
	maxWallTS                *time.Time
}

type energyEvent struct {
	event    traceEvent
	coord    tileCoord
	hasCoord bool
}

// Observer collects report statistics directly from runtime trace observations.
type Observer struct {
	collector *collector
}

var tileEndpointPattern = regexp.MustCompile(`Device\.Tile\[(\d+)\]\[(\d+)\]\.Core\.`)

// NewObserver creates a report observer for runtime trace events.
func NewObserver() *Observer {
	return &Observer{
		collector: newCollector(),
	}
}

func newCollector() *collector {
	return &collector{
		tileData:                 make(map[tileCoord]*tileAccumulator),
		queueData:                make(map[queueKey]*queueAccumulator),
		globalCycleSet:           make(map[int64]struct{}),
		globalBackpressureCycles: make(map[int64]struct{}),
		maxCycle:                 -1,
		maxX:                     -1,
		maxY:                     -1,
	}
}

// Observe records a runtime trace observation into the in-memory report collector.
func (o *Observer) Observe(observation core.TraceObservation) {
	if o == nil || o.collector == nil {
		return
	}

	event := traceEvent{
		Timestamp: observation.WallTime.Format(time.RFC3339Nano),
		Msg:       observation.Msg,
		Behavior:  observation.Behavior,
		Time:      observation.Time,
		ID:        observation.ID,
		OpCode:    observation.OpCode,
		Pred:      observation.Pred,
		Addr:      observation.Addr,
		Data:      observation.Data,
		X:         observation.X,
		Y:         observation.Y,
		Src:       observation.Src,
		Dst:       observation.Dst,
		From:      observation.From,
		To:        observation.To,
		Label:     observation.Label,
		Kind:      observation.Kind,
		Direction: observation.Direction,
		Color:     observation.Color,
		Occupancy: observation.Occupancy,
		Capacity:  observation.Capacity,
	}
	o.collector.observe(event)
}

// Build materializes a Report using the collected runtime events.
func (o *Observer) Build(opts GenerateOptions) Report {
	if o == nil || o.collector == nil {
		return Report{
			TestName: opts.TestName,
			LogPath:  opts.LogPath,
			Grid: GridInfo{
				Width:  opts.GridWidth,
				Height: opts.GridHeight,
			},
			Passed:        opts.Passed,
			MismatchCount: opts.MismatchCount,
		}
	}
	return o.collector.build(opts)
}

//nolint:gocyclo
func (c *collector) observe(event traceEvent) {
	if ts, err := time.Parse(time.RFC3339Nano, event.Timestamp); err == nil {
		if c.minWallTS == nil || ts.Before(*c.minWallTS) {
			t := ts
			c.minWallTS = &t
		}
		if c.maxWallTS == nil || ts.After(*c.maxWallTS) {
			t := ts
			c.maxWallTS = &t
		}
	}

	cycle, hasCycle := parseCycle(event.Time)
	if hasCycle && cycle > c.maxCycle {
		c.maxCycle = cycle
	}

	if event.Msg == "Queue" {
		c.observeQueue(event)
		return
	}

	coord, ok := resolveTileCoord(event)
	if !ok {
		c.observeEnergyEvent(event, tileCoord{}, false)
		return
	}
	c.observeEnergyEvent(event, coord, true)

	if coord.x > c.maxX {
		c.maxX = coord.x
	}
	if coord.y > c.maxY {
		c.maxY = coord.y
	}

	acc, exists := c.tileData[coord]
	if !exists {
		acc = &tileAccumulator{
			cycles:             make(map[int64]struct{}),
			backpressureCycles: make(map[int64]struct{}),
		}
		c.tileData[coord] = acc
	}

	isBackpressureEvent := event.Msg == "Backpressure"
	if hasCycle && !isBackpressureEvent {
		acc.cycles[cycle] = struct{}{}
		c.globalCycleSet[cycle] = struct{}{}
		if cycle > c.maxCycle {
			c.maxCycle = cycle
		}
	}

	if classifyAndCount(event, acc, cycle, hasCycle) {
		c.globalBackpressureCount++
		if hasCycle {
			c.globalBackpressureCycles[cycle] = struct{}{}
		}
	}
}

func (c *collector) observeEnergyEvent(event traceEvent, coord tileCoord, hasCoord bool) {
	switch event.Msg {
	case "Inst", "DataFlow", "Memory":
		c.energyEvents = append(c.energyEvents, energyEvent{event: event, coord: coord, hasCoord: hasCoord})
	}
}

func (c *collector) observeQueue(event traceEvent) {
	if event.X == nil || event.Y == nil || event.Occupancy == nil {
		return
	}

	key := queueKey{
		label:     event.Label,
		x:         *event.X,
		y:         *event.Y,
		kind:      event.Kind,
		direction: event.Direction,
		color:     event.Color,
	}

	acc, exists := c.queueData[key]
	if !exists {
		acc = &queueAccumulator{}
		c.queueData[key] = acc
	}
	if event.Capacity != nil && *event.Capacity > 0 {
		acc.capacity = *event.Capacity
	}
	acc.sampleCount++
	acc.occupancySum += int64(*event.Occupancy)
	if *event.Occupancy > acc.peakOccupancy {
		acc.peakOccupancy = *event.Occupancy
	}
}

//nolint:gocyclo,funlen
func (c *collector) build(opts GenerateOptions) Report {
	topN := opts.TopN
	if topN <= 0 {
		topN = 5
	}

	totalCycles := int64(0)
	if c.maxCycle >= 0 {
		totalCycles = c.maxCycle + 1
	}

	activeCycles := int64(len(c.globalCycleSet))
	idleCycles := totalCycles - activeCycles
	if idleCycles < 0 {
		idleCycles = 0
	}

	width := opts.GridWidth
	if width <= 0 {
		width = c.maxX + 1
	}
	height := opts.GridHeight
	if height <= 0 {
		height = c.maxY + 1
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}

	tiles := make([]TileStats, 0, len(c.tileData))
	for coord, acc := range c.tileData {
		activeTileCycles := int64(len(acc.cycles))
		util := 0.0
		if totalCycles > 0 {
			util = float64(activeTileCycles) * 100.0 / float64(totalCycles)
		}

		tiles = append(tiles, TileStats{
			X:                        coord.x,
			Y:                        coord.y,
			Coord:                    formatCoord(coord.x, coord.y),
			ActiveCycles:             activeTileCycles,
			UtilizationPct:           util,
			InstCount:                acc.instCount,
			SendCount:                acc.sendCount,
			RecvCount:                acc.recvCount,
			MemoryCount:              acc.memoryCount,
			TotalEvents:              acc.totalEvents,
			BackpressureCount:        acc.backpressureCount,
			ScheduleBubbleStallCount: acc.scheduleBubbleStallCount,
			OperandWaitStallCount:    acc.operandWaitStallCount,
			OutputBlockedStallCount:  acc.outputBlockedStallCount,
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
	var scheduleBubbleStallTotal int64
	var operandWaitStallTotal int64
	var outputBlockedStallTotal int64

	for _, tile := range tiles {
		instTotal += tile.InstCount
		sendTotal += tile.SendCount
		recvTotal += tile.RecvCount
		memoryTotal += tile.MemoryCount
		eventTotal += tile.TotalEvents
		scheduleBubbleStallTotal += tile.ScheduleBubbleStallCount
		operandWaitStallTotal += tile.OperandWaitStallCount
		outputBlockedStallTotal += tile.OutputBlockedStallCount
	}

	topHotTiles := buildTopHotTiles(tiles, topN)
	topBackpressureTiles := buildTopBackpressureTiles(tiles, topN)
	watchedQueues := buildQueueStats(c.queueData)
	wallClockDurationSec := 0.0
	if c.minWallTS != nil && c.maxWallTS != nil {
		d := c.maxWallTS.Sub(*c.minWallTS).Seconds()
		if d > 0 {
			wallClockDurationSec = d
		}
	}
	instThroughputPerCycle := 0.0
	eventThroughputPerCycle := 0.0
	if totalCycles > 0 {
		instThroughputPerCycle = float64(instTotal) / float64(totalCycles)
		eventThroughputPerCycle = float64(eventTotal) / float64(totalCycles)
	}
	instThroughputPerSec := 0.0
	if wallClockDurationSec > 0 {
		instThroughputPerSec = float64(instTotal) / wallClockDurationSec
	}

	result := Report{
		TestName:                 opts.TestName,
		LogPath:                  opts.LogPath,
		Grid:                     GridInfo{Width: width, Height: height},
		TotalCycles:              totalCycles,
		ActiveCycles:             activeCycles,
		IdleCycles:               idleCycles,
		Passed:                   opts.Passed,
		MismatchCount:            opts.MismatchCount,
		InstCount:                instTotal,
		SendCount:                sendTotal,
		RecvCount:                recvTotal,
		MemoryCount:              memoryTotal,
		TotalEvents:              eventTotal,
		WallClockDurationSec:     wallClockDurationSec,
		InstThroughputPerCycle:   instThroughputPerCycle,
		EventThroughputPerCycle:  eventThroughputPerCycle,
		InstThroughputPerSec:     instThroughputPerSec,
		BackpressureCount:        c.globalBackpressureCount,
		BackpressureCycles:       int64(len(c.globalBackpressureCycles)),
		ScheduleBubbleStallCount: scheduleBubbleStallTotal,
		OperandWaitStallCount:    operandWaitStallTotal,
		OutputBlockedStallCount:  outputBlockedStallTotal,
		ActiveTileCount:          len(tiles),
		Tiles:                    tiles,
		TopHotTiles:              topHotTiles,
		TopBackpressureTiles:     topBackpressureTiles,
		WatchedQueues:            watchedQueues,
	}
	if opts.EnergyModel != nil && opts.EnergyModel.Enabled {
		result.Energy = BuildEnergyReport(opts.EnergyModel, c.energyEvents, tiles, totalCycles, width, height)
	}
	return result
}

// GenerateFromLog builds a report by parsing a JSON trace log.
//
//nolint:gocyclo,funlen
func GenerateFromLog(opts GenerateOptions) (Report, error) {
	if opts.LogPath == "" {
		return Report{}, fmt.Errorf("log path is required")
	}

	file, err := os.Open(opts.LogPath)
	if err != nil {
		return Report{}, fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	collector := newCollector()

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

		collector.observe(event)
	}

	if err := scanner.Err(); err != nil {
		return Report{}, fmt.Errorf("scan log file: %w", err)
	}

	return collector.build(opts), nil
}

// SaveJSON writes a report as pretty-printed JSON.
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

// PrintSummary prints a compact report summary to stdout.
func PrintSummary(report Report) {
	PrintSummaryToWriter(report, os.Stdout)
}

// PrintSummaryToWriter prints a compact report summary to the writer.
//
//nolint:funlen
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
	fmt.Fprintf(w, "simulation time: wall=%.3fs\n", report.WallClockDurationSec)
	fmt.Fprintf(
		w,
		"throughput: inst/cycle=%.4f events/cycle=%.4f inst/s=%.2f\n",
		report.InstThroughputPerCycle,
		report.EventThroughputPerCycle,
		report.InstThroughputPerSec,
	)
	fmt.Fprintf(w, "backpressure: count=%d cycles=%d\n", report.BackpressureCount, report.BackpressureCycles)
	fmt.Fprintf(
		w,
		"stall breakdown: schedule_bubble=%d operand_wait=%d output_blocked=%d\n",
		report.ScheduleBubbleStallCount,
		report.OperandWaitStallCount,
		report.OutputBlockedStallCount,
	)
	fmt.Fprintf(w, "active tiles: %d\n", report.ActiveTileCount)
	if report.Passed != nil {
		fmt.Fprintf(w, "passed: %t\n", *report.Passed)
	}
	if report.MismatchCount != nil {
		fmt.Fprintf(w, "mismatch count: %d\n", *report.MismatchCount)
	}
	printEnergySummary(report.Energy, w)

	if len(report.TopHotTiles) > 0 {
		fmt.Fprintln(w, "top hot tiles:")
		for idx, tile := range report.TopHotTiles {
			fmt.Fprintf(w, "  %d) %s util=%.2f%% activeCycles=%d totalEvents=%d\n",
				idx+1, tile.Coord, tile.UtilizationPct, tile.ActiveCycles, tile.TotalEvents)
		}
	}
	if len(report.TopBackpressureTiles) > 0 {
		fmt.Fprintln(w, "top backpressure tiles:")
		for idx, tile := range report.TopBackpressureTiles {
			fmt.Fprintf(w, "  %d) %s bp=%d\n", idx+1, tile.Coord, tile.BackpressureCount)
		}
	}
	if len(report.WatchedQueues) > 0 {
		fmt.Fprintln(w, "watched queues:")
		for idx, queue := range report.WatchedQueues {
			fmt.Fprintf(
				w,
				"  %d) %s %s %s/%s avg=%.2f peak=%d cap=%d util=%.2f%% samples=%d\n",
				idx+1,
				queue.Coord,
				queue.Label,
				queue.Direction,
				queue.Color,
				queue.AvgOccupancy,
				queue.PeakOccupancy,
				queue.Capacity,
				queue.AvgUtilizationPct,
				queue.SampleCount,
			)
		}
	}
}

func printEnergySummary(energy *EnergyReport, w io.Writer) {
	if energy == nil {
		return
	}
	fmt.Fprintf(
		w,
		"energy: ok=%t dynamic=%.6f pJ static=%.6f pJ total=%.6f pJ\n",
		energy.EstimationOK,
		energy.DynamicEnergyPJ,
		energy.StaticEnergyPJ,
		energy.TotalEnergyPJ,
	)
	if len(energy.UnknownActions) > 0 {
		fmt.Fprintf(w, "energy unknown actions: %d\n", len(energy.UnknownActions))
	}
	if len(energy.UnresolvedEvents) > 0 {
		fmt.Fprintf(w, "energy unresolved events: %d\n", len(energy.UnresolvedEvents))
	}
}

//nolint:gocyclo
func classifyAndCount(event traceEvent, acc *tileAccumulator, cycle int64, hasCycle bool) bool {
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
	case "Backpressure":
		acc.backpressureCount++
		if hasCycle {
			acc.backpressureCycles[cycle] = struct{}{}
		}
		return true
	case "Stall":
		switch event.Behavior {
		case "schedule_bubble":
			acc.scheduleBubbleStallCount++
		case "operand_wait":
			acc.operandWaitStallCount++
		case "output_blocked":
			acc.outputBlockedStallCount++
		}
		acc.totalEvents++
	case "Queue":
		// Queue samples are aggregated separately in watchedQueues and should not
		// inflate existing event throughput counters.
	}
	return false
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

	var row int
	var col int
	if _, err := fmt.Sscanf(matches[0], "Device.Tile[%d][%d].Core.", &row, &col); err != nil {
		return tileCoord{}, false
	}

	// Endpoint naming is Tile[row][col], while report coordinates are (x=col, y=row).
	return tileCoord{x: col, y: row}, true
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

func buildQueueStats(queueData map[queueKey]*queueAccumulator) []QueueStats {
	if len(queueData) == 0 {
		return nil
	}

	stats := make([]QueueStats, 0, len(queueData))
	for key, acc := range queueData {
		avgOccupancy := 0.0
		if acc.sampleCount > 0 {
			avgOccupancy = float64(acc.occupancySum) / float64(acc.sampleCount)
		}
		avgUtilizationPct := 0.0
		if acc.capacity > 0 {
			avgUtilizationPct = avgOccupancy * 100.0 / float64(acc.capacity)
		}
		stats = append(stats, QueueStats{
			Label:             key.label,
			Kind:              key.kind,
			X:                 key.x,
			Y:                 key.y,
			Coord:             formatCoord(key.x, key.y),
			Direction:         key.direction,
			Color:             key.color,
			Capacity:          acc.capacity,
			SampleCount:       acc.sampleCount,
			AvgOccupancy:      avgOccupancy,
			PeakOccupancy:     acc.peakOccupancy,
			AvgUtilizationPct: avgUtilizationPct,
		})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Y != stats[j].Y {
			return stats[i].Y < stats[j].Y
		}
		if stats[i].X != stats[j].X {
			return stats[i].X < stats[j].X
		}
		if stats[i].Direction != stats[j].Direction {
			return stats[i].Direction < stats[j].Direction
		}
		return stats[i].Label < stats[j].Label
	})

	return stats
}

func buildTopBackpressureTiles(tiles []TileStats, topN int) []TopBackpressureTile {
	if len(tiles) == 0 || topN <= 0 {
		return nil
	}
	tmp := make([]TileStats, len(tiles))
	copy(tmp, tiles)
	sort.Slice(tmp, func(i, j int) bool {
		if tmp[i].BackpressureCount != tmp[j].BackpressureCount {
			return tmp[i].BackpressureCount > tmp[j].BackpressureCount
		}
		if tmp[i].Y != tmp[j].Y {
			return tmp[i].Y < tmp[j].Y
		}
		return tmp[i].X < tmp[j].X
	})
	if topN > len(tmp) {
		topN = len(tmp)
	}
	out := make([]TopBackpressureTile, 0, topN)
	for i := 0; i < topN; i++ {
		if tmp[i].BackpressureCount <= 0 {
			continue
		}
		out = append(out, TopBackpressureTile{
			X:                 tmp[i].X,
			Y:                 tmp[i].Y,
			Coord:             tmp[i].Coord,
			BackpressureCount: tmp[i].BackpressureCount,
		})
	}
	return out
}

func formatCoord(x, y int) string {
	return fmt.Sprintf("(%d,%d)", x, y)
}
