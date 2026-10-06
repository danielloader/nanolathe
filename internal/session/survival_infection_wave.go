package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Nanolathe Modern scenario balance: takeover units support an established
// attacking force, never form an opening swarm (DESIGN_SURVIVAL §6.7).
const (
	survivalInfectorFrom      = 10 * 60 * 30
	survivalInfectorLiveLimit = 2
)

func (s *Session) survivalInfector(def *content.UnitDef) bool {
	return s.orderRules().Infection(def).DurationTicks != 0
}

func (s *Session) survivalLiveInfectors() int {
	st := s.Survival
	st.walk = s.Units.AppendLiveSliced(st.walk[:0])
	n := 0
	for _, u := range st.walk {
		if u.Owner == st.attacker && !u.Dying && s.survivalInfector(u.Def) {
			n++
		}
	}
	return n
}

func (s *Session) survivalPlanWave(tick uint32, enter survival.Entry) survival.Wave {
	st := s.Survival
	infectors := make([]bool, len(st.pool.Units))
	any := false
	for i, u := range st.pool.Units {
		infectors[i] = s.survivalInfector(u.Def)
		any = any || infectors[i]
	}
	if !any {
		// Preserve both picks and RNG consumption for Strict and ordinary catalogs.
		return survival.Plan(st.wave, tick, &st.pool, st.tuning, st.opts, enter, s.SimRNG())
	}
	ordinaryEntry := func(angle uint16, i int) bool {
		return !infectors[i] && (enter == nil || enter(angle, i))
	}
	wave := survival.Plan(st.wave, tick, &st.pool, st.tuning, st.opts, ordinaryEntry, s.SimRNG())
	if tick < survivalInfectorFrom || wave.Units() < 4 || s.survivalLiveInfectors() >= survivalInfectorLiveLimit {
		return wave
	}
	// Replace at most one ordinary pick, retaining at least three escorts and
	// never spending more than the original plan. Canonical pool order, then
	// group/pick order, breaks ties without another random draw. Entry and tier
	// checks still apply; no legal replacement means no infector this wave.
	top := st.tuning.UnlockedTier(wave.Budget, &st.pool)
	for i, u := range st.pool.Units {
		if !infectors[i] || u.Tier > top {
			continue
		}
		for gi := range wave.Groups {
			g := &wave.Groups[gi]
			if u.Domain != g.Domain && !(g.Domain == survival.Ground && u.Domain == survival.Amphibious) {
				continue
			}
			if enter != nil && !enter(g.Angle, i) {
				continue
			}
			for pi, old := range g.Picks {
				if st.pool.Units[old].Cost >= u.Cost {
					g.Picks[pi] = i
					return wave
				}
			}
		}
	}
	return wave
}

// The cursor has advanced past the current pick. Earlier planned infector
// slots consume this wave's one attempt even if placement failed or they died.
// This also bounds a wave planned in Strict before switching to Modern.
func (s *Session) survivalInfectorSpawnAllowed(tick uint32) bool {
	st := s.Survival
	if tick < survivalInfectorFrom || st.plan.Units() < 4 {
		return false
	}
	for gi, g := range st.plan.Groups {
		if gi > st.nextG {
			break
		}
		end := len(g.Picks)
		if gi == st.nextG {
			end = st.nextP - 1
		}
		for _, i := range g.Picks[:end] {
			if s.survivalInfector(st.pool.Units[i].Def) {
				return false
			}
		}
	}
	// The live census walks every unit, so it runs only for a pick that
	// passed the cheap plan checks.
	return s.survivalLiveInfectors() < survivalInfectorLiveLimit
}

// survivalHuntRegion is a ground infector's static region (DESIGN_SURVIVAL
// §6.6) for its movement class at its current footprint anchor. Zero, for an
// aircraft or an unlabelled position, disables the reach filter.
func (s *Session) survivalHuntRegion(u *units.Unit) (*survivalClass, int32) {
	if u == nil || u.Def == nil || u.Def.CanFly {
		return nil, 0
	}
	c := s.survivalClassFor(u.Def)
	if c == nil {
		return nil, 0
	}
	ax, az := survivalAnchor(c.profile, u.X, u.Z)
	return c, survivalRegionAround(c, ax, az, 0)
}

// survivalAnchor is the footprint anchor of the class's footprint centred on a
// world position, the cell its regions label: floor((x + 8 - 8·fx) / 16).
func survivalAnchor(p movement.Profile, x, z numeric.Fixed) (int32, int32) {
	const half, cell = int64(8 << 16), int64(16 << 16)
	fx, fz := max(int64(p.FootPrintX), 1), max(int64(p.FootPrintZ), 1)
	return int32(numeric.FloorDiv(int64(x)+half-fx*half, cell)), int32(numeric.FloorDiv(int64(z)+half-fz*half, cell))
}

// survivalRegionAround returns want when the anchor or one of its eight
// neighbours carries it, tolerating a unit beside an impassable cell. With
// want zero it returns the anchor's own label, else the first labelled
// neighbour in row order.
func survivalRegionAround(c *survivalClass, ax, az, want int32) int32 {
	if id := c.regions.At(ax, az); id != 0 && (want == 0 || id == want) {
		return id
	}
	for dz := int32(-1); dz <= 1; dz++ {
		for dx := int32(-1); dx <= 1; dx++ {
			if id := c.regions.At(ax+dx, az+dz); id != 0 && (want == 0 || id == want) {
				return id
			}
		}
	}
	return 0
}
