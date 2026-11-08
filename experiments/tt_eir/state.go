package main

// Internal-state log of a run, in the format of the previous round's
// comparison scripts: per TT core the pages entering c0/c1/c24/c16 (content
// and cycle, cb_log_r*_c*.{csv,bin}) and the CB publish/reserve/release and
// wait events (trace_r*_c*.json). Pages are read from the consumer's receive
// queue (queue watches, end of every tick); CB operations come from
// core.TTCBObserver. Instrumentation only: it does not change the run.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sarchlab/zeonica/cgra"
	"github.com/sarchlab/zeonica/config"
	"github.com/sarchlab/zeonica/core"
)

var stateCBs = map[string]bool{"c0": true, "c1": true, "c24": true, "c16": true}

type stateQueue struct {
	x, y        int
	side, color string
	cb          string
	occ, popped int
	pushed      int
	cluster     [2]int
}

type stateLog struct {
	b        *build
	dev      cgra.Device
	queues   map[string]*stateQueue // by watch label
	byPort   map[string]*stateQueue // "x,y,side,color"
	pages    map[[2]int]*bytes.Buffer
	csv      map[[2]int]*strings.Builder
	ordinal  map[[2]int]map[string]int
	trace    map[[2]int][]map[string]any
	reserves map[[2]int]int
}

func clusterOf(x, y int) [2]int { return [2]int{y / 2, x / 3} }

func roleOf(x, y int) string {
	return map[[2]int]string{{0, 1}: "reader", {1, 1}: "unpack", {2, 1}: "math", {1, 0}: "pack", {0, 0}: "writer"}[[2]int{x % 3, y % 2}]
}

func newStateLog(b *build) (*stateLog, []core.QueueWatchSpec) {
	s := &stateLog{b: b, queues: map[string]*stateQueue{}, byPort: map[string]*stateQueue{}, pages: map[[2]int]*bytes.Buffer{},
		csv: map[[2]int]*strings.Builder{}, ordinal: map[[2]int]map[string]int{}, trace: map[[2]int][]map[string]any{}, reserves: map[[2]int]int{}}
	var watches []core.QueueWatchSpec
	for _, q := range b.prog.caps {
		if !stateCBs[q.cb] {
			continue
		}
		label := fmt.Sprintf("%d,%d,%s", q.x, q.y, q.cb)
		sq := &stateQueue{x: q.x, y: q.y, side: q.side, color: q.color, cb: q.cb, cluster: clusterOf(q.x, q.y)}
		s.queues[label] = sq
		s.byPort[fmt.Sprintf("%d,%d,%s,%s", q.x, q.y, q.side, q.color)] = sq
		watches = append(watches, core.QueueWatchSpec{Label: label, X: q.x, Y: q.y, Kind: "recv", Direction: q.side, Color: q.color})
		if _, ok := s.csv[sq.cluster]; !ok {
			s.csv[sq.cluster] = &strings.Builder{}
			s.csv[sq.cluster].WriteString("cycle,cb,ordinal,offset,bytes\n")
			s.pages[sq.cluster] = &bytes.Buffer{}
			s.ordinal[sq.cluster] = map[string]int{}
		}
	}
	return s, watches
}

func (s *stateLog) event(cl [2]int, e map[string]any) { s.trace[cl] = append(s.trace[cl], e) }

