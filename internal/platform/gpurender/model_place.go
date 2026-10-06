package gpurender

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// Placement contexts (docs/DESIGN_GPU_RENDERER.md §22, "Placement workers").
//
// Placing a frame's subjects is two kinds of work. Some of it is ORDERED: the
// shelf packer, the retained store's and body index's lookups, inserts and
// evictions, the texture page's first sightings and the water reflection
// batch all depend on everything placed before. The rest — the per-face
// arithmetic of the cold, warm and replayed appends, the outline rings and the
// live lanes — depends only on its own subject and the decisions the ordered
// work made for it. A job (a subject with its group, or one shadow) is
// allocated by allocSubjectJob or allocShadowJob; each of its packets is then
// DECIDED (decideLane, decidePath: the store and body index, the append path,
// the texture page) and FILLED (appendPlaced) into a placement context.
//
// The lane's own context is the frame's batch. Filling a job into it, decided
// inline, is the sequential lane, packet for packet what it has always been.

// modelPlaceCtx is one placement context: the batch its appends land in and
// the per-subject state an append carries from one face to the next.
type modelPlaceCtx struct {
	// The batch: runs over vertices and indices, and the parameter image's
	// entries — mapped faces and subject verdicts, twelve texels an entry,
	// one-based (model_quads.go).
	runs   []modelDirectRun
	verts  []ebiten.Vertex
	idx    []uint32
	params modelQuadParams
	// runPage is the atlas page colourRun stamps a run with — the region
	// page of the packet being filled — and runBase the first run the job
	// being filled may extend.
	runPage int32
	runBase int

	// soloPass is set while a carried child's second composition is appended:
	// the commit reads that image for coverage and keys alone, so its faces
	// take the coverage-only path and skip everything a colour needs.
	soloPass bool
	// outline is the context's planned outline rings (model_outline.go), and
	// soloOutline the range of them the group composition planned for the
	// packet the solo pass is about to repeat, so its rings are described and
	// walked once. Valid only between those two appends of soloOutlineFor.
	outline        []modelOutlineRing
	soloOutline    [2]int32
	soloOutlineFor *drawlist.ModelGeometry
	// soloSlot is the same reuse for the doubled lane's box (slotBoundsFor).
	soloSlot    image.Rectangle
	soloSlotFor *drawlist.ModelGeometry
	// groupReflection is the construction group region a child's reflection
	// also tests (water_reflections.go); reflectActive and reflectRegion are
	// the reflecting packet being appended and its region, nil for a packet
	// that does not reflect this frame. A worker records its faces'
	// reflections in reflected rather than appending them.
	groupReflection modelDirectRegion
	reflectActive   *drawlist.ModelGeometry
	reflectRegion   modelDirectRegion
	reflected       []modelFaceReflection
	// Seeded children keep their original keys through cached resolution;
	// seedGroupDelta belongs to the later merge/reflection admission. Other
	// packets apply keyDelta directly while appending corners.
	seedPhase                               modelSeedPhase
	seedGroupDelta                          int32
	keyDelta                                int32
	lightSources                            subjectLights
	lightX, lightY, lightScale, lightHeight float32
	standalone                              []int
	doubled                                 [4]drawlist.ModelVertex
	// noQuads is set for a subject whose frame origin the verdict entry cannot
	// carry, so its four-corner faces interpolate linearly rather than map
	// through a frame the fragment cannot recover.
	noQuads bool
	// fallbackOpacity is the fallback batch's body opacity: one, or the ALP
	// half-colour's half for a cloaked subject [03 R-COMP-01 §2].
	fallbackOpacity float32

	// capture is the store entry the cold append is filling, or nil; quadBase
	// and vertBase are where its parameter block and vertices begin, and
	// culledAt and materialAt the accounting before the append. body is the
	// body entry the cold append is recording, or nil (model_retain.go).
	capture              *modelRetained
	quadBase, vertBase   int
	culledAt, materialAt int
	body                 *modelRetainedBody

	// stats is the accounting the appends add to, flushed into the frame's
	// (flushStats), and prep the arena the outline walk takes its endpoint
	// quads from.
	stats modelPlaceStats
	prep  *modelPrepScratch

	// decided is set when the context's packets arrive decided (their store
	// entries, paths and texture pages settled before the append), so the
	// append neither decides nor resolves a texture it has not been handed;
	// texPage is then the texture page the lane being appended binds.
	decided bool
	texPage *ebiten.Image
	// worker is set for a context that appends beside others: it touches no
	// renderer state at all, the reflection batch included.
	worker bool
}

