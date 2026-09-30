package main

// Unit-sync bodies: the 0x2c record's movement list and scheduled status,
// implemented from research/formats/tad.md §7 [fmt tad §7]. Only fields the
// format marks Established are decoded. Where it marks a field Unknown, the
// reader stops or skips and counts, and never assigns a meaning.

// bitReader reads fields packed least-significant bit first with no
// alignment between entries; signed fields are two's complement [fmt tad §7].
type bitReader struct {
	b   []byte
	pos int // bit position
}

func (r *bitReader) left() int { return len(r.b)*8 - r.pos }

// u reads an unsigned field of n <= 32 bits.
func (r *bitReader) u(n int) (uint32, bool) {
	if n > r.left() {
		return 0, false
	}
	var v uint32
	for i := 0; i < n; i++ {
		p := r.pos + i
		if r.b[p>>3]&(1<<(p&7)) != 0 {
			v |= 1 << i
		}
	}
	r.pos += n
	return v, true
}

// s reads a signed field of n <= 32 bits.
func (r *bitReader) s(n int) (int32, bool) {
	v, ok := r.u(n)
	if !ok {
		return 0, false
	}
	if n < 32 && v&(1<<(n-1)) != 0 {
		v |= ^uint32(0) << n
	}
	return int32(v), true
}

// bitLen is the number of right shifts that take c to zero: the width of a
// definition index for a retained table of c rows, row zero included
// [fmt tad §7].
func bitLen(c int) int {
	n := 0
	for c > 0 {
		c >>= 1
		n++
	}
	return n
}

// groundMove is a ground movement entry: blocked flag, then up to three
// path points in whole world units [fmt tad §7 "Ground movement entry"].
type groundMove struct {
	Blocked bool
	N       int
	Pts     [3][2]int32
}

// airMove is an air movement entry [fmt tad §7 "Air movement entry"]. Only
// the fields a replay reads are kept; positions are whole world units (the
// integer part of the signed 16.16 words).
type airMove struct {
	Sel     int
	Flags   uint8
	Target  int // selector 1 bit 0: followed unit ID, zero for none
	HasGoal bool
	GoalX   int32 // selector 1 bit 5 position
	GoalZ   int32
	HasPos  bool
	PosX    int32 // selector 2 position
	PosZ    int32
	State   int
}

// moveEntry is one decoded movement entry.
type moveEntry struct {
	Slot   int
	Def    int
	Air    bool
	Ground groundMove
	AirMv  airMove
}

// syncStatus is the scheduled status of one unit slot [fmt tad §7
// "Scheduled status"].
type syncStatus struct {
	Present   bool
	Def       int // zero: the slot is empty
	Health    int32
	Remaining int // construction remaining byte, 0 built .. 255 unbuilt
	// Flags is the activation/status byte: bit 0 drives Activate and
	// Deactivate, bit 3 StartBuilding and StopBuilding [fmt tad §7].
	Flags            int
	Mode             int // occupancy mode; not a moving/stopped flag
	Attached         bool
	X, Y, Z          int32 // whole world units, when not attached
	RawX, RawY, RawZ int32 // signed 16.16, before the arithmetic right shift
	// Heading is the first of the three angle16 words (heading, pitch, bank).
	// Speed is the trailing signed 16.16 mover scalar speed, read only when
	// the body length fits the form that carries it [fmt tad §7].
	Heading  uint16
	HasSpeed bool
	Speed    int32
}

// syncCounts counts what the unit-sync decoder saw and what it skipped.
type syncCounts struct {
	Bodies        int64            `json:"bodies"`
	Idle          int64            `json:"idle_ff"`
	Ground        int64            `json:"ground_entries"`
	GroundPoints  int64            `json:"ground_points"`
	Air           int64            `json:"air_entries"`
	AirSel        [4]int64         `json:"-"`
	AirSelOut     map[string]int64 `json:"air_selectors"`
	DefAgree      int64            `json:"def_agree"`
	DefMismatch   int64            `json:"def_mismatch"`
	DefNoStart    int64            `json:"def_no_start"`
	DefOutOfTable int64            `json:"def_out_of_table"`
	// GrammarUnknown counts entries whose definition has no known canfly,
	// so neither grammar can be chosen; the rest of that body is skipped.
	GrammarUnknown int64 `json:"grammar_unknown"`
	// Selector3 counts air entries with selector 3, whose sender encoding
	// the format leaves Unknown; the rest of that body is skipped.
	Selector3 int64 `json:"air_selector3_skipped"`
	Truncated int64 `json:"truncated_bodies"`
	// Status outcomes. SpeedSkipped counts statuses whose trailing mover
	// speed word was present by length; its presence rule is Unknown
	// [fmt tad §7], so it is never read. LengthMismatch counts bodies whose
	// length fits neither the no-speed nor the speed form.
	StatusAbsent   int64 `json:"status_absent"`
	StatusEmpty    int64 `json:"status_empty"`
	StatusAttached int64 `json:"status_attached"`
	StatusPos      int64 `json:"status_position"`
	SpeedSkipped   int64 `json:"status_speed_word_skipped"`
	NoSpeed        int64 `json:"status_without_speed_word"`
	LengthMismatch int64 `json:"status_length_mismatch"`
	StatusDefAgree int64 `json:"status_def_agree"`
	StatusDefMis   int64 `json:"status_def_mismatch"`
	// BodiesSkipped counts bodies abandoned before their scheduled status.
	BodiesSkipped int64 `json:"bodies_skipped"`
}

