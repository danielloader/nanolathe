package survival

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Cleanup is a Survival Modern AI policy [DESIGN_SURVIVAL §16.8]: use a
// small share of idle constructors to keep observed base lanes open. A
// Clear reclaims features only; it never dismantles a constructed building.
const (
	cleanupRadius   = 96
	cleanupCorridor = 64
	cleanupNear     = 1200
	cleanupRest     = 600 // two feature refreshes before retrying a finished job
	cleanupTimeout  = 5400
)

type cleanupJob struct {
	who    handleGen
	x, z   int32
	since  uint32
	issued bool
	cancel bool
	rank   int32 // factory approach, corridor, or nearby metal
	metal  int32 // the selected observed feature, for instrumentation only
}

type cleanupRecent struct {
	x, z  int32
	until uint32
}

type cleanup struct {
	jobs     []cleanupJob
	recent   []cleanupRecent
	builders []int32
	features []aikit.Feature
	one      [1]pool.Handle
	// Counters are never read by a decision.
	orders, lanes, corridors, metal, stops int64
}

func (e *Economy) cleanupClaimed(u *aikit.OwnUnit) bool {
	for i := range e.cleanup.jobs {
		if e.cleanup.jobs[i].who == (handleGen{u.H, u.Gen}) {
			return true
		}
	}
	return false
}

// refreshCleanup releases completed and stale jobs, and stops our reclaim
// or travel order when its area or approach becomes unsafe. A later useful
// construction order is left alone.
func (e *Economy) refreshCleanup(b *core.Board) {
	c := &e.cleanup
	kept := c.jobs[:0]
	for _, j := range c.jobs {
		i := b.Index(j.who.h)
		if i < 0 || b.O.Own[i].Gen != j.who.gen {
			continue
		}
		u := &b.O.Own[i]
		if !cleanupSafe(b, u.X, u.Z, j.x, j.z) || b.Tick-j.since > cleanupTimeout {
			j.cancel = true
		}
		if j.cancel {
			if j.issued && (u.Order == aikit.OrderReclaim || u.Order == aikit.OrderMove) {
				if budgetLeft(b.K, 0) <= 0 {
					kept = append(kept, j)
					continue
				}
				c.one[0] = u.H
				b.K.Stop(c.one[:])
				c.stops++
			}
			c.recent = append(c.recent, cleanupRecent{j.x, j.z, b.Tick + cleanupRest})
			continue
		}
		if j.issued && b.Tick > j.since && u.Order != aikit.OrderReclaim && u.Order != aikit.OrderMove {
			c.recent = append(c.recent, cleanupRecent{j.x, j.z, b.Tick + cleanupRest})
			continue
		}
		kept = append(kept, j)
	}
	c.jobs = kept
	recent := c.recent[:0]
	for _, r := range c.recent {
		if b.Tick < r.until {
			recent = append(recent, r)
		}
	}
	c.recent = recent
}

// hideCleanup reserves active constructors before another layer can issue
// a conflicting command. Defensive jobs use their existing reservation.
func (e *Economy) hideCleanup(b *core.Board) {
	c := &e.cleanup
	c.builders = c.builders[:0]
	for _, i := range b.Builders {
		u := &b.O.Own[i]
		if !e.cleanupClaimed(u) {
			c.builders = append(c.builders, i)
		}
	}
	b.Builders = c.builders
}

func (e *Economy) cleanupTaken(x, z int32, tick uint32) bool {
	const separation = 2 * cleanupRadius
	for i := range e.cleanup.jobs {
		j := &e.cleanup.jobs[i]
		if aikit.Dist2(x, z, j.x, j.z) <= separation*separation {
			return true
		}
	}
	for _, r := range e.cleanup.recent {
		if tick < r.until && aikit.Dist2(x, z, r.x, r.z) <= separation*separation {
			return true
		}
	}
	return false
}