// resetFrame starts a frame on the context: an empty batch and no per-packet
// reuse, which holds slices and boxes of the last frame's packets only.
func (d *modelPlaceCtx) resetFrame() {
	d.soloPass = false
	d.groupReflection = modelDirectRegion{}
	clear(d.outline)
	d.outline = d.outline[:0]
	d.soloOutline, d.soloOutlineFor, d.soloSlotFor = [2]int32{}, nil, nil
	d.runs, d.verts, d.idx = d.runs[:0], d.verts[:0], d.idx[:0]
	d.runBase = 0
	d.params.reset()
	d.reflectActive = nil
	clear(d.reflected)
	d.reflected = d.reflected[:0]
}

// laneCtx is the lane's own placement context, whose batch is the frame's,
// wired to the renderer's preparation arena.
func (r *Renderer) laneCtx() *modelPlaceCtx {
	c := &r.modelDirect.modelPlaceCtx
	c.prep = &r.modelPrep
	return c
}

// modelPlaceStats is the accounting an append adds to: exactly the ModelStats
// counters a placement context may touch. An append that counted anything
// else would not compile, rather than have its count dropped when the
// workers' accounting is summed (model_place_pool.go).
type modelPlaceStats struct {
	DirectFaces, DirectCulled, LitModelFaces, MaterialFaces      int
	DirectWarm, DirectCaptured, DirectColdParams                 int
	DirectRetained, DirectRetainedLive, DirectRetainedOutline    int
	DirectOutlineRings, DirectOutlineTexels, DirectOutlineWalked int
}

// flushStats adds the context's accounting into the frame's and zeroes it.
func (d *modelPlaceCtx) flushStats(into *ModelStats) {
	s := &d.stats
	into.DirectFaces += s.DirectFaces
	into.DirectCulled += s.DirectCulled
	into.LitModelFaces += s.LitModelFaces
	into.MaterialFaces += s.MaterialFaces
	into.DirectWarm += s.DirectWarm
	into.DirectCaptured += s.DirectCaptured
	into.DirectColdParams += s.DirectColdParams
	into.DirectRetained += s.DirectRetained
	into.DirectRetainedLive += s.DirectRetainedLive
	into.DirectRetainedOutline += s.DirectRetainedOutline
	into.DirectOutlineRings += s.DirectOutlineRings
	into.DirectOutlineTexels += s.DirectOutlineTexels
	into.DirectOutlineWalked += s.DirectOutlineWalked
	*s = modelPlaceStats{}
}

// texturePage is the texture page the lane being appended binds: the shared
// page as it stands, or the one its decision recorded.
func (d *modelPlaceCtx) texturePage(r *Renderer) *ebiten.Image {
	if d.decided {
		return d.texPage
	}
	return r.texturePage()
}

// modelPlacePath is how a packet's cached lane is appended.
type modelPlacePath uint8

const (
	// modelPlaceAppend appends the lane warm or cold without capturing it.
	modelPlaceAppend modelPlacePath = iota
	// modelPlaceCapture appends it and captures the output into its entry.
	modelPlaceCapture
	// modelPlaceReplay replays it from its entry.
	modelPlaceReplay
)

