package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Camera and icon choices persist as independent presentation preferences;
// old files adopt their defaults and unsupported integers fall back instead
// of selecting a style through a low bit (DESIGN_GPU_RENDERER §16, §18).
func TestZoomAndIconPreferencesLoadAndRoundTrip(t *testing.T) {
	if p := DefaultPresentation(); p.ZoomStyle != ZoomSmooth || p.StrategicIconStyle != StrategicIconsModern {
		t.Fatalf("default styles = %d, %d, want smooth and Modern", p.ZoomStyle, p.StrategicIconStyle)
	}
	for _, tc := range []struct {
		name       string
		block      string
		zoom       int
		icons      int
		configPath string
	}{
		{"old file", "", ZoomSmooth, StrategicIconsModern, ""},
		{"old community path", `,"presentation":{"strategicIconConfig":"/icons/iconcfg.ini"}`, ZoomSmooth, StrategicIconsModern, "/icons/iconcfg.ini"},
		{"smooth Modern", `,"presentation":{"zoomStyle":0,"strategicIconStyle":0}`, ZoomSmooth, StrategicIconsModern, ""},
		{"smooth community", `,"presentation":{"zoomStyle":0,"strategicIconStyle":1}`, ZoomSmooth, StrategicIconsCommunity, ""},
		{"stepped Modern", `,"presentation":{"zoomStyle":1,"strategicIconStyle":0}`, ZoomStepped, StrategicIconsModern, ""},
		{"no zoom Modern icons", `,"presentation":{"zoomStyle":2,"strategicIconStyle":0}`, ZoomNone, StrategicIconsModern, ""},
		{"no zoom community icons", `,"presentation":{"zoomStyle":2,"strategicIconStyle":1}`, ZoomNone, StrategicIconsCommunity, ""},
		{"stepped community", `,"presentation":{"zoomStyle":1,"strategicIconStyle":1,"strategicIconConfig":"/icons/iconcfg.ini"}`, ZoomStepped, StrategicIconsCommunity, "/icons/iconcfg.ini"},
		{"negative styles", `,"presentation":{"zoomStyle":-1,"strategicIconStyle":-1}`, ZoomSmooth, StrategicIconsModern, ""},
		{"unsupported zoom", `,"presentation":{"zoomStyle":3,"strategicIconStyle":1}`, ZoomSmooth, StrategicIconsCommunity, ""},
		{"unsupported icons", `,"presentation":{"zoomStyle":1,"strategicIconStyle":3,"strategicIconConfig":"/icons/iconcfg.ini"}`, ZoomStepped, StrategicIconsModern, "/icons/iconcfg.ini"},
		{"styles past range", `,"presentation":{"zoomStyle":3,"strategicIconStyle":2}`, ZoomSmooth, StrategicIconsModern, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			body := `{"version":` + strconv.Itoa(FileVersion) + tc.block + `}`
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			s, err := LoadFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			want := DefaultPresentation()
			want.ZoomStyle, want.StrategicIconStyle = tc.zoom, tc.icons
			want.StrategicIconConfig = tc.configPath
			if s.Presentation != want {
				t.Fatalf("loaded presentation = %+v, want %+v", s.Presentation, want)
			}
			if err := s.SaveTo(path); err != nil {
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
			if string(stored.Presentation["zoomStyle"]) != strconv.Itoa(tc.zoom) || string(stored.Presentation["strategicIconStyle"]) != strconv.Itoa(tc.icons) {
				t.Fatalf("saved styles = %s, %s, want %d, %d", stored.Presentation["zoomStyle"], stored.Presentation["strategicIconStyle"], tc.zoom, tc.icons)
			}
			again, err := LoadFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if again.Presentation != want {
				t.Fatalf("round-trip presentation = %+v, want %+v", again.Presentation, want)
			}
		})
	}
}

func TestZoomLockPreferenceLoadAndRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, block string
		want        int
	}{
		{"old file", "", ZoomLockDefaultPercent},
		{"preferred detail", `,"presentation":{"zoomLockPercent":120}`, 120},
		{"preferred overview", `,"presentation":{"zoomLockPercent":20}`, 20},
		{"minimum", `,"presentation":{"zoomLockPercent":1}`, ZoomLockMinPercent},
		{"maximum", `,"presentation":{"zoomLockPercent":200}`, ZoomLockMaxPercent},
		{"zero", `,"presentation":{"zoomLockPercent":0}`, ZoomLockDefaultPercent},
		{"negative", `,"presentation":{"zoomLockPercent":-5}`, ZoomLockDefaultPercent},
		{"past maximum", `,"presentation":{"zoomLockPercent":201}`, ZoomLockDefaultPercent},
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
			if got := loaded.Presentation.ZoomLockPercent; got != tc.want {
				t.Fatalf("loaded lock = %d, want %d", got, tc.want)
			}
			if err := loaded.SaveTo(path); err != nil {
				t.Fatal(err)
			}
			again, err := LoadFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if again.Presentation.ZoomLockPercent != tc.want {
				t.Fatalf("round-trip lock = %d, want %d", again.Presentation.ZoomLockPercent, tc.want)
			}
		})
	}
}
