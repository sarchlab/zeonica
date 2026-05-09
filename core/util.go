package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	// PrintToggle enables verbose state table printing in debugging.
	PrintToggle = false
	// LevelTrace is a custom trace level below debug/info.
	LevelTrace slog.Level = slog.LevelDebug - 4
)

// TraceObservation captures the subset of a trace event needed for report generation.
type TraceObservation struct {
	WallTime  time.Time
	Msg       string
	Behavior  string
	Time      *float64
	ID        *int
	OpCode    string
	Pred      *bool
	Addr      *uint64
	Data      any
	X         *int
	Y         *int
	Src       string
	Dst       string
	From      string
	To        string
	Label     string
	Kind      string
	Direction string
	Color     string
	Occupancy *int
	Capacity  *int
}

var traceEnabled atomic.Bool
var traceObserver func(TraceObservation)

func init() {
	traceEnabled.Store(true)
}

// SetTraceEnabled controls whether trace events are written to the slog trace handler.
func SetTraceEnabled(enabled bool) {
	traceEnabled.Store(enabled)
}

// TraceEnabled reports whether trace output is enabled.
func TraceEnabled() bool {
	return traceEnabled.Load()
}

// DebugEnabled reports whether debug logging is enabled on the default logger.
func DebugEnabled() bool {
	return slog.Default().Enabled(context.Background(), slog.LevelDebug)
}

// SetTraceObserver registers a report observer for trace events.
func SetTraceObserver(observer func(TraceObservation)) {
	traceObserver = observer
}

// Trace writes a trace-level structured log record.
func Trace(msg string, args ...any) {
	if traceObserver != nil {
		if observation, valid := buildTraceObservation(msg, args...); valid {
			traceObserver(observation)
		}
	}
	if !TraceEnabled() {
		return
	}
	slog.Log(context.Background(), LevelTrace, msg, args...)
}

// ObserveDataFlow records a dataflow event for report generation without emitting trace output.
func ObserveDataFlow(behavior string, timeValue float64, from, to, src, dst string, attrs ...any) {
	observation := TraceObservation{
		WallTime: time.Now(),
		Msg:      "DataFlow",
		Behavior: behavior,
		Time:     float64Ptr(timeValue),
		From:     from,
		To:       to,
		Src:      src,
		Dst:      dst,
	}
	assignObservationFields(&observation, attrs...)
	observeTrace(observation)
}

// ObserveMemory records a memory event for report generation without emitting trace output.
func ObserveMemory(behavior string, timeValue float64, x, y int, src, dst string, attrs ...any) {
	observation := TraceObservation{
		WallTime: time.Now(),
		Msg:      "Memory",
		Behavior: behavior,
		Time:     float64Ptr(timeValue),
		X:        intPtr(x),
		Y:        intPtr(y),
		Src:      src,
		Dst:      dst,
	}
	assignObservationFields(&observation, attrs...)
	observeTrace(observation)
}

// ObserveInst records an instruction event for report generation without emitting trace output.
func ObserveInst(timeValue float64, x, y int, attrs ...any) {
	observation := TraceObservation{
		WallTime: time.Now(),
		Msg:      "Inst",
		Time:     float64Ptr(timeValue),
		X:        intPtr(x),
		Y:        intPtr(y),
	}
	assignObservationFields(&observation, attrs...)
	observeTrace(observation)
}

// ObserveBackpressure records a backpressure event for report generation without emitting trace output.
func ObserveBackpressure(timeValue float64, x, y int) {
	observeTrace(TraceObservation{
		WallTime: time.Now(),
		Msg:      "Backpressure",
		Time:     float64Ptr(timeValue),
		X:        intPtr(x),
		Y:        intPtr(y),
	})
}

// ObserveQueue records a watched queue occupancy sample for report generation.
func ObserveQueue(label, kind string, timeValue float64, x, y int, direction, color string, occupancy, capacity int) {
	observeTrace(TraceObservation{
		WallTime:  time.Now(),
		Msg:       "Queue",
		Behavior:  "sample",
		Time:      float64Ptr(timeValue),
		X:         intPtr(x),
		Y:         intPtr(y),
		Label:     label,
		Kind:      kind,
		Direction: direction,
		Color:     color,
		Occupancy: intPtr(occupancy),
		Capacity:  intPtr(capacity),
	})
}

func observeTrace(observation TraceObservation) {
	if traceObserver != nil {
		traceObserver(observation)
	}
}

//nolint:gocyclo
func buildTraceObservation(msg string, args ...any) (TraceObservation, bool) {
	observation := TraceObservation{
		WallTime: time.Now(),
		Msg:      msg,
	}
	if msg != "Inst" && msg != "Memory" && msg != "DataFlow" && msg != "Backpressure" && msg != "Stall" && msg != "Queue" {
		return observation, false
	}

	for i := 0; i < len(args); i++ {
		switch value := args[i].(type) {
		case slog.Attr:
			assignObservationField(&observation, value.Key, value.Value.Any())
		case string:
			if i+1 >= len(args) {
				continue
			}
			assignObservationField(&observation, value, args[i+1])
			i++
		}
	}

	return observation, true
}

