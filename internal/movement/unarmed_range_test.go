package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// An unarmed aircraft's first slot points at weapon record 0, so every air leg
// sized by the first slot's `Range` uses that record's range, not zero
// [06 R-WPN-05 §2][04 R-AIR-01 §4].
func TestFirstWeaponRangeOfAnUnarmedSlotIsTheSentinelsRange(t *testing.T) {
	u := &units.Unit{Def: &content.UnitDef{Weapon1Def: &content.WeaponDef{ID: 0, Range: 16}}}
	if got := firstWeaponRange(u); got != 16 {
		t.Fatalf("unarmed first-slot range = %d, want the sentinel's 16", got)
	}
}
