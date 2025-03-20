package core

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/sim/directconnection"
)

type TestMsg sim.MsgMeta

func (m *TestMsg) Meta() *sim.MsgMeta {
	return (*sim.MsgMeta)(m)
}

func (m *TestMsg) Clone() sim.Msg {
	clone := *m
	clone.ID = sim.GetIDGenerator().Generate()
	return &clone
}

var _ = Describe("ExtPort", func() {

	var (
		port       *ExtPort
		dstPort    *ExtPort
		engine     sim.Engine
		remotePort sim.RemotePort
	)
	BeforeEach(func() {
		port = NewExtPort(nil, 2, 3, 10, "PortA").(*ExtPort)
		conn := directconnection.MakeBuilder().
			WithEngine(engine).
			Build("TestConn")
		port.SetConnection(conn)
		conn.PlugIn(port)
		dstPort = NewExtPort(nil, 4, 4, 10, "DstPort").(*ExtPort)
		dstConn := directconnection.MakeBuilder().
			WithEngine(engine).
			Build("DstPort_To_SrcPort")
		dstPort.SetConnection(dstConn)
		conn.PlugIn(dstPort)
	})

	It("should return remote port", func() {
		extPort := ExtPort{
			name: "ext",
		}
		Expect(extPort.AsRemote()).To(Equal(sim.RemotePort("ext")))
	})
	It("Should return port name", func() {
		port := NewExtPort(nil, 0, 0, 10, "TestPort").(*ExtPort)
		Expect(port.Name()).To(Equal("TestPort"))
		Expect(port.AsRemote()).To(Equal(sim.RemotePort("TestPort")))
	})

	It("Should change channel", func() {
		port := NewExtPort(nil, 0, 0, 10, "Port").(*ExtPort)
		port.UseChannel(3)
		Expect(port.currentChannel).To(Equal(3))
	})

	It("Use Test Message to test", func() {
		port.UseChannel(1)

		msg := &TestMsg{
			ID:  sim.GetIDGenerator().Generate(),
			Src: port.AsRemote(),
			Dst: "DstPort",
		}

		Expect(port.Send(msg)).To(Succeed())
		Expect(msg.TrafficClass).To(Equal(1))
	})

	Describe("Multiple channels", func() {
		It("should correctly handle empty channel", func() {
			Expect(port.RetrieveOutgoing()).To(BeNil())
			Expect(port.PeekOutgoing()).To(BeNil())
		})
	})

	Describe("Mix channel operations", func() {
		// It("Should correctly handle multiple channels sending order", func() {
		// 	port.UseChannel(3)
		// 	msg3a := &TestMsg{Src: port.AsRemote(), Dst: remotePort}
		// 	msg3b := &TestMsg{Src: port.AsRemote(), Dst: remotePort}
		// 	port.Send(msg3a)
		// 	port.Send(msg3b)

		// 	port.UseChannel(1)
		// 	msg1 := &TestMsg{Src: port.AsRemote(), Dst: remotePort}
		// 	port.Send(msg1)

		// 	// Correct order should be  1 -> 3a -> 3b
		// 	Expect(port.RetrieveOutgoing()).To(BeIdenticalTo(msg1))
		// 	Expect(port.RetrieveOutgoing()).To(BeIdenticalTo(msg3a))
		// 	Expect(port.RetrieveOutgoing()).To(BeIdenticalTo(msg3b))
		// })
	})

	Describe("Handle exception", func() {
		It("Should reject nil message", func() {
			port.UseChannel(1)
			Expect(func() { port.Send(nil) }).To(Panic())
		})

		It("Should reject no source message", func() {
			msg := &TestMsg{Dst: remotePort}
			Expect(func() { port.Send(msg) }).To(Panic())
		})
	})

})
