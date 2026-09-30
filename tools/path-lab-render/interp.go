package main

import (
	"math"
	"sort"
)

// A frames file samples every unit every few ticks. Pictures between
// samples place a unit on the straight line between its two neighbouring
// samples; a unit missing from the frames in between (it died, or had not
// yet been built) is not drawn across the gap.

type sample struct {
	tick    int32
	x, z    float64
	heading uint16
	blocked bool
	moving  bool
}

// state is a unit as drawn at one moment.
type state struct {
	x, z    float64
	heading uint16
	blocked bool
	moving  bool
}

func (s sample) state() state {
	return state{x: s.x, z: s.z, heading: s.heading, blocked: s.blocked, moving: s.moving}
}

// logData is one log's frames rearranged per unit.
type logData struct {
	label  string
	path   string
	ticks  []int32 // the frame ticks, ascending
	step   int32   // the sampling interval
	series map[int][]sample
	ids    []int // units in the log, ascending
	score  *scoreEntry
}

func newLog(label, path string, frames []frame) *logData {
	l := &logData{label: label, path: path, series: map[int][]sample{}}
	for _, f := range frames {
		l.ticks = append(l.ticks, f.Tick)
		for _, u := range f.Units {
			id := int(u[0])
			s := l.series[id]
			if n := len(s); n > 0 && s[n-1].tick == f.Tick {
				continue // a unit listed twice in one frame keeps its first entry
			}
			l.series[id] = append(s, sample{tick: f.Tick, x: float64(u[1]), z: float64(u[2]),
				heading: uint16(u[3]), blocked: u[4] != 0, moving: u[5] != 0})
		}
	}
	for i := 1; i < len(l.ticks); i++ {
		if d := l.ticks[i] - l.ticks[i-1]; d > 0 && (l.step == 0 || d < l.step) {
			l.step = d
		}
	}
	if l.step == 0 {
		l.step = 1
	}
	for id := range l.series {
		l.ids = append(l.ids, id)
	}
	sort.Ints(l.ids)
	return l
}

// joined reports whether two consecutive samples of a unit are neighbours
// in the frames rather than the two sides of a gap.
func (l *logData) joined(a, b sample) bool {
	return float64(b.tick-a.tick) <= 1.5*float64(l.step)
}

// at is the unit's state at tick t, which need not be a sampled tick.
// Before a log's first frame a unit present in it stands where that frame
// shows it (a replay's first frame can come a few ticks after the window
// opens); after a unit's last sample it is held for one sampling interval,
// or to the window's end when the log's last frame still shows it.
func (l *logData) at(id int, t float64) (state, bool) {
	s := l.series[id]
	if len(s) == 0 {
		return state{}, false
	}
	first, last := s[0], s[len(s)-1]
	switch {
	case t <= float64(first.tick):
		if first.tick == l.ticks[0] || t == float64(first.tick) {
			return first.state(), true
		}
		return state{}, false
	case t >= float64(last.tick):
		if last.tick == l.ticks[len(l.ticks)-1] || t-float64(last.tick) < float64(l.step) {
			return last.state(), true
		}
		return state{}, false
	}
	k := sort.Search(len(s), func(i int) bool { return float64(s[i].tick) > t }) - 1
	a, b := s[k], s[k+1]
	if !l.joined(a, b) {
		if t-float64(a.tick) < float64(l.step) {
			return a.state(), true
		}
		return state{}, false
	}
	f := (t - float64(a.tick)) / float64(b.tick-a.tick)
	return state{
		x:       a.x + (b.x-a.x)*f,
		z:       a.z + (b.z-a.z)*f,
		heading: lerpHeading(a.heading, b.heading, f),
		blocked: a.blocked,
		moving:  a.moving,
	}, true
}

// lerpHeading turns from a toward b the short way round.
func lerpHeading(a, b uint16, f float64) uint16 {
	d := float64(int16(b - a))
	return uint16(int32(a) + int32(math.Round(d*f)))
}

// runs splits a unit's samples at gaps, one run per stretch the unit was
// continuously in the frames.
func (l *logData) runs(id int) [][]sample {
	s := l.series[id]
	var out [][]sample
	start := 0
	for i := 1; i <= len(s); i++ {
		if i == len(s) || !l.joined(s[i-1], s[i]) {
			out = append(out, s[start:i])
			start = i
		}
	}
	return out
}
