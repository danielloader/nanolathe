//go:build retail

package main

import (
	"os"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

// Real construction-aircraft and wet-structure definitions pass through the
// pointer update and installed cursor bank [07 §8][04 R-ORD-01 §7]. Optional
// captures retain the composed idle-latch frame for visual inspection.
func TestRetailConstructorCursorSharesWaterAdmission(t *testing.T) {
	cat, _ := retailcat.Shared(t)
	for _, tc := range []struct {
		name  string
		unit  string
		y     int32
		admit bool
	}{{"underwater", "armuwes", 0, false}, {"floating", "armfmkr", 51, true}} {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBattle(cat, testWorldON05(40, 40))
			b.sess.World.SeaLevel = 55
			dst := withMinimap(b)
			playW, playH, _ := b.sess.PlayArea()
			layout, _, _ := b.minimapLayout()
			const mx, my = int32(70), int32(50)
			wx, wz, ok := client.MinimapPointerWorld(layout, dst, playW, playH, mx, my)
			if !ok {
				t.Fatal("fixture pointer did not resolve")
			}
			actor := placeUnit(b, "armca", 8<<16, 8<<16)
			// Keep the elevated target's radar dot under the pointer after the
			// minimap's half-height shear [03 §3.9].
			target := placeUnit(b, tc.unit, numeric.Fixed(wx)<<16, numeric.Fixed(wz+(tc.y>>1))<<16)
			target.Y = numeric.Fixed(tc.y) << 16
			target.Health, target.MaxHealth, target.Remaining = 1, target.Def.MaxDamage, 1
			replaceSelectionForTest(t, b, actor)
			cl, closeContent := minimapCursorClient(t, b)
			defer closeContent()
			cl.Input().Mouse.SetPosition(float32(mx), float32(my))
			for _, latch := range []input.Latch{input.LatchNormal, input.LatchMove, input.LatchRepair} {
				b.battleState().Input.Latch = latch
				b.updateCursor(cl)
				if got := cl.Cursors().Index(); (got == render.CursorRepair) != tc.admit {
					t.Fatalf("%s latch %d chose %s; repair admission=%v", tc.unit, latch, render.CursorName(got), tc.admit)
				}
			}
			if prefix := os.Getenv("NANOLATHE_ISSUE56_CURSOR_SHOT"); prefix != "" {
				cs, err := openContent(Options{Root: probeRetail(t)})
				if err != nil {
					t.Fatal(err)
				}
				defer cs.Close()
				cl.SetTerrain(b.sess.World)
				cl.SetCamera(b.cam)
				cl.SetPalette(retailPaletteForTest(t, cs))
				cl.SetModelFS(cs.unmappedMount)
				b.battleState().Input.Latch = input.LatchNormal
				b.updateCursor(cl)
				file, err := os.Create(prefix + "-" + tc.name + ".png")
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				if err := encodeShot(file, cl); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
