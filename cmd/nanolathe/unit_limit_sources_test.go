package main

import (
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// TestConfiguredUnitLimitReachesBattleEntry locks issue #30 on the desktop
// path: the saved `unitLimit` and `--unit-limit` reach the feature sources a
// battle resolves, so they beat the shipped table's 1500 outside Strict.
func TestConfiguredUnitLimitReachesBattleEntry(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       gameplay.Mode
		saved, cli int
		want       int
	}{
		{name: "modern unset keeps table", mode: gameplay.Modern, want: 1500},
		{name: "modern saved default is a choice", mode: gameplay.Modern, saved: settings.DefaultUnitLimit, want: settings.DefaultUnitLimit},
		{name: "modern saved", mode: gameplay.Modern, saved: 500, want: 500},
		{name: "modern command line", mode: gameplay.Modern, saved: 500, cli: 2000, want: 2000},
		{name: "community saved", mode: gameplay.Community39, saved: 500, want: 500},
		// Strict resolves no table; battle entry sizes from the configured word.
		{name: "strict resolves none", mode: gameplay.Strict31, saved: 500, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
			s := settings.Defaults()
			s.UnitLimit = tc.saved
			if err := s.Save(); err != nil {
				t.Fatal(err)
			}
			features, err := session.ResolveCommunity(tc.mode, communitySources(Options{UnitLimit: tc.cli}, nil))
			if err != nil {
				t.Fatal(err)
			}
			if features.UnitLimit != tc.want {
				t.Fatalf("resolved unit limit = %d, want %d", features.UnitLimit, tc.want)
			}
		})
	}
}