// syncDecoder decodes unit-sync bodies for one recording. W is the
// definition-index width; grammar reports whether definition index d flies
// (canfly selects the air grammar) and whether that is known.
type syncDecoder struct {
	W       int
	grammar func(d int) (air, known bool)
	st      syncCounts
}

// decodeBody decodes one reconstructed 0x2c body: the bytes after the
// three-byte header and the four tick bytes. onMove receives each movement
// entry in order. The returned status is valid only when ok.
func (d *syncDecoder) decodeBody(body []byte, onMove func(moveEntry)) (syncStatus, bool) {
	d.st.Bodies++
	r := bitReader{b: body}
	var ss syncStatus
	for {
		slot, ok := r.u(16)
		if !ok {
			d.st.Truncated++
			return ss, false
		}
		if slot == 0xffff {
			break
		}
		def, ok := r.u(d.W)
		if !ok {
			d.st.Truncated++
			return ss, false
		}
		e := moveEntry{Slot: int(slot), Def: int(def)}
		air, known := d.grammar(int(def))
		if !known {
			d.st.GrammarUnknown++
			d.st.BodiesSkipped++
			return ss, false
		}
		e.Air = air
		if !air {
			if !d.ground(&r, &e.Ground) {
				d.st.Truncated++
				return ss, false
			}
			d.st.Ground++
			d.st.GroundPoints += int64(e.Ground.N)
		} else {
			res := d.air(&r, &e.AirMv)
			if res == airTruncated {
				d.st.Truncated++
				return ss, false
			}
			if res == airSelector3 {
				d.st.Selector3++
				d.st.BodiesSkipped++
				return ss, false
			}
			d.st.Air++
			d.st.AirSel[e.AirMv.Sel]++
		}
		onMove(e)
	}
	// Scheduled status: presence bit, then a W-bit definition index whose
	// zero ends the status (an empty slot).
	pres, ok := r.u(1)
	if !ok {
		d.st.Truncated++
		return ss, false
	}
	if pres == 0 {
		d.st.StatusAbsent++
		return ss, d.checkLength(&r, body, false)
	}
	ss.Present = true
	def, ok := r.u(d.W)
	if !ok {
		d.st.Truncated++
		return ss, false
	}
	ss.Def = int(def)
	if def == 0 {
		d.st.StatusEmpty++
		return ss, d.checkLength(&r, body, false)
	}
	h, ok1 := r.s(16)
	rem, ok2 := r.u(8)
	fl, ok3 := r.u(8)   // activation/status flags
	mode, ok4 := r.u(2) // occupancy mode
	att, ok5 := r.u(1)
	if !(ok1 && ok2 && ok3 && ok4 && ok5) {
		d.st.Truncated++
		return ss, false
	}
	ss.Health, ss.Remaining, ss.Flags = h, int(rem), int(fl)
	ss.Mode = int(mode)
	if att == 1 {
		ss.Attached = true
		if _, ok := r.u(15); !ok { // attached-to unit ID
			d.st.Truncated++
			return ss, false
		}
		if _, ok := r.s(8); !ok { // attachment piece
			d.st.Truncated++
			return ss, false
		}
		d.st.StatusAttached++
		return ss, d.checkLength(&r, body, false)
	}
	var xyz [3]int32
	for i := range xyz {
		v, ok := r.s(32)
		if !ok {
			d.st.Truncated++
			return ss, false
		}
		xyz[i] = v
	}
	for i := 0; i < 3; i++ { // heading, pitch, bank
		a, ok := r.u(16)
		if !ok {
			d.st.Truncated++
			return ss, false
		}
		if i == 0 {
			ss.Heading = uint16(a)
		}
	}
	ss.RawX, ss.RawY, ss.RawZ = xyz[0], xyz[1], xyz[2]
	ss.X, ss.Y, ss.Z = xyz[0]>>16, xyz[1]>>16, xyz[2]>>16
	d.st.StatusPos++
	ok = d.checkLength(&r, body, true)
	if ok && (r.pos+7)/8 != len(body) {
		// The length fits only the form with the mover speed word.
		if v, okv := r.s(32); okv {
			ss.HasSpeed, ss.Speed = true, v
		}
	}
	return ss, ok
}

