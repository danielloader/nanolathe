// Package poseblend owns Enhanced presentation history, never simulation state.
// Its held-axis policy is documented in DESIGN_GPU_RENDERER §13.5.
package poseblend

import (
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Delay is the presentation lookback for authored two/three-tick held poses.
// This is a Nanolathe rendering choice, not a retail script constant.
const Delay = 3
const one = int64(65536)

type point struct {
	tick  uint32
	value int64
}
type axis struct {
	keys    [Delay + 1]point
	n       int
	stepped bool
	since   uint32 // first held endpoint; never rewind into earlier continuous motion
}
type piece struct {
	identity frame.PieceView
	axes     [6]axis
}

// History keeps four distinct endpoints per transform axis. Continuous axes
// use the ordinary adjacent-tick blend; held axes cannot stall unrelated ones.
// A History has one recording owner. Blend writes only caller-owned dst.
type History struct {
	pieces []piece
	tick   uint32
	valid  bool
}

func (h *History) Reset() {
	clear(h.pieces)
	h.pieces = h.pieces[:0]
	h.tick, h.valid = 0, false
}
func (h *History) LastTick() (uint32, bool) { return h.tick, h.valid }

// Record owns copies of endpoint values. Repeated presentation of one tick is
// idempotent; missing publications and rollback restart rather than inventing
// endpoints. Membership and discrete flags are hard boundaries.
func (h *History) Record(tick uint32, pose []frame.PieceView) {
	if h.valid && tick == h.tick {
		return
	}
	if h.valid && (tick < h.tick || tick-h.tick != 1) {
		h.Reset()
	}
	if len(h.pieces) != len(pose) {
		h.Reset()
		if cap(h.pieces) < len(pose) {
			h.pieces = make([]piece, len(pose))
		} else {
			h.pieces = h.pieces[:len(pose)]
			clear(h.pieces)
		}
	}
	for i, p := range pose {
		t := &h.pieces[i]
		if !h.valid || !continuous(t.identity, p) {
			*t = piece{identity: p}
		}
		for j, v := range values(p) {
			t.axes[j].record(tick, v)
		}
	}
	h.tick, h.valid = tick, true
}

func continuous(a, b frame.PieceView) bool {
	return a.Index == b.Index && a.Name == b.Name && a.Hidden == b.Hidden && a.DontCache == b.DontCache && a.DontShade == b.DontShade && a.DontShadow == b.DontShadow
}
func values(p frame.PieceView) [6]int64 {
	return [6]int64{int64(p.Tx), int64(p.Ty), int64(p.Tz), int64(p.RotX), int64(p.RotY), int64(p.RotZ)}
}
func (a *axis) record(tick uint32, v int64) {
	if a.n > 0 {
		last := a.keys[a.n-1]
		if last.value == v {
			return
		}
		spacing := tick - last.tick
		// Keep one presentation phase through a variable gait cadence. Switching
		// back on a single dense key can otherwise move a monotone limb backward
		// when the next held key selects the delayed timeline again.
		if spacing > Delay {
			a.stepped = false
		} else if spacing > 1 {
			if !a.stepped {
				a.since = last.tick
			}
			a.stepped = true
		}
	}
	if a.n == len(a.keys) {
		copy(a.keys[:], a.keys[1:])
		a.n--
	}
	a.keys[a.n] = point{tick, v}
	a.n++
}
func mix(a, b, f int64, angle bool) int64 {
	delta := b - a
	if angle {
		delta = int64(int16(uint16(b) - uint16(a)))
	}
	return a + delta*f/one
}
func (a *axis) sample(at int64, angle bool) int64 {
	at = max(at, int64(a.since)*one)
	for i := 1; i < a.n; i++ {
		p, q := a.keys[i-1], a.keys[i]
		if at <= int64(q.tick)*one {
			f := max(int64(0), (at-int64(p.tick)*one)/int64(q.tick-p.tick))
			return mix(p.value, q.value, f, angle)
		}
	}
	return a.keys[a.n-1].value
}

// Blend starts with the current tick's discrete fields. Only visible continuous
// membership blends. Axes that exhibited two/three-tick holds sample tick+fraction-Delay;
// every other axis samples prev..cur normally. Unknown future endpoints hold.
// Longer authored holds retain ordinary interpolation, avoiding late corrections
// from an endpoint outside the lookback. fraction16 is clamped to [0,65535].
func (h *History) Blend(dst, prev, cur []frame.PieceView, tick uint32, fraction16 int64) []frame.PieceView {
	dst = append(dst[:0], cur...)
	if len(prev) != len(cur) {
		return dst
	}
	fraction16 = max(int64(0), min(one-1, fraction16))
	at := (int64(tick)-Delay)*one + fraction16
	for i, p := range cur {
		q := prev[i]
		if p.Hidden || q.Hidden || !continuous(p, q) {
			continue
		}
		pv, qv := values(p), values(q)
		tracked := h.valid && h.tick == tick && len(h.pieces) == len(cur) && continuous(h.pieces[i].identity, p)
		for j := range pv {
			value := mix(qv[j], pv[j], fraction16, j >= 3)
			if tracked {
				a := &h.pieces[i].axes[j]
				if a.stepped && a.n > 1 {
					value = a.sample(at, j >= 3)
				}
			}
			pv[j] = value
		}
		dst[i].Tx, dst[i].Ty, dst[i].Tz = numeric.Fixed(pv[0]), numeric.Fixed(pv[1]), numeric.Fixed(pv[2])
		dst[i].RotX, dst[i].RotY, dst[i].RotZ = uint16(pv[3]), uint16(pv[4]), uint16(pv[5])
	}
	return dst
}
