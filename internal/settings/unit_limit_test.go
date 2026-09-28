package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/community"
)

// TestUnitLimitDefaultAndClamp locks the configured per-player unit limit:
// Nanolathe raises the missing value and upper bound by user request
// (DESIGN_CONTENT_VFS §5); settings still clamp once at startup.
func TestUnitLimitDefaultAndClamp(t *testing.T) {
	if got := Defaults(); got.UnitLimit != 0 || got.ConfiguredUnitLimit() != DefaultUnitLimit {
		t.Fatalf("Defaults() UnitLimit/configured = %d/%d, want 0/%d", got.UnitLimit, got.ConfiguredUnitLimit(), DefaultUnitLimit)
	}
	for _, tc := range []struct{ in, want int }{
		{0, 0},
		{-1, MinUnitLimit},
		{19, MinUnitLimit},
		{20, 20},
		{500, 500},
		{501, 501},
		{1000, 1000},
		{2000, 2000},
		{3276, 3276},
		{3277, MaxUnitLimit},
	} {
		s := Defaults()
		s.UnitLimit = tc.in
		s.Normalize()
		if s.UnitLimit != tc.want {
			t.Errorf("Normalize(UnitLimit %d) = %d, want %d", tc.in, s.UnitLimit, tc.want)
		}
	}
}

// TestUnitLimitSurvivesRoundTrip proves the value reaches the session rather
// than being reset on every save: a stored limit is written, read back
// unchanged, and a file that omits the key records no choice and configures
// the default. The default is never written for the player.
func TestUnitLimitSurvivesRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Defaults()
	s.UnitLimit = 2000
	if err := s.SaveTo(path); err != nil {
		t.Fatalf("SaveTo: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got.UnitLimit != 2000 {
		t.Fatalf("UnitLimit = %d, want 2000", got.UnitLimit)
	}

	// A hand-written file with no `unitLimit` key takes the default, the way
	// the retail reader installs a default per absent value.
	body, err := json.Marshal(map[string]any{"version": FileVersion, "difficulty": 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	bare := filepath.Join(t.TempDir(), "bare.json")
	if err := os.WriteFile(bare, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err = LoadFrom(bare)
	if err != nil {
		t.Fatalf("LoadFrom(bare): %v", err)
	}
	if got.UnitLimit != 0 || got.ConfiguredUnitLimit() != DefaultUnitLimit {
		t.Fatalf("absent key UnitLimit/configured = %d/%d, want 0/%d", got.UnitLimit, got.ConfiguredUnitLimit(), DefaultUnitLimit)
	}
	untouched := filepath.Join(t.TempDir(), "untouched.json")
	if err := Defaults().SaveTo(untouched); err != nil {
		t.Fatalf("SaveTo(untouched): %v", err)
	}
	if raw, err := os.ReadFile(untouched); err != nil || strings.Contains(string(raw), "unitLimit") {
		t.Fatalf("untouched settings wrote a unit limit (err %v)", err)
	}
}

// TestUnitLimitSourcesBeatFeatureTable locks issue #30: every shipped feature
// table names a limit, so a player's chosen limit must be layered over it or
// it is never used. The layers resolve in the session's order: content, the
// rule set's table, the player's block, then the command line.
func TestUnitLimitSourcesBeatFeatureTable(t *testing.T) {
	gameplayLimit := 700
	for _, tc := range []struct {
		name       string
		player     community.Overrides
		cli, saved int
		want       int
	}{
		{name: "no choice keeps the table", want: 1500},
		{name: "saved default is a choice", saved: DefaultUnitLimit, want: DefaultUnitLimit},
		{name: "saved choice", saved: 500, want: 500},
		{name: "command line beats saved", cli: 2000, saved: 500, want: 2000},
		{name: "command line equal to the default", cli: DefaultUnitLimit, saved: 500, want: DefaultUnitLimit},
		{name: "player feature block keeps its own limit", player: community.Overrides{UnitLimit: &gameplayLimit}, saved: 500, want: gameplayLimit},
		{name: "command line beats player feature block", player: community.Overrides{UnitLimit: &gameplayLimit}, cli: 300, want: 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commandLine []community.Overrides
			player, layered := UnitLimitSources(tc.player, commandLine, tc.cli, tc.saved)
			all := append([]community.Overrides{{Table: "prota"}, player}, layered...)
			got, err := community.Resolve(false, all...)
			if err != nil {
				t.Fatal(err)
			}
			if got.UnitLimit != tc.want {
				t.Fatalf("resolved unit limit = %d, want %d", got.UnitLimit, tc.want)
			}
		})
	}
	// The caller's player block is a value, and its command-line slice is
	// copied rather than appended to in place.
	base := make([]community.Overrides, 1, 4)
	if _, out := UnitLimitSources(community.Overrides{}, base, 900, 0); &out[0] == &base[0] {
		t.Fatal("command-line layers share the caller's backing array")
	}
}
