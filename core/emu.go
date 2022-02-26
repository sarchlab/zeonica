package core

import (
	"strconv"
	"strings"
)

type state struct {
	PC               uint32
	TileX, TileY     uint32
	Registers        []uint32
	Code             []string
	RecvBufHead      []uint32
	RecvBufHeadReady []bool
	SendBufHead      []uint32
	SendBufHeadBusy  []bool
}

type instEmulator struct {
}

func (i instEmulator) RunInst(inst string, state *state) {
	tokens := strings.Split(inst, ",")
	for i := range tokens {
		tokens[i] = strings.TrimSpace(tokens[i])
	}

	instName := tokens[0]
	switch instName {
	case "WAIT":
		i.runWait(tokens, state)
	case "SEND":
		i.runSend(tokens, state)
	}

}

func (i instEmulator) runWait(inst []string, state *state) {
	dst := inst[1]
	src := inst[2]

	i.waitSrcMustBeNetRecvReg(src)
	srcIndex, err := strconv.Atoi(strings.TrimPrefix(src, "NET_RECV_"))
	if err != nil {
		panic(err)
	}

	if !state.RecvBufHeadReady[srcIndex] {
		return
	}

	state.RecvBufHeadReady[srcIndex] = false
	i.writeOperand(dst, state.RecvBufHead[srcIndex], state)
	state.PC++
}

func (i instEmulator) waitSrcMustBeNetRecvReg(src string) {
	if !strings.HasPrefix(src, "NET_RECV_") {
		panic("the source of a WAIT instruction must be NET_RECV registers")
	}
}

func (i instEmulator) runSend(inst []string, state *state) {
	dst := inst[1]
	src := inst[2]

	i.sendDstMustBeNetSendReg(dst)
	dstIndex, err := strconv.Atoi(strings.TrimPrefix(dst, "NET_SEND_"))
	if err != nil {
		panic(err)
	}

	if state.SendBufHeadBusy[dstIndex] {
		return
	}

	state.SendBufHeadBusy[dstIndex] = true
	val := i.readOperand(src, state)
	state.SendBufHead[dstIndex] = val
	state.PC++
}

func (i instEmulator) sendDstMustBeNetSendReg(src string) {
	if !strings.HasPrefix(src, "NET_SEND_") {
		panic("the destination of a SEND instruction must be NET_SEND registers")
	}
}

func (i instEmulator) readOperand(operand string, state *state) (value uint32) {
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic("invalid register index")
		}

		value = state.Registers[registerIndex]
	}

	return
}

func (i instEmulator) writeOperand(operand string, value uint32, state *state) {
	if strings.HasPrefix(operand, "$") {
		registerIndex, err := strconv.Atoi(strings.TrimPrefix(operand, "$"))
		if err != nil {
			panic("invalid register index")
		}

		state.Registers[registerIndex] = value
	}
}
