package main

import (
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Fresh single-player entry retains preference speed; save restoration keeps
// the saved scheduler instead [08 R-ENTRY-01 §3]. Exercise client installation
// so an unused preference helper cannot satisfy the regression.
func TestBattleInstallationConsumesGameSpeedPreference(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	prefs := settings.Defaults()
	prefs.GameSpeed = 4
	if err := prefs.Save(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		kind     mission.Type
		shell    *gameShell
		restored bool
		preview  bool
		want     int32
	}{
		{"direct skirmish", mission.TypeSkirmish, nil, false, false, 4},
		{"direct campaign", mission.TypeCampaign, nil, false, false, 4},
		{"menu skirmish", mission.TypeSkirmish, &gameShell{gameSpeed: 17}, false, false, 17},
		{"menu campaign", mission.TypeCampaign, &gameShell{gameSpeed: 17}, false, false, 17},
		{"restored battle", mission.TypeSkirmish, &gameShell{gameSpeed: 17}, true, false, 10},
		{"settings preview", mission.TypeSkirmish, &gameShell{gameSpeed: 17}, false, true, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &clock.State{Requested: 10, Active: 10, Paused: true, Carry: 0.25, ScaledAnchor: 37, GlobalTick: 91}
			sess := &session.Session{Clock: state, Snapshot: frame.NewBuffer(), Mission: &mission.Mission{Type: tc.kind}}
			b := &battleSession{sess: sess, cam: battleStartCameraFixture(), hud: &retailBattleHUD{}, shell: tc.shell, preview: tc.preview}
			if tc.restored {
				b.entrySavedCamera = &save.Camera{}
				state.Active = 8
			}
			before := state.SaveBox()
			cl, err := client.New(client.Options{Width: 640, Height: 480})
			if err != nil {
				t.Fatal(err)
			}
			installBattleClient(cl, b)
			if state.Requested != tc.want || !tc.restored && state.Active != tc.want {
				t.Fatalf("installed speed requested/active = %d/%d, want %d", state.Requested, state.Active, tc.want)
			}
			if tc.restored && state.SaveBox() != before {
				t.Fatal("client installation changed the restored scheduler")
			}
			if !state.Paused || state.Carry != 0.25 || state.ScaledAnchor != 37 || state.GlobalTick != 91 {
				t.Fatal("preference changed scheduler state beyond speed")
			}
			if len(cl.MessageLines()) != 0 {
				t.Fatal("preference initialization announced a speed change")
			}
		})
	}
}
