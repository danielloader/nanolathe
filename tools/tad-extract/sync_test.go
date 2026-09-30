package main

import (
	"encoding/binary"
	"testing"
)

// Unit-sync bodies below are authored here from the format description
// [fmt tad §7]; none is copied from a recording.

// bitWriter packs fields least-significant bit first with no alignment.
type bitWriter struct {
	b   []byte
	pos int
}

func (w *bitWriter) put(v uint32, n int) {
	for i := 0; i < n; i++ {
		if w.pos>>3 >= len(w.b) {
			w.b = append(w.b, 0)
		}
		if v&(1<<i) != 0 {
			w.b[w.pos>>3] |= 1 << (w.pos & 7)
		}
		w.pos++
	}
}

func (w *bitWriter) fixed(world int32) { w.put(uint32(world<<16), 32) }

const testW = 9

// testGrammar: definition 36 is a ground unit, 40 flies, 50 has no known
// canfly.
func testGrammar(d int) (bool, bool) {
	switch d {
	case 36:
		return false, true
	case 40:
		return true, true
	}
	return false, false
}

// positionedStatus appends a scheduled status for definition def at (x, z)
// and returns the bit length before any mover speed word.
func positionedStatus(w *bitWriter, def uint32, x, z int32) {
	w.put(1, 1) // present
	w.put(def, testW)
	w.put(0xfff0, 16) // health (signed word)
	w.put(0, 8)       // construction remaining
	w.put(9, 8)       // activation/status flags
	w.put(1, 2)       // occupancy
	w.put(0, 1)       // not attached
	w.fixed(x)
	w.fixed(-3)
	w.fixed(z)
	w.put(0x4000, 16)
	w.put(0, 16)
	w.put(0, 16)
}

func TestSyncBodyGroundAirStatus(t *testing.T) {
	var w bitWriter
	// Ground entry: slot 0, definition 36, blocked, two points (one negative).
	w.put(0, 16)
	w.put(36, testW)
	w.put(1, 1)
	w.put(2, 2)
	w.put(uint32(1200)&0xffff, 16)
	w.put(uint32(800)&0xffff, 16)
	w.put(uint32(0x10000-5)&0xffff, 16)
	w.put(uint32(64)&0xffff, 16)
	// Air entry, selector 1 with flags 0 (target), 4 (radius), 5 (position).
	w.put(7, 16)
	w.put(40, testW)
	w.put(1, 2)
	w.put(1|1<<4|1<<5, 8)
	w.put(0xffff, 16) // attachment piece -1
	w.put(502, 16)    // followed unit
	w.put(96, 16)     // radius
	w.fixed(3000)
	w.fixed(200)
	w.fixed(1500)
	w.put(2, 2) // movement state: airborne
	// Air entry, selector 2 with a turn heading.
	w.put(8, 16)
	w.put(40, testW)
	w.put(2, 2)
	w.put(1, 1)
	w.fixed(100)
	w.fixed(50)
	w.fixed(4000)
	w.fixed(1)
	w.fixed(0)
	w.fixed(-1)
	w.put(0x8000, 16)
	w.put(2, 2)
	w.put(0xffff, 16) // terminator
	positionedStatus(&w, 36, 1234, 567)
	withSpeed := append([]byte(nil), w.b...)
	var sw bitWriter
	sw.b, sw.pos = withSpeed, w.pos
	sw.put(0x12345, 32) // mover speed: present by length, never read

	for _, tc := range []struct {
		body  []byte
		speed bool
	}{{w.b, false}, {sw.b, true}} {
		d := syncDecoder{W: testW, grammar: testGrammar}
		var got []moveEntry
		st, ok := d.decodeBody(tc.body, func(e moveEntry) { got = append(got, e) })
		if !ok || len(got) != 3 {
			t.Fatalf("speed=%v: ok %v entries %d counts %+v", tc.speed, ok, len(got), d.st)
		}
		g := got[0]
		if g.Air || g.Slot != 0 || g.Def != 36 || !g.Ground.Blocked || g.Ground.N != 2 ||
			g.Ground.Pts[0] != [2]int32{1200, 800} || g.Ground.Pts[1] != [2]int32{-5, 64} {
			t.Fatalf("ground entry %+v", g)
		}
		a := got[1].AirMv
		if !got[1].Air || a.Sel != 1 || a.Target != 502 || !a.HasGoal || a.GoalX != 3000 || a.GoalZ != 1500 || a.State != 2 {
			t.Fatalf("selector-1 entry %+v", got[1])
		}
		b := got[2].AirMv
		if b.Sel != 2 || !b.HasPos || b.PosX != 100 || b.PosZ != 4000 || b.State != 2 {
			t.Fatalf("selector-2 entry %+v", got[2])
		}
		if !st.Present || st.Def != 36 || st.X != 1234 || st.Z != 567 || st.Y != -3 || st.Health != -16 || st.Flags != 9 {
			t.Fatalf("status %+v", st)
		}
		if tc.speed && (d.st.SpeedSkipped != 1 || d.st.NoSpeed != 0) || !tc.speed && (d.st.NoSpeed != 1 || d.st.SpeedSkipped != 0) {
			t.Fatalf("speed=%v: speed counts %+v", tc.speed, d.st)
		}
	}
}

