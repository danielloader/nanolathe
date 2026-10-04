package gpurender

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// FNT admission uses the whole unadjusted string rectangle, including its
// one-past right/bottom edges [03 R-FONT-01 §3].
func TestGlyphsWholeStringAdmissionAndBaseline(t *testing.T) {
	r, _ := schedulerFixture(t)
	font := &formats.FNT{Height: 2, Baseline: 1}
	font.Glyphs['A'] = &formats.FNTGlyph{Width: 3, Height: 2, Bits: []byte{0xfc}}
	for _, tc := range []struct {
		name string
		x, y int32
		want bool
	}{
		{"inside", 2, 2, true}, {"inclusive one-past bounds", 3, 3, true}, {"left", 1, 2, false}, {"top", 2, 1, false},
		{"right last pixel", 4, 2, false}, {"bottom last pixel", 2, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.sched.resetFrame(16, 16)
			r.Glyphs(drawlist.Glyphs{Font: font, Text: "A", X: tc.x, Y: tc.y, Color: 9, HasClip: true, Clip: drawlist.Rect{X: 2, Y: 2, W: 5, H: 4}})
			verts := r.sched.classVerts(schedOpaque)
			if (len(verts) != 0) != tc.want {
				t.Fatalf("vertices=%d, admitted=%v", len(verts), tc.want)
			}
			if tc.want && (verts[0].DstY != float32(tc.y-1) || verts[3].DstY != float32(tc.y+1)) {
				t.Fatalf("baseline overrun was clipped: %+v", verts)
			}
		})
	}
	// Host-storage clipping retains source coordinates for an admitted glyph
	// whose baseline places its first row above the framebuffer.
	r.sched.resetFrame(16, 16)
	r.Glyphs(drawlist.Glyphs{Font: font, Text: "A", X: 2, Y: 0, Color: 9})
	verts := r.sched.classVerts(schedOpaque)
	if len(verts) != 4 || verts[0].DstY != 0 || verts[3].DstY != 1 || verts[2].SrcY-verts[0].SrcY != 1 {
		t.Fatalf("host-storage crop=%+v", verts)
	}
}

// Text layout uses framebuffer pixels after anchor projection. Its clip follows
// the world, and its temporary transform bypass must survive rejected runs and
// restore the next world command (GPU design §16.3).
func TestWorldGlyphsKeepNativePixelsAndRestoreTransform(t *testing.T) {
	r, _ := schedulerFixture(t)
	font := &formats.FNT{Height: 5}
	font.Glyphs['A'] = &formats.FNTGlyph{Width: 3, Height: 5, Bits: []byte{0xff, 0xfe}}
	r.sched.resetFrame(64, 64)
	r.sched.setWorldTransform(.75, -.375, .625)
	r.worldW, r.worldH = 86, 86
	for _, dx := range []int32{-1, 0, 1} {
		r.Glyphs(drawlist.Glyphs{Font: font, Text: "AA", X: 32, Y: 32, ScreenOffsetX: -3 + dx, ScreenOffsetY: -8,
			HasClip: true, Clip: drawlist.Rect{X: 16, Y: 16, W: 40, H: 40}})
	}
	verts := r.sched.classVerts(schedOpaque)
	if len(verts) != 24 {
		t.Fatalf("native runs produced %d vertices, want three two-glyph runs", len(verts))
	}
	for run := 0; run < 3; run++ {
		first, second := verts[run*8], verts[run*8+4]
		if first.DstX != float32(20+run) || first.DstY != 17 || second.DstX-first.DstX != 3 || verts[run*8+3].DstY-first.DstY != 5 {
			t.Fatalf("run %d lost native size, offset or outline spacing: %+v", run, verts[run*8:run*8+8])
		}
	}
	// An anchor inside record bounds can still be outside the transformed clip.
	r.Glyphs(drawlist.Glyphs{Font: font, Text: "A", X: 12, Y: 32, HasClip: true, Clip: drawlist.Rect{X: 16, Y: 16, W: 40, H: 40}})
	// Whole-string framebuffer admission must also reject the far edge even
	// though the recording extends beyond it.
	r.Glyphs(drawlist.Glyphs{Font: font, Text: "AA", X: 80, Y: 32})
	if got := len(r.sched.classVerts(schedOpaque)); got != len(verts) {
		t.Fatalf("rejected text added %d vertices", got-len(verts))
	}
	if !r.sched.worldOn || r.sched.txx(32) != 23.625 || r.sched.txy(32) != 24.625 {
		t.Fatal("glyph replay did not restore the world transform")
	}
	r.Fill(drawlist.Fill{Rect: drawlist.Rect{X: 32, Y: 32, W: 8, H: 8}})
	verts = r.sched.classVerts(schedOpaque)
	last := verts[len(verts)-4:]
	if last[0].DstX != 23.625 || last[0].DstY != 24.625 || last[3].DstX-last[0].DstX != 6 || last[3].DstY-last[0].DstY != 6 {
		t.Fatalf("world fill following text lost its transform: %+v", last)
	}
}
