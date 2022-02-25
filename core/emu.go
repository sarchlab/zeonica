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
	SendBufHeadReady []bool
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

}

func (i instEmulator) waitSrcMustBeNetRecvReg(src string) {
	if !strings.HasPrefix(src, "NET_RECV_") {
		panic("the source of a WAIT instruction must be NET_RECV registers")
	}
}
