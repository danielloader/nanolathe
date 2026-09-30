package profiles_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// markerFS answers Stat for a fixed marker set and nothing else. The marker
// check reads no bytes, so this is the whole surface it needs.
type markerFS struct{ present map[string]bool }

func (m markerFS) Open(string) (vfs.File, error) { return nil, os.ErrNotExist }
func (m markerFS) ReadFileLimit(string, int64) ([]byte, error) {
	return nil, os.ErrNotExist
}
func (m markerFS) ReadDir(string) ([]vfs.EntryInfo, error) { return nil, os.ErrNotExist }
func (m markerFS) Stat(name string) (vfs.EntryInfo, error) {
	folded := strings.ToLower(name)
	if !m.present[folded] {
		return vfs.EntryInfo{}, os.ErrNotExist
	}
	return vfs.EntryInfo{Name: folded, Path: folded, IsDir: !strings.Contains(folded, ".")}, nil
}
func (m markerFS) CacheStamp(string) (string, error) { return "", os.ErrNotExist }

func markers(names ...string) markerFS {
	present := make(map[string]bool, len(names))
	for _, name := range names {
		present[strings.ToLower(name)] = true
	}
	return markerFS{present: present}
}

// shippedContent reads the content section of one of the repository's
// authored mod configs, the only place a mod's profile lives now.
func shippedContent(t *testing.T, dir string) profiles.Profile {
	t.Helper()
	meta, err := modlibrary.ReadConfigFile(testsupport.ModConfigPath(t, dir))
	if err != nil {
		t.Fatal(err)
	}
	return meta.Content()
}

// The base game's profile is the empty layout and the retail limits, and it
// is the one every content set without a Nanolathe config reports.
func TestRetailProfile(t *testing.T) {
	retail := profiles.Retail()
	if retail.Name != profiles.RetailName || len(retail.Directories) != 0 || len(retail.Detect) != 0 || retail.Limits != (profiles.Limits{}) || retail.Presentation != (profiles.Presentation{}) {
		t.Fatalf("Retail() = %+v", retail)
	}
	if missing := retail.MissingMarkers(markers()); len(missing) != 0 {
		t.Fatalf("the retail profile names markers: %v", missing)
	}
}

// Markers select nothing: they are an install check that a config describes
// the content it is packaged with, and every one must be a complete tree.
func TestMissingMarkersNamesEveryAbsentTree(t *testing.T) {
	escalation := shippedContent(t, "escalation-10.2.0")
	if missing := escalation.MissingMarkers(markers(escalation.Detect...)); len(missing) != 0 {
		t.Fatalf("complete layout misses %v", missing)
	}
	for i := range escalation.Detect {
		partial := append([]string(nil), escalation.Detect[:i]...)
		partial = append(partial, escalation.Detect[i+1:]...)
		if missing := escalation.MissingMarkers(markers(partial...)); len(missing) != 1 || missing[0] != escalation.Detect[i] {
			t.Fatalf("without %s: missing = %v", escalation.Detect[i], missing)
		}
	}
	// An archive name is packaging, not a tree: it satisfies no marker.
	if missing := escalation.MissingMarkers(markers("TAESC.gp3", "unitsE")); len(missing) != len(escalation.Detect)-1 {
		t.Fatalf("an archive filename satisfied a marker: missing %v", missing)
	}
	if missing := escalation.MissingMarkers(nil); len(missing) != len(escalation.Detect) {
		t.Fatalf("no overlay: missing %v", missing)
	}
}

// TestParseRejectsShapesNoLoaderCouldUse keeps a hand-written content
// section from silently doing nothing.
func TestParseRejectsShapesNoLoaderCouldUse(t *testing.T) {
	for _, tt := range []struct{ name, body, want string }{
		{name: "unknown field", body: `{"limits":{},"directories":{}}`, want: "reading the content section failed"},
		{name: "old limit field", body: `{"limits":{"unit_limit":1500}}`, want: "reading the content section failed"},
		{name: "gameplay block", body: `{"gameplay":{"table":"prota"}}`, want: "reading the content section failed"},
		{name: "nested directory", body: `{"layout":{"units":"mod/units"}}`, want: "not a single directory"},
		{name: "empty row", body: `{"layout":{"units":""}}`, want: "row is empty"},
		{name: "nested marker", body: `{"detect":["a\\b"]}`, want: "not a single directory"},
		{name: "negative page size", body: `{"presentation":{"build_menu_page_size":-1}}`, want: "build_menu_page_size -1 is negative"},
		{name: "negative limit", body: `{"limits":{"tnt_bytes":-1}}`, want: "negative"},
		{name: "trailing data", body: `{} {}`, want: "trailing data"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := profiles.Parse([]byte(tt.body), "x", "house.json content")
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "logical path house.json content") {
				t.Fatalf("Parse error = %v, want one containing %q and naming the document", err, tt.want)
			}
		})
	}
	authored, err := profiles.Parse([]byte(`{"detect":["houseUnits"],"layout":{"units":"houseUnits"},"limits":{"units":900}}`), "house", "house.json content")
	if err != nil {
		t.Fatal(err)
	}
	if authored.Name != "house" || authored.Directories["units"] != "houseUnits" || authored.Limits.Units != 900 || authored.Detect[0] != "houseUnits" {
		t.Fatalf("authored profile = %+v", authored)
	}
}
