package cgra

import (
	"github.com/sarchlab/akita/v4/sim"
)

// MoveMsg moves data from one tile to another in a CGRA.
type MoveMsg struct {
	sim.MsgMeta

	Data  Data
	Color int
	//create a new branch predicate data
	//Predicate int
}

// Meta returns the meta data of the msg.
func (m *MoveMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// Clone creates a new MoveMsg with the same content.
func (m *MoveMsg) Clone() sim.Msg {
	newM := *m
	newM.ID = sim.GetIDGenerator().Generate()

	return &newM
}

// MoveMsgBuilder is a factory for MoveMsg.
type MoveMsgBuilder struct {
	src, dst sim.RemotePort
	sendTime sim.VTimeInSec
	data     Data
	color    int
	// predicate value
	//predicate int
}

// WithSrc sets the source port of the msg.
func (m MoveMsgBuilder) WithSrc(src sim.RemotePort) MoveMsgBuilder {
	m.src = src
	return m
}

// WithDst sets the destination port of the msg.
func (m MoveMsgBuilder) WithDst(dst sim.RemotePort) MoveMsgBuilder {
	m.dst = dst
	return m
}

// WithSendTime sets the send time of the msg.
func (m MoveMsgBuilder) WithSendTime(sendTime sim.VTimeInSec) MoveMsgBuilder {
	m.sendTime = sendTime
	return m
}

// WithData sets the data of the msg.
func (m MoveMsgBuilder) WithData(data Data) MoveMsgBuilder {
	m.data = data
	return m
}

// WithData sets the color of the msg
func (m MoveMsgBuilder) WithColor(color int) MoveMsgBuilder {
	m.color = color
	return m
}

//WithPredicate sets the predicate of the msg
// func (m MoveMsgBuilder) WithPredicate(predicate int) MoveMsgBuilder {
// 	m.predicate = predicate
// 	return m
// }

// Build creates a MoveMsg.
func (m MoveMsgBuilder) Build() *MoveMsg {
	return &MoveMsg{
		MsgMeta: sim.MsgMeta{
			ID:  sim.GetIDGenerator().Generate(),
			Src: m.src,
			Dst: m.dst,
		},
		Data:  m.data,
		Color: m.color,
	}
}