// modelPlacePacket is one append of a job: a packet and the region it
// composes into, and what deciding it settled.
type modelPlacePacket struct {
	g *drawlist.ModelGeometry
	// group is the carrier whose clip applies to a group child, else nil;
	// keyDelta is added to every key of the packet.
	group           *drawlist.ModelGeometry
	region          modelDirectRegion
	groupReflection modelDirectRegion
	keyDelta        int32
	shadow, solo    bool

	// decideLane: the raster the cached lane is drawn from, its scale, the
	// atlas texel of its local (0,0) and of the native packet's, the store
	// entry and whether it is replayable, the warm body, and whether the
	// packet is keyed or reflects. A doubled lane that is not replayed has
	// its local (0,0) settled by the append (slotPending): it takes a walk
	// over every corner of the lane, and nothing ordered depends on it.
	wb                           image.Rectangle
	raster                       *drawlist.ModelGeometry
	rx, ry                       int32
	scale, ox, oy, nox, noy      float32
	e                            *modelRetained
	body                         *modelRetainedBody
	hit, warm, keyed, reflecting bool
	slotPending                  bool
	// decidePath: the path, and the body a cold keyed append records.
	path   modelPlacePath
	record *modelRetainedBody
	// texCached and texLive are the texture page as the cached lane and the
	// per-frame lanes find it: a cold cached lane can open a page. For a
	// packet decided ahead, cachedSlots and liveSlots are the texture slots of
	// the cold cached lane's faces and of the live lane's, resolved in order
	// (model_place_pool.go).
	texCached, texLive     *ebiten.Image
	cachedSlots, liveSlots []modelTextureSlot
}

// modelPlaceJob is one job: a subject with its group, or one shadow. p0 and
// p1 bound its packets.
type modelPlaceJob struct {
	p0, p1 int32
	// The pool's fill (model_place_pool.go): the participant whose context
	// holds the job's output, and the output's spans there — vertices,
	// indices, runs, parameter slots and recorded face reflections.
	w                                      int32
	v0, v1, i0, i1, r0, r1, q0, q1, f0, f1 int32
	// The layout: where the output lands in the frame's batch, and whether
	// its first run joins the batch's last, whose first vertex is delta
	// vertices before the job's.
	v, ix, q int32
	joined   bool
	delta    int32
}

// fillJob appends a job's packets, in order, into context d. Each packet's
// faces draw into the page its own region is on: a construction group's
// separate child region can open a page after its group region was placed on
// the one before.
func (r *Renderer) fillJob(d *modelPlaceCtx, job *modelPlaceJob, packets []modelPlacePacket) {
	for i := job.p0; i < job.p1; i++ {
		d.runPage = packets[i].region.page
		r.appendPlaced(d, &packets[i])
	}
}

// decideLane settles, for one packet, everything its append needs from the
// ordered state before its verdict entry is made: where its raster lands, the
// retained store's entry and the body index's (the ordered store operations,
// retainedEntry), whether the entry replays, and whether the body appends the
// lane warm.
//
// Atlas texel of the raster's local (0,0) and the scale the corners take. A
// doubled lane's local box, rounded down to an even corner, lands on the
// packet's own world-bounds origin so its two-by-two blocks line up with the
// native pixels the commit resolves [03 R-REN-03A §7]; native corners are
// doubled about that origin instead.
func (r *Renderer) decideLane(p *modelPlacePacket) {
	g, region := p.g, p.region
	wb := modelWorldBounds(g)
	rx := region.x + 2*int32(wb.Min.X-region.bounds.Min.X)
	ry := region.y + 2*int32(wb.Min.Y-region.bounds.Min.Y)
	raster, scale := g, float32(2)
	local := modelLocalBounds(g)
	nox := float32(rx) - float32(local.Min.X)*2
	noy := float32(ry) - float32(local.Min.Y)*2
	ox, oy := nox, noy
	reflecting := !p.shadow && g.ReflectWater && !r.reflections.disabled
	e, body, keyed := r.retainedEntry(g, p.keyDelta, p.group, p.solo, reflecting)
	hit := e != nil && e.captured && e.matches(g)
	if hit {
		if e.doubled {
			raster, scale = g.Supersample, 1
			slot := e.slotFor(g.Supersample)
			ox = float32(rx) - float32(slot.Min.X&^1)
			oy = float32(ry) - float32(slot.Min.Y&^1)
		}
	} else if ss := g.Supersample; ss != nil && !modelSlotBoundsEmpty(ss) {
		raster, scale = ss, 1
		p.slotPending = true
	}
	// The warm append needs the body entry to describe this raster's faces
	// exactly; otherwise the lane is cold and, for a keyed packet, records the
	// body anew. A reflecting packet is cold only when it is not warm.
	warm := !hit && body != nil && body.matches(raster.Faces, raster == g.Supersample, r.texturePage())
	if !warm {
		body = nil
		if reflecting && keyed {
			r.modelStats.DirectColdReflect++
		}
	}
	p.wb, p.raster, p.rx, p.ry, p.scale, p.ox, p.oy, p.nox, p.noy = wb, raster, rx, ry, scale, ox, oy, nox, noy
	p.e, p.body, p.hit, p.warm, p.keyed, p.reflecting = e, body, hit, warm, keyed, reflecting
}

