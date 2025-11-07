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
