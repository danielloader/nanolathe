package movement

// Nanolathe Modern policy (docs/DESIGN_MOVEMENT_PATH.md "Modern traffic").
// Friendly units never share cells. A ground mover steers round what is ahead
// of it, its routes are pulled taut and searched with a weight that follows
// the load, the units of a group are given places to stand, and a unit that
// cannot reach its place settles where it may. Each piece changes what a
// unit wants or which route it is given; the commit validator is retail's
// and refuses a step onto a held cell as always [04 R-COLL-01 §2].
//
// None of the numbers below is a retail constant. Each was chosen on
// recorded games (docs/PATHFINDING_LAB.md) and is Nanolathe Modern policy
// tuning.
const (
	// modernSearchWeight is the heuristic weight of a route search while
	// route searching is idle, one and a half in 16.16, and
	// modernBusyWeight the weight while it is busy: the searches of the last
	// ticks were charged more than modernBusyWork units of work a tick
	// ("Modern search weight").
	modernSearchWeight = 0x18000
	modernBusyWeight   = 0x30000
	modernBusyWork     = 3000
	// modernThroughAfter is how long, in ticks, a unit's searches must have
	// found nothing before one is searched again through its friends
	// ("Modern routes through friends").
	modernThroughAfter = 150
	// modernClaimAlong and modernClaimAgainst are what a friend's claim adds
	// to a search step that goes its way and to one that goes against it, in
	// the search's cost units ("Modern route claims").
	modernClaimAlong   = 4
	modernClaimAgainst = 24
	// modernSettleAfter is how long, in ticks, a unit near its place makes no
	// way toward it before it settles where it stands, and modernSealedAfter
	// how long one stands short of a goal the ground closes off before it
	// settles there ("Modern arrival places").
	modernSettleAfter = 20
	modernSealedAfter = 150
)

// modernTraffic is Modern's answer, built once: the pilots are values
// without state, and what they remember lives on the System.
var modernTraffic = Traffic{
	// "Modern steering".
	Sidestep:   true,
	PassBehind: true,
	NoBrake:    true,
	ClearOnly:  true,
	NoOvertake: true,
	ToWaypoint: true,
	KeepRound:  true,
	// "Modern route smoothing": a straight leg may cross a friend on its
	// own way.
	LegsThroughMovers: true,
	// "Modern search weight".
	HeuristicScale: modernSearchWeight,
	BusyScale:      modernBusyWeight,
	BusyWork:       modernBusyWork,
	// "Modern routes through friends": friends with somewhere to go first,
	// then every friend.
	Through:      2,
	ThroughAfter: modernThroughAfter,
	// "Modern route claims" and "Modern arrival places".
	Pilot: Pilots{
		ClaimsPilot{Rank: true, Per: modernClaimAlong, Oncoming: modernClaimAgainst},
		ArrivePilot{Places: true, Exchange: true, Stuck: modernSettleAfter, StuckParked: true, Sealed: modernSealedAfter},
	},
}

// Traffic is Modern's traffic policy
// (docs/DESIGN_MOVEMENT_PATH.md "Modern traffic").
func (*ModernRules) Traffic(*System) Traffic { return modernTraffic }
