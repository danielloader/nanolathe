package client

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"sort"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
)

// InfectionTileSize is the edge of the reduced overlay tile the compositor
// samples; composition reads nothing else of the original art.
const InfectionTileSize = 64

// infectionTile is infection art reduced to the compositor's 64×64
// straight-alpha sampling tile (DESIGN_GPU_RENDERER §38).
type infectionTile [InfectionTileSize * InfectionTileSize]color.NRGBA

// infectionTileOf takes art exactly InfectionTileSize square as an already
// reduced tile and reads it without resampling, which is how the engine ships
// its overlay. Art of any other size is box-filtered into a tile with
// premultiplied, coverage-aware sums so sparse veins survive.
func infectionTileOf(overlay image.Image) (*infectionTile, error) {
	if overlay == nil || overlay.Bounds().Empty() {
		return nil, fmt.Errorf("infection overlay is absent or empty")
	}
	b := overlay.Bounds()
	tile := new(infectionTile)
	exact := b.Dx() == InfectionTileSize && b.Dy() == InfectionTileSize
	for y := range InfectionTileSize {
		for x := range InfectionTileSize {
			if exact {
				tile[y*InfectionTileSize+x] = color.NRGBAModel.Convert(overlay.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			} else {
				tile[y*InfectionTileSize+x] = infectionOverlaySample(overlay, x, y, InfectionTileSize, InfectionTileSize)
			}
		}
	}
	return tile, nil
}

// PrepareInfectedModelSkin composes original RGBA infection art with this
// registry's active unit textures and palette. It runs once during detached
// battle preparation, never during recording or phase 7. Optional static
// overrides keep their exact pixels and take priority over generated frames.
// This is Nanolathe presentation tuning (DESIGN_GPU_RENDERER §38), not retail.
func (r *ModelTextureRegistry) PrepareInfectedModelSkin(name string, pal *palette.Tables, overlay image.Image, overrides *ModelSkin) (*ModelSkin, error) {
	fail := func(reason string) (*ModelSkin, error) {
		var providers []string
		if r != nil && r.fs != nil {
			providers = r.fs.ProviderIDs()
		}
		return nil, fmt.Errorf("nanolathe: %s: logical path skins/%s, providers searched [%s], expected palette-indexed infected unit skin", reason, name, strings.Join(providers, ", "))
	}
	if r == nil {
		return fail("model texture registry is absent")
	}
	if strings.TrimSpace(name) == "" {
		return fail("empty model skin name")
	}
	if pal == nil || pal.Base == ([256][4]byte{}) {
		return fail("infection palette is absent or empty")
	}
	tile, err := infectionTileOf(overlay)
	if err != nil {
		return fail(err.Error())
	}
	// Integer RGB results share a quantization cache across every texture,
	// frame, and flat colour in this preparation.
	painter := infectionPainter{pal: pal, tile: tile, quantized: make(map[uint32]byte)}
	anyCoverage := false
	for _, pixel := range painter.tile {
		anyCoverage = anyCoverage || pixel.A != 0
	}
	if !anyCoverage {
		return fail("infection overlay has no visible coverage")
	}
	skin := &ModelSkin{name: name, frames: make(map[string]*formats.GAFFrame), generated: make(map[string]map[*formats.GAFFrame]*formats.GAFFrame), flat: new([256]byte)}
	for i, rgb := range pal.Base {
		skin.flat[i] = painter.index(infectionFlatStain(rgb))
	}
	if overrides != nil {
		for key, f := range overrides.frames {
			if ref, ok := r.resolve(key); ok && ref.kind != texStatic {
				return fail("static infection override targets a team or animated texture: " + key)
			}
			skin.frames[key] = f
		}
	}
	// Only unit textures are skin subjects. A malformed or unsupported texture
	// used solely by a feature/projectile must not prevent a unit presentation
	// bank from being prepared. Model loads already include the full catalog.
	names := make(map[string]bool)
	for load, m := range r.loads {
		if load.kind != modelLoadUnit && load.kind != modelLoadStandalone || m == nil || m.compiled == nil {
			continue
		}
		for _, piece := range m.compiled.Pieces {
			for _, pr := range piece.Primitives {
				if pr.TextureName != "" && pr.IsColored&1 == 0 {
					names[strings.ToLower(pr.TextureName)] = true
				}
			}
		}
	}
	ordered := make([]string, 0, len(names))
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		ref, ok := r.resolve(key)
		if !ok || ref.kind == texTeam || skin.frames[key] != nil {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(key))
		painter.phase = h.Sum32()
		frames := []formats.GAFFrameRef{{Frame: ref.frame}}
		if ref.kind == texAnimated && ref.entry != nil {
			frames = ref.entry.Frames
		}
		bank := make(map[*formats.GAFFrame]*formats.GAFFrame, len(frames))
		for i, source := range frames {
			if source.Frame == nil || bank[source.Frame] != nil {
				continue
			}
			f, err := painter.frame(source.Frame)
			if err != nil {
				return fail(fmt.Sprintf("compose infection texture %s frame %d: %v", key, i, err))
			}
			bank[source.Frame] = f
		}
		skin.generated[key] = bank
	}
	return skin, nil
}

