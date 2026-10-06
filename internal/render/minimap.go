package render

// Minimap and radar surfaces [03 §3.4][07 §10][03 §3.6–§3.12].

import (
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// RadarSurface is an indexed radar surface descriptor [03 §3.6].
// W,H are RadarW/H 1..126 [07 §10][03 §3.4]. Pitch is (W+3)&~3 DWORD-aligned [03 §3.6][03 §4.1]. Bits are w*h indexed pixels (PALETTE.PAL indices).
type RadarSurface struct {
	W, H  int
	Pitch int    // (W+3)&~3 DWORD-aligned [03 §3.6]
	Bits  []byte // w*h indexed pixels (PALETTE.PAL indices), len = w*h when non-nil [03 §4.3]
	// Desc [12]uint32 // optional descriptor metadata
}

// At returns pixel at (x,y) with bounds check [03 §3.6].
func (r *RadarSurface) At(x, y int) (byte, bool) {
	if r == nil || r.Bits == nil {
		return 0, false
	}
	if x < 0 || y < 0 || x >= r.W || y >= r.H {
		return 0, false
	}
	idx := y*r.W + x
	if idx < 0 || idx >= len(r.Bits) {
		return 0, false
	}
	return r.Bits[idx], true
}

// Set writes pixel at (x,y) with bounds check [03 §3.6].
func (r *RadarSurface) Set(x, y int, v byte) bool {
	if r == nil || r.Bits == nil {
		return false
	}
	if x < 0 || y < 0 || x >= r.W || y >= r.H {
		return false
	}
	idx := y*r.W + x
	if idx < 0 || idx >= len(r.Bits) {
		return false
	}
	r.Bits[idx] = v
	return true
}

// BuildRadarPicture builds PICTURE from terrain or baked bytes [03 §3.7][03 §3.4][07 §10].
// playW = Wpix-32, playH = Hpix-128; RadarW/H are letterboxed via camera.LayoutMinimap.
// baked == nil or len==0 uses 2× supersampled tile sampling and ALP 2×2→1 blending [03 §3.7][fmt tnt][fmt pal].
// A baked source supplies fixed 2×2 blocks at its declared row stride, without
// cropping or ratio scaling. A source that fails BakedRadarSourceFits returns
// nil as checked host validation; retail's unsafe undersized-source outcome is
// unknown. The battle HUD tests the source first and substitutes the generated
// picture, so an unusable baked minimap never reaches this rejection there.
// The ALP table is mandatory: there is no nearest-neighbor compatibility path.
// The letterbox bars are not this function's to fill. No radar surface covers
// them: PICTURE, MAPPED and FINAL are all allocated at exactly the fitted
// RadarW x RadarH, and the presenter blits FINAL at (padX, padY) and paints no
// fill [03 R-MM-01 §3]. What shows in the bars is whatever the composer frame
// already holds there, and that is now settled: it is the side rail's own art.
//
// This previously carried an open-question marker, "which pixels those are —
// the side-panel shell's own art versus a cleared frame — is Unknown". Two
// established statements close it from opposite ends. The battle presenter
// fills the whole surface with palette index 0 once, stamps `PANELSIDE` at
// (0, 0), and never stamps it again; the per-frame composer repaints only the
// two horizontal strips and dirty GUI windows, so nothing touches the rail's
// columns under the radar canvas for the rest of the battle [07 R-HUD-05]. And
// the stock `PANELSIDE` raster is opaque at every pixel of its authored
// 129×480, radar area included, so the clear underneath is never what shows:
// over the 126×126 the canvas covers, ARMINT and CORINT each carry 15,876
// opaque pixels and not one transparent or index-0 pixel [fmt gaf]. The bars
// are therefore the panel art — the dithered near-black panel texture, which is
// why "cleared to 0" was a plausible reading, but the mechanism is the art and
// modded art would show through.
//
// Retail's own repaint pre-pass copies FINAL over that art each frame within
// the fitted rectangle only, which is why the bars persist rather than being
// overwritten [03 R-MM-01 §1].
func BuildRadarPicture(t *world.Terrain, playW, playH int32, m camera.Minimap, baked []byte, bakedW, bakedH int, tables *palette.Tables) *RadarSurface {
	if m.W <= 0 || m.H <= 0 || tables == nil {
		return nil
	}
	w := int(m.W)
	h := int(m.H)
	pitch := (w + 3) &^ 3 // [03 §3.6][03 §4.1] pitch (w+3)&~3
	bits := make([]byte, w*h)

	// Baked path: retain the stored row stride for fixed-half reduction [03 §3.7].
	if len(baked) > 0 {
		if !BakedRadarSourceFits(m, baked, bakedW, bakedH) {
			return nil
		}
		reduceHalfALP(bits, w, h, baked, bakedW, tables)
		return &RadarSurface{W: w, H: h, Pitch: pitch, Bits: bits}
	}

	if t == nil || len(t.TileSet) == 0 || len(t.TileIndices) == 0 || playW <= 0 || playH <= 0 {
		return nil
	}

	tw := 2 * w
	th := 2 * h
	temp := make([]byte, tw*th)

	tileW := int(t.CellW / 2) // [03 §2.2] TileW=Wcells/2 [03 §3.7]
	tileH := int(t.CellH / 2)
	if tileW <= 0 || tileH <= 0 || len(t.TileIndices) < tileW*tileH {
		return nil
	}
	tileCount := len(t.TileSet)

	// 2× supersampled sampling [03 §3.7].
	for ty := 0; ty < th; ty++ {
		for tx := 0; tx < tw; tx++ {
			// worldX = PlayRight * x / (2*RadarW)    // trunc IDIV [03 §3.7]
			// worldZ = PlayBottom * y / (2*RadarH)
			worldX := int32(int64(playW) * int64(tx) / int64(tw)) // TRUNC IDIV [03 §3.7]
			worldZ := int32(int64(playH) * int64(ty) / int64(th))

			// tileX = floorDiv(worldX,32) etc sign-corrected SAR 5 [03 §2.1]
			tileX := numeric.FloorDiv(int64(worldX), 32)
			tileZ := numeric.FloorDiv(int64(worldZ), 32)

			var pix byte
			if tileX < 0 || tileX >= int64(tileW) || tileZ < 0 || tileZ >= int64(tileH) {
				// Unreachable by construction, and kept as an assertion rather
				// than a behavior: `worldX = PlayRight * x / (2*RadarW)` with
				// `x < 2*RadarW` lies in [0, PlayRight), and likewise for Z, so
				// the generated picture's loop never leaves the tile map and
				// there is no out-of-domain sample to define. The tile-index
				// guard below is the only real one [03 R-MM-01 §3]. Suppressing
				// the picture is the safe answer to a malformed terrain record.
				return nil
			} else {
				idx := int(tileZ)*tileW + int(tileX)
				if idx < 0 || idx >= len(t.TileIndices) {
					return nil
				}
				tileIdx := t.TileIndices[idx]
				// Guard an authored tile index outside the tile set [03 §3.7].
				if int(tileIdx) >= tileCount {
					tileIdx = 0
				}
				// intra-tile offset (world &31) with sign-correct floor [03 §3.7]
				ox := int(int64(worldX) - tileX*32) // 0..31
				oz := int(int64(worldZ) - tileZ*32)
				if ox < 0 {
					ox += 32
				}
				if ox >= 32 {
					ox %= 32
				}
				if oz < 0 {
					oz += 32
				}
				if oz >= 32 {
					oz %= 32
				}
				// Tile 32×32 indexed pixels row-major [fmt tnt]
				tile := t.TileSet[tileIdx]
				pix = tile[oz*32+ox] // [03 §3.7] pix = *(u8*)(tileBase + (worldZ&31)*32 + (worldX&31))
			}
			temp[ty*tw+tx] = pix // opaque indexed sample [03 §3.7]
		}
	}

	// Two-level row-first ALP blend [03 §3.7].
	reduceHalfALP(bits, w, h, temp, tw, tables)

	return &RadarSurface{W: w, H: h, Pitch: pitch, Bits: bits}
}

// BakedRadarSourceFits reports whether a baked minimap of bakedW×bakedH stored
// pixels can feed the fixed 2×2 reducer for the fitted picture m: its declared
// rectangle must be complete in baked and cover twice the picture in each
// dimension. Retail performs no such check, and what it reads from an
// undersized source is Unknown [03 §3.7]; this is host validation only. The
// height test precedes the division, so a nonpositive height never divides.
func BakedRadarSourceFits(m camera.Minimap, baked []byte, bakedW, bakedH int) bool {
	w, h := int(m.W), int(m.H)
	return w > 0 && h > 0 && bakedW >= 2*w && bakedH >= 2*h && bakedW <= len(baked)/bakedH
}

// reduceHalfALP reduces fixed 2×2 source blocks with row-first ALP blends.
// Callers validate the source extent; srcW remains the authored row stride,
// even when only a top-left prefix supplies the picture [03 §3.7].
func reduceHalfALP(dst []byte, dstW, dstH int, src []byte, srcW int, tables *palette.Tables) {
	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			topLeft := 2*y*srcW + 2*x
			bottomLeft := topLeft + srcW
			top := tables.Alpha[int(src[topLeft])*256+int(src[topLeft+1])]
			bottom := tables.Alpha[int(src[bottomLeft])*256+int(src[bottomLeft+1])]
			dst[y*dstW+x] = tables.Alpha[int(top)*256+int(bottom)]
		}
	}
}

