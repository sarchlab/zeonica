package core

import (
	"fmt"
	"github.com/sarchlab/akita/v4/sim"
	"sort"
	"sync"
)

// HookPosPortMsgSend marks when a message is sent out from the port.
var HookPosPortMsgSend = &sim.HookPos{Name: "Port Msg Send"}

// HookPosPortMsgRecvd marks when an inbound message arrives at a the given port
var HookPosPortMsgRecvd = &sim.HookPos{Name: "Port Msg Recv"}

// HookPosPortMsgRetrieve marks when an outbound message is sent over a
// connection
var HookPosPortMsgRetrieve = &sim.HookPos{Name: "Port Msg Retrieve"}

// A RemotePort is a string that refers to another port.
//type RemotePort string

// A Port is owned by a component and is used to plugin connections
type Port interface {
	sim.Named
	sim.Hookable

	AsRemote() sim.RemotePort

	SetConnection(conn sim.Connection)
	Component() sim.Component

	// For connection
	Deliver(msg sim.Msg) *sim.SendError
	NotifyAvailable()
	RetrieveOutgoing() sim.Msg
	PeekOutgoing() sim.Msg

	// For component
	CanSend() bool
	Send(msg sim.Msg) *sim.SendError
	RetrieveIncoming() sim.Msg
	PeekIncoming() sim.Msg
}

// DefaultPort implements the port interface.
type defaultPort struct {
	sim.HookableBase

	lock sync.Mutex
	name string
	comp sim.Component
	conn sim.Connection

	incomingBuf sim.Buffer
	outgoingBuf sim.Buffer
}

// AsRemote returns the remote port name.
func (p *defaultPort) AsRemote() sim.RemotePort {
	return sim.RemotePort(p.name)
}

// SetConnection sets which connection plugged in to this port.
func (p *defaultPort) SetConnection(conn sim.Connection) {
	if p.conn != nil {
		connName := p.conn.Name()
		newConnName := conn.Name()
		panicMsg := fmt.Sprintf(
			"connection already set to %s, now connecting to %s",
			connName, newConnName,
		)
		panic(panicMsg)
	}

	p.conn = conn
}

// Component returns the owner component of the port.
func (p *defaultPort) Component() sim.Component {
	return p.comp
}

// Name returns the name of the port.
func (p *defaultPort) Name() string {
	return p.name
}

// CanSend checks if the port can send a message without error.
func (p *defaultPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	canSend := p.outgoingBuf.CanPush()

	return canSend
}

// Send is used to send a message out from a component
func (p *defaultPort) Send(msg sim.Msg) *sim.SendError {
	p.lock.Lock()

	p.msgMustBeValid(msg)

	if !p.outgoingBuf.CanPush() {
		p.lock.Unlock()
		return sim.NewSendError()
	}

	wasEmpty := (p.outgoingBuf.Size() == 0)
	p.outgoingBuf.Push(msg)

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)
	p.lock.Unlock()

	if wasEmpty {
		p.conn.NotifySend()
	}

	return nil
}

// Deliver is used to deliver a message to a component
func (p *defaultPort) Deliver(msg sim.Msg) *sim.SendError {
	p.lock.Lock()

	if !p.incomingBuf.CanPush() {
		p.lock.Unlock()
		return sim.NewSendError()
	}

	wasEmpty := (p.incomingBuf.Size() == 0)

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.incomingBuf.Push(msg)
	p.lock.Unlock()

	if p.comp != nil && wasEmpty {
		p.comp.NotifyRecv(p)
	}

	return nil
}

// RetrieveIncoming is used by the component to take a message from the incoming
// buffer
func (p *defaultPort) RetrieveIncoming() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Pop()
	if item == nil {
		return nil
	}

	msg := item.(sim.Msg)
	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieve,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	return msg
}