// The utility economy sees no feature in an assigned clearing area. Its
// own opportunistic reclaim therefore cannot duplicate this layer's work.
func (e *Economy) cleanupFeatures(o *aikit.Obs) []aikit.Feature {
	c := &e.cleanup
	c.features = c.features[:0]
	for _, f := range o.Features {
		if !e.cleanupTaken(f.X, f.Z, o.Tick) {
			c.features = append(c.features, f)
		}
	}
	return c.features
}

func (e *Economy) planCleanup(b *core.Board) {
	cons, defense := 0, 0
	for _, i := range b.Builders {
		u := &b.O.Own[i]
		if u.Info.Role.Has(aikit.RoleCommander) {
			continue
		}
		cons++
		if e.st.jobs.claimed(u) {
			defense++
		}
	}
	// Preserve one economic and at least one defensive constructor; take
	// no more than a quarter, with one possible once three constructors exist.
	limit := min(max(cons/4, 1), cons-max(defense, 1)-1)
	limit = max(limit, 0)
	if len(e.cleanup.jobs) > limit {
		for i := limit; i < len(e.cleanup.jobs); i++ {
			e.cleanup.jobs[i].cancel = true
		}
		e.refreshCleanup(b)
	}
	for len(e.cleanup.jobs) < limit {
		var best cleanupJob
		bestScore := int64(-1)
		found := false
		for _, i := range b.Builders {
			u := &b.O.Own[i]
			if u.Order != aikit.OrderIdle || u.QueueLen != 0 || !u.Built || u.Info.Role.Has(aikit.RoleCommander) ||
				u.Info.Def == nil || !u.Info.Def.CanReclamate || e.st.jobs.claimed(u) || e.cleanupClaimed(u) {
				continue
			}
			for _, f := range b.O.Features {
				if !f.Reclaimable || f.Defensive || e.cleanupTaken(f.X, f.Z, b.Tick) ||
					aikit.Dist2(u.X, u.Z, f.X, f.Z) > cleanupNear*cleanupNear ||
					!cleanupReach(b, u, f.X, f.Z) || !cleanupSafe(b, u.X, u.Z, f.X, f.Z) || cleanupAtWork(b, f.X, f.Z) {
					continue
				}
				rank := e.cleanupRank(b, &f)
				if rank == 0 {
					continue
				}
				d := int64(aikit.Dist(u.X, u.Z, f.X, f.Z))
				score := -d
				if rank == 1 {
					score = int64(f.Metal) * 1000 / (d + 100)
				}
				if !found || rank > best.rank || rank == best.rank && score > bestScore {
					best = cleanupJob{who: handleGen{u.H, u.Gen}, x: f.X, z: f.Z, since: b.Tick, rank: rank, metal: f.Metal}
					bestScore, found = score, true
				}
			}
		}
		if !found {
			return
		}
		e.cleanup.jobs = append(e.cleanup.jobs, best)
	}
}

// A feature's whole footprint can obstruct the front (+Z) exit approach.
func cleanupLane(f *aikit.Feature, info *aikit.UnitInfo, x, z int32) bool {
	if info == nil || !info.Role.Has(aikit.RoleFactory) {
		return false
	}
	x0 := (x/16 - info.FootX/2 - 1) * 16
	x1 := (x/16 - info.FootX/2 + info.FootX + 1) * 16
	z0 := (z/16 - info.FootZ/2 + info.FootZ) * 16
	z1 := z0 + 12*16
	return f.X+f.FootX*8 > x0 && f.X-f.FootX*8 < x1 && f.Z+f.FootZ*8 > z0 && f.Z-f.FootZ*8 < z1
}

