package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	PrintToggle                  = false
	LevelTrace        slog.Level = slog.LevelInfo + 1
	LevelWaveform     slog.Level = slog.LevelInfo + 2
	EnableWaveformLog            = true // Set to false to disable waveform logging for performance
)

// PortState represents data on a single port for a given cycle
type PortState struct {
	Direction string `json:"direction"`       // "North", "East", "South", "West", etc.
	HasData   bool   `json:"has_data"`        // true if data was received/sent this cycle
	Data      uint32 `json:"data,omitempty"`  // the actual data value (if HasData)
	Pred      bool   `json:"pred"`            // predicate bit
	Color     int    `json:"color"`           // color/channel index (0=Red, 1=Yellow, 2=Blue)
	Ready     bool   `json:"ready,omitempty"` // whether buffer was ready when checked
	Sent      bool   `json:"sent,omitempty"`  // for output: true if data was successfully sent
}

// MemOpLog represents a memory operation that occurred in this cycle
type MemOpLog struct {
	Type  string `json:"type"`           // "LOAD" | "STORE" | "READ_RESPONSE" | "WRITE_DONE"
	Addr  uint32 `json:"addr,omitempty"` // memory address
	Value uint32 `json:"value"`          // data value
	Port  string `json:"port,omitempty"` // which port (e.g., "North", "Router", "Local")
}

// BlockReason provides structured information about why a PE is blocked
type BlockReason struct {
	Code      string `json:"code"`                // e.g., "RECV_NOT_READY", "SEND_BUF_BUSY", "NO_INST"
	OpCode    string `json:"opcode,omitempty"`    // the instruction that's blocked
	Direction string `json:"direction,omitempty"` // which direction caused the block
	Color     int    `json:"color,omitempty"`     // which color caused the block
	DirIdx    int    `json:"dir_idx,omitempty"`   // direction index (0-7)
	ColorIdx  int    `json:"color_idx,omitempty"` // color index (0-3)
	Message   string `json:"message"`             // human-readable reason
}

// PEStateLog is the canonical summary for one PE at one cycle
type PEStateLog struct {
	Time        float64      `json:"time"`                    // cycle time (in ns)
	X           uint32       `json:"x"`                       // PE X coordinate
	Y           uint32       `json:"y"`                       // PE Y coordinate
	PC          int32        `json:"pc,omitempty"`            // program counter / PCInBlock
	OpCode      string       `json:"opcode,omitempty"`        // current instruction
	InstGroupID int32        `json:"inst_group_id,omitempty"` // instruction group index
	Status      string       `json:"status"`                  // "Running" | "Blocked" | "Idle"
	BlockReason *BlockReason `json:"block_reason,omitempty"`  // if Status == "Blocked"
	Inputs      []PortState  `json:"inputs"`                  // received data this cycle
	Outputs     []PortState  `json:"outputs"`                 // sent data this cycle
	MemoryOps   []MemOpLog   `json:"memory_ops,omitempty"`    // LOAD/STORE this cycle
}

// CycleAccumulator collects all activities for a PE during one cycle
type CycleAccumulator struct {
	PC          int32        // PCInBlock at start of cycle
	OpCode      string       // opcode being attempted
	Status      string       // "Running" | "Blocked" | "Idle"
	BlockReason *BlockReason // reason for blocking
	Inputs      []PortState  // data received this cycle
	Outputs     []PortState  // data sent this cycle
	MemoryOps   []MemOpLog   // memory operations
	Changed     bool         // true if accumulator has meaningful updates this cycle
}

// NewCycleAccumulator creates a fresh accumulator for a new cycle
func NewCycleAccumulator() *CycleAccumulator {
	return &CycleAccumulator{
		PC:        -1,
		OpCode:    "",
		Status:    "Idle",
		Inputs:    make([]PortState, 0),
		Outputs:   make([]PortState, 0),
		MemoryOps: make([]MemOpLog, 0),
		Changed:   false,
	}
}

