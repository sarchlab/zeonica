package core

import (
	"github.com/sarchlab/zeonica/cgra"
)

type routingRule struct {
	src   cgra.Side
	dst   cgra.Side
	color string
}

// type Context struct {
// 	content string
// }

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
	//consider adding new feature
	// ConfigMem []*Context

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

func (i instEmulator) getDirecIndex(side string) int {
	var srcIndex int

	switch side {
	case "NORTH":
		srcIndex = int(cgra.North)
	case "WEST":
		srcIndex = int(cgra.West)
	case "SOUTH":
		srcIndex = int(cgra.South)
	case "EAST":
		srcIndex = int(cgra.East)
	default:
		panic("invalid side")
	}

	return srcIndex
}

func (i instEmulator) RouterSrcMustBeDirection(src string) {
	arr := []string{"NORTH", "SOUTH", "WEST", "EAST"}
	res := false
	for _, s := range arr {
		if s == src {
			res = true
			break
		}
	}

	if res {
		panic("the source of a ROUTER_FORWARD instruction must be directions")
	}
}

func (i instEmulator) getColorIndex(color string) int {
	switch color {
	case "R":
		return 0
	case "Y":
		return 1
	case "B":
		return 2
	default:
		panic("Wrong Color")
	}
}