// decidePath settles a decided packet's path: replay when the entry holds
// the lane and the parameter image can take it — noQuads is the verdict
// entry's (subjectVerdicts) and count the parameter image's slots after it —
// capture on a primed key, else an append that primes the key. A cold append
// of a keyed packet records its body: the body index's insert or eviction is
// made here, in order with the store's.
func (r *Renderer) decidePath(p *modelPlacePacket, noQuads bool, count int) {
	e := p.e
	fits := p.hit && !noQuads && count+e.quads <= modelDirectParamCap
	r.modelStats.countRetainMiss(e, p.hit, fits, p.warm)
	switch {
	case fits:
		p.path = modelPlaceReplay
	case e != nil && e.primed:
		p.path = modelPlaceCapture
	default:
		p.path = modelPlaceAppend
		if e != nil {
			e.primed = true
		}
	}
	p.record = nil
	if p.path != modelPlaceReplay && !p.warm && p.keyed {
		p.record = r.modelDirect.retain.acquireBody(p.g.Cache)
	}
}

// appendPlaced appends one packet's lanes into its region, in retail's order:
// the cached faces, the outline endpoints, then the live faces
// [03 R-REN-03A §4]. On a context whose packets are not decided ahead, it
// decides the packet itself, in the order the sequential lane always has.
//
// A packet the recorder marked reusable (§13.12) and that composes alone —
// no group delta, no solo pass, no water reflection — takes its cached lane
// from the retained store when the store holds it (model_retain.go): the
// packed vertices are replayed at this frame's placement, and only the
// verdict entry and the battle light are made anew. The outline and the live
// lane are per-frame and are appended cold after the cached lane whether it
// was replayed or not, exactly as they follow a cold cached lane. The first
// sighting of a key primes a stub, the second captures the cold append's
// output, and every later one replays it, so a subject that rebuilds every
// frame never pays for a copy nobody reads. A key miss whose body the index
// holds — the same object under a new revision — appends the lane warm
// (appendWarmLane), captured all the same when the key is primed; a cold
// append records the body for the next revision.
func (r *Renderer) appendPlaced(d *modelPlaceCtx, p *modelPlacePacket) {
	d.soloPass, d.groupReflection = p.solo, p.groupReflection
	if !d.decided {
		r.decideLane(p)
	}
	if p.slotPending {
		// The doubled lane's box, rounded down to an even corner, is where
		// its local (0,0) lands.
		ssLocal := d.slotBoundsFor(p.raster, p.solo)
		p.ox = float32(p.rx) - float32(ssLocal.Min.X&^1)
		p.oy = float32(p.ry) - float32(ssLocal.Min.Y&^1)
	}
	g, shadow := p.g, p.shadow
	entry := 0
	d.noQuads = false
	if !shadow {
		// The reveal rides the raster (a doubled lane carries its own copy);
		// the waterline and Digger ride the packet, which is the one the
		// recorder configures; the frame is where this raster lands.
		entry = d.subjectVerdicts(p.raster.Reveal, g, p.keyDelta, p.group, p.ox, p.oy)
	}
	if !d.decided {
		r.decidePath(p, d.noQuads, d.params.count)
	}
	mode := 0
	if shadow {
		mode = modelDirectShadow
	}
	d.keyDelta = p.keyDelta
	d.seedPhase = modelSeedForPacket(p)
	d.seedGroupDelta = p.keyDelta
	if d.seedPhase != modelSeedNone {
		d.keyDelta = 0
	}
	d.lightSources = subjectLights{}
	if !shadow && !d.soloPass {
		// A solo image's colour is never read, so it carries no light, and the
		// world geometry it repeats reflects once, not twice.
		wb, region := p.wb, p.region
		d.lightSources = r.lighting.near(float32(wb.Min.X+wb.Max.X)*0.5, float32(wb.Min.Y+wb.Max.Y)*0.5, float32(wb.Dx()+wb.Dy())*0.5)
		d.lightX = float32(region.bounds.Min.X) + (p.ox-float32(region.x))*0.5
		d.lightY = float32(region.bounds.Min.Y) + (p.oy-float32(region.y))*0.5
		d.lightScale, d.lightHeight = p.scale*0.5, g.WorldHeight
		if p.reflecting {
			d.reflectActive, d.reflectRegion = g, region
		}
		if !d.worker {
			r.reflections.active, r.reflections.region = g, region
		}
	}
	d.texPage = p.texCached
	switch p.path {
	case modelPlaceReplay:
		r.replayRetained(d, p.e, g, p.ox, p.oy, entry, shadow)
	case modelPlaceCapture:
		d.beginCapture(p.e)
		r.appendCachedLane(d, p, mode, entry)
		d.finishCapture(g, p.raster == g.Supersample, p.ox, p.oy)
		if p.e.captured {
			d.stats.DirectCaptured++
		} else if !p.warm {
			d.stats.DirectColdParams++
		}
	default:
		r.appendCachedLane(d, p, mode, entry)
	}
	if d.seedPhase != modelSeedNone {
		d.seedPhase = modelSeedLive
	}
	d.texPage = p.texLive
	if !shadow && len(g.Outline) != 0 {
		// The outline endpoints are one native pixel each, from the native
		// packet's rows drawn at 2× about the native origin: retail walks the
		// rows of the 1× image after the anti-alias resolve [03 R-COMP-01 §3],
		// so an endpoint is a whole pixel. The doubled lane's own rows would
		// put an endpoint at a doubled column, straddling two pixels' blocks
		// and resolving to half an endpoint in each (model_outline.go).
		r.appendOutline(d, g, p.nox, p.noy, entry)
	}
	r.appendDirectLane(d, p.raster.LiveFaces, p.liveSlots, p.raster, p.ox, p.oy, p.scale, mode|modelDirectLive, entry)
	d.keyDelta, d.seedPhase = 0, modelSeedNone
	d.soloPass, d.groupReflection, d.reflectActive = false, modelDirectRegion{}, nil
	if !d.worker {
		r.reflections.active = nil
	}
}

