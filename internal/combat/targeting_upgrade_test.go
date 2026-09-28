package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

// Issue #36: a turret must acquire an enemy that is on radar but outside line
// of sight, inside weapon range, exactly while its owner has a complete,
// activated `istargetingupgrade` unit [04 R-SPEC-01 §8][06 §3.1]. The registry
// falls back to the secondary (radar-contact) list only when the visible scan
// finds nothing, and the gate is the owner's own active facility.
//
// Both rule sets must honor the gate. Modern replaces sampling and scoring,
// not the registry population it scores (docs/DESIGN_WEAPONS_PROJECTILES.md
// "Modern threat targeting and incoming fire"); its present-knowledge recheck
// must therefore accept a current radar contact from the secondary list, and
// still refuse one whose contact has since been lost.
func TestTargetingUpgradeAcquiresRadarOnlyContact(t *testing.T) {
	type facility int
	const (
		none facility = iota
		active
		deactivated
	)
	rulesets := []struct {
		name  string
		rules Rules
	}{
		{"strict", StrictRules{}},
		{"modern", &ModernRules{}},
	}
	for _, rs := range rulesets {
		for _, tc := range []struct {
			name     string
			facility facility
			lostSeen bool // radar contact lost after the rebuild filed it
			want     bool
		}{
			{"no facility", none, false, false},
			{"active facility", active, false, true},
			{"deactivated facility", deactivated, false, false},
			// Strict reads the rebuild's list up to thirty ticks stale with
			// no sensor re-test [06 §3.1]; Modern's knowledge recheck refuses
			// a contact the viewer no longer holds.
			{"active facility, contact lost", active, true, rs.name == "strict"},
		} {
			t.Run(rs.name+"/"+tc.name, func(t *testing.T) {
				f := newRegistryFixture(t, false) // not cloaked: merely out of LOS
				f.enemy.Def.DamageModifier = 65536
				if tc.facility != none {
					targ := &content.UnitDef{UnitName: "targ", MaxDamage: 100, Limit: -1, IsTargetingUpgrade: true, OnOffable: true}
					h, err := f.world.Create(targ, 0, numeric.FixedFromInt(12), numeric.FixedFromInt(100), numeric.FixedFromInt(12))
					if err != nil {
						t.Fatal(err)
					}
					f.world.Unit(h).Activated = tc.facility == active
				}
				wdef := &content.WeaponDef{ID: 99, Range: 1000, LineOfSight: true, WeaponVelocity: 16 << 16, DamageDefault: 60, ReloadTime: 30}
				f.shooter.InstallWeapon(0, wdef)
				f.shooter.SlotAt(0).Flags |= 0x02
				f.sensorTick(1)
				if f.enemy.Flags&visibility.SeenBit == 0 {
					t.Fatal("fixture is wrong: the radar pass must set the hostile's seen bit")
				}
				if directlyVisibleAtRebuild(f.shooter.Owner, f.enemy, f.vis) {
					t.Fatal("fixture is wrong: the hostile must be outside line of sight")
				}

				s := &Service{Rules: rs.rules}
				s.rebuildTargetRegistry(targetRegistryPeriod, f.shooter.Owner, f.world, f.vis, f.terrain, f.econ)
				if tc.lostSeen {
					f.enemy.Flags &^= visibility.SeenBit
				}
				got, ok := s.acquireTargetForSlot(f.shooter, f.shooter.SlotAt(0), 0, f.world, f.vis, f.terrain, nil, f.econ)
				want := pool.Handle(0)
				if tc.want {
					want = f.enemy.Handle
				}
				if ok != tc.want || got != want {
					t.Fatalf("acquired (%d, %v), want (%d, %v)", got, ok, want, tc.want)
				}
			})
		}
	}
}
