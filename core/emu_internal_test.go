package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("InstEmulator", func() {
	var (
		ie instEmulator
		s  coreState
	)

	BeforeEach(func() {
		ie = instEmulator{}
		s = coreState{
			PC:               0,
			TileX:            0,
			TileY:            0,
			Registers:        make([]uint32, 4),
			Code:             make([]string, 0),
			RecvBufHead:      make([]uint32, 4),
			RecvBufHeadReady: make([]bool, 4),
			SendBufHead:      make([]uint32, 4),
			SendBufHeadBusy:  make([]bool, 4),
		}
	})

	Context("when running WAIT", func() {
		It("should wait for data to arrive", func() {
			s.RecvBufHeadReady[0] = false

			inst := "WAIT, $0, NET_RECV_NORTH"

			ie.RunInst(inst, &s)

			Expect(s.PC).To(Equal(uint32(0)))
		})

		It("should move data if the data is ready", func() {
			s.RecvBufHeadReady[0] = true
			s.RecvBufHead[0] = 4

			inst := "WAIT, $2, NET_RECV_NORTH"

			ie.RunInst(inst, &s)

			Expect(s.PC).To(Equal(uint32(1)))
			Expect(s.Registers[2]).To(Equal(uint32(4)))
			Expect(s.RecvBufHeadReady[0]).To(BeFalse())
		})
	})

	Context("when running Send", func() {
		It("should wait if sendBuf is busy", func() {
			s.SendBufHeadBusy[0] = true

			inst := "SEND, NET_RECV_NORTH, $0"

			ie.RunInst(inst, &s)

			Expect(s.PC).To(Equal(uint32(0)))
		})

		It("should send data", func() {
			s.SendBufHeadBusy[0] = false
			s.Registers[0] = 4

			inst := "SEND, NET_RECV_NORTH, $0"

			ie.RunInst(inst, &s)

			Expect(s.PC).To(Equal(uint32(1)))
			Expect(s.SendBufHeadBusy[0]).To(BeTrue())
			Expect(s.SendBufHead[0]).To(Equal(uint32(4)))
		})
	})

})