// LogPEState emits a canonical PEState summary log entry for a PE at the current cycle
func LogPEState(time float64, x, y uint32, acc *CycleAccumulator) {
	if !EnableWaveformLog {
		return
	}

	peState := &PEStateLog{
		Time:        time,
		X:           x,
		Y:           y,
		PC:          acc.PC,
		OpCode:      acc.OpCode,
		Status:      acc.Status,
		BlockReason: acc.BlockReason,
		Inputs:      acc.Inputs,
		Outputs:     acc.Outputs,
		MemoryOps:   acc.MemoryOps,
	}

	slog.Log(context.Background(), LevelWaveform, "PEState", slog.Any("state", peState))
}

// UpdateCycleAccumulatorFromCheckFlags updates accumulator with blocking information from CheckFlags
func UpdateCycleAccumulatorFromCheckFlags(acc *CycleAccumulator, opCode string, pc int32, isBlocked bool, blockReason *BlockReason) {
	acc.PC = pc
	acc.OpCode = opCode
	if isBlocked {
		acc.Status = "Blocked"
		acc.BlockReason = blockReason
		acc.Changed = true
	} else if acc.Status != "Running" {
		// Only update to Running if we weren't already blocked
		acc.Status = "Running"
		acc.Changed = true
	}
}

// AddInputPort adds a received data entry to the accumulator
func (acc *CycleAccumulator) AddInputPort(direction string, hasData bool, data uint32, pred bool, color int, ready bool) {
	if hasData || ready { // Only add if there's something meaningful
		acc.Inputs = append(acc.Inputs, PortState{
			Direction: direction,
			HasData:   hasData,
			Data:      data,
			Pred:      pred,
			Color:     color,
			Ready:     ready,
		})
		acc.Changed = true
	}
}

// AddOutputPort adds a sent data entry to the accumulator
func (acc *CycleAccumulator) AddOutputPort(direction string, hasData bool, data uint32, pred bool, color int, sent bool) {
	if hasData || sent { // Only add if there's something meaningful
		acc.Outputs = append(acc.Outputs, PortState{
			Direction: direction,
			HasData:   hasData,
			Data:      data,
			Pred:      pred,
			Color:     color,
			Sent:      sent,
		})
		acc.Changed = true
	}
}

// AddMemoryOp adds a memory operation to the accumulator
func (acc *CycleAccumulator) AddMemoryOp(opType string, addr uint32, value uint32, port string) {
	acc.MemoryOps = append(acc.MemoryOps, MemOpLog{
		Type:  opType,
		Addr:  addr,
		Value: value,
		Port:  port,
	})
	acc.Changed = true
}

func Trace(msg string, args ...any) {
	slog.Log(context.Background(), LevelTrace, msg, args...)
}