// resizeRadarBits returns a length-n byte slice reusing v's storage when it
// fits. Every caller writes all n bytes before reading any, so the retained
// tail is never observed and no clearing is owed.
func resizeRadarBits(v []byte, n int) []byte {
	if cap(v) < n {
		return slices.Grow(v[:0], n)[:n]
	}
	return v[:n]
}

// buildMappedInto implements the MAPPED composite over a caller-owned surface:
// the picture masked by the authoritative LOS grids [03 §3.8][03 §3.3]. It is a
// pure presentation operation [03 §3.6]. The composite writes every pixel of
// the result from the picture and the LOS grids, so the reused storage carries
// nothing of the previous composite; dst nil allocates.
func buildMappedInto(dst, picture *RadarSurface, wordMask []uint16, byteGrid []uint8, mapW, mapH int, localSlot uint8, dcb byte, guiRemap []byte) *RadarSurface {
	if picture == nil || picture.Bits == nil || picture.W <= 0 || picture.H <= 0 {
		return nil
	}
	w := picture.W
	h := picture.H
	pitch := (w + 3) &^ 3 // [03 §3.6]
	out := dst
	if out == nil {
		out = &RadarSurface{}
	}
	bits := resizeRadarBits(out.Bits, w*h)
	mask := uint16(1 << (localSlot & 0x1F)) // [03 §3.8] bit 1<<(player&0x1F)
	for y := 0; y < h; y++ {
		visY := 0
		if mapH > 0 && h > 0 {
			visY = y * mapH / h // TRUNC integer scaled [03 §3.8]
		}
		if visY < 0 {
			visY = 0
		}
		if mapH > 0 && visY >= mapH {
			visY = mapH - 1
		}
		for x := 0; x < w; x++ {
			visX := 0
			if mapW > 0 && w > 0 {
				visX = x * mapW / w // TRUNC [03 §3.8]
			}
			if visX < 0 {
				visX = 0
			}
			if mapW > 0 && visX >= mapW {
				visX = mapW - 1
			}
			visIdx := visY*mapW + visX
			var word uint16
			if visIdx >= 0 && visIdx < len(wordMask) {
				word = wordMask[visIdx]
			}
			var bVal uint8
			if visIdx >= 0 && visIdx < len(byteGrid) {
				bVal = byteGrid[visIdx]
			}
			src := picture.Bits[y*w+x]
			var value byte
			// word→byte→remap→DCB gate order [03 §3.8].
			if word&mask == 0 {
				value = dcb // [03 §3.8] unexplored → configured fog fill
			} else if bVal == 0 {
				if len(guiRemap) == 256 {
					value = guiRemap[src] // [03 §3.8] GUI remap
				} else {
					value = src // no remap when nil [03 §3.8]
				}
			} else {
				value = src
			}
			bits[y*w+x] = value
		}
	}
	*out = RadarSurface{W: w, H: h, Pitch: pitch, Bits: bits}
	return out
}

