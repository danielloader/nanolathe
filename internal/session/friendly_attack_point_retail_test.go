package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// A friendly hovered target resolves to the same point attack as bare ground
// [04 R-ORD-02 §1][04 R-ORD-01 §3]. At this legal stock placement, replacing
// the click with the solar's origin makes the terrain impact miss its damage
// box. Preserve the producer's point [04 R-ORD-01 §13], including in area orders.
func TestRetailFriendlyAttackMatchesGroundPointDamage(t *testing.T) {
	f := loadRetailFixture(t)
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		t.Run(string(mode), func(t *testing.T) {
			var groundDamage int32
			for _, producer := range []string{"ground", "single", "batch"} {
				t.Run(producer, func(t *testing.T) {
					s := f.session(t)
					s.SetGameplay(mode)
					stepRetail(s, 2)
					for _, ai := range s.AI {
						if ai != nil {
							for i := range ai.Deadlines {
								ai.Deadlines[i] = ^uint32(0)
							}
						}
					}
					for _, u := range s.Units.IterSliced() {
						if u != nil && u.Alive {
							for i := 0; i < combat.NumSlots; i++ {
								u.SlotAt(i).Flags &^= units.SlotFlagEnabled
							}
						}
					}
					tower := placeCompleteRetailUnit(t, s, "CORLLT", 0, numeric.FixedFromInt(600), numeric.FixedFromInt(600))
					solar := placeCompleteRetailUnit(t, s, "CORSOLAR", 0, numeric.FixedFromInt(672), numeric.FixedFromInt(608))
					point := orders.ResolvePos{X: solar.X - 16<<16, Y: solar.Y, Z: solar.Z + 16<<16}
					command := HumanOrderCommand{Handles: []pool.Handle{tower.Handle}, Code: 3, Position: point}
					switch producer {
					case "single":
						command.Target = solar.Handle
					case "batch":
						command.Targets = []HumanOrderTarget{{Target: solar.Handle, Position: point}}
					}
					if err := s.EnqueueHumanCommand(HumanCommand{Kind: HumanOrder, Order: command}); err != nil {
						t.Fatal(err)
					}
					before := solar.Health
					for i := 0; i < 180; i++ {
						s.Econ.Players[0].Stock[economy.Energy] = 10000
						s.Step(s.Clock.ScaledAnchor + 1)
					}
					aim := tower.SlotAt(0).Target
					if aim.X != point.X || aim.Z != point.Z {
						t.Errorf("weapon aim = (%d,%d), want clicked point (%d,%d)", aim.X, aim.Z, point.X, point.Z)
					}
					damage := before - solar.Health
					if damage <= 0 {
						t.Fatal("friendly solar took no damage")
					}
					if producer == "ground" {
						groundDamage = damage
					} else if damage != groundDamage {
						t.Errorf("damage = %d, want ground-point control's %d", damage, groundDamage)
					}
				})
			}
		})
	}
}
