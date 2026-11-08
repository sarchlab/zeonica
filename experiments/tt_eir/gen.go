package main

// Generates E-IR YAML for TT-Metal kernels. One TT core is a 3x2 PE cluster:
//
//	row 1:  reader(0,1) -E-> unpack(1,1) -E-> math(2,1)
//	row 0:  writer(0,0) <-W- pack(1,0)  <-SW- math
//
//	reader -> unpack  East/West            R: c0 (in0 tiles)   Y: c1 (in1 tiles)
//	unpack -> math    East/West            R: srcA  Y: srcB  B: dst init (zero or reloaded partial)
//	math   -> pack    SouthWest/NorthEast  R: dst
//	pack   -> unpack  North/South          R: c24 (partials / tilized in0)  Y: c26 (final, untilize)
//	pack   -> writer  West/East            R: c16 (output)
//	host   -> reader  West R/Y (DRAM pages, Driver.FeedInDataToCore)
//	writer -> host    West R   (DRAM pages, Driver.CollectDataFromCore)
//
// Each CB is the receive queue of the consumer port, sized by
// Core.SetRecvQueueCapacity. Producers reserve space with credits that the
// consumer's CB_POP returns on the reverse link (unpack -> reader East R/Y,
// unpack -> pack North R, writer -> pack West R; c16 and c24 share one pool).
// Progress credits travel on color B: unpack -> reader/writer, pack ->
// reader/writer. Programs use loops where every iteration is the same (reader,
// math, writer) and straight-line code where the kernel's block structure
// differs between iterations (unpack, pack).

import (
	"fmt"
	"strings"
)

type operand struct{ port, color string }

func p(port, color string) operand { return operand{port, color} }

type yop struct {
	opcode   string
	src, dst []operand
	comment  string
	with     []yop // further operations issued in the same instruction group
}

type peProgram struct {
	x, y    int
	name    string
	loop    bool
	ops     []yop
	pending string
}

func (pe *peProgram) note(c string) { pe.pending = c }

func (pe *peProgram) op(opcode string, src []operand, dst ...operand) {
	pe.ops = append(pe.ops, yop{opcode: opcode, src: src, dst: dst, comment: pe.pending})
	pe.pending = ""
}

// mk builds one operation for group.
func mk(opcode string, src []operand, dst ...operand) yop {
	return yop{opcode: opcode, src: src, dst: dst}
}

// group appends one instruction group with several operations (issued together).
func (pe *peProgram) group(ops ...yop) {
	g := ops[0]
	g.comment, g.with = pe.pending, ops[1:]
	pe.ops = append(pe.ops, g)
	pe.pending = ""
}

// Queue capacity request for a consumer port (circular buffer size in tiles).
type queueCap struct {
	x, y        int
	side, color string
	tiles       int
	cb          string
}

// Token the host places in a receive queue before the run (initial CB credits).
type credit struct {
	x, y        int
	side, color string
	value       int
}

type eirProgram struct {
	width, height int
	pes           []*peProgram
	caps          []queueCap
	credits       []credit
	progress      []queueCap // progress-credit ports (tiles unused)
}

var opIDs = map[string]int{}

func opID(opcode string) int {
	if id, ok := opIDs[opcode]; ok {
		return id
	}
	opIDs[opcode] = len(opIDs) + 1
	return opIDs[opcode]
}

func (e *eirProgram) yaml() string {
	var b strings.Builder
	fmt.Fprintf(&b, "array_config:\n  rows: %d\n  columns: %d\n  cores:\n", e.height, e.width)
	for _, pe := range e.pes {
		kind := "once"
		if pe.loop {
			kind = "loop"
		}
		fmt.Fprintf(&b, "  - row: %d\n    column: %d\n    core_id: %s\n    entries:\n    - entry_id: entry0\n      type: %s\n      instructions:\n", pe.y, pe.x, pe.name, kind)
		for _, o := range pe.ops {
			if o.comment != "" {
				fmt.Fprintf(&b, "      # %s\n", o.comment)
			}
			fmt.Fprintf(&b, "      - operations:\n")
			for _, op := range append([]yop{o}, o.with...) {
				writeOp(&b, op)
			}
		}
	}
	return b.String()
}

func writeOp(b *strings.Builder, o yop) {
	fmt.Fprintf(b, "        - opcode: %s\n          id: %d\n", o.opcode, opID(o.opcode))
	for _, list := range []struct {
		key string
		ops []operand
	}{{"src_operands", o.src}, {"dst_operands", o.dst}} {
		if len(list.ops) == 0 {
			continue
		}
		fmt.Fprintf(b, "          %s:\n", list.key)
		for _, x := range list.ops {
			fmt.Fprintf(b, "          - {operand: %q, color: %s}\n", x.port, x.color)
		}
	}
}

func imm(v int) operand { return operand{fmt.Sprintf("#%d", v), "R"} }

func at(port string, idx int, color string) operand {
	return operand{fmt.Sprintf("%s@%d", port, idx), color}
}

type cluster struct {
	r, c                       int
	reader, unpack, math, pack *peProgram
	writer                     *peProgram
}

func (e *eirProgram) cluster(r, c int) *cluster {
	ox, oy := 3*c, 2*r
	newPE := func(dx, dy int, role string, loop bool) *peProgram {
		pe := &peProgram{x: ox + dx, y: oy + dy, name: fmt.Sprintf("t%d_%d.%s", r, c, role), loop: loop}
		e.pes = append(e.pes, pe)
		return pe
	}
	return &cluster{r: r, c: c, reader: newPE(0, 1, "reader", true), unpack: newPE(1, 1, "unpack", false),
		math: newPE(2, 1, "math", true), pack: newPE(1, 0, "pack", false), writer: newPE(0, 0, "writer", true)}
}

func (e *eirProgram) cap(pe *peProgram, side, color string, tiles int, cb string) {
	e.caps = append(e.caps, queueCap{pe.x, pe.y, side, color, tiles, cb})
}
