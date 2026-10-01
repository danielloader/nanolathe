package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// These are Nanolathe host preferences: older files adopt the default while
// an explicit zero remains off (interface design "Modern radar dots").
func TestRadarDotsLoadAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, block string
		want        int
	}{
		{"old file", "", RadarDotsVisible},
		{"omitted style", `,"presentation":{"fps":120}`, RadarDotsVisible},
		{"no dots", `,"presentation":{"radarDots":0}`, RadarDotsNone},
		{"visible dots", `,"presentation":{"radarDots":1}`, RadarDotsVisible},
		{"attackable dots", `,"presentation":{"radarDots":2}`, RadarDotsAttackable},
		{"negative style", `,"presentation":{"radarDots":-1}`, RadarDotsVisible},
		{"unsupported style", `,"presentation":{"radarDots":3}`, RadarDotsVisible},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			body := `{"version":` + strconv.Itoa(FileVersion) + tc.block + `}`
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := loaded.Presentation.RadarDots; got != tc.want {
				t.Fatalf("loaded style = %d, want %d", got, tc.want)
			}
			if err := loaded.SaveTo(path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var stored struct {
				Presentation map[string]json.RawMessage `json:"presentation"`
			}
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			if got := string(stored.Presentation["radarDots"]); got != strconv.Itoa(tc.want) {
				t.Fatalf("stored style = %q, want %d", got, tc.want)
			}
			again, err := LoadFrom(path)
			if err != nil || again.Presentation.RadarDots != tc.want {
				t.Fatalf("round-trip style = %d, want %d: %v", again.Presentation.RadarDots, tc.want, err)
			}
		})
	}
}

func TestRadarDotsSettingsLayersPreserveZero(t *testing.T) {
	base := Defaults()
	recommendation := json.RawMessage(`{"presentation":{"radarDots":2}}`)
	player := json.RawMessage(`{"presentation":{"radarDots":0}}`)
	recommended, err := Layer(base, recommendation)
	if err != nil || recommended.Presentation.RadarDots != RadarDotsAttackable {
		t.Fatalf("recommendation style = %d: %v", recommended.Presentation.RadarDots, err)
	}
	effective, err := Layer(base, recommendation, player)
	if err != nil || effective.Presentation.RadarDots != RadarDotsNone || base.Presentation.RadarDots != RadarDotsVisible {
		t.Fatalf("player/base style = %d/%d: %v", effective.Presentation.RadarDots, base.Presentation.RadarDots, err)
	}
	patch, err := Diff(effective, recommended, ModScoped)
	if err != nil || string(patch) != string(player) {
		t.Fatalf("player patch = %s, want %s: %v", patch, player, err)
	}
}
