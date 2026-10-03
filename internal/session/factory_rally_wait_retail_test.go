//go:build retail

package session

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Rally markers wait 60 ticks before rotating and install no movement goal
// [04 R-ORD-01 §2]. Cancelling an active factory product exposed those markers
// to movement activation, which erased the wait and rotated one every tick.
func TestFactoryRallyWaitSurvivesProductionCancellationRetail(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, command := range []struct {
			name string
			code int
		}{{"QPatrol", 9}, {"QMove", 2}} {
			t.Run(string(mode)+"/"+command.name, func(t *testing.T) {
				s := aiE2ESkirmishAtMode(t, "ashap plateau", 7, SkirmishDefaultDifficulty, mode)
				driver := int32(1 << 20)
				step := func() { s.Step(driver); driver++ }
				for i := 0; i < 60 && s.State != StateBattle; i++ {
					step()
				}
				factory := placeCompleteRetailUnit(t, s, "armvp", s.LocalOwner, numeric.FixedFromInt(1000), numeric.FixedFromInt(1000))
				issue := func(c HumanCommand) {
					t.Helper()
					if err := s.EnqueueHumanCommand(c); err != nil {
						t.Fatal(err)
					}
				}
				// Selection is the client's local state (DESIGN_MULTIPLAYER
				// §7.3); the issue's deselect/reselect repro touches nothing the
				// session holds, so the loop below only keeps its timing.
				for i := 0; i < 3; i++ {
					issue(HumanCommand{Kind: HumanOrder, Order: HumanOrderCommand{
						Handles: []pool.Handle{factory.Handle}, Code: command.code, Queued: true,
						Position: orders.ResolvePos{X: numeric.FixedFromInt(int64(1300 + i*200)), Z: numeric.FixedFromInt(1200)},
					}})
					for j := 0; j < 10; j++ {
						step()
					}
				}
				for i := 0; i < 100; i++ {
					step()
				}
				connectors := func() [][2]hud.QueueWorldPoint {
					f := s.Snapshot.Current()
					var lines [][2]hud.QueueWorldPoint
					for _, op := range hud.QueueOverlay(f, hud.QueueOverlayOptions{
						Tick: f.Tick, ShiftHeld: true, LocalOwner: s.LocalOwner, PageUnit: factory.Handle,
						Project: func(x, y, z numeric.Fixed) hud.QueuePoint {
							return hud.QueuePoint{X: int32(x >> 16), Y: int32(z >> 16)}
						},
					}) {
						if op.Unit == factory.Handle && op.Kind == hud.QueuePrimitiveDash {
							lines = append(lines, [2]hud.QueueWorldPoint{op.WorldA, op.WorldB})
						}
					}
					return lines
				}
				before := connectors()
				if len(before) != 3 {
					t.Fatalf("rally connectors = %v, want three", before)
				}
				issue(HumanCommand{Kind: HumanFactoryBuild, FactoryBuild: HumanFactoryBuildCommand{Builder: factory.Handle, Product: "armflash", Count: 1}})
				q := orders.QueueForUnit(factory)
				var building *orders.Node
				for i := 0; i < 400; i++ {
					step()
					if n := q.Head(); n != nil && orders.DescriptorFor(n.ID).Name == "BuildingBuild" && n.Target != 0 && n.DynamicGate&2 != 0 {
						building = n
						break
					}
				}
				if building == nil {
					t.Fatal("factory did not begin an active product")
				}
				product := s.Units.Unit(building.Target)
				issue(HumanCommand{Kind: HumanFactoryBuild, FactoryBuild: HumanFactoryBuildCommand{Builder: factory.Handle, Product: "armflash", Count: -1}})
				step()
				if product == nil || !product.Dying || q.LenPrimary() != 3 {
					t.Fatal("production cancellation did not kill the nanoframe and retain the rally points")
				}
				// Several complete deadline cycles must leave the published geometry
				// stable, including deselection/reselection from the issue's repro.
				for i := 0; i < 180; i++ {
					step()
					if got := connectors(); !slices.Equal(got, before) {
						t.Fatalf("tick %d: rally connectors changed from %v to %v", s.Clock.GlobalTick, before, got)
					}
					for j, n := range q.Primary() {
						if orders.DescriptorFor(n.ID).Name != command.name || n.DynamicGate != 1 || n.Deadline <= int32(s.Clock.GlobalTick) || n.MoveState != 0 || s.Movement.HasGroundGoal(factory.Handle, n) {
							t.Fatalf("tick %d marker %d: gate=%#x deadline=%d movement=%d; rally wait was overwritten", s.Clock.GlobalTick, j, n.DynamicGate, n.Deadline, n.MoveState)
						}
					}
					if s.Movement.HasPathRequest(factory.Handle) {
						t.Fatal("factory rally submitted a movement path")
					}
				}
			})
		}
	}
}