func PrintState(state *coreState) {
	if !PrintToggle {
		return
	}
	fmt.Printf("==============State@(%d, %d)==============\n", state.TileX, state.TileY)

	// 创建寄存器表格
	regTable := table.NewWriter()
	regTable.SetTitle("Registers (32 registers in 4 rows)")

	// 添加表头
	regTable.AppendHeader(table.Row{"Row", "R0-R7", "R8-R15", "R16-R23", "R24-R31"})

	// 添加4行寄存器数据
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

	// 创建缓冲区表格
	bufTable := table.NewWriter()
	bufTable.SetTitle("Buffer Status")

	// 方向名称
	directions := []string{"N", "E", "S", "W", "NE", "NW", "SE", "SW", "R", "D1", "D2", "D3"}

	// 添加表头
	header := []interface{}{"Buffer Type"}
	for _, dir := range directions {
		header = append(header, dir)
	}
	bufTable.AppendHeader(header)

	// RecvBufHead (红色数据)
	recvRedRow := []interface{}{"RecvBufHead[Red]"}
	for i := 0; i < 12; i++ {
		recvRedRow = append(recvRedRow, int32(state.RecvBufHead[0][i].First()))
	}
	bufTable.AppendRow(recvRedRow)

	// RecvBufHead (黄色数据)
	recvYellowRow := []interface{}{"RecvBufHead[Yellow]"}
	for i := 0; i < 12; i++ {
		recvYellowRow = append(recvYellowRow, int32(state.RecvBufHead[1][i].First()))
	}
	bufTable.AppendRow(recvYellowRow)

	// RecvBufHead (蓝色数据)
	recvBlueRow := []interface{}{"RecvBufHead[Blue]"}
	for i := 0; i < 12; i++ {
		recvBlueRow = append(recvBlueRow, int32(state.RecvBufHead[2][i].First()))
	}
	bufTable.AppendRow(recvBlueRow)

	// RecvBufHeadReady (红色数据)
	recvRedReadyRow := []interface{}{"RecvBufHeadReady[Red]"}
	for i := 0; i < 12; i++ {
		recvRedReadyRow = append(recvRedReadyRow, state.RecvBufHeadReady[0][i])
	}
	bufTable.AppendRow(recvRedReadyRow)

	// RecvBufHeadReady (黄色数据)
	recvYellowReadyRow := []interface{}{"RecvBufHeadReady[Yellow]"}
	for i := 0; i < 12; i++ {
		recvYellowReadyRow = append(recvYellowReadyRow, state.RecvBufHeadReady[1][i])
	}
	bufTable.AppendRow(recvYellowReadyRow)

	// RecvBufHeadReady (蓝色数据)
	recvBlueReadyRow := []interface{}{"RecvBufHeadReady[Blue]"}
	for i := 0; i < 12; i++ {
		recvBlueReadyRow = append(recvBlueReadyRow, state.RecvBufHeadReady[2][i])
	}
	bufTable.AppendRow(recvBlueReadyRow)

	// SendBufHead (红色数据)
	sendRedRow := []interface{}{"SendBufHead[Red]"}
	for i := 0; i < 12; i++ {
		sendRedRow = append(sendRedRow, int32(state.SendBufHead[0][i].First()))
	}
	bufTable.AppendRow(sendRedRow)

	// SendBufHead (黄色数据)
	sendYellowRow := []interface{}{"SendBufHead[Yellow]"}
	for i := 0; i < 12; i++ {
		sendYellowRow = append(sendYellowRow, int32(state.SendBufHead[1][i].First()))
	}
	bufTable.AppendRow(sendYellowRow)

	// SendBufHead (蓝色数据)
	sendBlueRow := []interface{}{"SendBufHead[Blue]"}
	for i := 0; i < 12; i++ {
		sendBlueRow = append(sendBlueRow, int32(state.SendBufHead[2][i].First()))
	}
	bufTable.AppendRow(sendBlueRow)

	// SendBufHeadBusy (红色数据)
	sendRedBusyRow := []interface{}{"SendBufHeadBusy[Red]"}
	for i := 0; i < 12; i++ {
		sendRedBusyRow = append(sendRedBusyRow, state.SendBufHeadBusy[0][i])
	}
	bufTable.AppendRow(sendRedBusyRow)

	// SendBufHeadBusy (黄色数据)
	sendYellowBusyRow := []interface{}{"SendBufHeadBusy[Yellow]"}
	for i := 0; i < 12; i++ {
		sendYellowBusyRow = append(sendYellowBusyRow, state.SendBufHeadBusy[1][i])
	}
	bufTable.AppendRow(sendYellowBusyRow)

	// SendBufHeadBusy (蓝色数据)
	sendBlueBusyRow := []interface{}{"SendBufHeadBusy[Blue]"}
	for i := 0; i < 12; i++ {
		sendBlueBusyRow = append(sendBlueBusyRow, state.SendBufHeadBusy[2][i])
	}
	bufTable.AppendRow(sendBlueBusyRow)

	fmt.Println(bufTable.Render())
	fmt.Println("================================================")
}

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