// minimapSelectedStatus is the unit status word's selected bit (bit 4), the
// gate on both the sensor circles and the weapon/interceptor rings
// [03 R-MM-01 §3].
const minimapSelectedStatus uint32 = 0x10

// MinimapContact carries world coords already in map pixels (short world>>16) plus owner, flags, def distances [03 §3.9].
type MinimapContact struct {
	WorldX, WorldZ, WorldY int32 // map pixels (short narrow already), WorldY high word for shear [03 §3.9]
	Owner                  uint8
	Palette                byte // caller-resolved owning-player palette index [03 §3.9]
	// Hovered marks the one contact the host's pointer record currently names.
	// Layer 3's `radlogohigh` ring is drawn on it, after the blip [03 §3.9].
	// The word is host presentation state, never simulation state, so the
	// caller resolves it and hands the answer in [07 R-HUD-03 §1][I6].
	Hovered bool
	Stealth bool // definition metadata; BlinkSuppress owns blip blinking [03 §3.9]
	// RangeStatus is the selected-unit circle gate of [03 §3.9] "Selected-unit
	// circle gate correction": the selected/range-status bit is set AND the
	// instance is active or the definition is not on/off-capable. It governs
	// layer 4 only; the blip layer has no such term.
	RangeStatus bool
	// Status and BlinkSuppress are copied from the immutable unit record. The
	// contact gate uses FriendlyMask/owner, while the suppress byte admits a
	// blip only during the shared blink phase. [03 §3.9]
	Status        uint32
	BlinkSuppress uint8
	Visible       bool
	LocalPlayer   uint8
	// Options is the mode-flags word whose bit 9 is the blip gate's first
	// disjunct: the **full-radar bit**, which the `+Radar` cheat toggles and
	// the world rebuild clears [03 R-MM-01 §3][07 R-CAM-01 §6]
	// [08 R-ENTRY-01 §3]. Battle entry starts it clear.
	Options uint32
	// MinimapMode carries the render-flags word's mapping and LOS mask bits,
	// the `+Mapping`/`+LOS` toggles. The gate's second disjunct is both bits
	// clear, which the world-rebuild tail arranges for a watcher slot
	// [03 R-MM-01 §3][07 R-CAM-01 §14].
	MinimapMode  uint8
	RawDistRadar int32
	RawDistSonar int32
	RawDistJamR  int32
	RawDistJamS  int32 // radar distances, 0 means absent [03 §3.10][07 §10]
	RingEnabled  bool
	RingDashed   bool
	RingRange    int32
}

