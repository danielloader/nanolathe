package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// Construction kickout keeps two ghost drawers (community patch engine
// CP-CON-6). A site the click-snap preview owns uses the patch's physical
// palette entries, bypassing the GUI map; the installed GUI colour 6 is red,
// so treating the clear-site selector as that index hid valid sites. Every
// other site keeps the engine ghost, whose colour is the illegal entry 4 plus
// an offset masked in by the site-valid bit [07 §9] — 6 in retail (entry 10),
// 10 under the patch's clearance state (entry 14, yellow); drawing physical
// 240 there showed a black ghost over the player's own units (#33). Without
// kickout the clearance and preview flags must change nothing.
func TestBuildGhostCommunityColors(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		kickout, valid, clear, preview bool
		want                           uint8
	}{
		{"preview clear", true, true, false, true, 234},
		{"preview needs clearance", true, true, true, true, 240},
		{"preview rejected", true, false, false, true, 214},
		{"clear", true, true, false, false, 17},
		{"needs clearance", true, true, true, false, 23},
		{"rejected", true, false, false, false, 19},
		{"retail clear", false, true, false, false, 17},
		{"retail rejected", false, false, false, false, 19},
		{"retail ignores clearance and preview", false, true, true, true, 17},
		{"retail rejected ignores preview", false, false, false, true, 19},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cl, err := client.New(client.Options{Width: 640, Height: 480})
			if err != nil {
				t.Fatal(err)
			}
			pal := &palette.Tables{}
			pal.Logical[10] = 17
			pal.Logical[4] = 19
			pal.Logical[6] = 21
			pal.Logical[14] = 23
			cl.SetPalette(pal)
			b := &battleSession{sess: &session.Session{Community: community.Features{ConstructionKickout: tc.kickout}}, cam: &camera.Camera{ViewW: 640, ViewH: 480}}
			state := b.battleState()
			state.ArmPlacement("fixture", 2, 2)
			state.Input.BuildCellX, state.Input.BuildCellZ = 12, 10
			state.Input.PointerX, state.Input.PointerY = 200, 160
			state.Input.BuildOK, state.Input.BuildNeedsClear, state.Input.BuildSnapPreview = tc.valid, tc.clear, tc.preview
			cl.SetUIStage(battleHUDUIStage{hud: &retailBattleHUD{}, battle: b})
			shot := cl.ComposeFrameSnapshot()
			left, top, _, _ := b.placementRect()
			if got := shot.Indexed[int(top)*shot.Width+int(left)]; got != tc.want {
				t.Fatalf("outline palette index = %d, want %d", got, tc.want)
			}
		})
	}
}
