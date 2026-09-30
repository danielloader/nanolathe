package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Traffic is the answer to Rules.Traffic: how a ground mover behaves toward
// the units around it while every unit keeps its own cells. The zero value
// is retail's behaviour — a refused step halves the speed, clamps the unit
// against its old footprint and arms a repath, and nothing steers around,
// follows or gives way [04 R-COLL-01 §1][04 R-COLL-01 §7] — and is what
// Strict 3.1 and Community 3.9 answer.
//
// Modern answers modernTraffic (modern_traffic.go;
// docs/DESIGN_MOVEMENT_PATH.md "Modern traffic"). The pathfinding
// laboratory's sets answer other values of the same switches and numbers
// (docs/PATHFINDING_LAB.md).
type Traffic struct {
	// Sidestep lets a mover want a heading past what stands ahead of it
	// (traffic_steer.go): Modern steering. The five switches after it are
	// its rules, each of which the laboratory measured with and without.
	Sidestep bool
	// PassBehind steers a mover behind a friend crossing its way instead of
	// leaving it to have gone by.
	PassBehind bool
	// NoBrake leaves the speed alone when what is directly ahead is held;
	// NoOvertake follows every same-way leader, however slow.
	NoBrake    bool
	NoOvertake bool
	// ClearOnly makes a mover with no way round keep the heading it wants:
	// without it a mover none of whose headings is free for the whole look
	// ahead turns to the one with the most room, and the one with the most
	// room changes from tick to tick in a crowd.
	ClearOnly bool
	// ToWaypoint makes the sidestep look no farther ahead than the point of
	// its route the mover is making for: what lies beyond it lies off the
	// route, which turns there. Without it a mover whose route rounds a
	// corner turns away from the wall beyond the corner before it has
	// reached the point, comes round, and turns away again.
	ToWaypoint bool
	// KeepRound makes a mover whose last step was refused keep the way round
	// it chose, while that way is free, on the visits its own way looks free
	// too. A unit that stands against something turns where it stands, and
	// its probes start from a position that creeps: the way it wants looks
	// free on one visit and held on the next. When that way and the way
	// round lie either side of the direction behind it, the unit turns a few
	// degrees towards one, then towards the other, and reaches neither.
	KeepRound bool
	// Through, when positive, gives a request whose search found nothing
	// another search that reads friendly units as absent: at one, the friends
	// that have somewhere to go; at two, when that finds nothing either,
	// every friend. The unit then follows its route as far as the units on
	// it let it (traffic_through.go). ThroughAfter is how long, in ticks, the
	// unit's own searches must have found nothing before one is searched
	// again: friends setting off leave a unit among them without a route
	// for a moment, and that mends itself.
	Through      uint8
	ThroughAfter uint32
	// LegsThroughMovers lets the kernel's pass over a finished route read
	// friendly movers as absent: a straight leg may cross ground a friend
	// on its own way stands on, which the search's route may not.
	LegsThroughMovers bool
	// Pilot, when set, takes part in every ground mover's visit and every
	// route search (pilot.go): Modern's route claims and arrival places.
	Pilot Pilot
	// HeuristicScale, when positive, is the heuristic weight of every route
	// search, as 16.16: retail weighs the heuristic nine, four and a half
	// or one and a half times by its player's load
	// [04 R-PATH-01 §6][04 R-PATH-01 §10], and the heavier the weight the
	// greedier the route.
	HeuristicScale int32
	// BusyScale, when positive, is the heuristic weight in place of
	// HeuristicScale while route searching is busy: the searches of the last
	// ticks were charged more than BusyWork units of work a tick, smoothed
	// over eight ticks. A search looks at less ground when many are asking.
	BusyScale int32
	BusyWork  int32
}

// trafficState is one mover's traffic bookkeeping, dense by handle, never
// saved: a load starts every unit with the zero value, which is a unit that
// has steered round nothing yet.
type trafficState struct {
	// side is the side a sidestep chose, +1 left or -1 right, zero for
	// none, kept until sideUntil.
	side      int8
	sideUntil uint32
	// round is the heading the sidestep last chose.
	round uint16
	// steering marks a unit whose last visit steered, and ahead what it
	// steered round.
	steering bool
	ahead    uint8
	// through is the reading the unit's route was planned under, zero for
	// its search's own (traffic_through.go); routeless marks a unit whose
	// own searches have found nothing since tick routelessFrom.
	through       uint8
	routeless     bool
	routelessFrom uint32
	// hasGoal marks a unit whose present goal is goalX, goalZ.
	hasGoal      bool
	goalX, goalZ numeric.Fixed
}