// MinimapContactBlitter is the resolved authored-art adapter. It is called
// after the visibility/gate checks; a nil adapter means the authored blip is
// absent and therefore leaves FINAL untouched. [03 §3.9]
type MinimapContactBlitter func(dst *RadarSurface, x, y int, palette byte, hovered bool)

// MinimapContactGate is the contacts pass's visibility gate: a contact reaches
// any of the pass's layers when the full-radar option bit is set, or the
// render-flags word's mapping and LOS bits are both clear, or the unit carries
// either of the friendly-contact status bits (mask 0x300 — the seen marker a
// radar contact or the line-of-sight probe writes, and the sonar bit), or the
// unit's owner is the viewing player [03 §3.9] "Blip gate" [03 R-MM-01 §3].
//
// Visible is the publisher's own resolution of the last two disjuncts against
// authoritative state; it is folded in here rather than recomputed.
func MinimapContactGate(c MinimapContact) bool {
	return c.Visible || c.Options&(1<<9) != 0 || c.MinimapMode&3 == 0 ||
		c.Status&0x300 != 0 || c.Owner == c.LocalPlayer
}

// MinimapBlipAdmitted adds the blip layers' blink term to MinimapContactGate:
// the blip draws when the unit's per-instance blink-suppress countdown reads
// zero OR the shared blink phase bit is set [03 §3.9] layer 2.
//
// Definition `stealth` is not a term. Stealth is the sensor phase's contact
// callback reject [03 R-VIS-01 §5], which is where a stealthy unit fails to
// gain the seen bit; a stealthy unit admitted by line of sight draws a steady
// blip like any other. Nor is the selected-unit circle gate a term here: a unit
// whose blip is suppressed on a non-blink phase still runs its range branches,
// which is what [03 §3.9] "Contact layering and ring-only cases" names.
func MinimapBlipAdmitted(c MinimapContact, blink BlinkState) bool {
	return MinimapContactGate(c) && (c.BlinkSuppress == 0 || blink.Phase&1 != 0)
}

