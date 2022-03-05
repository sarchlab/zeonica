package api

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/zeonica/cgra"
	"gitlab.com/akita/akita/v2/sim"
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
		mockDevice         *MockDevice
		mockDeviceSidePort *MockPort
		portFactory        *mockPortFactory
		driver             *driverImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		mockDeviceSidePort = NewMockPort(mockCtrl)
		mockDeviceSidePort.EXPECT().Name().Return("DevicePort").AnyTimes()
		mockDeviceSidePort.EXPECT().SetConnection(gomock.Any()).AnyTimes()

		mockDevice = NewMockDevice(mockCtrl)
		mockDevice.EXPECT().GetSize().Return(4, 4).AnyTimes()
		mockDevice.EXPECT().
			GetSidePorts(gomock.Any(), gomock.Any()).
			DoAndReturn(func(side cgra.Side, portRange [2]int) []sim.Port {
				ports := make([]sim.Port, portRange[1]-portRange[0])
				for i := range ports {
					ports[i] = mockDeviceSidePort
				}
				return ports
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
			sim.NewTickingComponent("driver", nil, 1, driver)
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
				portFactory.ports["driver.Device_North_0"],
				portFactory.ports["driver.Device_North_1"],
				portFactory.ports["driver.Device_North_2"],
			}))
		Expect(driver.feedInTasks[0].remotePorts).
			To(Equal([]sim.Port{
				mockDeviceSidePort,
				mockDeviceSidePort,
				mockDeviceSidePort,
			}))
		Expect(driver.feedInTasks[0].stride).To(Equal(3))
	})

	It("should do feed in", func() {
		remotePort1 := NewMockPort(mockCtrl)
		remotePort2 := NewMockPort(mockCtrl)
		remotePort3 := NewMockPort(mockCtrl)
		localPort1 := portFactory.ports["driver.Device_North_0"]
		localPort2 := portFactory.ports["driver.Device_North_1"]
		localPort3 := portFactory.ports["driver.Device_North_2"]

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
