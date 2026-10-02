package main

import (
	"encoding/json"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// Applying another setting must keep the configured word a save restored,
// independently of the persisted preference [08 R-SESS-01 §9].
func TestNLScreenUnrelatedApplyPreservesRestoredUnitLimit(t *testing.T) {
	for _, change := range []string{"card", "preset", "override"} {
		t.Run(change, func(t *testing.T) {
			file := settings.Defaults()
			file.Gameplay = gameplay.Strict31
			g, s := settingsRegressionScreen(settingsRegressionMod(), file)
			g.applyRestoredUnitLimit(&save.BattleImage{Summary: save.Summary{MaxUnits: 250, HasMaxUnits: true}})
			s.draft = s.freshDraft(g)
			switch change {
			case "card":
				s.draft.pres.WaterSurface = 0
				s.touched["water"] = true
			case "preset":
				s.applyPresetToDraft(nlPresetEntry{name: "Water", patch: json.RawMessage(`{"presentation":{"waterSurface":0}}`)}, []bool{false, true, false})
			case "override":
				s.draft.override = true
			}
			s.apply()
			if got := g.setup.UnitLimit; got != 250 || g.captureSettings().UnitLimit != 0 {
				t.Fatalf("configured/persisted limit %d/%d, want 250/0", got, g.captureSettings().UnitLimit)
			}
			s.draft.unitLimit = 500
			s.touched["unitlimit"] = true
			s.apply()
			if g.setup.UnitLimit != 500 || g.captureSettings().UnitLimit != 500 {
				t.Fatalf("explicit limit edit did not apply: %d/%d", g.setup.UnitLimit, g.captureSettings().UnitLimit)
			}
		})
	}
}

func TestNLScreenControlsProfileChecksAllAssignedLocks(t *testing.T) {
	for _, control := range []string{"bar", "card"} {
		t.Run(control, func(t *testing.T) {
			mod := settingsRegressionMod()
			mod.Config.Settings = json.RawMessage(`{"switchAlt":0}`)
			mod.Config.Locks = []string{"switchAlt"}
			g, s := settingsRegressionScreen(mod, settings.Defaults())
			if control == "bar" {
				s.chooseProfile(2)
			} else {
				for _, c := range s.controlCards() {
					if c.key == "profile" {
						s.setCard(c, 2)
					}
				}
			}
			if s.dialog != "override" || s.draft.controls != 0 || s.draft.keys.Profile() != input.ProfileRetail || g.switchAlt {
				t.Fatalf("profile bypassed digit-key lock: dialog %q, profile %d, keys %s, groups %v", s.dialog, s.draft.controls, s.draft.keys.Profile(), g.switchAlt)
			}
			// Canceling the dialog has no profile or settings mutation to undo.
			s.dialog, s.pendingAction = "", nil
			s.apply()
			if g.switchAlt || g.lockOverridden(mod) {
				t.Fatal("canceled profile selection changed the locked setting")
			}
			s.chooseProfile(2)
			s.confirmOverride()
			s.apply()
			if !g.switchAlt || !g.lockOverridden(mod) {
				t.Fatal("confirmed override did not apply the profile")
			}
			restarted, _ := settingsRegressionScreen(mod, g.captureSettings())
			if !restarted.switchAlt || !restarted.lockOverridden(mod) {
				t.Fatal("profile override did not survive a settings round trip")
			}
		})
	}
}

func TestNLScreenControlsProfileLeavesUnassignedLocksAlone(t *testing.T) {
	for _, tc := range []struct {
		path    string
		profile int
		locked  bool
	}{
		{"audio", 2, true},
		{"presentation.reloadBars", 2, true},
		{"skirmish.numPlayers", 2, true},
		{"skirmish.numPlayers", 1, false}, // Retail keeps the player's row count.
		{"presentation.megamapFlash", 1, false},
		{"presentation.glint", 2, false},
		{"interfaceType", 2, false},
	} {
		mod := settingsRegressionMod()
		mod.Config.Locks = []string{tc.path}
		_, s := settingsRegressionScreen(mod, settings.Defaults())
		s.chooseProfile(tc.profile)
		if got := s.dialog == "override"; got != tc.locked {
			t.Errorf("profile %d with lock %s: confirmation %v, want %v", tc.profile, tc.path, got, tc.locked)
		}
	}
}
