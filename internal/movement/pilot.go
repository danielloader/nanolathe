package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Pilot is a part of the traffic policy that takes part in a ground mover's
// visit, in route searches and in the giving of orders, named in
// Traffic.Pilot. Modern names two, route claims and arrival places
// (docs/DESIGN_MOVEMENT_PATH.md "Modern traffic"). Strict 3.1 and Community
// 3.9 name none and never call one.
//
// A pilot never moves a unit and never enters a held cell: the commit
// validator runs after it, unchanged, and refuses a step onto a held cell as
// always [04 R-COLL-01 §2]. What it may change is what a route search costs
// and where an order sends its unit.
//
// A pilot is a value without state, like every rule object
// (docs/DESIGN_GAMEPLAY_RULES.md §3): one is built for the process and serves
// every session. What it remembers lives on the System, in PilotState. Every
// method runs on the simulation's own goroutine, in the sweep's order, and
// must use integer arithmetic, iterate no map and draw from neither random
// stream [I1][I2][I4].
type Pilot interface {
	// BeginTick runs once a tick, before any unit is visited.
	BeginTick(s *System, tick uint32)
	// Visit runs once per visit of a grounded mover that has an order, after
	// the follower's service and the steering, before the unit's heading and
	// speed are integrated.
	Visit(s *System, v *Visit)
	// GroupOrdered runs at the command boundary when one command gave
	// several units a ground move: members in pool order, and the point the
	// command named.
	GroupOrdered(s *System, owner uint8, members []pool.Handle, x, z numeric.Fixed, tick uint32)
	// Search runs once per opened search, after the request's passability
	// view is composed, and may rewrite the configuration.
	Search(s *System, r path.Request, cfg *path.SearchConfig)
	// Forget runs when a unit's movement state is dropped.
	Forget(s *System, h pool.Handle)
}

// Visit is one mover's visit as a pilot sees it: the unit, its collision
// record as its last commit left it, its route record, nil for none, and its
// order.
type Visit struct {
	Unit    *units.Unit
	Coll    *CollisionState
	Profile Profile
	Route   *Route
	Head    *orders.Node
	Tick    uint32
}

// NoPilot does nothing. A pilot embeds it and writes the methods it needs.
type NoPilot struct{}

func (NoPilot) BeginTick(*System, uint32) {}
func (NoPilot) Visit(*System, *Visit)     {}
func (NoPilot) GroupOrdered(*System, uint8, []pool.Handle, numeric.Fixed, numeric.Fixed, uint32) {
}
func (NoPilot) Search(*System, path.Request, *path.SearchConfig) {}
func (NoPilot) Forget(*System, pool.Handle)                      {}

// NoteGroupOrder is the command boundary's report of one command that gave
// several units a ground move.
func (s *System) NoteGroupOrder(owner uint8, members []pool.Handle, x, z numeric.Fixed, tick uint32) {
	if s == nil || len(members) < 2 {
		return
	}
	if p := s.rules().Traffic(s).Pilot; p != nil {
		p.GroupOrdered(s, owner, members, x, z, tick)
	}
}