func assignObservationFields(observation *TraceObservation, args ...any) {
	for i := 0; i < len(args); i++ {
		switch value := args[i].(type) {
		case slog.Attr:
			assignObservationField(observation, value.Key, value.Value.Any())
		case string:
			if i+1 >= len(args) {
				continue
			}
			assignObservationField(observation, value, args[i+1])
			i++
		}
	}
}

//nolint:gocyclo
func assignObservationField(observation *TraceObservation, key string, value any) {
	switch key {
	case "Behavior":
		observation.Behavior = fmt.Sprint(value)
	case "Time":
		if converted, ok := toFloat64(value); ok {
			observation.Time = float64Ptr(converted)
		}
	case "ID", "OpID":
		if converted, ok := toInt(value); ok {
			observation.ID = intPtr(converted)
		}
	case "OpCode":
		observation.OpCode = fmt.Sprint(value)
	case "Pred":
		if converted, ok := toBool(value); ok {
			observation.Pred = boolPtr(converted)
		}
	case "Addr", "PhysAddr":
		if converted, ok := toUint64(value); ok {
			observation.Addr = uint64Ptr(converted)
		}
	case "Data":
		observation.Data = value
	case "X":
		if converted, ok := toInt(value); ok {
			observation.X = intPtr(converted)
		}
	case "Y":
		if converted, ok := toInt(value); ok {
			observation.Y = intPtr(converted)
		}
	case "Src":
		observation.Src = fmt.Sprint(value)
	case "Dst":
		observation.Dst = fmt.Sprint(value)
	case "From":
		observation.From = fmt.Sprint(value)
	case "To":
		observation.To = fmt.Sprint(value)
	case "Label":
		observation.Label = fmt.Sprint(value)
	case "Kind":
		observation.Kind = fmt.Sprint(value)
	case "Direction":
		observation.Direction = fmt.Sprint(value)
	case "Color":
		observation.Color = fmt.Sprint(value)
	case "Occupancy":
		if converted, ok := toInt(value); ok {
			observation.Occupancy = intPtr(converted)
		}
	case "Capacity":
		if converted, ok := toInt(value); ok {
			observation.Capacity = intPtr(converted)
		}
	}
}

func toBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	default:
		return false, false
	}
}

func toFloat64(value any) (float64, bool) {
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
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	default:
		return 0, false
	}
}

func toUint64(value any) (uint64, bool) {
	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int32:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case int64:
		if typed < 0 {
			return 0, false
		}
		return uint64(typed), true
	case uint:
		return uint64(typed), true
	case uint32:
		return uint64(typed), true
	case uint64:
		return typed, true
	default:
		return 0, false
	}
}

func intPtr(value int) *int {
	ptr := new(int)
	*ptr = value
	return ptr
}

func float64Ptr(value float64) *float64 {
	ptr := new(float64)
	*ptr = value
	return ptr
}

func boolPtr(value bool) *bool {
	ptr := new(bool)
	*ptr = value
	return ptr
}

func uint64Ptr(value uint64) *uint64 {
	ptr := new(uint64)
	*ptr = value
	return ptr
}

func int64Ptr(value int64) *int64 {
	ptr := new(int64)
	*ptr = value
	return ptr
}

