package main

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// authoredTNTWithSizedMinimap appends a present w×h authored minimap to the
// shared 10×10-cell fixture, whose terrain tiles are palette index 37. The
// fixture's play area fits a 126×31 radar picture, so a source must store at
// least 252×62 pixels to feed the fixed two-by-two reducer [03 §3.7].
func authoredTNTWithSizedMinimap(w, h int, pixel func(x, y int) byte) []byte {
	data := authoredTNTWithoutMinimap(false)
	offset := len(data)
	data = append(data, make([]byte, 8+w*h)...)
	binary.LittleEndian.PutUint32(data[0x28:], uint32(offset))
	binary.LittleEndian.PutUint32(data[0x2c:], 1)
	binary.LittleEndian.PutUint32(data[offset:], uint32(w))
	binary.LittleEndian.PutUint32(data[offset+4:], uint32(h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			data[offset+8+y*w+x] = pixel(x, y)
		}
	}
	return data
}

// radarFixture loads the authored TNT through the production terrain loader
// and builds the battle radar through the production catalog path.
func radarFixture(t *testing.T, data []byte, pal *palette.Tables) (*world.Terrain, *render.RadarSurface) {
	t.Helper()
	fs := absentMinimapFS{data: data}
	terrain, err := world.Load(fs, nil, "no-mini")
	if err != nil {
		t.Fatalf("terrain load: %v", err)
	}
	cat := &content.Catalog{Maps: map[string]*content.MapHeader{"no-mini": {LogicalTNT: "maps/no-mini.tnt"}}}
	return terrain, buildBattleRadar(fs, cat, "no-mini", terrain, pal)
}

// Host fallback (DESIGN_PRESENTATION_CLIENT §3.1 C4): an authored minimap too
// small for the fixed reducer is treated as absent. The battle still gets a
// picture, and it is byte-identical to the generated terrain picture of the
// same map with its present flag clear. One missing column or row is enough
// to fall back; the exact required size keeps the authored picture.
func TestUndersizedTNTMinimapFallsBackToGeneratedRadar(t *testing.T) {
	pal := &palette.Tables{}
	for i := range pal.Alpha {
		pal.Alpha[i] = byte(i / 256)
	}
	generatedTerrain, err := world.Load(absentMinimapFS{data: authoredTNTWithoutMinimap(false)}, nil, "no-mini")
	if err != nil {
		t.Fatal(err)
	}
	generated := buildBattleRadar(nil, nil, "no-mini", generatedTerrain, pal)
	if generated == nil {
		t.Fatal("flag-clear map produced no generated picture")
	}
	if layout := camera.LayoutMinimap(generatedTerrain.PlayRight, generatedTerrain.PlayBottom); layout.W != 126 || layout.H != 31 {
		t.Fatalf("fixture radar picture %dx%d, want 126x31", layout.W, layout.H)
	}
	authored := func(int, int) byte { return 91 }
	for _, tc := range []struct {
		name       string
		w, h       int
		authorship bool
	}{
		{"common third-party 128x128", 128, 128, false},
		{"one column short", 251, 62, false},
		{"one row short", 252, 61, false},
		{"exact fixed-half size", 252, 62, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, radar := radarFixture(t, authoredTNTWithSizedMinimap(tc.w, tc.h, authored), pal)
			if radar == nil {
				t.Fatal("authored minimap stopped the radar picture, which blocks battle entry")
			}
			if tc.authorship {
				for i, pixel := range radar.Bits {
					if pixel != 91 {
						t.Fatalf("pixel %d = %d, want authored 91", i, pixel)
					}
				}
				return
			}
			if radar.W != generated.W || radar.H != generated.H || radar.Pitch != generated.Pitch || !bytes.Equal(radar.Bits, generated.Bits) {
				t.Fatalf("fallback picture %dx%d differs from the generated picture %dx%d", radar.W, radar.H, generated.W, generated.H)
			}
		})
	}
}

// A safely sized authored source keeps the established fixed two-by-two
// reduction exactly: stored stride, top-left blocks and the row-first
// three-lookup ALP order [03 §3.7]. The asymmetric table and the position
// pattern fail any ratio resize, crop or column-first pairing.
func TestFittingTNTMinimapKeepsFixedHalfReduction(t *testing.T) {
	pal := &palette.Tables{}
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			pal.Alpha[a*256+b] = byte((a + 2*b) & 255)
		}
	}
	pattern := func(x, y int) byte { return byte((x + 7*y) & 255) }
	alp := func(a, b byte) byte { return pal.Alpha[int(a)*256+int(b)] }
	for _, size := range [][2]int{{252, 62}, {252, 252}, {300, 80}} {
		w, h := size[0], size[1]
		terrain, radar := radarFixture(t, authoredTNTWithSizedMinimap(w, h, pattern), pal)
		if radar == nil {
			t.Fatalf("%dx%d authored minimap produced no picture", w, h)
		}
		for y := 0; y < radar.H; y++ {
			for x := 0; x < radar.W; x++ {
				top := alp(pattern(2*x, 2*y), pattern(2*x+1, 2*y))
				bottom := alp(pattern(2*x, 2*y+1), pattern(2*x+1, 2*y+1))
				if got, want := radar.Bits[y*radar.W+x], alp(top, bottom); got != want {
					t.Fatalf("%dx%d source: pixel %d,%d = %d, want fixed-half %d", w, h, x, y, got, want)
				}
			}
		}
		// The loader hands a fitting source to the reducer unchanged.
		baked := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				baked[y*w+x] = pattern(x, y)
			}
		}
		layout := camera.LayoutMinimap(terrain.PlayRight, terrain.PlayBottom)
		direct := render.BuildRadarPicture(terrain, terrain.PlayRight, terrain.PlayBottom, layout, baked, w, h, pal)
		if direct == nil || !bytes.Equal(direct.Bits, radar.Bits) {
			t.Fatalf("%dx%d source: battle radar differs from the direct builder", w, h)
		}
	}
}
