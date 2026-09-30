package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Routes through friends (Nanolathe Modern policy,
// docs/DESIGN_MOVEMENT_PATH.md "Modern routes through friends";
// Traffic.Through). A route search reads a mobile unit that has stood for a
// second as a wall [04 R-PATH-01 §14], and a request
// whose setup finds no ground nearer its goal than where the unit stands
// publishes nothing [04 R-PATH-01 §4]. A unit friends stand round is
// therefore left without a route for as long as they stand, and being left
// without one it stands, and is a wall to the units behind it: a crowd in a
// narrow place holds itself.
//
// Under this policy a request whose search found nothing is searched again
// with friendly units read as absent: first those that have somewhere to go,
// which will not be standing there for long, and then, when that finds
// nothing either, every friendly unit. Another player's units, structures,
// features and the ground stay what they are. The route leads through the
// friends, and is searched only for a unit whose own searches have found
// nothing for ThroughAfter ticks: a unit among friends that are setting off
// is without a route for a moment, and has one as soon as they have gone.
// The unit follows the route as far as the friends let it, since the commit
// refuses a step onto a held cell as always, and waits behind one that is on
// its own way.
//
// Nothing here moves a unit, enters a held cell or draws from a random
// stream, and a request whose own search finds a route is not touched.

// throughFor reports whether request r is one the policy searches again: a
// ground unit's, for a live order.
func (s *System) throughFor(r path.Request) bool {
	if s.world == nil || s.Terrain == nil {
		return false
	}
	u := s.world.Unit(r.Unit)
	if u == nil || u.Handle != r.Unit || u.Def == nil || u.Def.CanFly || u.Move.Mode&3 != 1 {
		return false
	}
	_, _, live := s.livePathOrder(r)
	return live
}

// throughKeeps is the wall test of a search under reading through: at one,
// another player's unit or a friendly one with nowhere to go; at two and
// above, another player's unit.
func (s *System) throughKeeps(owner uint8, through uint8) func(id int) bool {
	if through >= 2 {
		return s.hostileMover(owner)
	}
	return s.keepsItsGround(owner)
}

// throughDue reports whether h's request, whose search under reading through
// found nothing, is due another: its own searches have found nothing for the
// policy's wait. The first of them starts the wait.
func (s *System) throughDue(h pool.Handle, through uint8) bool {
	if through != 0 {
		return true
	}
	st := handleRow(s.traffic, h)
	if !st.routeless {
		st.routeless, st.routelessFrom = true, s.tick
		setHandleRow(&s.traffic, h, st)
	}
	return s.tick-st.routelessFrom >= s.trafficNow.ThroughAfter
}

// noteThrough records the reading h's finished search ran under, when it
// published a route. A route its own search found ends the wait.
func (s *System) noteThrough(h pool.Handle, through uint8, found bool) {
	if !found {
		return
	}
	st := handleRow(s.traffic, h)
	if st.through == through && (through != 0 || !st.routeless) {
		return
	}
	st.through = through
	if through == 0 {
		st.routeless = false
	}
	setHandleRow(&s.traffic, h, st)
}
