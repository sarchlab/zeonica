package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
)

type routingRule struct {
	src   cgra.Side
	dst   cgra.Side
	color string
}

// type Trigger struct {
// 	src    [4]bool
// 	color  int
// 	branch string
// }

type coreState struct {
	PC           uint32
	TileX, TileY uint32
	Registers    []uint32
	Memory       []uint32
	Code         []string

	RecvBufHead      [][]uint32 //[Color][Direction]
	RecvBufHeadReady [][]bool
	SendBufHead      [][]uint32
	SendBufHeadBusy  [][]bool

	routingRules []*routingRule
	//triggers     []*Trigger

}

type instEmulator struct {
}

func (i instEmulator) RunInst(inst string, state *coreState) {

}