// PrintState prints a formatted snapshot of core runtime state.
//
//nolint:gocyclo,funlen
func PrintState(state *coreState) {
	if !PrintToggle {
		return
	}
	fmt.Printf("==============State@(%d, %d)==============\n", state.TileX, state.TileY)

	// Create register table
	regTable := table.NewWriter()
	regTable.SetTitle("Registers (32 registers in 4 rows)")

	// Add table header
	regTable.AppendHeader(table.Row{"Row", "R0-R7", "R8-R15", "R16-R23", "R24-R31"})

	// Add 4 rows of register data
	for row := 0; row < 4; row++ {
		regRow := make([]interface{}, 5)
		regRow[0] = fmt.Sprintf("Row%d", row)
		for col := 0; col < 4; col++ {
			startReg := row*8 + col*8
			regValues := ""
			for i := 0; i < 8; i++ {
				if i > 0 {
					regValues += " "
				}
				regValues += fmt.Sprintf("%d", int32(state.Registers[startReg+i].First()))
			}
			regRow[col+1] = regValues
		}
		regTable.AppendRow(regRow)
	}

	fmt.Println(regTable.Render())
	fmt.Println()

	// Create buffer table
	bufTable := table.NewWriter()
	bufTable.SetTitle("Buffer Status")

	// Direction names
	directions := []string{"N", "E", "S", "W", "NE", "NW", "SE", "SW", "R", "D1", "D2", "D3"}

	// Add table header
	header := []interface{}{"Buffer Type"}
	for _, dir := range directions {
		header = append(header, dir)
	}
	bufTable.AppendHeader(header)

	// RecvBufHead (red data)
	recvRedRow := []interface{}{"RecvBufHead[Red]"}
	for i := 0; i < 12; i++ {
		recvRedRow = append(recvRedRow, int32(state.RecvBufHead[0][i].First()))
	}
	bufTable.AppendRow(recvRedRow)

	// RecvBufHead (yellow data)
	recvYellowRow := []interface{}{"RecvBufHead[Yellow]"}
	for i := 0; i < 12; i++ {
		recvYellowRow = append(recvYellowRow, int32(state.RecvBufHead[1][i].First()))
	}
	bufTable.AppendRow(recvYellowRow)

	// RecvBufHead (blue data)
	recvBlueRow := []interface{}{"RecvBufHead[Blue]"}
	for i := 0; i < 12; i++ {
		recvBlueRow = append(recvBlueRow, int32(state.RecvBufHead[2][i].First()))
	}
	bufTable.AppendRow(recvBlueRow)

	// RecvBufHeadReady (red data)
	recvRedReadyRow := []interface{}{"RecvBufHeadReady[Red]"}
	for i := 0; i < 12; i++ {
		recvRedReadyRow = append(recvRedReadyRow, state.RecvBufHeadReady[0][i])
	}
	bufTable.AppendRow(recvRedReadyRow)

	// RecvBufHeadReady (yellow data)
	recvYellowReadyRow := []interface{}{"RecvBufHeadReady[Yellow]"}
	for i := 0; i < 12; i++ {
		recvYellowReadyRow = append(recvYellowReadyRow, state.RecvBufHeadReady[1][i])
	}
	bufTable.AppendRow(recvYellowReadyRow)

	// RecvBufHeadReady (blue data)
	recvBlueReadyRow := []interface{}{"RecvBufHeadReady[Blue]"}
	for i := 0; i < 12; i++ {
		recvBlueReadyRow = append(recvBlueReadyRow, state.RecvBufHeadReady[2][i])
	}
	bufTable.AppendRow(recvBlueReadyRow)

	// SendBufHead (red data)
	sendRedRow := []interface{}{"SendBufHead[Red]"}
	for i := 0; i < 12; i++ {
		sendRedRow = append(sendRedRow, int32(state.SendBufHead[0][i].First()))
	}
	bufTable.AppendRow(sendRedRow)

	// SendBufHead (yellow data)
	sendYellowRow := []interface{}{"SendBufHead[Yellow]"}
	for i := 0; i < 12; i++ {
		sendYellowRow = append(sendYellowRow, int32(state.SendBufHead[1][i].First()))
	}
	bufTable.AppendRow(sendYellowRow)

	// SendBufHead (blue data)
	sendBlueRow := []interface{}{"SendBufHead[Blue]"}
	for i := 0; i < 12; i++ {
		sendBlueRow = append(sendBlueRow, int32(state.SendBufHead[2][i].First()))
	}
	bufTable.AppendRow(sendBlueRow)

	// SendBufHeadBusy (red data)
	sendRedBusyRow := []interface{}{"SendBufHeadBusy[Red]"}
	for i := 0; i < 12; i++ {
		sendRedBusyRow = append(sendRedBusyRow, state.SendBufHeadBusy[0][i])
	}
	bufTable.AppendRow(sendRedBusyRow)

	// SendBufHeadBusy (yellow data)
	sendYellowBusyRow := []interface{}{"SendBufHeadBusy[Yellow]"}
	for i := 0; i < 12; i++ {
		sendYellowBusyRow = append(sendYellowBusyRow, state.SendBufHeadBusy[1][i])
	}
	bufTable.AppendRow(sendYellowBusyRow)

	// SendBufHeadBusy (blue data)
	sendBlueBusyRow := []interface{}{"SendBufHeadBusy[Blue]"}
	for i := 0; i < 12; i++ {
		sendBlueBusyRow = append(sendBlueBusyRow, state.SendBufHeadBusy[2][i])
	}
	bufTable.AppendRow(sendBlueBusyRow)

	fmt.Println(bufTable.Render())
	fmt.Println("================================================")
}

// LogState writes a structured debug checkpoint for the core state.
func LogState(state *coreState) {
	slog.Debug("StateCheckpoint",
		"X", state.TileX, "Y", state.TileY,
		"PCInBlock", state.PCInBlock,
		"SelectedBlock", state.SelectedBlock,
		"Registers", state.Registers,
		"States", state.States,
		"RecvBufHead", state.RecvBufHead,
		"RecvBufHeadReady", state.RecvBufHeadReady,
		"SendBufHead", state.SendBufHead,
		"SendBufHeadBusy", state.SendBufHeadBusy,
	)
}
