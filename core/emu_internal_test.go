package core

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("InstEmulator", func() {
	var (
		ie    instEmulator
		state state
	)

	BeforeEach(func() {
		ie = instEmulator{}
		state = state{
			PC: 0,
			TileX, TileY: 0,
			Registers:        make([]uint32{}, 256),
			Code:             []string{""},
			RecvBufHead:      make([]uint32{}, 4),
			RecvBufHeadReady: make([]bool, 4),
			SendBufHead:      make([]uint32, 4),
			SendBufHeadReady: make([]bool, 4),
		}
	})

	Context("when running WAIT", func() {
		It("should wait for data to arrive", func() {
			state.RecvBufHeadRead[0] = false

		})
	})

})
