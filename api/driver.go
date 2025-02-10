// Package api defines the driver API for the wafer-scale engine.
package api

import (
	"fmt"
	"strings"

	"github.com/sarchlab/akita/v3/sim"
	"github.com/sarchlab/zeonica/cgra"
)

// Driver provides the interface to control an accelerator.
type Driver interface {
	sim.Component

	// RegisterDevice registers a device to the driver. The driver will
	// establish connections to the device.
	RegisterDevice(device cgra.Device)

	// FeedIn provides the data to the accelerator. The data is fed into the
	// provides ports. The stride is the difference between the indices of
	// the data that is sent to adjacent ports in the same cycle.
	FeedIn(data []uint32, side cgra.Side, portRange [2]int, stride int, color string)

	// Collect collects the data from the accelerator. The data is collected
	// from the provided ports. The stride is the difference between the
	// indices of the data that is collected from adjacent ports in the same
	// cycle.
	Collect(data []uint32, side cgra.Side, portRange [2]int, stride int, color string)

	// MapProgram maps to the provided program to a core at the given coordinate.
	MapProgram(program string, core [2]int)
	//new manual mapping for different kernel to different PE
	SetPerPEKernels(kernels PerPEKernels) error

	PreloadMemory(x int, y int, data uint32, baseAddr uint32)
	ReadMemory(x int, y int, addr uint32) uint32

	// Run will run all the tasks that have been added to the driver.
	Run()
}

type portFactory interface {
	make(c sim.Component, name string) sim.Port
}

type driverImpl struct {
	*sim.TickingComponent

	device      cgra.Device
	portFactory portFactory

	feedInTasks  [4][]*feedInTask  //Four Directions, every direction has a task queue.
	collectTasks [4][]*collectTask //Four Directions
}

//struct for manually mapping different kernel to different PE
type PerPEKernels map[[2]int]string

func (d *driverImpl) PreloadMemory(x int, y int, data uint32, baseAddr uint32) {
	tile := d.device.GetTile(x, y)
	// fmt.Printf(
    //     "[DEBUG] PreloadMemory(x=%d, y=%d) -> Tile: %v\n",
    //     x, y, tile,
    // )
	tile.WriteMemory(x, y, data, baseAddr)
}

func (d *driverImpl) ReadMemory(x int, y int, addr uint32) uint32 {
	tile := d.device.GetTile(x, y)
	return tile.GetMemory(x, y, addr)
}

// Tick runs the driver for one cycle.
func (d *driverImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = d.doFeedIn() || madeProgress
	madeProgress = d.doCollect() || madeProgress

	return madeProgress
}

func (d *driverImpl) doFeedIn() bool {
	madeProgress := false
	for i := 0; i < 4; i++ { //Colors
		for _, task := range d.feedInTasks[i] {
			madeProgress = d.doOneFeedInTask(task) || madeProgress
		}

		d.removeFinishedFeedInTasks()
	}

	return madeProgress
}

func (d *driverImpl) removeFinishedFeedInTasks() {
	for i := 0; i < 4; i++ { //Colors
		for j := len(d.feedInTasks[i]) - 1; j >= 0; j-- {
			if d.feedInTasks[i][j].isFinished() {
				d.feedInTasks[i] = append(
					d.feedInTasks[i][:j], d.feedInTasks[i][j+1:]...)
			}
		}
	}
}

func (d *driverImpl) doOneFeedInTask(task *feedInTask) bool {
	madeProgress := false

	canSendAll := true
	for _, port := range task.localPorts {
		if !port.CanSend() {
			canSendAll = false
			break
		}
	}

	if !canSendAll {
		return false
	}

	for i, port := range task.localPorts {
		msg := cgra.MoveMsgBuilder{}.
			WithSrc(port).
			WithDst(task.remotePorts[i]).
			WithData(task.data[task.round*task.stride+i]).
			WithColor(task.color).
			WithSendTime(d.Engine.CurrentTime()). // Set the current engine time here
			Build()
		
		err := port.Send(msg)
		//fmt.Println(msg)
		if err != nil {
			panic("CGRA cannot handle the data rate")
		}
		fmt.Printf("%10f, Feed in %d to %s\n",
			d.Engine.CurrentTime()*1e9,
			task.data[task.round*task.stride+i],
			task.remotePorts[i].Name())
		
        madeProgress = true
	}
	
	task.round++
	return madeProgress
}

func (d *driverImpl) doCollect() bool {
	madeProgress := false
	for i := 0; i < 4; i++ {
		for _, task := range d.collectTasks[i] {
			madeProgress = d.doOneCollectTask(task) || madeProgress
		}

		d.removeFinishedCollectTasks()
	}

	return madeProgress
}

func (d *driverImpl) doOneCollectTask(task *collectTask) bool {
	if !d.allDataReady(task) {
		return false
	}

	for i, port := range task.ports {
		msg := port.Retrieve(d.Engine.CurrentTime()).(*cgra.MoveMsg)
		task.data[task.round*task.stride+i] = msg.Data
	}

	task.round++

	return true
}

func (*driverImpl) allDataReady(task *collectTask) bool {
	for _, port := range task.ports {
		item := port.Peek()
		if item == nil {
			return false
		}
	}

	return true
}

