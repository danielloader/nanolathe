//go:build retail

package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The installed palettes decide what the ghost actually looks like. The engine
// ghost resolves GUI entries 10 / 14 / 4 through the GUIPAL-to-display map
// [03 §4.3]: bright green, the pale yellow of entry 14 [03 R-MM-01 §1] and
// dark red. The click-snap preview's physical 234 / 214 are a darker green and
// red; its physical 240 is black in the stock palette (#33 is exactly that
// black drawn where the engine ghost belonged) and only a mod palette such as
// ProTA's makes it yellow [community patch engine CP-CON-6].
func TestBuildGhostColorsResolveThroughInstalledPalette(t *testing.T) {
	fs := vfs.New()
	if err := fs.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Skipf("retail assets unavailable: %v", err)
	}
	defer fs.Close()
	pal, err := palette.Load(fs)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                           string
		kickout, valid, clear, preview bool
		want                           [3]byte
	}{
		{"clear", true, true, false, false, [3]byte{83, 223, 79}},
		{"needs clearance", true, true, true, false, [3]byte{247, 227, 103}},
		{"rejected", true, false, false, false, [3]byte{171, 23, 0}},
		{"preview clear", true, true, false, true, [3]byte{51, 191, 43}},
		{"preview needs clearance", true, true, true, true, [3]byte{0, 0, 0}},
		{"preview rejected", true, false, false, true, [3]byte{127, 7, 0}},
		{"retail clear", false, true, true, true, [3]byte{83, 223, 79}},
		{"retail rejected", false, false, false, true, [3]byte{171, 23, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cl, err := client.New(client.Options{Width: 640, Height: 480})
			if err != nil {
				t.Fatal(err)
			}
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
			at := (int(top)*shot.Width + int(left)) * 4
			got := [3]byte{shot.RGBA[at], shot.RGBA[at+1], shot.RGBA[at+2]}
			if got != tc.want {
				t.Fatalf("outline rgb = %v (palette index %d), want %v", got, shot.Indexed[int(top)*shot.Width+int(left)], tc.want)
			}
		})
	}
}
