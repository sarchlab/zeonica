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

		mockTile = NewMockTile(mockCtrl)
		mockTile.EXPECT().SetRemotePort(gomock.Any(), gomock.Any()).AnyTimes()

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
					ports[i] = mockDeviceSidePort
				}
				return ports
			}).AnyTimes()

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

		driver.FeedIn(data, cgra.North, [2]int{0, 3}, 3)

		Expect(driver.feedInTasks).To(HaveLen(1))
		Expect(driver.feedInTasks[0].data).To(Equal(data))
		Expect(driver.feedInTasks[0].localPorts).
			To(Equal([]sim.Port{
				portFactory.ports["Driver.DeviceNorth[0]"],
				portFactory.ports["Driver.DeviceNorth[1]"],
				portFactory.ports["Driver.DeviceNorth[2]"],
			}))
		Expect(driver.feedInTasks[0].remotePorts).
			To(Equal([]sim.Port{
				mockDeviceSidePort,
				mockDeviceSidePort,
				mockDeviceSidePort,
			}))
		Expect(driver.feedInTasks[0].stride).To(Equal(3))
	})

	It("should handle Collect API", func() {
		data := make([]uint32, 6)

		driver.Collect(data, cgra.North, [2]int{0, 3}, 3)

		Expect(driver.collectTasks).To(HaveLen(1))
		Expect(driver.collectTasks[0].data).To(Equal(data))
		Expect(driver.collectTasks[0].ports).
			To(Equal([]sim.Port{
				portFactory.ports["Driver.DeviceNorth[0]"],
				portFactory.ports["Driver.DeviceNorth[1]"],
				portFactory.ports["Driver.DeviceNorth[2]"],
			}))
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

		driver.feedInTasks = []*feedInTask{
			{
				data:        data,
				localPorts:  []sim.Port{localPort1, localPort2, localPort3},
				remotePorts: []sim.Port{remotePort1, remotePort2, remotePort3},
				stride:      3,
			},
		}

		expectPortsToSend(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]*MockPort{remotePort1, remotePort2, remotePort3},
			[]uint32{1, 2, 3},
		)

		driver.Tick(0)

		expectPortsToSend(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]*MockPort{remotePort1, remotePort2, remotePort3},
			[]uint32{4, 5, 6},
		)

		driver.Tick(1)

		Expect(driver.feedInTasks).To(BeEmpty())
	})

	It("should do collect", func() {
		localPort1 := portFactory.ports["Driver.DeviceNorth[0]"]
		localPort2 := portFactory.ports["Driver.DeviceNorth[1]"]
		localPort3 := portFactory.ports["Driver.DeviceNorth[2]"]

		data := make([]uint32, 6)

		driver.collectTasks = []*collectTask{
			{
				data:   data,
				ports:  []sim.Port{localPort1, localPort2, localPort3},
				stride: 3,
				round:  0,
			},
		}

		expectPortsToRecv(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]uint32{1, 2, 3},
		)

		driver.Tick(0)

		expectPortsToRecv(
			[]*MockPort{localPort1, localPort2, localPort3},
			[]uint32{4, 5, 6},
		)

		driver.Tick(1)

		Expect(driver.collectTasks).To(BeEmpty())
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
					Expect(msg.Src).To(Equal(port))
					Expect(msg.Dst).To(Equal(remotePorts[i]))
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
			port.EXPECT().Peek().Return(msg)
			port.EXPECT().Retrieve(gomock.Any()).Return(msg)
		}(port, data[i], i)
	}
}
