package api

import (
	"testing"

	gomock "github.com/golang/mock/gomock"
	"github.com/sarchlab/akita/v4/sim"
)

func TestDoOneFeedInTaskBackpressureDoesNotPanic(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	engine := sim.NewSerialEngine()
	d := &driverImpl{}
	d.TickingComponent = sim.NewTickingComponent("Driver", engine, 1*sim.GHz, d)

	port := NewMockPort(ctrl)
	port.EXPECT().CanSend().Return(true).Times(2)
	port.EXPECT().Name().Return("mock-port").AnyTimes()
	port.EXPECT().AsRemote().Return(sim.RemotePort("driver-local")).AnyTimes()
	port.EXPECT().Send(gomock.Any()).Return(sim.NewSendError()).Times(1)
	port.EXPECT().Send(gomock.Any()).Return(nil).Times(1)

	task := &feedInTask{
		data:        []uint32{7},
		localPorts:  []sim.Port{port},
		remotePorts: []sim.RemotePort{sim.RemotePort("device-remote")},
		stride:      1,
		color:       0,
		rounds:      1,
		portRounds:  []int{0},
	}

	if progressed := d.doOneFeedInTask(task); progressed {
		t.Fatal("expected no progress when Send returns backpressure error")
	}
	if task.portRounds[0] != 0 {
		t.Fatalf("expected round to stay 0 after backpressure, got %d", task.portRounds[0])
	}

	if progressed := d.doOneFeedInTask(task); !progressed {
		t.Fatal("expected progress once backpressure clears")
	}
	if task.portRounds[0] != 1 {
		t.Fatalf("expected round to advance to 1, got %d", task.portRounds[0])
	}
}