// onQueue handles one end-of-tick occupancy sample of a watched CB queue.
func (s *stateLog) onQueue(o core.TraceObservation) {
	q := s.queues[o.Label]
	if q == nil || o.Occupancy == nil {
		return
	}
	pushed := *o.Occupancy - q.occ + q.popped
	q.occ, q.popped = *o.Occupancy, 0
	if pushed <= 0 {
		return
	}
	items := config.CoreOf(s.dev.GetTile(q.x, q.y)).RecvQueueContents(sides[q.side], q.color)
	t := *o.Time
	for _, d := range items[len(items)-pushed:] {
		b := make([]byte, 4*len(d.Data))
		for k, w := range d.Data {
			binary.LittleEndian.PutUint32(b[4*k:], w)
		}
		n := s.ordinal[q.cluster][q.cb]
		s.ordinal[q.cluster][q.cb]++
		fmt.Fprintf(s.csv[q.cluster], "%g,%s,%d,%d,%d\n", t, q.cb, n, s.pages[q.cluster].Len(), len(b))
		s.pages[q.cluster].Write(b)
		q.pushed++
		if blk := s.b.blockTiles[q.cb]; blk > 0 && q.pushed%blk == 0 {
			stage := "pack"
			if q.cb == "c0" || q.cb == "c1" {
				stage = "reader"
			}
			s.event(q.cluster, map[string]any{"buffer": q.cb, "action": "cb_publish", "tiles": blk, "stage": stage, "start_cycle": t, "end_cycle": t})
		}
	}
}

// onCB handles one executed CB operation.
func (s *stateLog) onCB(e core.TTCBEvent) {
	cl := clusterOf(e.X, e.Y)
	stage := roleOf(e.X, e.Y)
	cb := s.b.labels[fmt.Sprintf("%d,%d,%s,%s,%s", e.X, e.Y, e.Op, e.Port, e.Color)]
	if e.Op == "CB_RESERVE" && stage == "pack" {
		seq := s.b.reserveSeq[[2]int{e.X, e.Y}]
		if len(seq) > 0 {
			cb = seq[s.reserves[[2]int{e.X, e.Y}]%len(seq)]
		}
		s.reserves[[2]int{e.X, e.Y}]++
	}
	if cb == "" {
		return
	}
	if e.End > e.Start {
		action := "wait_dependency"
		if e.Op == "CB_RESERVE" {
			action = "wait_capacity"
		}
		s.event(cl, map[string]any{"buffer": cb, "action": action, "stage": stage, "start_cycle": e.Start, "end_cycle": e.End, "reason": e.Op})
	}
	switch e.Op {
	case "CB_POP":
		if q := s.byPort[fmt.Sprintf("%d,%d,%s,%s", e.X, e.Y, e.Port, e.Color)]; q != nil {
			q.popped += e.Tiles
		}
		s.event(cl, map[string]any{"buffer": cb, "action": "cb_release", "tiles": e.Tiles, "stage": stage, "start_cycle": e.End, "end_cycle": e.End})
	case "CB_RESERVE":
		s.event(cl, map[string]any{"buffer": cb, "action": "cb_reserve", "tiles": e.Tiles, "stage": stage, "start_cycle": e.End, "end_cycle": e.End})
	case "CB_WAIT":
		s.event(cl, map[string]any{"buffer": cb, "action": "cb_consume", "tiles": e.Tiles, "stage": stage, "start_cycle": e.End, "end_cycle": e.End})
	}
}

func (s *stateLog) write(dir string, cycles float64) {
	check(os.MkdirAll(dir, 0o755))
	for cl, b := range s.csv {
		base := filepath.Join(dir, "cb_log_r"+strconv.Itoa(cl[0])+"_c"+strconv.Itoa(cl[1]))
		check(os.WriteFile(base+".csv", []byte(b.String()), 0o644))
		check(os.WriteFile(base+".bin", s.pages[cl].Bytes(), 0o644))
		tr := append([]map[string]any{{"stage": "multicore", "action": "parallel_core_region", "start_cycle": 0.0, "end_cycle": cycles,
			"reason": "E-IR run: all TT cores simulated"}}, s.trace[cl]...)
		writeJSON(filepath.Join(dir, fmt.Sprintf("trace_r%d_c%d.json", cl[0], cl[1])), tr)
		if cl == [2]int{0, 0} {
			writeJSON(filepath.Join(dir, "trace.json"), tr)
		}
	}
}
