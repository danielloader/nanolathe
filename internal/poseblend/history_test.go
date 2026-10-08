package poseblend

import (
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"slices"
	"testing"
)

func TestHeldAxisAndContinuousAim(t *testing.T) {
	var h History
	var dst []frame.PieceView
	prev := []frame.PieceView{{RotZ: 65530}}
	for tick := uint32(0); tick <= 30; tick++ {
		cur := []frame.PieceView{{Tx: numeric.Fixed(tick/3) * 9 << 16, RotY: uint16(tick * 120), RotZ: uint16(65530 + uint16(tick/3)*12)}}
		h.Record(tick, cur)
		if tick >= 6 {
			for _, f := range []int64{0, 16384, 32768, 49152} {
				dst = h.Blend(dst, prev, cur, tick, f)
				want := (int64(tick)-3)*3*65536 + f*3
				if delta := int64(dst[0].Tx) - want; delta < -9 || delta > 9 {
					t.Fatalf("tick %d/%d held = %d want %d", tick, f, dst[0].Tx, want)
				}
				aim := prev[0].RotY + uint16(120*f/65536)
				if dst[0].RotY != aim {
					t.Fatalf("continuous aim delayed: %d want %d", dst[0].RotY, aim)
				}
				angle := uint16(int64(65530) + (int64(tick)-3)*4 + f*4/65536)
				if delta := int16(dst[0].RotZ - angle); delta < -1 || delta > 1 {
					t.Fatalf("wrapped angle %d want %d", dst[0].RotZ, angle)
				}
			}
		}
		if cur[0].Tx != numeric.Fixed(tick/3)*9<<16 {
			t.Fatal("mutated committed pose")
		}
		prev = cur
	}
	if got := testing.AllocsPerRun(100, func() { h.Record(30, prev); dst = h.Blend(dst, prev, prev, 30, 32768) }); got != 0 {
		t.Fatalf("steady sample allocates: %v", got)
	}
}

func TestHistoryDiscontinuitiesAndLongHolds(t *testing.T) {
	var h History
	p := []frame.PieceView{{}}
	for tick := uint32(0); tick <= 3; tick++ {
		p[0].Tx = numeric.Fixed(tick/3) * 9 << 16
		h.Record(tick, p)
	}
	// Owned endpoints survive caller mutation and repeated tick recording.
	p[0].Tx = 90 << 16
	h.Record(3, p)
	out := h.Blend(nil, []frame.PieceView{{}}, []frame.PieceView{{Tx: 9 << 16}}, 3, 32768)
	if delta := out[0].Tx - 98304; delta < -9 || delta > 9 {
		t.Fatal("record retained mutable source or repeated tick overwrote history")
	}
	for _, tick := range []uint32{8, 1} {
		h.Record(tick, p)
		out = h.Blend(out, []frame.PieceView{{Tx: 80 << 16}}, p, tick, 32768)
		if out[0].Tx != 85<<16 {
			t.Fatal("gap or rollback retained stepped history")
		}
	}
	h.Reset()
	p[0] = frame.PieceView{}
	for tick := uint32(0); tick <= 12; tick++ {
		if tick == 3 {
			p[0].Tx = 9 << 16
		}
		if tick == 12 {
			p[0].Tx = 90 << 16
		}
		h.Record(tick, p)
	}
	out = h.Blend(out, []frame.PieceView{{Tx: 9 << 16}}, p, 12, 32768)
	if out[0].Tx != (9+90)<<15 {
		t.Fatal("long hold extrapolated or retroactively blended")
	}
	p[0].Hidden = true
	h.Record(13, p)
	out = h.Blend(out, []frame.PieceView{{}}, p, 13, 0)
	if !slices.Equal(out, p) {
		t.Fatal("visibility boundary blended")
	}
	h.Reset()
	if _, ok := h.LastTick(); ok {
		t.Fatal("reset retained a timestamp")
	}
}

// Gait scripts can alternate dense and held keys. Their presentation timeline
// must not switch phase on each key or on a later run of dense keys.
func TestVariableCadenceNeverReversesMonotoneMotion(t *testing.T) {
	var h History
	var out []frame.PieceView
	prev := []frame.PieceView{{}}
	last := numeric.Fixed(0)
	for tick := uint32(0); tick <= 20; tick++ {
		key := tick
		switch tick {
		case 1, 2:
			key = 0
		case 5:
			key = 4
		case 8:
			key = 7
		}
		cur := []frame.PieceView{{Tx: numeric.Fixed(key) * 10 << 16}}
		h.Record(tick, cur)
		for _, f := range []int64{0, 16384, 32768, 49152} {
			out = h.Blend(out, prev, cur, tick, f)
			if out[0].Tx < last {
				t.Fatalf("tick %d/%d reverses %d -> %d", tick, f, last, out[0].Tx)
			}
			// The largest authored slope is ten units/tick; at 120 Hz no frame
			// should jump more than one quarter of it (plus fixed-point rounding).
			if tick >= 4 && out[0].Tx-last > (10<<16)/4+32 {
				t.Fatalf("tick %d/%d changed phase: %d -> %d", tick, f, last, out[0].Tx)
			}
			last = out[0].Tx
		}
		prev = cur
	}
}

func TestFirstTwoTickHoldDoesNotRewindContinuousAxis(t *testing.T) {
	var h History
	var out []frame.PieceView
	prev := []frame.PieceView{{}}
	last := numeric.Fixed(0)
	for tick := uint32(0); tick <= 12; tick++ {
		key := tick
		if tick == 4 {
			key = 3
		}
		cur := []frame.PieceView{{Tx: numeric.Fixed(key) * 10 << 16}}
		h.Record(tick, cur)
		for _, f := range []int64{0, 16384, 32768, 49152} {
			out = h.Blend(out, prev, cur, tick, f)
			if out[0].Tx < last {
				t.Fatalf("first hold at %d/%d rewound %d -> %d", tick, f, last, out[0].Tx)
			}
			last = out[0].Tx
		}
		prev = cur
	}
}
