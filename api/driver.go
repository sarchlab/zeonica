// Package api defines the driver API for the wafer-scale engine.
package api

import (
	"github.com/sarchlab/zeonica/cgra"
	"gitlab.com/akita/akita/v2/sim"
)

// Driver provides the interface to control an accelerator.
type Driver interface {
	// FeedIn provides the data to the accelerator. The data is fed into the
	// provides ports. The stride is the difference between the indices of
	// the data that is sent to adjacent ports in the same cycle.
	FeedIn(data []uint32, side cgra.Side, portRange [2]int, stride int)

	// Collect collects the data from the accelerator. The data is collected
	// from the provided ports. The stride is the difference between the
	// indices of the data that is collected from adjacent ports in the same
	// cycle.
	Collect(data []uint32, side cgra.Side, portRange [2]int, stride int)

	// MapProgram maps to the provided program to a core at the given cordinate.
	MapProgram(program string, core [2]int)

	// Run will run all the tasks that have been added to the driver.
	Run()
}

type driverImpl struct {
	*sim.TickingComponent

	device cgra.Device

	feedInTasks  []*feedInTask
	collectTasks []*collectTask
}

// Tick runs the driver for one cycle.
func (d *driverImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	madeProgress = d.doFeedIn() || madeProgress

	return madeProgress
}

func (d *driverImpl) doFeedIn() bool {
	madeProgress := false

	for _, task := range d.feedInTasks {
		madeProgress = d.doOneFeedInTask(task) || madeProgress
	}

	d.removeFinishedFeedInTasks()

	return madeProgress
}

func (d *driverImpl) removeFinishedFeedInTasks() {
	for i := len(d.feedInTasks) - 1; i >= 0; i-- {
		if d.feedInTasks[i].isFinished() {
			d.feedInTasks = append(
				d.feedInTasks[:i], d.feedInTasks[i+1:]...)
		}
	}
}

func (d *driverImpl) doOneFeedInTask(task *feedInTask) bool {
	madeProgress := false

	for i, port := range task.ports {
		msg := cgra.MoveMsgBuilder{}.
			WithDst(port).
			WithData(task.data[task.round*task.stride+i]).
			Build()
		err := port.Send(msg)
		if err != nil {
			panic("CGRA cannot handle the data rate")
		}

		madeProgress = true
	}

	task.round++

	return madeProgress
}

type feedInTask struct {
	data   []uint32
	ports  []sim.Port
	stride int
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
) {
	task := &feedInTask{
		data:   data,
		ports:  d.device.GetSidePorts(side, portRange),
		stride: stride,
	}

	d.feedInTasks = append(d.feedInTasks, task)
}

type collectTask struct {
	data   []uint32
	ports  []sim.Port
	stride int
}

func (d *driverImpl) Collect(
	data []uint32,
	side cgra.Side,
	portRange [2]int,
	stride int,
) {
	task := &collectTask{
		data:   data,
		stride: stride,
	}

	d.collectTasks = append(d.collectTasks, task)
}