// RetrieveOutgoing is used by the component to take a message from the outgoing
// buffer
func (p *defaultPort) RetrieveOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.outgoingBuf.Pop()
	if item == nil {
		return nil
	}

	msg := item.(sim.Msg)
	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieve,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.outgoingBuf.Size() == p.outgoingBuf.Capacity()-1 {
		p.comp.NotifyPortFree(p)
	}

	return msg
}

// PeekIncoming returns the first message in the incoming buffer without
// removing it.
func (p *defaultPort) PeekIncoming() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Peek()
	if item == nil {
		return nil
	}

	msg := item.(sim.Msg)

	return msg
}

// PeekOutgoing returns the first message in the outgoing buffer without
// removing it.
func (p *defaultPort) PeekOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.outgoingBuf.Peek()
	if item == nil {
		return nil
	}

	msg := item.(sim.Msg)

	return msg
}

// NotifyAvailable is called by the connection to notify the port that the
// connection is available again
func (p *defaultPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}
}

// NewPort creates a new port with default behavior.
func NewPort(
	comp sim.Component,
	incomingBufCap, outgoingBufCap int,
	name string,
) Port {
	p := new(defaultPort)
	p.comp = comp
	p.incomingBuf = sim.NewBuffer(name+".IncomingBuf", incomingBufCap)
	p.outgoingBuf = sim.NewBuffer(name+".OutgoingBuf", outgoingBufCap)
	p.name = name

	return p
}

func (p *defaultPort) msgMustBeValid(msg sim.Msg) {
	portMustBeMsgSrc(p, msg)
	dstMustNotBeEmpty(msg.Meta().Dst)
	srcDstMustNotBeTheSame(msg)
}

func portMustBeMsgSrc(port Port, msg sim.Msg) {
	if port.Name() != string(msg.Meta().Src) {
		panic("sending port is not msg src")
	}
}

func dstMustNotBeEmpty(port sim.RemotePort) {
	if port == "" {
		panic("dst is not given")
	}
}

func srcDstMustNotBeTheSame(msg sim.Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic("sending back to src")
	}
}

// Ext mutichannel port
type ExtPort struct {
	*sim.HookableBase

	lock sync.Mutex
	name string
	comp sim.Component
	conn sim.Connection

	incomingBuf    sim.Buffer         // keep the incoming buffer
	sendChannels   map[int]sim.Buffer // send multi channels
	currentChannel int                // current channel index
	sortedChannels []int              // sort the order of channels
	sendBufSize    int                // capacity
	maxChannels    int
}

func NewExtPort(
	comp sim.Component,
	incomingBufCap, sendBufCap int,
	name string,
) Port {
	return &ExtPort{
		HookableBase: sim.NewHookableBase(),
		name:         name,
		comp:         comp,
		incomingBuf:  sim.NewBuffer(name+".Incoming", incomingBufCap),
		sendChannels: make(map[int]sim.Buffer),
		sendBufSize:  sendBufCap,
	}
}

func (p *ExtPort) AsRemote() sim.RemotePort {
	return sim.RemotePort(p.name)
}

// UseChannel
func (p *ExtPort) UseChannel(channel int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.currentChannel = channel
}

func (p *ExtPort) CanSend() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	// check if current channel exists
	buf, exists := p.sendChannels[p.currentChannel]
	if !exists {
		return true // new channel can be created
	}
	return buf.CanPush()
}

func (p *ExtPort) Send(msg sim.Msg) *sim.SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.msgMustBeValid(msg)

	buf, exists := p.sendChannels[p.currentChannel]

	if !exists {
		// check if new channel can be created
		if len(p.sendChannels) >= p.maxChannels {
			return sim.NewSendError() // do not create new channel
		}
		buf = sim.NewBuffer(
			fmt.Sprintf("%s.Send-Ch%d", p.name, p.currentChannel),
			p.sendBufSize,
		)
		p.sendChannels[p.currentChannel] = buf
		p.updateSortedChannels()
	}

	if !buf.CanPush() {
		return sim.NewSendError()
	}

	wasEmpty := buf.Size() == 0
	buf.Push(msg)

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgSend,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if wasEmpty {
		p.conn.NotifySend()
	}

	return nil
}

