package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Steering (Nanolathe Modern policy, docs/DESIGN_MOVEMENT_PATH.md "Modern
// steering"; Traffic.Sidestep). A ground mover looks a few cells ahead along
// the heading it wants. When its footprint could not stand
// there, it wants the nearest heading to either side along which it could,
// so it turns past the obstruction before it reaches it instead of driving
// into it, halving its speed and waiting sixty ticks for a route around
// [04 R-COLL-01 §1][04 R-MOV-01 §7]. It never enters a held cell: the commit
// validator is untouched and still refuses any step onto one.
//
// Who steers is decided by what is ahead:
//
//   - a friendly mover making way the same way is followed; without
//     NoOvertake one markedly slower than this unit can go is overtaken;
//   - a friendly mover crossing is passed behind under PassBehind, and
//     otherwise left alone: it will have gone;
//   - a friendly mover coming the other way is passed on the right, a rule
//     both units apply, so they part without either knowing the other's
//     choice;
//   - anything else — ground, a structure, a parked or held unit, another
//     player's unit — is passed on whichever side is freer, and the side is
//     kept until the obstruction is behind.
//
// Everything here reads committed state in the unit's own visit, in sweep
// order, with integer arithmetic; it draws from neither random stream.

const (
	// steerLook is how many cells ahead a mover looks, and steerHold how
	// many ticks it keeps to the side it chose.
	steerLook = 3
	steerHold = 24
	// steerStep is one candidate heading step, a sixteenth of a circle.
	steerStep = 0x1000
	// steerSteps is how many steps to either side are tried: a quarter turn.
	steerSteps = 4
	// steerCellRaw is one cell of travel in raw 16.16, per 8192 of a trig
	// table entry: (16<<16) >> 13.
	steerCellRaw = 128
	// steerOvertake is the share of this unit's maximum speed, in
	// hundredths, below which a same-way leader is overtaken, not followed.
	steerOvertake = 85
	// steerPassRadius is how near a waypoint a sidestepping unit may pass,
	// in world units, for the waypoint to count as taken.
	steerPassRadius = 56
)

// probeResult is what stands in the way along one heading.
type probeResult struct {
	free   int32 // whole cells of travel before the footprint could not stand
	occ    int   // the occupant that stopped it, -1 for none
	static bool  // ground or map edge stopped it
}

// footprintHeld reports what, if anything, keeps a footprint from standing
// at anchor: the first failing cell in the validator's own row-major order,
// static test before occupant test [04 R-COLL-01 §2].
func (s *System) footprintHeld(self int, profile Profile, anchor Cell, fx, fz int16) (occ int, static, held bool) {
	s.labStats.Anchors++
	if !commitRectInBounds(s.Terrain, anchor, fx, fz) {
		return -1, true, true
	}
	for dz := int32(0); dz < int32(max(fz, 1)); dz++ {
		for dx := int32(0); dx < int32(max(fx, 1)); dx++ {
			c := Cell{X: anchor.X + dx, Z: anchor.Z + dz}
			if s.Terrain != nil && !profile.IsPassableCommitCell(s.Terrain, c.X, c.Z) {
				return -1, true, true
			}
			if s.Grid != nil {
				if id, ok := s.Grid.OccupantAt(c); ok && id != self {
					return id, false, true
				}
			}
		}
	}
	return -1, false, false
}

// probeFine is how many samples a probe takes per cell of travel: a ray
// sampled once a cell steps over the corner of a cell it clips.
const probeFine = 4

// probe walks the footprint along heading from the unit's position and
// reports how many whole cells of travel it could stand for.
func (s *System) probe(coll *CollisionState, profile Profile, heading uint16, cells int32) probeResult {
	sin := int64(numeric.Sin(numeric.Angle(heading)))
	cos := int64(numeric.Cos(numeric.Angle(heading)))
	bx, bz := coll.HalfBias()
	last := coll.CachedAnchor
	res := probeResult{occ: -1}
	s.labStats.Probes++
	for k := int64(1); k <= int64(cells)*probeFine; k++ {
		// The position step is (-sin, -cos) along the heading [04 R-MOV-01 §4].
		px := int64(coll.X) - sin*(steerCellRaw/probeFine)*k
		pz := int64(coll.Z) - cos*(steerCellRaw/probeFine)*k
		a := QuantizedAnchor(int32(px), int32(pz), bx, bz)
		if a != last {
			occ, static, held := s.footprintHeld(coll.ID, profile, a, coll.FootPrintX, coll.FootPrintZ)
			if held {
				res.occ, res.static = occ, static
				return res
			}
			last = a
		}
		res.free = int32(k / probeFine)
	}
	return res
}

func headingApart(a, b uint16) uint16 {
	d := int16(a - b)
	if d < 0 {
		d = -d
	}
	return uint16(d)
}

// steerAround is the sidestep decision for one visit: the heading the
// follower should want instead of desired, and whether it should brake
// because what is directly ahead along its present heading is held.
func (s *System) steerAround(u *units.Unit, coll *CollisionState, profile Profile, route *Route, desired uint16, tick uint32, tr Traffic) (uint16, bool) {
	heading, brake := s.steerRound(u, coll, profile, route, desired, tick, tr)
	st := handleRow(s.traffic, u.Handle)
	steering := heading != desired
	if steering {
		s.labStats.SteerTicks[st.ahead]++
		if !st.steering {
			s.labStats.SteerStarts[st.ahead]++
		}
	}
	if steering != st.steering {
		st.steering = steering
		setHandleRow(&s.traffic, u.Handle, st)
	}
	return heading, brake
}

// steerRound is steerAround's decision.
func (s *System) steerRound(u *units.Unit, coll *CollisionState, profile Profile, route *Route, desired uint16, tick uint32, tr Traffic) (uint16, bool) {
	s.labStats.Visits++
	st := handleRow(s.traffic, u.Handle)
	look := int32(steerLook)
	// The destination policies own the last stretch: a unit orbiting an
	// occupied goal would never arrive.
	if !routeEndClear(route, coll.X>>16, coll.Z>>16) {
		if st.side != 0 {
			st.side = 0
			setHandleRow(&s.traffic, u.Handle, st)
		}
		return desired, false
	}
	if tr.ToWaypoint && route != nil && route.Active && route.Count >= 2 {
		// No farther than the point the route turns at, and a cell at the
		// least.
		p := route.Points[1]
		dx, dz := int64(p.X)-int64(coll.X>>16), int64(p.Z)-int64(coll.Z>>16)
		cells := int32((isqrtInt64Arrive(dx*dx+dz*dz) + 15) >> 4)
		look = min(look, max(cells, 1))
	}
	ahead := s.probe(coll, profile, desired, look)
	if ahead.free >= look {
		if tr.KeepRound && coll.Blocked && st.side != 0 && tick < st.sideUntil && headingApart(st.round, desired) <= steerSteps*steerStep {
			// The way wanted is free on this visit, and the unit's last
			// step was refused: it keeps the way round it chose while that
			// is free, and the choice is good for as long again.
			if on := s.probe(coll, profile, st.round, look); on.free >= look {
				st.sideUntil = tick + steerHold
				setHandleRow(&s.traffic, u.Handle, st)
				return st.round, false
			}
		}
		if st.side != 0 && tick >= st.sideUntil {
			st.side = 0
			setHandleRow(&s.traffic, u.Handle, st)
		}
		return desired, false
	}
	prefer := int8(-1) // right: headings grow to the left [04 §5.1]
	sticky := st.side != 0 && tick < st.sideUntil
	if sticky {
		prefer = st.side
	}
	kind := uint8(SteerOther)
	switch {
	case ahead.static:
		kind = SteerStatic
	case ahead.occ >= 0:
		if c := handleRow(s.Collisions, pool.Handle(ahead.occ)); c != nil && c.Building {
			kind = SteerStructure
		}
	}
	if !ahead.static && ahead.occ >= 0 {
		if other, ok := s.friendlyMover(u, ahead.occ); ok {
			apart := headingApart(coll.Heading, other.Heading)
			switch {
			case s.sameWayMover(coll, other, pool.Handle(ahead.occ)) && makingWay(other):
				kind = SteerSlower
			case !makingWay(other) && other.Speed > 0:
				kind = SteerHeld
			case !makingWay(other):
				kind = SteerParked
			case apart < alliedPassMinTurn:
				kind = SteerCrossing
			default:
				kind = SteerOncoming
			}
			switch {
			case s.sameWayMover(coll, other, pool.Handle(ahead.occ)) && makingWay(other):
				// A friend on a route the same way is a queue: only a leader
				// making way markedly slower than this unit can go is
				// overtaken, and under NoOvertake none is.
				if tr.NoOvertake || int64(other.Speed)*100 >= int64(coll.MaxVelocity)*steerOvertake {
					return desired, false
				}
			case !makingWay(other):
				// Parked, or held while crossing or coming the other way:
				// something to go round, on the side its centre is not.
				if !sticky {
					prefer = sideAwayFrom(coll, other, desired)
				}
			case apart < alliedPassMinTurn:
				if !tr.PassBehind {
					return desired, false // crossing: it will have gone
				}
				// Pass behind it: toward the side it comes from. A heading
				// greater than ours, within a half turn, points to our
				// left, so the unit travels leftward and came from the
				// right.
				if !sticky {
					if int16(other.Heading-coll.Heading) > 0 {
						prefer = -1
					} else {
						prefer = 1
					}
				}
			default:
				if !sticky {
					prefer = -1 // oncoming: both keep right
				}
			}
		}
	}
	best, bestFree, bestSide := desired, ahead.free, int8(0)
	found := false
	try := func(side int8, i int32) {
		h := desired + uint16(int32(side)*i*steerStep)
		p := s.probe(coll, profile, h, look)
		if p.free >= look {
			best, bestFree, bestSide, found = h, p.free, side, true
		} else if p.free > bestFree {
			best, bestFree, bestSide = h, p.free, side
		}
	}
	if sticky {
		// A unit that has begun to pass on one side keeps to it while any
		// heading on that side is free: what it passes is nearer now, and
		// the smaller turn is often the one back across its front.
		for i := int32(1); i <= steerSteps && !found; i++ {
			try(prefer, i)
		}
		for i := int32(1); i <= steerSteps && !found; i++ {
			try(-prefer, i)
		}
	} else {
		for i := int32(1); i <= steerSteps && !found; i++ {
			try(prefer, i)
			if !found {
				try(-prefer, i)
			}
		}
	}
	brake := false
	if now := s.probe(coll, profile, coll.Heading, 1); now.free == 0 && !tr.NoBrake {
		// What is directly ahead is held: slow rather than run into it,
		// unless it is a leader to follow.
		brake = true
		if !now.static && now.occ >= 0 {
			if other, ok := s.friendlyMover(u, now.occ); ok && makingWay(other) && headingApart(coll.Heading, other.Heading) < jamReleaseQueueTurn {
				brake = false
			}
		}
	}
	if bestSide == 0 || !found && tr.ClearOnly {
		return desired, brake
	}
	if !found {
		kind = SteerBoxed
	}
	st.side, st.sideUntil, st.round, st.ahead = bestSide, tick+steerHold, best, kind
	setHandleRow(&s.traffic, u.Handle, st)
	return best, brake
}

// sideAwayFrom is the side to pass a unit at rest on: the one its centre is
// not on, seen along the wanted heading, and the right when it is dead
// ahead.
func sideAwayFrom(mine, other *CollisionState, heading uint16) int8 {
	sin := int64(numeric.Sin(numeric.Angle(heading)))
	cos := int64(numeric.Cos(numeric.Angle(heading)))
	dx, dz := int64(other.X-mine.X)>>8, int64(other.Z-mine.Z)>>8
	// Forward is (-sin, -cos) [04 R-MOV-01 §4]; headings grow to the left
	// [04 §5.1], so a positive cross product of the offset with forward
	// puts the unit on the left.
	if cross := dz*sin - dx*cos; cross < 0 {
		return 1
	}
	return -1
}

// passWaypoint takes the waypoint a sidestepping unit has gone past beside:
// the ordinary test wants the unit within five world units of it
// [04 R-MOV-01 §3], and a unit that went round something standing on the
// waypoint would turn back for it.
func (s *System) passWaypoint(u *units.Unit, route *Route, tick uint32) {
	if route == nil || !route.Active || route.Count < 3 {
		return
	}
	// Only a unit that is steering round something now: a side chosen long
	// ago says nothing about the waypoint ahead, and a unit that stands beside
	// the first point of a new route has not gone round anything. Taking
	// that point from it takes the turn the route makes round what holds it.
	st := handleRow(s.traffic, u.Handle)
	if st.side == 0 || !st.steering || tick >= st.sideUntil {
		return
	}
	// Nor from a unit whose last step was refused: it is passing nothing.
	// The point after the one it stands beside may lie back past it, as it
	// does when a route first leads away from what holds the unit and then
	// round it, and the unit would count as past a point it has yet to reach.
	if coll := handleRow(s.Collisions, u.Handle); coll == nil || coll.Blocked {
		return
	}
	x, z := int64(u.X)>>16, int64(u.Z)>>16
	p1, p2 := route.Points[1], route.Points[2]
	dx, dz := x-int64(p1.X), z-int64(p1.Z)
	if dx*dx+dz*dz > steerPassRadius*steerPassRadius {
		return
	}
	if dx*int64(p2.X-p1.X)+dz*int64(p2.Z-p1.Z) <= 0 {
		return
	}
	copy(route.Points[0:], route.Points[1:route.Count])
	route.Count--
	route.Dirty = true
}
