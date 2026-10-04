//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// An in-range cannot-get-there wake may start construction while retaining
// the approach goal. Placement, script readiness and work must preserve the
// inactive route, rather than installing another synthetic straight route
// [04 R-ORD-01 §5][05 R-WORK-01 §14].
func TestMobileBuildFailurePreservesInactiveApproachRetail(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		t.Run(string(mode), func(t *testing.T) {
			s := aiE2ESkirmishAtMode(t, "ashap plateau", aiE2ESeed, SkirmishDefaultDifficulty, mode)
			// The ordinary unit phase binds this lookup before any handler's
			// synchronous goal handoff. This fixture invokes that handoff directly.
			s.Movement.BindWorld(s.Units)
			var commander *units.Unit
			for _, u := range s.Units.IterSliced() {
				if u.Def.Commander && u.Owner == uint8(s.LocalOwner) {
					commander = u
					break
				}
			}
			if commander == nil {
				t.Fatal("no local commander")
			}
			def, ok := s.Catalog.Unit("armcv")
			if !ok {
				t.Skip("armcv is absent from the reference install")
			}
			h, err := s.Units.Create(def, commander.Owner, commander.X+world.CellToWorld(6), commander.Y, commander.Z)
			if err != nil {
				t.Fatal(err)
			}
			s.CompleteUnit(h)
			builder := s.Units.Unit(h)
			if builder.COBBinding() == nil {
				t.Fatal("builder has no production script binding")
			}
			if err := construction.QueueMobileBuild(builder, "armsolar", builder.X, builder.Z+world.CellToWorld(7), 1, s.Catalog); err != nil {
				t.Fatal(err)
			}
			node := orders.QueueOfUnit(builder).Head()
			node.Phase, node.DynamicGate, node.Deadline = uint8(construction.State1), orders.ApproachWakeGate, -1
			s.Build.EnsureWalkPublic(builder, node)
			if !s.Movement.HasGroundGoal(h, node) || s.Build.OutOfReachPublic(builder, node) {
				t.Fatal("fixture must own an unsatisfied approach goal within fallback reach")
			}
			// Reproduce the unsuccessful publisher's result without depending
			// on a particular terrain obstruction or search budget. Its empty
			// publication preserves stale route bytes [04 §7.3] C14.
			s.Movement.CancelPathRequest(h)
			route := s.Movement.Routes[h]
			if !route.Active || route.Count != 2 {
				t.Fatal("fixture must have two active points before the failed publication")
			}
			route.Publish(nil)
			points, count := route.Points, route.Count
			node.Satisfied |= 0x40
			step := func() {
				p := &s.Econ.Players[builder.Owner]
				p.Capacity, p.Stock = [2]float32{100000, 100000}, [2]float32{100000, 100000}
				s.Step(s.Clock.ScaledAnchor + 1)
			}
			for tick := 0; tick < 5 && node.Target == 0; tick++ {
				step()
			}
			if node.Phase != uint8(construction.State2) || node.Target == 0 {
				t.Fatalf("in-range failure did not bind a frame for readiness: phase=%d target=%d", node.Phase, node.Target)
			}
			// Allocation and the product's lifecycle are committed before the
			// stock script finishes its readiness path [04 R-ORD-01 §5].
			target := node.Target
			product := s.Units.Unit(target)
			wantGate := construction.InterruptCancel | construction.InterruptStop | units.PendingScriptTouched
			if product == nil || !product.Alive || product.Owner != builder.Owner || product.Health != 0 || product.Remaining != 1 || builder.InBuildStance || node.DynamicGate != wantGate || node.Deadline != -1 {
				t.Fatal("bound frame did not enter the stock script readiness wait")
			}
			pq := orders.QueueOfUnit(product)
			if pq == nil || pq.Head() == nil || pq.Head().ID != orders.Lookup("GetBuilt") || pq.Head().Owner != target || pq.Head().Target != h {
				t.Fatal("placement did not commit the product's GetBuilt relationship")
			}
			if owner, ok := s.Build.BuilderLink(target); !ok || owner != h {
				t.Fatal("placement did not commit the builder-product link")
			}
			remaining := product.Remaining
			// Cross both modes' ordinary follower poll boundaries. Inactivity
			// alone must not re-arm a route with two stored points and no
			// blockage while the script becomes ready and work begins
			// [04 R-MOV-03 §2].
			for tick := 0; tick < 90; tick++ {
				step()
				q := orders.QueueOfUnit(builder)
				if q == nil || q.Head() != node || node.Target != target || s.Units.Unit(target) != product {
					t.Fatal("readiness or work replaced the build record or its product")
				}
				if !builder.InBuildStance && (product.Health != 0 || product.Remaining != remaining) {
					t.Fatal("construction advanced before the stock script became ready")
				}
				if !s.Movement.HasGroundGoal(h, node) {
					t.Fatal("readiness or work discarded the retained retail approach goal")
				}
				if route.Active || route.Points != points || route.Count != count || s.Movement.HasPathRequest(h) {
					t.Fatalf("readiness or work restarted the inactive, unblocked approach at follow-up tick %d", tick)
				}
			}
			if !builder.InBuildStance || node.Phase != uint8(construction.State3) || product.Health <= 0 || product.Remaining >= remaining {
				t.Fatal("preserving the inactive approach stopped construction work")
			}
		})
	}
}