func (e *Economy) cleanupRank(b *core.Board, f *aikit.Feature) int32 {
	st := e.st
	if f.Blocking {
		for _, i := range b.Factories {
			u := &b.O.Own[i]
			if cleanupLane(f, u.Info, u.X, u.Z) {
				return 3
			}
		}
		for i := range b.O.Allies {
			u := &b.O.Allies[i]
			if u.Built && cleanupLane(f, u.Info, u.X, u.Z) {
				return 3
			}
		}
		if nearSegment(f.X, f.Z, st.cx, st.cz, st.hx, st.hz, cleanupCorridor) {
			return 2
		}
		for _, i := range b.Factories {
			u := &b.O.Own[i]
			if nearSegment(f.X, f.Z, st.hx, st.hz, u.X, u.Z+u.Info.FootZ*8+96, cleanupCorridor) {
				return 2
			}
		}
	}
	if f.Metal > 0 {
		return 1
	}
	return 0
}

func nearSegment(x, z, ax, az, bx, bz, radius int32) bool {
	dx, dz := int64(bx-ax), int64(bz-az)
	len2 := dx*dx + dz*dz
	if len2 == 0 {
		return aikit.Dist2(x, z, ax, az) <= int64(radius)*int64(radius)
	}
	t := clampI(int64(x-ax)*dx+int64(z-az)*dz, 0, len2)
	px, pz := ax+int32(dx*t/len2), az+int32(dz*t/len2)
	return aikit.Dist2(x, z, px, pz) <= int64(radius)*int64(radius)
}

// Existing reclaimers near the area may already be doing utility work.
func cleanupAtWork(b *core.Board, x, z int32) bool {
	for _, i := range b.Builders {
		u := &b.O.Own[i]
		if u.Order == aikit.OrderReclaim && aikit.Dist2(u.X, u.Z, x, z) <= 256*256 {
			return true
		}
	}
	return false
}

func cleanupReach(b *core.Board, u *aikit.OwnUnit, x, z int32) bool {
	if class, ok := aikit.MoveClassOf(u.Info); ok {
		r := b.K.Map.Reach(class)
		region := r.At(u.X, u.Z)
		return region > 0 && region == r.At(x, z)
	}
	return u.Info.Role.Has(aikit.RoleAir) || u.Info.Def == nil
}

// Reject an observed threat anywhere along the direct approach, including
// the clearing area's edge: the queued Clear can walk between features.
func cleanupSafe(b *core.Board, ax, az, bx, bz int32) bool {
	steps := max(aikit.Dist(ax, az, bx, bz)/128, 1)
	for i := int32(0); i <= steps; i++ {
		x, z := ax+(bx-ax)*i/steps, az+(bz-az)*i/steps
		for _, p := range [...][2]int32{{0, 0}, {cleanupRadius, 0}, {-cleanupRadius, 0}, {0, cleanupRadius}, {0, -cleanupRadius}} {
			if b.Threat != nil && b.Threat.At(x+p[0], z+p[1]) > 0 {
				return false
			}
		}
	}
	return true
}

func (e *Economy) emitCleanup(b *core.Board) {
	c := &e.cleanup
	for i := range c.jobs {
		j := &c.jobs[i]
		if j.issued || j.cancel {
			continue
		}
		if budgetLeft(b.K, 2) <= 0 {
			return
		}
		b.K.Clear(j.who.h, j.x, j.z, cleanupRadius)
		j.issued, j.since = true, b.Tick
		c.orders++
		c.metal += int64(j.metal)
		switch j.rank {
		case 3:
			c.lanes++
		case 2:
			c.corridors++
		}
	}
}

func (e *Economy) explainCleanup(x *aikit.Explain) {
	c := &e.cleanup
	x.Notes = append(x.Notes, fmt.Sprintf("survival cleanup: %d active, %d clears (%d lanes, %d corridors), %d selected metal, %d stops", len(c.jobs), c.orders, c.lanes, c.corridors, c.metal, c.stops))
	for _, j := range c.jobs {
		x.Goals = append(x.Goals, aikit.Goal{Label: "survival clear", Chosen: j.issued, X: j.x, Z: j.z})
	}
}