// rebuildFinalExactInto composes FINAL over a caller-owned surface. FINAL is
// wiped from MAPPED before any contact is drawn, so every byte of the reused
// storage is overwritten by that copy and a retained surface produces exactly
// the bytes a fresh one does; dst nil allocates as before.
func rebuildFinalExactInto(dst, mapped *RadarSurface, m camera.Minimap, playW, playH int32, contacts []MinimapContact, blink BlinkState, blit MinimapContactBlitter, radarColor, jammerColor, ringColor byte) *RadarSurface {
	if mapped == nil || mapped.W <= 0 || mapped.H <= 0 || len(mapped.Bits) < mapped.W*mapped.H {
		return nil
	}
	final := dst
	if final == nil {
		final = &RadarSurface{}
	}
	final.W, final.H, final.Pitch = mapped.W, mapped.H, (mapped.W+3)&^3
	final.Bits = resizeRadarBits(final.Bits, mapped.W*mapped.H)
	copy(final.Bits, mapped.Bits)
	// Finish one contact before starting the next: later units can overwrite
	// earlier units' hover art and circles [03 §3.9]. Only the regular blip
	// uses the blink term; a hovered admitted contact keeps its hover art.
	for _, c := range contacts {
		if !MinimapContactGate(c) {
			continue
		}
		rx, ry := RadarProjection(c.WorldX, c.WorldZ, c.WorldY, playW, playH, m)
		if blit != nil {
			if MinimapBlipAdmitted(c, blink) {
				blit(final, int(rx), int(ry), c.Palette, false)
			}
			if c.Hovered {
				blit(final, int(rx), int(ry), c.Palette, true)
			}
		}
		// The publisher folds selection and activation into RangeStatus.
		// Each nonzero authored distance is scaled separately; zero and
		// negative projected radii still reach the circle helper [03 §3.10].
		if c.RangeStatus {
			for _, sensor := range [...]struct {
				distance int32
				color    byte
			}{
				{c.RawDistRadar, radarColor}, {c.RawDistSonar, radarColor},
				{c.RawDistJamR, jammerColor}, {c.RawDistJamS, jammerColor},
			} {
				if sensor.distance != 0 {
					r := RadarRadius(sensor.distance, m.W, playW)
					drawCircle(final, int(rx), int(ry), int(r), sensor.color)
				}
			}
		}
		// Weapon slots follow this unit's sensors, independently of its
		// activation gate. Extra slot records immediately follow their owner
		// and have nil regular art at the HUD adapter [03 §3.9].
		if c.Status&minimapSelectedStatus != 0 && c.RingEnabled {
			r := RadarRadius(c.RingRange, m.W, playW)
			if c.RingDashed {
				drawDashedCircle(final, int(rx), int(ry), int(r), ringColor, blink.Phase&1 != 0)
			} else {
				drawCircle(final, int(rx), int(ry), int(r), ringColor)
			}
		}
	}
	return final
}

