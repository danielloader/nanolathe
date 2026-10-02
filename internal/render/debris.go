package render

import (
	"github.com/nanolathe-gg/nanolathe/internal/effects"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// DebrisTrailBank and the two entry names are the art identity of the two
// producers the debris draw reaches. Both are rows of the engine's own fixed
// effect-slot table, bound from `fx` at startup; an entry name means nothing
// without its bank [03 R-FX-01 §3][06 R-WFX-01 §1].
const (
	DebrisTrailBank  = "fx"
	debrisTrailStrip = 9
	// DebrisSmokePuffEntry and DebrisFlameTrailEntry are exported so the
	// presentation store can resolve each entry's frame count, which retail's
	// container stores at init [03 R-FX-01 §3].
	DebrisSmokePuffEntry  = "smoke 1"
	DebrisFlameTrailEntry = "flamestream"
)

// The debris draw's two producers make STRIP-9 CONTAINERS, not one-shot
// blits. Retail's containers survive on the strip list, are swept by the
// per-tick update of [03 R-FX-01 §3] and draw at barrier 9 every frame until
// they retire, which is what makes a burning piece leave a trail behind it.
//
// The types below carry that container and its sub-records for the
// PRESENTATION-owned store the client keeps (internal/client's debris trail
// store). They are a separate list from the simulation's own strip table —
// retail shares one 1000-slot pool and one strip-9 vector between the two, and
// this build does not; see the store's bound.

// DebrisTrailMaxParticles bounds one container's sub-record vector. The smoke
// container spawns exactly one puff and never spawns again (its window closes
// at its birth tick), and the fire container lays `lifetime + 1` segments with
// a lifetime of 1..3, so four is the most either class can ever hold
// [03 R-FX-01 §3].
const DebrisTrailMaxParticles = 4

// debrisSmokeFrameHold is the puff's authored animation hold. The debris
// producer's init passes frameHold 0, and the smoke family reads a zero hold as
// seven [03 R-FX-01 §3][03 R-FX-02 §3].
const debrisSmokeFrameHold = 7

// debrisFlameTrailHold is the trail segment's phase modulus: the leading
// argument of the flame-stream trail init, 1 at every researched site
// including the debris fire particle, so the segment's frame advances every
// tick [03 R-FX-01 §3].
const debrisFlameTrailHold = 1

// debrisSmokeWindShift and debrisSmokeRiseShift are the strips-5/9 smoke
// puffer's per-tick drift scales: both published wind words are added to the
// RAW 16.16 X and Z words times eight, and the map's AUTHORED gravity word is
// added to the raw Y word times four. The vent's class uses sixteen for the
// rise; this producer is the strips-5/9 class, not the vent [03 R-FX-01 §3]
// [R-WIND-01].
const (
	debrisSmokeWindShift = 8
	debrisSmokeRiseShift = 4
)

// DebrisTrailFrameCounts carries the two bound entries' frame counts less one,
// the quantity retail's container stores at init. The smoke puff draws its own
// last frame against it and the trail segment wraps its cursor modulo it
// [03 R-FX-01 §3][06 R-WFX-01 §5]. Zero means the entry did not resolve, which
// leaves the puff without a last frame and the segment without a wrap point —
// the same "not decidable yet" state the session's frame-count seam has.
type DebrisTrailFrameCounts struct {
	Smoke int32
	Flame int32
}

// DebrisTrailParticle is one sub-record of a debris-trail container: a smoke
// puff or a flame-stream segment [03 R-FX-01 §3].
type DebrisTrailParticle struct {
	X, Y, Z numeric.Fixed
	// Frame is the sub-record's own animation cursor into the bound entry.
	Frame int32
	// FrameDelay is the smoke puff's hold countdown; each expiry advances the
	// cursor and redraws the next hold, one CRT value per advance.
	FrameDelay int32
	// LastFrame is the smoke puff's own drawn final frame; the puff retires
	// when its cursor reaches it. Zero means the container had no frame count.
	LastFrame int32
	// Phase is the trail segment's modulo counter.
	Phase int32
	// Expiry is the sub-record's tick deadline; it is removed once the deadline
	// has strictly passed. Zero means no deadline.
	Expiry uint32
}

// DebrisTrailContainer is one presentation-owned strip-9 container the debris
// draw produced [04 R-COB-04 §2][03 R-FX-01 §3].
type DebrisTrailContainer struct {
	Family frame.StripFamily
	Entry  string
	// Born is the committed tick the draw created the container on.
	Born uint32
	// Deadline is `tick + lifetime` at init: the container's window end. It
	// bounds both the spawn gate and, for the smoke class, the removal verdict.
	Deadline uint32
	// NextSpawn is the gate's next due tick; zero means no gate was armed.
	NextSpawn uint32
	// FrameCountBase is the bound entry's frame count less one.
	FrameCountBase int32
	// FrameDelayParam is the smoke family's authored hold; zero leaves the
	// animation clock idle.
	FrameDelayParam int32
	// PhaseModulus is the trail family's segment hold; zero leaves its counter
	// idle, as retail's own modulo would fault on it.
	PhaseModulus int32
	// Src is the container's source point A. Every trail segment starts at a
	// fresh copy of it, and the smoke puff spawns there.
	Src [3]numeric.Fixed
	// Travel is the trail segment's per-tick step. The debris fire particle's
	// A and B are the same jittered point, so its step is zero on every axis —
	// a stationary flame sprite [03 R-FX-01 §3].
	Travel [3]numeric.Fixed

	Particles [DebrisTrailMaxParticles]DebrisTrailParticle
	Count     int
}

// DebrisTrailEmission is what one debris draw's two producers created: at most
// two containers, in the order the research names them — smoke puff (engine
// bit 1), then fire particle (engine bit 0) [04 R-COB-04 §2][03 R-FX-01 §3].
// The array is fixed because the pair is: a record carries at most one of each
// bit, so no debris draw can ever emit a third.
//
// CRTDraws is the number of presentation values the pair spent: one for the
// smoke puff's last frame and four for the fire particle (three jitter axes and
// the container life).
type DebrisTrailEmission struct {
	Containers [2]DebrisTrailContainer
	Count      int
	CRTDraws   int
}

// DebrisTrails builds the two containers one rendered frame's debris draw
// creates, drawing every random value from the PRESENTATION CRT copy the caller
// owns. The simulation stream is never reachable from here, and a nil source
// suppresses both producers rather than substituting unjittered points or a
// puff with no life [04 R-COB-04 §2][03 R-FX-01 §3][I4][I6].
//
// Established arithmetic:
//
//   - the smoke puff (engine bit 1) is the strips-5/9 smoke puffer's init
//     `(0, 1, 0, 0, 0)` on `smoke 1`: one puff at the piece, starting at frame
//     0, with a hold of seven and a window that closes at the birth tick, so
//     the container spawns once and lives only as long as its puff. Its one
//     spawn draw is the puff's own last frame,
//     `crtRand·(frameCount − 3)/0x8000 + 2`;
//   - the fire particle (engine bit 0) is the flame-stream trail class with
//     source and target both the piece's position jittered per axis by
//     `crtRand · 3 / 0x8000 − 1` WHOLE units (three draws), and a container
//     life of `crtRand · 3 / 0x8000 + 1` ticks (a fourth), hold 1. The four
//     draws are spent before the pool is consulted, so a dropped particle still
//     costs them. The container lays `lifetime + 1` coincident segments, one
//     per tick, all extinguished together the tick after its deadline.
//
// tick is the COMMITTED tick the container is born on; the store steps it from
// the next committed tick onward, which is where retail's phase-11 sweep first
// sees a container its draw pass made.
func DebrisTrails(v frame.DebrisView, crt CRTRandomSource, tick uint32, counts DebrisTrailFrameCounts) DebrisTrailEmission {
	var e DebrisTrailEmission
	if crt == nil {
		// Both producers draw: the puff's last frame and the particle's four
		// values. With no stream bound neither container is built, rather than
		// one being built with a substituted value [I4][I9].
		return e
	}
	if v.Smoke {
		c := DebrisTrailContainer{
			Family:          frame.StripFamilySmokePuff,
			Entry:           DebrisSmokePuffEntry,
			Born:            tick,
			Deadline:        tick, // lifetime 0: the window closes at birth
			NextSpawn:       tick + 1,
			FrameCountBase:  counts.Smoke,
			FrameDelayParam: debrisSmokeFrameHold,
			Src:             [3]numeric.Fixed{v.X, v.Y, v.Z},
		}
		// The family's constructor spawns its first puff immediately, spending
		// exactly one CRT value for that puff's last frame [03 R-FX-01 §3].
		c.Particles[0] = DebrisTrailParticle{
			X: v.X, Y: v.Y, Z: v.Z,
			FrameDelay: c.FrameDelayParam,
			LastFrame:  effects.SmokeLastFrame(counts.Smoke, crt.Rand()),
		}
		c.Count = 1
		e.CRTDraws++
		e.Containers[e.Count] = c
		e.Count++
	}
	if v.Fire {
		x, y, z := debrisFireJitter(crt, v.X, v.Y, v.Z)
		lifetime := crt.Rand()*3/0x8000 + 1
		e.CRTDraws += 4
		c := DebrisTrailContainer{
			Family:         frame.StripFamilyFlameTrail,
			Entry:          DebrisFlameTrailEntry,
			Born:           tick,
			Deadline:       tick + uint32(lifetime),
			NextSpawn:      tick + 1,
			FrameCountBase: counts.Flame,
			PhaseModulus:   debrisFlameTrailHold,
			Src:            [3]numeric.Fixed{x, y, z},
			// A == B, so the researched step `((B − A) · trunc(65536/life)) >> 16`
			// is zero on every axis. It is written out rather than assumed so a
			// later producer with two distinct points reaches the same code.
			Travel: [3]numeric.Fixed{
				effects.FlameTrailStep(0, lifetime), effects.FlameTrailStep(0, lifetime), effects.FlameTrailStep(0, lifetime),
			},
		}
		// The trail family's init lays its first segment unconditionally, and
		// spends no draw doing it; the gate then lays one per tick while
		// `nextSpawn ≤ deadline` [03 R-FX-01 §3][03 R-STRIP-01 §3].
		c.laySegment()
		e.Containers[e.Count] = c
		e.Count++
	}
	return e
}

// Retired is the removal verdict the sweep evaluates BEFORE the update work
// [03 R-STRIP-01 §2]: "the list is empty" for the flame-stream trail, and
// "the list is empty AND the window has passed" for the strips-5/9 smoke
// puffer. Retail's own comparison is `deadline < tick` on unsigned words.
func (c *DebrisTrailContainer) Retired(tick uint32) bool {
	if c == nil {
		return true
	}
	if c.Count != 0 {
		return false
	}
	if c.Family == frame.StripFamilySmokePuff {
		return tick > c.Deadline
	}
	return true
}

// Step is the container's per-tick update [03 R-FX-01 §3][03 R-STRIP-01 §2]:
// advance every sub-record, remove the ones whose deadline has passed with
// stable in-place compaction, then spawn if the gate is due. The only draws are
// the smoke puff's animation-clock redraws, one per frame advance
// [03 R-STRIP-01 §3].
//
// windX and windZ are the two published wind words and gravityWord the map's
// authored gravity key, both read unconverted [R-WIND-01][03 R-FX-01 §3].
func (c *DebrisTrailContainer) Step(tick uint32, windX, windZ, gravityWord int32, crt CRTRandomSource) {
	if c == nil {
		return
	}
	c.advance(tick, windX, windZ, gravityWord, crt)
	c.expire(tick)
	c.spawn(tick)
}

func (c *DebrisTrailContainer) advance(tick uint32, windX, windZ, gravityWord int32, crt CRTRandomSource) {
	switch c.Family {
	case frame.StripFamilySmokePuff:
		for i := 0; i < c.Count; i++ {
			p := &c.Particles[i]
			// No velocity: the three adds go against the RAW 16.16 words, so a
			// puff drifts a world unit or two over its whole life and rises
			// slowly — it does not travel [03 R-FX-01 §3].
			p.X = numeric.Fixed(p.X.Raw() + int64(windX)*debrisSmokeWindShift)
			p.Z = numeric.Fixed(p.Z.Raw() + int64(windZ)*debrisSmokeWindShift)
			p.Y = numeric.Fixed(p.Y.Raw() + int64(gravityWord)*debrisSmokeRiseShift)
			// The animation clock counts the hold down; on zero it advances the
			// frame and redraws the next hold as half to full of the authored
			// value, one CRT value per advance [03 R-STRIP-01 §3]. With no
			// stream bound the clock stands still rather than advancing free.
			if p.FrameDelay > 0 {
				p.FrameDelay--
				if p.FrameDelay == 0 && c.FrameDelayParam > 0 && crt != nil {
					p.Frame++
					p.FrameDelay = effects.SmokeFrameHold(c.FrameDelayParam, crt.Rand())
				}
			}
			// The retirement compare runs on EVERY visit, not only the visits
			// that advanced the cursor [03 R-FX-01 §3 addendum]. A zero last
			// frame means the container resolved no frame count, which reads as
			// "not decidable yet" rather than "retire now".
			if p.LastFrame > 0 && p.Frame >= p.LastFrame {
				p.Expiry = debrisTrailExpiredAt(tick)
			}
		}
	case frame.StripFamilyFlameTrail:
		for i := 0; i < c.Count; i++ {
			p := &c.Particles[i]
			p.X = p.X.Add(c.Travel[0])
			p.Y = p.Y.Add(c.Travel[1])
			p.Z = p.Z.Add(c.Travel[2])
			if c.PhaseModulus <= 0 {
				continue
			}
			p.Phase = (p.Phase + 1) % c.PhaseModulus
			if p.Phase == 0 && c.FrameCountBase > 0 {
				// `frame = (frame + 1) mod (frameCount − 1)`: the entry's last
				// frame is never shown [03 R-FX-01 §3].
				p.Frame = (p.Frame + 1) % c.FrameCountBase
			}
		}
	}
}

// expire removes sub-records whose deadline has strictly passed, with stable
// in-place compaction [03 R-STRIP-01 §2].
func (c *DebrisTrailContainer) expire(tick uint32) {
	write := 0
	for read := 0; read < c.Count; read++ {
		p := c.Particles[read]
		if p.Expiry != 0 && tick > p.Expiry {
			continue
		}
		c.Particles[write] = p
		write++
	}
	for i := write; i < c.Count; i++ {
		c.Particles[i] = DebrisTrailParticle{}
	}
	c.Count = write
}

// spawn is the gate: at most one spawn per update, due when the next-spawn tick
// is at or before BOTH the global tick and the container's window end (both
// inclusive) [03 R-FX-01 §3]. The smoke container's window closes at its birth
// tick, so its gate never fires again; the trail's stays open for `lifetime`
// more ticks, which is what lays `lifetime + 1` segments in all.
func (c *DebrisTrailContainer) spawn(tick uint32) {
	if c.NextSpawn == 0 || c.NextSpawn > tick || c.NextSpawn > c.Deadline {
		return
	}
	c.NextSpawn++
	if c.Family != frame.StripFamilyFlameTrail {
		return
	}
	c.laySegment()
}

// laySegment writes one flame-stream segment: the `flamestream` entry at a
// fresh copy of the source — every segment starts there, not at the previous
// segment — frame 0, phase 0, and the container's deadline for its expiry
// [03 R-FX-01 §3].
func (c *DebrisTrailContainer) laySegment() {
	if c.Count >= len(c.Particles) {
		return
	}
	c.Particles[c.Count] = DebrisTrailParticle{
		X: c.Src[0], Y: c.Src[1], Z: c.Src[2],
		Expiry: c.Deadline,
	}
	c.Count++
}

// AppendViews appends this container's live sub-records as barrier-9 strip
// views, in vector order — the composer's own walk order within an object
// [03 §1][03 R-FX-02 §1].
func (c *DebrisTrailContainer) AppendViews(tick uint32, out []frame.StripView) []frame.StripView {
	if c == nil {
		return out
	}
	for i := 0; i < c.Count; i++ {
		p := c.Particles[i]
		view := frame.StripView{
			Strip:  debrisTrailStrip,
			Family: c.Family,
			Bank:   DebrisTrailBank,
			Entry:  c.Entry,
			Frame:  p.Frame,
			X:      p.X, Y: p.Y, Z: p.Z,
		}
		// The same remaining-life hint the session's own strip views carry, so
		// a presentation-owned container's flame takes its ground pool out the
		// way a simulation-owned one does (DESIGN_GPU_RENDERER §31.7). A
		// sub-record with no deadline of its own reports the container's.
		deadline := p.Expiry
		if deadline == 0 {
			deadline = c.Deadline
		}
		if deadline != 0 {
			view.HasRemaining = true
			if deadline > tick {
				view.Remaining = deadline - tick
			}
		}
		out = append(out, view)
	}
	return out
}

// DebrisTrailWind is the committed wind read as the two drift words the puff
// update applies — the FIRST to world X, the SECOND to world Z [R-WIND-01].
// The committed frame carries only heading and strength, so presentation
// re-evaluates the pair the simulation derived at publication; both sides call
// the one world implementation so the words cannot drift apart [01 §7.3].
func DebrisTrailWind(w frame.WindView) (int32, int32) {
	return world.WindVectors(w.Strength, w.Heading)
}

// debrisTrailExpiredAt returns a deadline the current compaction pass already
// treats as passed, which is how the puff's frame-cursor retirement reaches the
// `tick > expiry` test without a second removal path.
func debrisTrailExpiredAt(tick uint32) uint32 {
	if tick > 1 {
		return tick - 1
	}
	return 1
}

// debrisFireJitter is the fire particle's per-axis `crtRand · 3 / 0x8000 − 1`
// whole-unit offset, three draws in X, Y, Z order. A raw 16.16 value's high
// word is its integer part, so adding a whole unit is adding 65536 to the raw
// value [04 R-COB-04 §2][03 R-FX-01 §3].
func debrisFireJitter(crt CRTRandomSource, x, y, z numeric.Fixed) (numeric.Fixed, numeric.Fixed, numeric.Fixed) {
	jx := int64(crt.Rand()*3/0x8000 - 1)
	jy := int64(crt.Rand()*3/0x8000 - 1)
	jz := int64(crt.Rand()*3/0x8000 - 1)
	return x + numeric.Fixed(jx*65536), y + numeric.Fixed(jy*65536), z + numeric.Fixed(jz*65536)
}
