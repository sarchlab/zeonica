package main

import (
	_ "embed"
	"strings"

	"github.com/sarchlab/zeonica/config"
	"github.com/tebeka/atexit"
	"gitlab.com/akita/akita/v2/sim"
)

//go:embed passthrough.cgraasm
var program string

func main() {
	engine := sim.NewSerialEngine()

	device := config.CreateDevice(engine)
	device.Tiles[0].Core.Code = strings.Split(program, "\n")

	device.Tiles[0].Core.TickLater(0)

	engine.Run()
	atexit.Exit(0)
}
