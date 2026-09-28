package session

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Issue #36, on authored content. The Core Targeting Facility opens its
// owner's secondary fallback [04 R-SPEC-01 §8][06 §3.1]: a Gaat Gun (sight 350,
// weapon range 400) whose owner's radar detects an ARMFLASH 380 units away,
// outside every owned unit's sight, acquires it through ordinary autonomous
// maintenance only while the facility is activated. Modern threat targeting
// must honour that fallback as Strict does
// (DESIGN_WEAPONS_PROJECTILES "Modern threat targeting and incoming fire").
func TestTargetingFacilityOpensRadarFallback(t *testing.T) {
	f := loadRetailFixture(t)
	fx := numeric.FixedFromInt
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		for _, active := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/active=%v", mode, active), func(t *testing.T) {
				s := f.session(t)
				s.SetGameplay(mode)
				stepRetail(s, 2)
				tower := placeCompleteRetailUnit(t, s, "CORHLT", 0, fx(600), fx(600))
				placeCompleteRetailUnit(t, s, "CORRAD", 0, fx(900), fx(600))
				facility := placeCompleteRetailUnit(t, s, "CORTARG", 0, fx(300), fx(600))
				if !facility.Def.IsTargetingUpgrade || !facility.Activated {
					t.Fatalf("authored CORTARG: targeting upgrade %v, active on completion %v",
						facility.Def.IsTargetingUpgrade, facility.Activated)
				}
				facility.Activated = active
				enemy := placeCompleteRetailUnit(t, s, "ARMFLASH", 1, fx(600), fx(980))
				acquired := false
				for tick := 0; tick < 120 && !acquired; tick++ {
					s.Step(s.Clock.ScaledAnchor + 1)
					if s.IsUnitVisible(0, enemy) {
						t.Fatalf("tick %d: fixture is wrong: the Flash must stay out of sight", tick)
					}
					slot := tower.SlotAt(0)
					acquired = slot.Target.Kind == units.TargetUnit && slot.Target.Unit == enemy.Handle
				}
				if acquired != active {
					t.Fatalf("%s: facility active %v, radar contact acquired %v", mode, active, acquired)
				}
			})
		}
	}
}