func (p *ExtPort) RetrieveOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, ch := range p.sortedChannels {
		buf := p.sendChannels[ch]
		if buf.Size() > 0 {
			item := buf.Pop()
			msg := item.(sim.Msg)

			if buf.Size() == 0 {
				p.updateSortedChannels()
			}

			hookCtx := sim.HookCtx{
				Domain: p,
				Pos:    HookPosPortMsgRetrieve,
				Item:   msg,
			}
			p.InvokeHook(hookCtx)

			if buf.Size() == buf.Capacity()-1 {
				p.comp.NotifyPortFree(p)
			}

			return msg
		}
	}

	return nil
}

func (p *ExtPort) Deliver(msg sim.Msg) *sim.SendError {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.incomingBuf.CanPush() {
		return sim.NewSendError()
	}

	wasEmpty := p.incomingBuf.Size() == 0

	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRecvd,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	p.incomingBuf.Push(msg)

	if p.comp != nil && wasEmpty {
		p.comp.NotifyRecv(p)
	}

	return nil
}

func (p *ExtPort) RetrieveIncoming() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Pop()
	if item == nil {
		return nil
	}

	msg := item.(sim.Msg)
	hookCtx := sim.HookCtx{
		Domain: p,
		Pos:    HookPosPortMsgRetrieve,
		Item:   msg,
	}
	p.InvokeHook(hookCtx)

	if p.incomingBuf.Size() == p.incomingBuf.Capacity()-1 {
		p.conn.NotifyAvailable(p)
	}

	return msg
}

func (p *ExtPort) updateSortedChannels() {
	channels := make([]int, 0, len(p.sendChannels))
	for ch := range p.sendChannels {
		channels = append(channels, ch)
	}
	sort.Ints(channels)
	p.sortedChannels = channels
}

func (p *ExtPort) PeekIncoming() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	item := p.incomingBuf.Peek()
	if item == nil {
		return nil
	}
	return item.(sim.Msg)
}

func (p *ExtPort) PeekOutgoing() sim.Msg {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, ch := range p.sortedChannels {
		buf := p.sendChannels[ch]
		if buf.Size() > 0 {
			item := buf.Peek()
			return item.(sim.Msg)
		}
	}
	return nil
}

func (p *ExtPort) Name() string {
	return p.name
}

func (p *ExtPort) Component() sim.Component {
	return p.comp
}

func (p *ExtPort) SetConnection(conn sim.Connection) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.conn = conn
}

func (p *ExtPort) NotifyAvailable() {
	if p.comp != nil {
		p.comp.NotifyPortFree(p)
	}
}

func (p *ExtPort) msgMustBeValid(msg sim.Msg) {
	p.portMustBeMsgSrc(msg)
	p.dstMustNotBeEmpty(msg.Meta().Dst)
	p.srcDstMustNotBeTheSame(msg)
}

func (p *ExtPort) portMustBeMsgSrc(msg sim.Msg) {
	if p.Name() != string(msg.Meta().Src) {
		panic(fmt.Sprintf(
			"Msg source port mismatch: msg.Src=%s, port=%s",
			msg.Meta().Src, p.Name(),
		))
	}
}

func (p *ExtPort) dstMustNotBeEmpty(dst sim.RemotePort) {
	if dst == "" {
		panic("msg destination is empty")
	}
}

func (p *ExtPort) srcDstMustNotBeTheSame(msg sim.Msg) {
	if msg.Meta().Src == msg.Meta().Dst {
		panic(fmt.Sprintf(
			"msg loopback: src=dst=%s",
			msg.Meta().Src,
		))
	}
}

func (p *ExtPort) SetMaxChannels(max int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.maxChannels = max
}
