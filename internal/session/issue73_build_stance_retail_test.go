//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

type issue73NanoObserver struct {
	forward        interface{ EmitNanolathe(frame.Event) bool }
	builder        *units.Unit
	unready, ready int
}

func (o *issue73NanoObserver) EmitNanolathe(e frame.Event) bool {
	if e.Source == o.builder.Handle {
		if o.builder.InBuildStance {
			o.ready++
		} else {
			o.unready++
		}
	}
	return o.forward.EmitNanolathe(e)
}

// Stock ground constructors delay work until their authored readiness write;
// ARMCA retains retail's aircraft exception [04 R-ORD-01 §5][04 R-ORD-02 §2].
// The observer forwards every event through the ordinary particle/RNG path.
func TestIssue73StockBuildReadinessRetail(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern} {
		for _, key := range []string{"armck", "armack", "armcom", "armca"} {
			t.Run(string(mode)+"/"+key, func(t *testing.T) {
				s := aiE2ESkirmishAtMode(t, "ashap plateau", aiE2ESeed, SkirmishDefaultDifficulty, mode)
				driver := int32(1 << 20)
				step := func() { s.Step(driver); driver++ }
				for i := 0; i < 60 && s.State != StateBattle; i++ {
					step()
				}
				var commander *units.Unit
				for _, u := range s.Units.IterSliced() {
					if u != nil && u.Def != nil && u.Def.Commander && u.Owner == uint8(s.LocalOwner) {
						commander = u
						break
					}
				}
				if commander == nil {
					t.Fatal("no local commander")
				}
				def, ok := s.Catalog.Unit(key)
				if !ok {
					t.Skipf("%s absent from reference install", key)
				}
				builder := commander
				if key != "armcom" {
					x, z := commander.X-world.CellToWorld(4), commander.Z+world.CellToWorld(4)
					h, err := s.Units.Create(def, uint8(s.LocalOwner), x, s.World.HeightAt(x, z), z)
					if err != nil {
						t.Fatal(err)
					}
					s.CompleteUnit(h)
					builder = s.Units.Unit(h)
				}
				if builder.COBBinding() == nil {
					t.Fatal("builder has no production script binding")
				}
				observer := &issue73NanoObserver{forward: s.Build.Presentation, builder: builder}
				s.Build.Presentation = observer
				x, z := commander.X-world.CellToWorld(12), commander.Z+world.CellToWorld(18)
				if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanMobileBuild, MobileBuild: HumanMobileBuildCommand{
					Builder: builder.Handle, Product: "armsolar", WX: x, WY: s.World.HeightAt(x, z), WZ: z,
				}}); err != nil {
					t.Fatal(err)
				}
				var product *units.Unit
				var placedTick, readyTick, workTick uint32
				for i := 0; i < 1600; i++ {
					p := &s.Econ.Players[s.LocalOwner]
					p.Capacity, p.Stock = [2]float32{100000, 100000}, [2]float32{100000, 100000}
					step()
					if product == nil {
						for _, u := range s.Units.IterSliced() {
							if u != nil && u.Def != nil && u.Def.UnitName == "ARMSOLAR" && u.Owner == builder.Owner {
								product, placedTick = u, s.Clock.GlobalTick
								break
							}
						}
					}
					if product == nil {
						continue
					}
					if builder.InBuildStance && readyTick == 0 {
						readyTick = s.Clock.GlobalTick
					}
					if product.Health > 0 && workTick == 0 {
						workTick = s.Clock.GlobalTick
					}
					if !def.CanFly && !builder.InBuildStance && (product.Health != 0 || product.Remaining != 1 || observer.unready != 0) {
						t.Fatalf("early ground work: placed=%d tick=%d health=%d remaining=%g unready spray=%d", placedTick, s.Clock.GlobalTick, product.Health, product.Remaining, observer.unready)
					}
					if workTick != 0 && readyTick != 0 {
						if !def.CanFly && (workTick < readyTick || observer.ready == 0) {
							t.Fatal("ground HP and spray did not follow readiness together")
						}
						if def.CanFly && observer.unready == 0 {
							t.Fatal("stock aircraft did not exercise its pre-readiness work exception")
						}
						t.Logf("placement=%d readiness=%d first HP=%d unready spray=%d ready spray=%d product=%d", placedTick, readyTick, workTick, observer.unready, observer.ready, product.Handle)
						return
					}
				}
				t.Fatalf("no completed readiness/work trace: placed=%d ready=%d work=%d messages=%v", placedTick, readyTick, workTick, s.Build.Messages())
			})
		}
	}
}
