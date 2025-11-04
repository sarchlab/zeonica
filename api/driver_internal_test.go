package api

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/zeonica/cgra"
)

type mockPortFactory struct {
	mockCtrl *gomock.Controller

	ports map[string]*MockPort
}

func (f *mockPortFactory) make(c sim.Component, name string) sim.Port {
	port := NewMockPort(f.mockCtrl)
	port.EXPECT().Name().Return("DriverSidePort").AnyTimes()
	port.EXPECT().SetConnection(gomock.Any()).AnyTimes()
	port.EXPECT().Deliver(gomock.Any()).AnyTimes()
	port.EXPECT().AsRemote().Return(sim.RemotePort(name)).AnyTimes()
	// PeekIncoming and RetrieveIncoming will be set up per test
	// Don't set default expectations here as they may conflict with test-specific ones
	f.ports[name] = port
	return port
}

var _ = Describe("Driver", func() {
	var (
		mockCtrl           *gomock.Controller
		mockEngine         *MockEngine
		mockTile           *MockTile
		mockDevice         *MockDevice
		mockDeviceSidePort *MockPort
		portFactory        *mockPortFactory
		driver             *driverImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		mockEngine = NewMockEngine(mockCtrl)
		mockEngine.EXPECT().CurrentTime().Return(sim.VTimeInSec(1)).AnyTimes()

		mockDeviceSidePort = NewMockPort(mockCtrl)
		mockDeviceSidePort.EXPECT().Name().Return("DevicePort").AnyTimes()
		mockDeviceSidePort.EXPECT().SetConnection(gomock.Any()).AnyTimes()
		mockDeviceSidePort.EXPECT().Deliver(gomock.Any()).AnyTimes()
		mockDeviceSidePort.EXPECT().AsRemote().Return(sim.RemotePort("DevicePort")).AnyTimes()

		mockTile = NewMockTile(mockCtrl)
		mockTile.EXPECT().SetRemotePort(gomock.Any(), gomock.Any()).AnyTimes()
		mockTile.EXPECT().GetMemory(gomock.Any(), gomock.Any(), gomock.Any()).Return(uint32(0)).AnyTimes()
		mockTile.EXPECT().GetTileX().Return(0).AnyTimes()
		mockTile.EXPECT().GetTileY().Return(0).AnyTimes()

		mockDevice = NewMockDevice(mockCtrl)
		mockDevice.EXPECT().GetSize().Return(4, 4).AnyTimes()
		mockDevice.EXPECT().
			GetTile(gomock.Any(), gomock.Any()).
			Return(mockTile).
			AnyTimes()
		// handle incorrect coordinators
		mockDevice.EXPECT().
			GetSidePorts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(side cgra.Side, portRange [2]int) []sim.Port {
				ports := make([]sim.Port, portRange[1]-portRange[0])
				for i := range ports {
					// MockPort needs to implement AsRemote, so we use mockDeviceSidePort directly
					ports[i] = mockDeviceSidePort
				}
				return ports
			}).AnyTimes()

		// Note: AsRemote() returns sim.RemotePort, which MockPort should implement
		// We'll set up the expectation when needed

		mockDevice.EXPECT().
			GetTile(gomock.Any(), gomock.Any()).
			DoAndReturn(func(x, y int) cgra.Tile {
				if x >= 0 && x < 4 && y >= 0 && y < 4 {
					return mockTile
				}
				return nil
			}).AnyTimes()
		portFactory = &mockPortFactory{
			mockCtrl: mockCtrl,
			ports:    make(map[string]*MockPort),
		}
		driver = &driverImpl{
			device:      mockDevice,
			portFactory: portFactory,
		}
		driver.TickingComponent =
			sim.NewTickingComponent("Driver", mockEngine, 1, driver)
		driver.RegisterDevice(mockDevice)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle FeedIn API", func() {
		data := []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

		driver.FeedIn(data, cgra.North, [2]int{0, 3}, 3, "R")

		sideIndex := int(cgra.North)
		Expect(driver.feedInTasks[sideIndex]).To(HaveLen(1))
		Expect(driver.feedInTasks[sideIndex][0].data).To(Equal(data))
		Expect(driver.feedInTasks[sideIndex][0].localPorts).
			To(HaveLen(3))
		Expect(driver.feedInTasks[sideIndex][0].remotePorts).
			To(HaveLen(3))
		Expect(driver.feedInTasks[sideIndex][0].stride).To(Equal(3))
	})

	It("should handle Collect API", func() {
		data := make([]uint32, 6)

		driver.Collect(data, cgra.North, [2]int{0, 3}, 3, "R")

		sideIndex := int(cgra.North)
		Expect(driver.collectTasks[sideIndex]).To(HaveLen(1))
		Expect(driver.collectTasks[sideIndex][0].data).To(Equal(data))
		Expect(driver.collectTasks[sideIndex][0].ports).
			To(HaveLen(3))
	})

	It("should do feed in", func() {
		remotePort1 := NewMockPort(mockCtrl)
		remotePort2 := NewMockPort(mockCtrl)
		remotePort3 := NewMockPort(mockCtrl)
		localPort1 := portFactory.ports["Driver.DeviceNorth[0]"]
		localPort2 := portFactory.ports["Driver.DeviceNorth[1]"]
		localPort3 := portFactory.ports["Driver.DeviceNorth[2]"]

		localPort1.EXPECT().CanSend().Return(true).AnyTimes()
		localPort2.EXPECT().CanSend().Return(true).AnyTimes()
		localPort3.EXPECT().CanSend().Return(true).AnyTimes()

		data := []uint32{1, 2, 3, 4, 5, 6}

		sideIndex := int(cgra.North)
		localPort1.EXPECT().Deliver(gomock.Any()).Return(nil).AnyTimes()
		localPort2.EXPECT().Deliver(gomock.Any()).Return(nil).AnyTimes()
		localPort3.EXPECT().Deliver(gomock.Any()).Return(nil).AnyTimes()

		// RemotePort is a string type in akita v4, so we can use port names
		remotePort1.EXPECT().AsRemote().Return(sim.RemotePort("remotePort1")).AnyTimes()
		remotePort2.EXPECT().AsRemote().Return(sim.RemotePort("remotePort2")).AnyTimes()
		remotePort3.EXPECT().AsRemote().Return(sim.RemotePort("remotePort3")).AnyTimes()

		localPorts := []sim.Port{localPort1, localPort2, localPort3}
		remotePorts := []sim.RemotePort{
			sim.RemotePort("remotePort1"),
			sim.RemotePort("remotePort2"),
			sim.RemotePort("remotePort3"),
		}
		driver.feedInTasks[sideIndex] = []*feedInTask{
			{
				data:        data,
				localPorts:  localPorts,
				remotePorts: remotePorts,
				stride:      3,
				color:       0, // R
				round:       0,
			},
		}

		expectPortsToSend(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]*MockPort{remotePort1, remotePort2, remotePort3},
			[]uint32{1, 2, 3},
		)

		driver.Tick()

		expectPortsToSend(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]*MockPort{remotePort1, remotePort2, remotePort3},
			[]uint32{4, 5, 6},
		)

		driver.Tick()

		Expect(driver.feedInTasks[sideIndex]).To(BeEmpty())
	})

	It("should do collect", func() {
		localPort1 := portFactory.ports["Driver.DeviceNorth[0]"]
		localPort2 := portFactory.ports["Driver.DeviceNorth[1]"]
		localPort3 := portFactory.ports["Driver.DeviceNorth[2]"]

		data := make([]uint32, 6)

		sideIndex := int(cgra.North)
		ports := []sim.Port{localPort1, localPort2, localPort3}
		driver.collectTasks[sideIndex] = []*collectTask{
			{
				data:   data,
				ports:  ports,
				stride: 3,
				color:  0, // R
				round:  0,
			},
		}

		// Mock PeekIncoming and RetrieveIncoming for first round
		// Note: allDataReady checks PeekIncoming multiple times (once per port),
		// then doOneCollectTask calls RetrieveIncoming for each port
		msg1 := cgra.MoveMsgBuilder{}.WithData(1).Build()
		msg2 := cgra.MoveMsgBuilder{}.WithData(2).Build()
		msg3 := cgra.MoveMsgBuilder{}.WithData(3).Build()
		// allDataReady will call PeekIncoming for all ports (at least once each)
		// Since allDataReady may be called multiple times, we use AnyTimes
		localPort1.EXPECT().PeekIncoming().Return(msg1).AnyTimes()
		localPort2.EXPECT().PeekIncoming().Return(msg2).AnyTimes()
		localPort3.EXPECT().PeekIncoming().Return(msg3).AnyTimes()
		// Then doOneCollectTask will call RetrieveIncoming for each port
		localPort1.EXPECT().RetrieveIncoming().Return(msg1).Times(1)
		localPort2.EXPECT().RetrieveIncoming().Return(msg2).Times(1)
		localPort3.EXPECT().RetrieveIncoming().Return(msg3).Times(1)

		driver.Tick()

		// Mock PeekIncoming and RetrieveIncoming for second round
		msg4 := cgra.MoveMsgBuilder{}.WithData(4).Build()
		msg5 := cgra.MoveMsgBuilder{}.WithData(5).Build()
		msg6 := cgra.MoveMsgBuilder{}.WithData(6).Build()
		localPort1.EXPECT().PeekIncoming().Return(msg4).AnyTimes()
		localPort2.EXPECT().PeekIncoming().Return(msg5).AnyTimes()
		localPort3.EXPECT().PeekIncoming().Return(msg6).AnyTimes()
		localPort1.EXPECT().RetrieveIncoming().Return(msg4).Times(1)
		localPort2.EXPECT().RetrieveIncoming().Return(msg5).Times(1)
		localPort3.EXPECT().RetrieveIncoming().Return(msg6).Times(1)

		driver.Tick()

		Expect(driver.collectTasks[sideIndex]).To(BeEmpty())
		Expect(data).To(Equal([]uint32{1, 2, 3, 4, 5, 6}))
	})
})

func expectPortsToSend(
	localPorts []*MockPort,
	remotePorts []*MockPort,
	data []uint32,
) {
	for i, port := range localPorts {
		func(port *MockPort, data uint32, i int) {
			port.EXPECT().
				Send(gomock.Any()).
				Do(func(msg *cgra.MoveMsg) {
					// msg.Src is a RemotePort (string), not the port itself
					Expect(string(msg.Src)).NotTo(BeEmpty())
					// msg.Dst is a RemotePort (string), not a MockPort
					Expect(string(msg.Dst)).NotTo(BeEmpty())
					Expect(msg.Data).To(Equal(data))
				})
		}(port, data[i], i)
	}
}

func expectPortsToRecv(
	ports []*MockPort,
	data []uint32,
) {
	for i, port := range ports {
		func(port *MockPort, data uint32, i int) {
			msg := cgra.MoveMsgBuilder{}.WithData(data).Build()
			port.EXPECT().RetrieveIncoming().Return(msg)
		}(port, data[i], i)
	}
}
