package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

// A unit with a mover runs only its front record [04 §3.3][04 R-ORD-01 §0]. A
// construction vehicle that is still parking off its factory pad, being carried
// or waiting to be finished holds a Shift-queued build behind that record
// [04 R-FAC-02 §4]. The play-test report this locks: builds queued on a
// vehicle the moment it left the plant stamped their nanoframe from the pad,
// with no approach, and built it from there because placement and work have no
// range test [05 R-WORK-01 §12].
func TestQueuedBuildWaitsBehindAMobileBuildersStandingHead(t *testing.T) {
	for _, row := range []string{"Park", "BeCarried", GetBuiltOrder} {
		t.Run(row, func(t *testing.T) {
			// The site is far outside the builder's reach, so any placement
			// here would be a placement at range.
			svc, builder, build := approachFixture(t, 24, 24)
			svc.Economy = &economy.Service{}
			build.Phase, build.DynamicGate, build.Deadline = uint8(State0), 0, -1
			q := orders.QueueForUnit(builder)
			svc.RegisterOrderHandlers(q)
			if row == GetBuiltOrder {
				builder.Remaining = 0.5 // GetBuilt holds the head only while its unit is unfinished
			}
			standing := q.PushHead(orders.Lookup(row), orders.Node{Owner: builder.Handle, Phase: 1, DynamicGate: 0xE0, Deadline: -1})
			if standing == nil {
				t.Fatalf("%s descriptor missing", row)
			}
			for tick := uint32(1); tick <= 60; tick++ {
				svc.StepUnit(TickContext{Tick: tick, World: svc.World, Economy: svc.Economy}, builder.Handle)
			}
			if q.Head() != standing {
				t.Fatalf("the %s head was replaced by %s", row, orders.DescriptorFor(q.Head().ID).Name)
			}
			if build.Phase != uint8(State0) || build.Target != 0 || svc.Movement.HasGroundGoal(builder.Handle, build) {
				t.Fatalf("the build behind %s ran: phase %d, product %d, owns the mover's goal %v", row, build.Phase, build.Target, svc.Movement.HasGroundGoal(builder.Handle, build))
			}
			// Once the standing record is gone the build is the head and starts
			// its approach on the next visit, walking rather than placing.
			q.RemovePrimaryNode(standing, false)
			builder.Remaining = 0
			svc.StepUnit(TickContext{Tick: 61, World: svc.World, Economy: svc.Economy}, builder.Handle)
			svc.StepUnit(TickContext{Tick: 62, World: svc.World, Economy: svc.Economy}, builder.Handle)
			if build.Phase != uint8(State1) || build.Target != 0 || !svc.Movement.HasGroundGoal(builder.Handle, build) {
				t.Fatalf("the build at the head did not start its approach: phase %d, product %d, owns the mover's goal %v", build.Phase, build.Target, svc.Movement.HasGroundGoal(builder.Handle, build))
			}
		})
	}
}
