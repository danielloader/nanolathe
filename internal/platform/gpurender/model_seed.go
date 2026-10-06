package gpurender

import "github.com/nanolathe-gg/nanolathe/internal/drawlist"

// A seed keeps two keys until the cached colour has resolved: red is the
// seeded key consumed by later writers, green the original cached winner.
// The resized transform (1 -> 0, all other bytes unchanged) is monotone, so
// MAX over both channels preserves the completed-image copy [03 R-REN-03A §4].
// Testing cached colour against red instead would let an originally rejected
// key-zero face replace the winning key-one colour.
type modelSeedPhase uint8

const (
	modelSeedNone modelSeedPhase = iota
	modelSeedRaw
	modelSeedResized
	modelSeedLive
)

func modelHasSeed(g *drawlist.ModelGeometry) bool {
	return g != nil && g.KeyPlane && g.CachedSeed.Width > 0 && g.CachedSeed.Height > 0
}

func modelSeedForPacket(p *modelPlacePacket) modelSeedPhase {
	if p.shadow || !modelHasSeed(p.g) {
		return modelSeedNone
	}
	bounds := p.region.bounds
	// A carried child's independent image is sized before it joins the parent.
	if p.group != nil || p.solo {
		bounds = modelWorldBounds(p.g)
	}
	if int(p.g.CachedSeed.Width) == bounds.Dx() && int(p.g.CachedSeed.Height) == bounds.Dy() {
		return modelSeedRaw
	}
	return modelSeedResized
}

func modelSeedChildDelta(g *drawlist.ModelGeometry, delta int32) int32 {
	if modelHasSeed(g) {
		return modelSeedDelta(delta)
	}
	return 0
}

// A seeded image contains byte keys. Beyond either endpoint this delta
// produces the same saturated key for every source byte, so reducing it here
// is exact for merge, clipping and reflection alike. Two signed height words
// can differ by more than a signed word; packing their raw delta would lose it.
func modelSeedDelta(delta int32) int32 {
	return max(-255, min(255, delta))
}

func (d *modelDirectLane) hasSeedLiveRuns(page int32) bool {
	for i := range d.runs {
		run := &d.runs[i]
		if run.page == page && run.iLen != 0 && run.seedPhase == modelSeedLive {
			return true
		}
	}
	return false
}

// Reflection's negative mode keeps its existing zero/-one encodings intact.
// Seeded modes carry bounded integer operands, exactly representable as float32;
// the original key still belongs to the source region, the shifted key to the
// group's occlusion region. This changes admission, not reflection appearance.
func modelSeedReflectionMode(phase modelSeedPhase, delta int32, grouped bool) float32 {
	delta = modelSeedDelta(delta)
	group := int32(0)
	if grouped {
		group = 1
	}
	return -float32(2 + int32(phase-1)*131072 + (delta+32768)*2 + group)
}