// An idle body is 16 + 1 + W bits: the terminator, presence and an empty
// slot; the historical eleven-byte packet is its W=9 form [fmt tad §7].
func TestSyncBodyIdleWidth(t *testing.T) {
	var w bitWriter
	w.put(0xffff, 16)
	w.put(1, 1)
	w.put(0, testW)
	if len(w.b)+7 != 11 {
		t.Fatalf("idle body %d bytes, reconstructed 0x2c %d, want 11", len(w.b), len(w.b)+7)
	}
	d := syncDecoder{W: testW, grammar: testGrammar}
	st, ok := d.decodeBody(w.b, func(moveEntry) { t.Fatal("idle body has no movement") })
	if !ok || !st.Present || st.Def != 0 || d.st.StatusEmpty != 1 {
		t.Fatalf("idle body: ok %v %+v %+v", ok, st, d.st)
	}
	if bitLen(283) != 9 || bitLen(256) != 9 || bitLen(255) != 8 {
		t.Fatal("width is the bit length of the retained row count")
	}
}

// Unknown material stops the body and is counted, never guessed.
func TestSyncBodyUnknowns(t *testing.T) {
	var w bitWriter
	w.put(3, 16)
	w.put(40, testW)
	w.put(3, 2) // selector 3: no audited sender encoding
	w.put(0, 30)
	d := syncDecoder{W: testW, grammar: testGrammar}
	if _, ok := d.decodeBody(w.b, func(moveEntry) {}); ok || d.st.Selector3 != 1 {
		t.Fatalf("selector 3: %+v", d.st)
	}
	var u bitWriter
	u.put(3, 16)
	u.put(50, testW) // no known canfly
	u.put(0, 30)
	if _, ok := d.decodeBody(u.b, func(moveEntry) {}); ok || d.st.GrammarUnknown != 1 {
		t.Fatalf("unknown grammar: %+v", d.st)
	}
	// A status body one byte longer than either form fits no length.
	var l bitWriter
	l.put(0xffff, 16)
	positionedStatus(&l, 36, 1, 1)
	body := append(l.b, 0, 0, 0, 0, 0, 0)
	if _, ok := d.decodeBody(body, func(moveEntry) {}); ok || d.st.LengthMismatch != 1 {
		t.Fatalf("length mismatch: %+v", d.st)
	}
}

// TestMoveTrackerJoins feeds authored subpackets through the tracker: a
// start, a Smartpak sync whose movement entry and scheduled status name the
// started unit, a reclaim transition and a snapshot.
func TestMoveTrackerJoins(t *testing.T) {
	su := &setup{Header: header{MaxUnits: 500}}
	su.Players = []player{{Number: 1, Side: 0}, {Number: 2, Side: 1}}
	su.Status = []playerInfo{{Number: 1, DPID: 200}, {Number: 2, DPID: 100}}
	m := &moveTracker{x: newExtractor(su), tracks: map[*buildEvent]*unitTrack{},
		reclaims: map[int][][3]int32{}, economy: map[int][][7]int32{}}
	m.dec = syncDecoder{W: testW, grammar: testGrammar}
	// Player 1 ranks second by transport identity: IDs 501..1000.
	m.onSub(subContext{Sender: 1, Tick: 999}, buildStart(36, 501, 1000, 50, 2000))
	var w bitWriter
	w.put(0, 16) // slot 0 = unit 501
	w.put(36, testW)
	w.put(0, 1)
	w.put(1, 2)
	w.put(1100, 16)
	w.put(2100, 16)
	w.put(0xffff, 16)
	positionedStatus(&w, 36, 1010, 2010)
	sync := append([]byte{0xfd, 0, 0}, w.b...)
	binary.LittleEndian.PutUint16(sync[1:], uint16(len(w.b)+7))
	m.onSub(subContext{Sender: 1, Tick: 1000}, sync) // 1000 mod 500 = slot 0
	m.onSub(subContext{Sender: 1, Tick: 1001}, sync) // same path again: dropped as a repeat
	m.onSub(subContext{Sender: 1, Tick: 1002}, []byte{0x0f, 0xff, 10, 0, 20, 0})
	snap := make([]byte, 58)
	snap[0] = 0x28
	binary.LittleEndian.PutUint32(snap[46:], 0x44fa0000) // 2000.0 metal produced
	m.onSub(subContext{Sender: 1, Tick: 1003}, snap)

	ev := m.x.byNet[501]
	tr := m.tracks[ev]
	if tr == nil || len(tr.Path) != 1 || m.tc.PathRepeat != 1 {
		t.Fatalf("path %+v repeats %d", tr, m.tc.PathRepeat)
	}
	if p := tr.Path[0]; p[0] != 1000 || p[2] != 1 || p[3] != 1100 || p[4] != 2100 {
		t.Fatalf("path row %v", p)
	}
	if m.dec.st.DefAgree != 2 || m.dec.st.StatusDefAgree != 1 || len(tr.Pos) != 1 || tr.Pos[0][1] != 1010 {
		t.Fatalf("joins %+v pos %v", m.dec.st, tr.Pos)
	}
	if r := m.reclaims[1]; len(r) != 1 || r[0] != [3]int32{1002, 10, 20} {
		t.Fatalf("reclaims %v", r)
	}
	if e := m.economy[1]; len(e) != 1 || e[0][1] != 2000 {
		t.Fatalf("economy %v", e)
	}
}