type infectionPainter struct {
	pal       *palette.Tables
	tile      *infectionTile
	phase     uint32
	quantized map[uint32]byte
}

// Box filtering uses premultiplied samples, so transparent black cannot turn
// the edges of sparse original art black. Bounds need not start at zero.
func infectionOverlaySample(src image.Image, x, y, w, h int) color.NRGBA {
	b := src.Bounds()
	x0, y0 := x*b.Dx()/w, y*b.Dy()/h
	x1, y1 := max(x0+1, (x+1)*b.Dx()/w), max(y0+1, (y+1)*b.Dy()/h)
	var r, g, blue, a, count uint64
	for sy := y0; sy < y1; sy++ {
		for sx := x0; sx < x1; sx++ {
			cr, cg, cb, ca := src.At(b.Min.X+sx, b.Min.Y+sy).RGBA()
			r, g, blue, a, count = r+uint64(cr), g+uint64(cg), blue+uint64(cb), a+uint64(ca), count+1
		}
	}
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: byte(r * 255 / a), G: byte(g * 255 / a), B: byte(blue * 255 / a), A: byte(a / count / 257)}
}

func (p *infectionPainter) index(rgb [3]int) byte {
	key := uint32(rgb[0])<<16 | uint32(rgb[1])<<8 | uint32(rgb[2])
	if index, ok := p.quantized[key]; ok {
		return index
	}
	index := nearestPaletteIndex(p.pal, rgb[0], rgb[1], rgb[2])
	p.quantized[key] = index
	return index
}

// A restrained warm stain follows the source brightness instead of
// painting over dark panel seams or raising black recesses.
func infectionStain(rgb [4]byte) [3]int {
	luma := (int(rgb[0])*3 + int(rgb[1])*6 + int(rgb[2])) / 10
	return [3]int{(int(rgb[0])*82 + min(255, luma*3/2)*18) / 100,
		(int(rgb[1])*82 + luma*3/5*18) / 100, (int(rgb[2])*82 + luma*13/20*18) / 100}
}

// Flat-only models have no overlay-bearing pixels. Give their broad faces a
// stronger warm/rose stain so the fallback survives ordinary palette steps.
func infectionFlatStain(rgb [4]byte) [3]int {
	luma := (int(rgb[0])*3 + int(rgb[1])*6 + int(rgb[2])) / 10
	return [3]int{(int(rgb[0])*70 + min(255, luma*3/2)*30) / 100,
		(int(rgb[1])*70 + luma/2*30) / 100, (int(rgb[2])*70 + luma*3/5*30) / 100}
}

func (p *infectionPainter) sample(x, y, w, h int) color.NRGBA {
	// A half-tile crop makes broad masses readable on a face only a few screen
	// pixels wide; shrinking the whole artwork would turn its veins into noise.
	const patch = InfectionTileSize / 2
	x0, y0 := x*patch/w, y*patch/h
	x1, y1 := max(x0+1, (x+1)*patch/w), max(y0+1, (y+1)*patch/h)
	var r, g, b, a, count int
	for sy := y0; sy < y1; sy++ {
		for sx := x0; sx < x1; sx++ {
			tx := (sx + int(p.phase&63)) & 63
			ty := (sy + int(p.phase>>6&63)) & 63
			v := p.tile[ty*InfectionTileSize+tx]
			r, g, b, a, count = r+int(v.R)*int(v.A), g+int(v.G)*int(v.A), b+int(v.B)*int(v.A), a+int(v.A), count+1
		}
	}
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{R: byte(r / a), G: byte(g / a), B: byte(b / a), A: byte(a / count)}
}