// BlinkState carries the committed minimap phase consumed by contact and ring
// presentation. Countdown ownership remains in the phase-12 session state
// and is never presented here [R-CORE-03][03 §3.6].
type BlinkState struct {
	Phase uint8 // semantic bit 0 [R-CORE-03].
}

// IsBlinkOn reports whether stealth contacts should be visible [03 §3.9].
func (b BlinkState) IsBlinkOn() bool {
	return b.Phase&1 != 0
}

// RadarProjection projects world coords to radar pixels [03 §3.9][07 §10].
// rx=worldX*RadarW/PlayRight etc with half shear SAR 1 [03 §3.9].
func RadarProjection(worldX, worldZ, worldY int32, playW, playH int32, m camera.Minimap) (rx, ry int32) {
	if playW == 0 || playH == 0 || m.W == 0 || m.H == 0 {
		return 0, 0
	}
	// TRUNC via IDIV [03 §3.9][07 §10]; use int64 intermediate.
	rx = int32(int64(worldX) * int64(m.W) / int64(playW))
	// ry = ((short)(worldZ) - ((short)(worldY)>>1)) * RadarH / PlayBottom // SAR 1 half shear [03 §3.9]
	ry = int32((int64(worldZ-(worldY>>1)) * int64(m.H)) / int64(playH))
	return rx, ry
}

// RadarRadius computes truncated radar radius rRadar=RadarW*dist/PlayRight etc [03 §3.10][07 §10].
func RadarRadius(dist int32, radarSize int32, playSize int32) int32 {
	if playSize == 0 || radarSize == 0 || dist == 0 {
		return 0
	}
	return int32(int64(radarSize) * int64(dist) / int64(playSize)) // TRUNC [03 §3.10]
}

// drawCircle joins the 32 authored angular samples with clipped integer lines.
// A zero radius paints the centre; signed radii use the same trig and clip path.
// The angular increment is 0x800 (32 segments), and the fixed-point trig table
// is the shared retail table [03 §3.10][04 §5.1].
func drawCircle(s *RadarSurface, cx, cy, r int, color byte) {
	if s == nil || s.Bits == nil {
		return
	}
	for i := 0; i < 32; i++ {
		x0, y0 := circlePoint(cx, cy, r, i)
		x1, y1 := circlePoint(cx, cy, r, i+1)
		line(s, x0, y0, x1, y1, color)
	}
}

func circlePoint(cx, cy, r, segment int) (int, int) {
	a := numeric.Angle(uint16(segment * 0x800))
	return cx + int(numeric.MulRound(int32(r), numeric.Cos(a))), cy + int(numeric.MulRound(int32(r), numeric.Sin(a)))
}

func line(s *RadarSurface, x0, y0, x1, y1 int, color byte) {
	dx := x1 - x0
	if dx < 0 {
		dx = -dx
	}
	dy := y1 - y0
	if dy < 0 {
		dy = -dy
	}
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		s.Set(x0, y0, color)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// drawDashedCircle emits alternating 32-segment arcs. Phase selects the
// established segment parity; there are no extra endpoint writes [03 §3.10].
func drawDashedCircle(s *RadarSurface, cx, cy, r int, color byte, phase bool) {
	if s == nil || s.Bits == nil {
		return
	}
	offset := 0
	if phase {
		offset = 1
	}
	for i := 0; i < 32; i++ {
		if (i+offset)&1 == 0 {
			continue
		}
		x0, y0 := circlePoint(cx, cy, r, i)
		x1, y1 := circlePoint(cx, cy, r, i+1)
		line(s, x0, y0, x1, y1, color)
	}
}