// reflectFace reflects one appended face of the reflecting packet: into the
// reflection batch at once, or, on a worker, as a record the placing goroutine
// appends in order once the frame's jobs are laid out (water_reflections.go).
func (d *modelPlaceCtx) reflectFace(r *Renderer, f *drawlist.ModelFace, ox, oy, s, cx, cy float32, quad int, page int32) {
	if !d.worker {
		r.reflectModelFace(f, ox, oy, s, cx, cy, float32(modelDirectFatten), quad, page)
		return
	}
	d.reflected = append(d.reflected, modelFaceReflection{f: f, ox: ox, oy: oy, scale: s, cx: cx, cy: cy, fat: float32(modelDirectFatten), quad: quad, page: page,
		g: d.reflectActive, region: d.reflectRegion, group: d.groupReflection, keyDelta: d.keyDelta, seedPhase: d.seedPhase, seedGroupDelta: d.seedGroupDelta})
}

// modelSlotBoundsEmpty reports whether modelSlotBounds(g) is empty without
// walking the corners when the packet has a box, which a non-empty box
// already makes the union's.
func modelSlotBoundsEmpty(g *drawlist.ModelGeometry) bool {
	if g.Width > 0 && g.Height > 0 {
		return false
	}
	return modelLocalBounds(g).Empty()
}

// slotBoundsFor is modelSlotBounds with the group composition's own walk reused
// by the solo pass that repeats the same packet an instant later: the doubled
// lane's box costs a pass over every corner of the packet, and the two appends
// ask for the same one. Only the solo pass reads the memo, and only the
// composition that precedes it writes one, so it never outlives its packet.
func (d *modelPlaceCtx) slotBoundsFor(ss *drawlist.ModelGeometry, solo bool) image.Rectangle {
	if solo {
		if d.soloSlotFor == ss {
			return d.soloSlot
		}
		return modelSlotBounds(ss)
	}
	b := modelSlotBounds(ss)
	d.soloSlotFor, d.soloSlot = ss, b
	return b
}
