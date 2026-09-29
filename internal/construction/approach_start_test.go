package construction

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

// TestBuildWalkStartsWithoutWaitingForTheSearch locks the goal installer's
// acceptance rule on the mobile-build approach [04 R-PATH-01 §8]
// [04 R-PATH-01 §13]. Installing the rectangle goal hands the builder the
// synthetic straight line at the rectangle's goal point, so it starts walking
// while the asynchronous search runs, and zeroes a last-request tick at least
// ten ticks old, so a builder that polled recently is not held back by the
// 60-tick repath throttle.
//
// The approach used to skip the installer. A commander given a build order a
// second after its last path request stood still for the rest of the throttle
// before its first step: the reported hesitation.
func TestBuildWalkStartsWithoutWaitingForTheSearch(t *testing.T) {
	svc, builder, node := approachFixture(t, 10, 10)
	const issued = uint32(1000)
	route := svc.Movement.Routes[builder.Handle]
	if route == nil {
		t.Fatal("fixture builder has no route follower")
	}
	// The builder's last admitted path request was 30 ticks ago: past the
	// installer's ten-tick window, inside the 60-tick throttle.
	route.LastRequestTick = issued - 30

	firstMove, admitted := int64(-1), int64(-1)
	for tick := issued; tick < issued+60; tick++ {
		pumpApproach(svc, builder, tick)
		if tick == issued {
			if !route.Active || route.Count != 2 {
				t.Fatalf("installing the approach goal left the route active=%v count=%d, want the synthetic two-point line", route.Active, route.Count)
			}
			if route.LastRequestTick != 0 {
				t.Fatalf("installing the approach goal kept last-request tick %d, want it zeroed once at least ten ticks old", route.LastRequestTick)
			}
		}
		beforeX, beforeZ := builder.X, builder.Z
		svc.Movement.BeginTick(tick)
		svc.Movement.StepUnit(builder.Handle, tick)
		svc.Movement.EndTick(tick)
		if firstMove < 0 && (builder.X != beforeX || builder.Z != beforeZ) {
			firstMove = int64(tick)
		}
		svc.Movement.Scheduler.Tick(tick)
		if admitted < 0 && route.LastRequestTick >= issued {
			admitted = int64(tick)
		}
		if firstMove >= 0 && admitted >= 0 {
			break
		}
	}
	if firstMove != int64(issued) {
		t.Fatalf("builder first moved at tick %d, want %d: the order's own tick", firstMove, issued)
	}
	if admitted != int64(issued) {
		t.Fatalf("approach path request admitted at tick %d, want %d", admitted, issued)
	}
	if node.Phase != uint8(State1) || node.MoveState != orders.MoveEnRoute {
		t.Fatalf("approach state=(phase %d, move %d), want State1/en-route", node.Phase, node.MoveState)
	}
}