// LabStats counts what the traffic policy did, for cost accounting that does
// not depend on the clock: visits that steered, probes walked and footprint
// anchors tested; and what route searches cost under any rule set, the
// searches finished and the work they were charged. The pathfinding
// laboratory's replays read it.
type LabStats struct {
	Visits, Probes, Anchors uint64
	Searches, Pops          uint64
	// SteerStarts counts the times a mover began to steer round something,
	// and SteerTicks the visits it spent steering, by what was ahead
	// (SteerStatic and the rest).
	SteerStarts, SteerTicks [SteerKinds]uint64
	// Refused counts refused ground steps by what refused them
	// (RefusedStatic and the rest).
	Refused [RefusedKinds]uint64
	// RefusedNear is the part of Refused that happened within 128 world
	// units of the goal of the mover's order.
	RefusedNear [RefusedKinds]uint64
}

// What refused a step. A friend going the same way and making way is told
// apart further: slower by its kind, below four fifths of its own top speed
// (turning or speeding up), off to one side of the mover's heading (the two
// are merging), or none of these.
const (
	RefusedStatic = iota
	RefusedStructure
	RefusedOther
	RefusedParked
	RefusedHeld
	RefusedCrossing
	RefusedOncoming
	RefusedSlowerKind
	RefusedSlowed
	RefusedMerging
	RefusedLeader
	RefusedKinds
)

// noteRefusedKind counts one refused ground step for the laboratory.
func (s *System) noteRefusedKind(u *units.Unit, coll *CollisionState, static bool, blockerID int) {
	kind := RefusedOther
	switch {
	case static:
		kind = RefusedStatic
	case blockerID < 0:
	default:
		if c := handleRow(s.Collisions, pool.Handle(blockerID)); c != nil && c.Building {
			kind = RefusedStructure
			break
		}
		other, ok := s.friendlyMover(u, blockerID)
		if !ok {
			break
		}
		apart := headingApart(coll.Heading, other.Heading)
		switch {
		case !makingWay(other) && other.Speed > 0:
			kind = RefusedHeld
		case !makingWay(other):
			kind = RefusedParked
		case s.sameWayMover(coll, other, pool.Handle(blockerID)):
			// Where the friend stands against the mover's heading.
			bearing := HeadingFromDelta(int64(other.X)-int64(coll.X), int64(other.Z)-int64(coll.Z))
			switch {
			case other.MaxVelocity*10 < coll.MaxVelocity*9:
				kind = RefusedSlowerKind
			case int64(other.Speed)*5 < int64(other.MaxVelocity)*4:
				kind = RefusedSlowed
			case headingApart(bearing, coll.Heading) > 0x1555:
				kind = RefusedMerging
			default:
				kind = RefusedLeader
			}
		case apart < alliedPassMinTurn:
			kind = RefusedCrossing
		default:
			kind = RefusedOncoming
		}
	}
	s.labStats.Refused[kind]++
	if d2, ok := s.distSqToGoal(u); ok && d2 <= uint64(128<<16)*uint64(128<<16) {
		s.labStats.RefusedNear[kind]++
	}
}

// What a sidestep steers round. SteerBoxed is a mover with no way round that
// turned to the heading with the most room.
const (
	SteerNone = iota
	SteerStatic
	SteerStructure
	SteerParked
	SteerHeld
	SteerSlower
	SteerCrossing
	SteerOncoming
	SteerOther
	SteerBoxed
	SteerKinds
)

// CountRefusals makes the system count refused steps by what refused them,
// under any rule set. The laboratory's replay asks for it.
func (s *System) CountRefusals() { s.countRefusals = true }

// LabStats returns the counts since the system was made.
func (s *System) LabStats() LabStats { return s.labStats }

// Traffic is retail's: none.
func (StrictRules) Traffic(*System) Traffic { return Traffic{} }

// makingWay reports whether the mover behind coll moved freely on its last
// cell crossing and still has speed: a leader a follower may simply follow.
func makingWay(other *CollisionState) bool {
	return other != nil && !other.Blocked && other.Speed > 0
}

func (s *System) clearTraffic(h pool.Handle) {
	if s == nil {
		return
	}
	if int(h) < len(s.traffic) {
		s.traffic[h] = trafficState{}
	}
	if p := s.trafficNow.Pilot; p != nil {
		p.Forget(s, h)
	}
}
