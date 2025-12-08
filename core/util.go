package core

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jedib0t/go-pretty/v6/table"
)

const (
	PrintToggle            = false
	LevelTrace  slog.Level = slog.LevelInfo + 1
)

func Trace(msg string, args ...any) {
	slog.Log(context.Background(), LevelTrace, msg, args...)
}

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