func (d *driverImpl) removeFinishedCollectTasks() {
	for i := 0; i < 4; i++ {
		for j := len(d.collectTasks[i]) - 1; j >= 0; j-- {
			if d.collectTasks[i][j].isFinished() {
				d.collectTasks[i] = append(
					d.collectTasks[i][:j], d.collectTasks[i][j+1:]...)
			}
		}
	}
}

// RegisterDevice registers a device to the driver. The driver will
// establish connections to the device.
func (d *driverImpl) RegisterDevice(device cgra.Device) {
	d.device = device

	d.establishConnectionOneSide(d.device, cgra.North)
	d.establishConnectionOneSide(d.device, cgra.South)
	d.establishConnectionOneSide(d.device, cgra.East)
	d.establishConnectionOneSide(d.device, cgra.West)
}

func (d *driverImpl) establishConnectionOneSide(
	device cgra.Device,
	side cgra.Side,
) {
	width, height := device.GetSize()
	maxIndex := 0
	switch side {
	case cgra.North, cgra.South:
		maxIndex = width - 1
	case cgra.East, cgra.West:
		maxIndex = height - 1
	}

	ports := device.GetSidePorts(side, [2]int{0, maxIndex + 1})
	for i, port := range ports {
		d.connectOnePort(side, i, port)
	}
}

func (d *driverImpl) localPortName(side cgra.Side, index int) string {
	return fmt.Sprintf("Device%s[%d]", side.Name(), index)
}

func (d *driverImpl) connectOnePort(side cgra.Side, index int, port sim.Port) {
	portName := d.localPortName(side, index)
	localPort := d.portFactory.make(d, d.Name()+"."+portName)
	d.AddPort(portName, localPort)

	conn := sim.NewDirectConnection(
		localPort.Name()+"."+port.Name(),
		d.Engine,
		d.Freq,
	)
	conn.PlugIn(localPort, 1)
	conn.PlugIn(port, 1)

	d.setTileRemovePort(side, index, localPort)
}

func (d *driverImpl) setTileRemovePort(
	side cgra.Side,
	index int,
	localPort sim.Port,
) {
	width, height := d.device.GetSize()
	var tile cgra.Tile
	switch side {
	case cgra.North:
		tile = d.device.GetTile(index, 0)
	case cgra.South:
		tile = d.device.GetTile(index, height-1)
	case cgra.East:
		tile = d.device.GetTile(width-1, index)
	case cgra.West:
		tile = d.device.GetTile(0, index)
	}
	tile.SetRemotePort(side, localPort)
}

type feedInTask struct {
	data []uint32

	localPorts  []sim.Port
	remotePorts []sim.Port

	stride int
	color  int
	round  int
}

func (t *feedInTask) isFinished() bool {
	return t.round >= len(t.data)/t.stride
}

func (d *driverImpl) FeedIn(
	data []uint32,
	side cgra.Side,
	portRange [2]int,
	stride int,
	color string,
) {
	task := &feedInTask{
		data:        data,
		localPorts:  d.getLocalPorts(side, portRange),
		remotePorts: d.device.GetSidePorts(side, portRange),
		stride:      stride,
		color:       d.getColorIndex(color),
	}
	sideIndex := int(side)
	//fmt.Println(color)
	d.feedInTasks[sideIndex] = append(d.feedInTasks[sideIndex], task)
}

func (d *driverImpl) getColorIndex(color string) int {
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

func (d *driverImpl) getLocalPorts(
	side cgra.Side,
	portRange [2]int,
) []sim.Port {
	ports := make([]sim.Port, 0, portRange[1]-portRange[0]+1)

	for i := portRange[0]; i < portRange[1]; i++ {
		ports = append(ports, d.GetPortByName(d.localPortName(side, i)))
	}

	return ports
}

type collectTask struct {
	data   []uint32
	ports  []sim.Port
	stride int
	color  int
	round  int
}

func (t *collectTask) isFinished() bool {
	return t.round >= len(t.data)/t.stride
}

func (d *driverImpl) Collect(
	data []uint32,
	side cgra.Side,
	portRange [2]int,
	stride int,
	color string,
) {
	task := &collectTask{
		data:   data,
		ports:  d.getLocalPorts(side, portRange),
		stride: stride,
		color:  d.getColorIndex(color),
	}

	sideIndex := int(side)
	//fmt.Println(color)
	d.collectTasks[sideIndex] = append(d.collectTasks[sideIndex], task)
}

// MapProgram dispatches a program to a core.
func (d *driverImpl) MapProgram(program string, core [2]int) {
	tile := d.device.GetTile(core[0], core[1])
	tile.MapProgram(strings.Split(program, "\n"), core[0], core[1])
}

func (d *driverImpl) SetPerPEKernels(kernels PerPEKernels) error {
    deviceWidth, deviceHeight := d.device.GetSize()
    
    for coord, code := range kernels {
        x, y := coord[0], coord[1]
        
        // Verify the coordinate
        if x < 0 || x >= deviceWidth || y < 0 || y >= deviceHeight {
            return fmt.Errorf("invalid coordinate [%d,%d] for device size %dx%d", 
                x, y, deviceWidth, deviceHeight)
        }
        
        // map the program to the core
        d.MapProgram(code, coord)
    }
    
    return nil
}

// Run runs all the tasks in the driver.
func (d *driverImpl) Run() {
	d.TickNow(d.Engine.CurrentTime())
	err := d.Engine.Run()
	if err != nil {
		panic(err)
	}
}
