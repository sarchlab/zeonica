package cgra

import "github.com/sarchlab/akita/v3/sim"

// MoveMsg moves data from one tile to another in a CGRA.
type MoveMsg struct {
	sim.MsgMeta

	Data  uint32
	Color int
}

// Meta returns the meta data of the msg.
func (m *MoveMsg) Meta() *sim.MsgMeta {
	return &m.MsgMeta
}

// MoveMsgBuilder is a factory for MoveMsg.
type MoveMsgBuilder struct {
	src, dst sim.Port
	sendTime sim.VTimeInSec
	data     uint32
	color    int
}

// WithSrc sets the source port of the msg.
func (m MoveMsgBuilder) WithSrc(src sim.Port) MoveMsgBuilder {
	m.src = src
	return m
}

// WithDst sets the destination port of the msg.
func (m MoveMsgBuilder) WithDst(dst sim.Port) MoveMsgBuilder {
	m.dst = dst
	return m
}

// WithSendTime sets the send time of the msg.
func (m MoveMsgBuilder) WithSendTime(sendTime sim.VTimeInSec) MoveMsgBuilder {
	m.sendTime = sendTime
	return m
}

// WithData sets the data of the msg.
func (m MoveMsgBuilder) WithData(data uint32) MoveMsgBuilder {
	m.data = data
	return m
}

// WithData sets the color of the msg
func (m MoveMsgBuilder) WithColor(color int) MoveMsgBuilder {
	m.color = color
	return m
}

// Build creates a MoveMsg.
func (m MoveMsgBuilder) Build() *MoveMsg {
	return &MoveMsg{
		MsgMeta: sim.MsgMeta{
			ID:       sim.GetIDGenerator().Generate(),
			Src:      m.src,
			Dst:      m.dst,
			SendTime: m.sendTime,
		},
		Data:  m.data,
		Color: m.color,
	}
}