func (p *infectionPainter) frame(root *formats.GAFFrame) (*formats.GAFFrame, error) {
	clones := make(map[*formats.GAFFrame]*formats.GAFFrame)
	visiting := make(map[*formats.GAFFrame]bool)
	var clone func(*formats.GAFFrame) (*formats.GAFFrame, error)
	clone = func(src *formats.GAFFrame) (*formats.GAFFrame, error) {
		if src == nil {
			return nil, fmt.Errorf("nil composite child")
		}
		if visiting[src] {
			return nil, fmt.Errorf("cyclic composite frame")
		}
		if f := clones[src]; f != nil {
			return f, nil
		}
		w, h := int(src.Width), int(src.Height)
		if w == 0 || h == 0 || len(src.Pixels) != w*h || int(src.SubframeCount) != len(src.Subframes) {
			return nil, fmt.Errorf("inconsistent decoded frame geometry or child count")
		}
		visiting[src] = true
		// Keep authored header/offset/dispatch metadata. This immutable bank is
		// consumed only by the indexed unit mapper, never encoded-storage readers.
		out := *src
		out.Transparent = append([]bool(nil), src.Transparent...)
		paint := func(pixels []byte, transparent []bool) []byte {
			result := append([]byte(nil), pixels...)
			for i, index := range pixels {
				if i < len(transparent) && transparent[i] {
					continue
				}
				x := i%w + int(root.XOffset) - int(src.XOffset)
				y := i/w + int(root.YOffset) - int(src.YOffset)
				ink := p.sample(x, y, int(root.Width), int(root.Height))
				rgb := p.pal.Base[index]
				stained := infectionStain(rgb)
				// At most 75% local overlay; taper highlights and modulate the art by
				// source brightness so authored metal and panel relief stay legible.
				light := (int(rgb[0])*3 + int(rgb[1])*6 + int(rgb[2])) / 10
				// Expand partial coverage so broad masses survive both tiny face
				// filtering and the final palette steps, without filling clear gaps.
				coverage := int(ink.A) * (510 - int(ink.A)) / 255
				alpha := coverage * 75 / 100
				if light > 192 {
					alpha = alpha * (640 - light) / 448
				}
				inkLight := (int(ink.R)*3 + int(ink.G)*6 + int(ink.B)) / 10
				for channel, value := range [3]byte{ink.R, ink.G, ink.B} {
					// Sparse low-saturation art otherwise collapses back onto the
					// palette's gray ramps. Strengthen its own hue before blending,
					// retaining the rose/olive variation and broad alpha pattern.
					chroma := max(0, inkLight+(int(value)-inkLight)*5/2)
					target := min(255, chroma*light/max(96, inkLight))
					stained[channel] = (stained[channel]*(255-alpha) + target*alpha) / 255
				}
				result[i] = p.index(stained)
				// Preserve raw-key transparency even for consumers that inspect
				// the key rather than the decoded transparency plane.
				if src.Compressed == 0 && result[i] == src.ColorKey && index != src.ColorKey {
					result[i] = index
				}
			}
			return result
		}
		out.Pixels = paint(src.Pixels, src.Transparent)
		if len(src.PlainPixels) == len(src.Pixels) && len(src.PlainPixels) != 0 && &src.PlainPixels[0] == &src.Pixels[0] {
			out.PlainPixels, out.PlainTransparent = out.Pixels, out.Transparent
		} else {
			if len(src.PlainPixels) != 0 && len(src.PlainPixels) != w*h {
				return nil, fmt.Errorf("inconsistent decoded plain raster")
			}
			out.PlainPixels = paint(src.PlainPixels, src.PlainTransparent)
			out.PlainTransparent = append([]bool(nil), src.PlainTransparent...)
		}
		out.Subframes = make([]*formats.GAFFrame, len(src.Subframes))
		for i, child := range src.Subframes {
			f, err := clone(child)
			if err != nil {
				return nil, err
			}
			out.Subframes[i] = f
		}
		delete(visiting, src)
		clones[src] = &out
		return &out, nil
	}
	return clone(root)
}
