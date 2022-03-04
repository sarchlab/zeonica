package api

import (
	gomock "github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/zeonica/cgra"
	"gitlab.com/akita/akita/v2/sim"
)

var _ = Describe("Driver", func() {
	var (
		mockCtrl   *gomock.Controller
		mockDevice *MockDevice
		driver     *driverImpl
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())

		mockDevice = NewMockDevice(mockCtrl)

		driver = &driverImpl{
			device: mockDevice,
		}
		driver.TickingComponent =
			sim.NewTickingComponent("driver", nil, 1, driver)
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	It("should handle FeedIn API", func() {
		port1 := NewMockPort(mockCtrl)
		port2 := NewMockPort(mockCtrl)
		port3 := NewMockPort(mockCtrl)
		mockDevice.EXPECT().
			GetSidePorts(cgra.North, [2]int{0, 2}).
			Return([]sim.Port{port1, port2, port3})

		data := []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}

		driver.FeedIn(data, cgra.North, [2]int{0, 2}, 3)

		Expect(driver.feedInTasks).To(HaveLen(1))
		Expect(driver.feedInTasks[0].data).To(Equal(data))
		Expect(driver.feedInTasks[0].ports).
			To(Equal([]sim.Port{port1, port2, port3}))
		Expect(driver.feedInTasks[0].stride).To(Equal(3))
	})

	It("should do feed in", func() {
		port1 := NewMockPort(mockCtrl)
		port2 := NewMockPort(mockCtrl)
		port3 := NewMockPort(mockCtrl)

		data := []uint32{1, 2, 3, 4, 5, 6}

		driver.feedInTasks = []*feedInTask{
			{
				data:   data,
				ports:  []sim.Port{port1, port2, port3},
				stride: 3,
			},
		}

		expectPortsToReceive(
			[]*MockPort{port1, port2, port3},
			[]uint32{1, 2, 3},
		)

		driver.Tick(0)

		expectPortsToReceive(
			[]*MockPort{port1, port2, port3},
			[]uint32{4, 5, 6},
		)

		driver.Tick(1)

		Expect(driver.feedInTasks).To(BeEmpty())
	})
})

func expectPortsToReceive(ports []*MockPort, data []uint32) {
	for i, port := range ports {
		func(port *MockPort, data uint32) {
			port.EXPECT().
				Send(gomock.Any()).
				Do(func(msg *cgra.MoveMsg) {
					Expect(msg.Data).To(Equal(data))
				})
		}(port, data[i])
	}
}