// checkLength compares the bits consumed with the body length. The sender
// rounds the whole record up to bytes; after a positioned status a signed
// 32-bit mover speed may follow, present only when the unit has a mover,
// which has no wire presence bit [fmt tad §7]. That word is never read: a
// body whose length fits the speed form is counted as skipping it.
func (d *syncDecoder) checkLength(r *bitReader, body []byte, speedPossible bool) bool {
	bytesFor := func(bits int) int { return (bits + 7) / 8 }
	if bytesFor(r.pos) == len(body) {
		if speedPossible {
			d.st.NoSpeed++
		}
		return true
	}
	if speedPossible && bytesFor(r.pos+32) == len(body) {
		d.st.SpeedSkipped++
		return true
	}
	d.st.LengthMismatch++
	return false
}

// ground reads a ground entry after its slot and definition: blocked flag
// 1 bit, point count 2 bits, then that many signed X16/Z16 pairs
// [fmt tad §7 "Ground movement entry"].
func (d *syncDecoder) ground(r *bitReader, g *groundMove) bool {
	b, ok1 := r.u(1)
	n, ok2 := r.u(2)
	if !ok1 || !ok2 {
		return false
	}
	g.Blocked, g.N = b == 1, int(n)
	for i := 0; i < g.N; i++ {
		x, okx := r.s(16)
		z, okz := r.s(16)
		if !okx || !okz {
			return false
		}
		g.Pts[i] = [2]int32{x, z}
	}
	return true
}

type airResult int

const (
	airOK airResult = iota
	airTruncated
	airSelector3
)

// air reads an air entry after its slot and definition [fmt tad §7 "Air
// movement entry"]: a 2-bit selector, its payload, then the 2-bit
// movement-state value. Selector 3 has no audited sender encoding, so the
// reader stops there rather than invent one.
func (d *syncDecoder) air(r *bitReader, a *airMove) airResult {
	sel, ok := r.u(2)
	if !ok {
		return airTruncated
	}
	a.Sel = int(sel)
	fixed3 := func() (x, y, z int32, ok bool) {
		var v [3]int32
		for i := range v {
			w, ok := r.s(32)
			if !ok {
				return 0, 0, 0, false
			}
			v[i] = w >> 16
		}
		return v[0], v[1], v[2], true
	}
	switch sel {
	case 0:
	case 1:
		f, ok := r.u(8)
		if !ok {
			return airTruncated
		}
		a.Flags = uint8(f)
		// Conditional fields in the exact wire order: bit 0, 4, 3, 6, 5.
		if f&(1<<0) != 0 {
			if _, ok := r.s(16); !ok { // attachment piece
				return airTruncated
			}
			t, ok := r.u(16)
			if !ok {
				return airTruncated
			}
			a.Target = int(t)
		}
		if f&(1<<4) != 0 { // horizontal arrival radius
			if _, ok := r.s(16); !ok {
				return airTruncated
			}
		}
		if f&(1<<3) != 0 { // vertical offset
			if _, ok := r.s(16); !ok {
				return airTruncated
			}
		}
		if f&(1<<6) != 0 { // heading angle
			if _, ok := r.u(16); !ok {
				return airTruncated
			}
		}
		if f&(1<<5) != 0 {
			x, _, z, ok := fixed3()
			if !ok {
				return airTruncated
			}
			a.HasGoal, a.GoalX, a.GoalZ = true, x, z
		}
	case 2:
		turn, ok := r.u(1)
		if !ok {
			return airTruncated
		}
		x, _, z, ok := fixed3()
		if !ok {
			return airTruncated
		}
		if _, _, _, ok := fixed3(); !ok { // velocity
			return airTruncated
		}
		if turn == 1 {
			if _, ok := r.u(16); !ok { // target heading
				return airTruncated
			}
		}
		a.HasPos, a.PosX, a.PosZ = true, x, z
	case 3:
		return airSelector3
	}
	st, ok := r.u(2)
	if !ok {
		return airTruncated
	}
	a.State = int(st)
	return airOK
}
