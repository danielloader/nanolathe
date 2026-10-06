package combat

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// A death latch leaves allocation and weapon-slot state in place until the
// visit's finalization. The weapon consumer must still resolve its retained
// target and fire before that point [04 R-COB-02 §2][06 R-WPN-04 §1].
func TestAllocatedDeathLatchPreservesWeaponVisit(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern} {
		t.Run(string(mode), func(t *testing.T) {
			for _, tc := range []struct {
				name                  string
				shooter, target, high bool
			}{
				{name: "ordinary"},
				{name: "shooter latched", shooter: true},
				{name: "retained target latched", target: true},
				{name: "both latched", shooter: true, target: true},
				{name: "authored high-bit maximum", high: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					w, terrain, shooter, target := newTestWorldAndUnits(t)
					attachTestCOB(shooter, cob.NewVM(&cob.Program{
						Code:        []uint32{0x10021001, 0, 0x10065000},
						Scripts:     map[string]int{"QueryPrimary": 0, "FirePrimary": 0, "RockUnit": 0, "TargetCleared": 0},
						ScriptsByID: []int{0}, Pieces: []string{"base"},
					}))
					targetProgram := progWithAim([]uint32{0x10021001, 1, 0x10023004, 0, 0x10021001, 0, 0x10065000}, "SweetSpot", 0)
					targetProgram.Statics = 1
					attachTestCOB(target, cob.NewVM(targetProgram))
					weapon := turretSpreadWeapon(907, 32)
					weapon.ReloadTime = 100
					weapon.EnergyPerShot, weapon.MetalPerShot = 7, 11
					slot := armedTurretSlot(t, shooter, target, weapon)
					slot.Reload = 1 // decrement must precede the admitted shot
					if tc.shooter {
						shooter.Health = 0
						units.MarkDeath(shooter, units.DeathKilled, target.Handle)
					}
					if tc.target {
						target.Health = 0
						units.MarkDeath(target, units.DeathKilled, shooter.Handle)
					}
					if tc.high {
						// Seed the established completed-unit consumer state. Unit
						// initialization belongs to its own package [06 §4.2].
						shooter.Health, shooter.MaxHealth = 100, -2147483548
						weapon.Accuracy = 0
					}
					econ := &economy.Service{}
					econ.Players[shooter.Owner].Stock[economy.Energy] = 100
					econ.Players[shooter.Owner].Stock[economy.Metal] = 100
					var callbacks []string
					sink := func(e cob.LifecycleEvent) {
						if e.Phase != "start" {
							return
						}
						switch e.Name {
						case "FirePrimary", "RockUnit", "TargetCleared":
							callbacks = append(callbacks, e.Name)
							if target.GetScript().DebugSnapshot().Statics[0] != 1 {
								t.Error("firing callback started before synchronous SweetSpot")
							}
							if econ.Players[shooter.Owner].Stock[economy.Energy] != 100 {
								t.Error("callback start followed resource debit")
							}
						}
					}
					shooter.COBBinding().Callbacks.SetLifecycleSink(sink)
					random := rng.NewSimulation(77)
					svc := Service{Rules: rulesForModern(mode == gameplay.Modern)}
					sum := svc.StepWeaponsForUnit(shooter, 1, w, nil, terrain, econ, nil, &random, nil)
					if sum.Fired != 1 || svc.Count() != 1 || random.Draws() != 2 {
						t.Fatalf("weapon visit: fired=%d projectiles=%d draws=%d", sum.Fired, svc.Count(), random.Draws())
					}
					if want := []string{"FirePrimary", "RockUnit"}; !reflect.DeepEqual(callbacks, want) {
						t.Fatalf("callbacks=%v, want %v", callbacks, want)
					}
					wantReload := int32(100)
					if tc.shooter || tc.high {
						wantReload = 120
					}
					if slot.Reload != wantReload || slot.Target.Unit != target.Handle || shooter.Pending&0x400 == 0 {
						t.Fatalf("shot state: reload=%d target=%+v events=%#x", slot.Reload, slot.Target, shooter.Pending)
					}
					if econ.Players[shooter.Owner].Stock[economy.Energy] != 93 || econ.Players[shooter.Owner].Stock[economy.Metal] != 89 {
						t.Fatalf("shot debit=%v", econ.Players[shooter.Owner].Stock)
					}
					if !shooter.Alive || !target.Alive || shooter.Dying != tc.shooter || target.Dying != tc.target {
						t.Fatal("weapon consumer changed the pending finalization state")
					}
				})
			}
		})
	}
}
