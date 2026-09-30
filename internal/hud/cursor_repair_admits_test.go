package hud

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Each repair shape uses the shared admission; MOVE does not add code 2's
// stricter unsigned health compare [07 §8][04 R-ORD-02 §7].
func TestRepairCursorRowsUseSharedAdmission(t *testing.T) {
	actor := unit(0, &content.UnitDef{CanMove: true, CanReclamate: true})
	target := unit(0, &content.UnitDef{MaxDamage: 100})
	target.Remaining = 1
	target.Health = 101
	h := CursorHover{OverWorld: true, Target: target}
	for _, latch := range []input.Latch{input.LatchNormal, input.LatchMove, input.LatchRepair} {
		for _, admit := range []bool{false, true} {
			sel := CursorSelection{Viewer: 0, Units: []*units.Unit{actor},
				RepairAdmits: func(a, b *units.Unit) bool {
					if a != actor || b != target {
						t.Fatal("repair admission received different unit copies")
					}
					return admit
				}}
			got := ChooseCursor(latch, sel, h)
			if (got == render.CursorRepair) != admit {
				t.Fatalf("latch %d admission %v chose %s", latch, admit, render.CursorName(got))
			}
		}
	}
}
