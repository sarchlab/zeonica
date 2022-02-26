// Package api defines the driver API for the wafer-scale engine.
package api

import "gitlab.com/akita/akita/v2/sim"

// Driver provides the interface to control an accelerator.
type Driver interface {
	// FeedIn provides the data to the accelerator. The data is fed into the
	// provides ports. The stride is the difference between the indices of
	// the data that is sent to adjacent ports in the same cycle.
	FeedIn(data []uint32, side int, portRange [2]int, stride int)

	// Collect collects the data from the accelerator. The data is collected
	// from the provided ports. The stride is the difference between the
	// indices of the data that is collected from adjacent ports in the same
	// cycle.
	Collect(data []uint32, side int, portRange [2]int, stride int)

	// MapProgram maps to the provided program to a core at the given cordinate.
	MapProgram(program string, core [2]int)

	// Run will run all the tasks that have been added to the driver.
	Run()
}

type driverImpl struct {
	*sim.TickingComponent

	feedInTasks  []*feedInTask
	collectTasks []*collectTask
}

// Tick runs the driver for one cycle.
func (d *driverImpl) Tick(now sim.VTimeInSec) (madeProgress bool) {
	return false
}

type feedInTask struct {
	data   []uint32
	ports  []sim.Port
	stride int
}

func (d *driverImpl) FeedIn(
	data []uint32,
	side int,
	portRange [2]int,
	stride int,
) {
	task := &feedInTask{
		data:   data,
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
	side int,
	portRange [2]int,
	stride int,
) {
	task := &collectTask{
		data:   data,
		stride: stride,
	}

	d.collectTasks = append(d.collectTasks, task)
}
