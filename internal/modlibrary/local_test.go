package modlibrary

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A local package, zip or folder, takes the download path and the standard
// content check against the base install (§4.5, §5.3 step 4): one that would
// not start is refused and leaves nothing installed.
func TestInstallLocalValidatesAgainstTheBase(t *testing.T) {
	base := t.TempDir()
	writeTree(t, base, map[string]string{"gamedata/moveinfo.tdf": "[CLASS0]{}", "gamedata/sidedata.tdf": "[SIDE0]{}"})

	lib := openTestLibrary(t)
	archive := writeZip(t, t.TempDir(), "sample.zip", zipItem{name: MetadataFile, body: metadataJSON(t, sampleMetadata())}, zipItem{name: "units/a.fbi", body: "a"})
	mod, err := lib.InstallLocal(archive, []string{base})
	if err != nil {
		t.Fatal(err)
	}
	if mod.ID != "sample" || mod.Receipt.Source != "local:sample.zip" || mod.Receipt.SHA256 == "" {
		t.Fatalf("installed %+v", mod)
	}

	folder := filepath.Join(t.TempDir(), "My Pack")
	writeTree(t, folder, map[string]string{"units/b.fbi": "b"})
	local, err := lib.InstallLocal(folder, []string{base})
	if err != nil {
		t.Fatal(err)
	}
	if !local.Local || local.ID != "local-my-pack" || local.Receipt.Source != "local:My Pack" {
		t.Fatalf("folder installed as %+v", local)
	}

	refused := openTestLibrary(t)
	if _, err := refused.InstallLocal(archive, []string{t.TempDir()}); err == nil || !strings.Contains(err.Error(), "gamedata/moveinfo.tdf") {
		t.Fatalf("a package that would not start = %v, want the missing product named", err)
	}
	assertNothingInstalled(t, refused)
	if _, err := refused.InstallLocal(filepath.Join(t.TempDir(), "absent.zip"), []string{base}); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: mod package is not readable") {
		t.Fatalf("a missing package = %v", err)
	}
}

// Select is the command-line resolution both commands share (§4.3): none is
// no mod, an absent mod or an unmet requirement is an error that names it.
func TestSelectResolvesCommandLineSelectors(t *testing.T) {
	base := t.TempDir()
	writeTree(t, base, map[string]string{"gamedata/moveinfo.tdf": "[CLASS0]{}", "gamedata/sidedata.tdf": "[SIDE0]{}"})
	lib := openTestLibrary(t)
	needs := sampleMetadata()
	needs.ID, needs.Requires = "needs", []string{"maps/expansion.ota"}
	for _, meta := range []Metadata{sampleMetadata(), needs} {
		archive := writeZip(t, t.TempDir(), meta.ID+".zip", zipItem{name: MetadataFile, body: metadataJSON(t, meta)}, zipItem{name: "units/a.fbi", body: "a"})
		if _, err := lib.InstallArchive(archive, InstallOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok, err := lib.Select("none", []string{base}); ok || err != nil {
		t.Fatalf("Select(none) = %v, %v", ok, err)
	}
	if mod, ok, err := lib.Select("sample", []string{base}); !ok || err != nil || mod.Version != "1.0" {
		t.Fatalf("Select(sample) = %+v, %v, %v", mod, ok, err)
	}
	if _, _, err := lib.Select("sample@2.0", []string{base}); err == nil || !strings.Contains(err.Error(), "mod is not installed: logical path sample@2.0") {
		t.Fatalf("Select of an absent version = %v", err)
	}
	if _, _, err := lib.Select("needs", []string{base}); err == nil || !strings.Contains(err.Error(), "maps/expansion.ota") {
		t.Fatalf("Select with an unmet requirement = %v, want the path named", err)
	}
	writeTree(t, base, map[string]string{"maps/expansion.ota": "[GlobalHeader]{}"})
	if _, ok, err := lib.Select("needs", []string{base}); !ok || err != nil {
		t.Fatalf("Select with the requirement met = %v, %v", ok, err)
	}
}

// A metadata-less local package has no config: it installs, lists and is
// selected as plain content with no recommendations, whatever directories
// it ships, because nothing detects content any more (§4.5).
func TestLocalPackageIsPlainContent(t *testing.T) {
	base := t.TempDir()
	writeTree(t, base, map[string]string{"gamedata/moveinfo.tdf": "[CLASS0]{}", "gamedata/sidedata.tdf": "[SIDE0]{}"})
	lib := openTestLibrary(t)
	folder := filepath.Join(t.TempDir(), "ProTA4.8")
	writeTree(t, folder, map[string]string{
		"gamedatP/moveinfo.tdf": "[CLASS0]{}", "gamedatP/sidedata.tdf": "[SIDE0]{}",
		"downloadP/a.tdf": "a", "guiP/a.gui": "a", "unitpicsP/a.pcx": "a", "weaponP/a.tdf": "a",
	})
	local, err := lib.InstallLocal(folder, []string{base})
	if err != nil {
		t.Fatal(err)
	}
	if local.HasConfig() || local.Controls != "" || local.MinimumGameplay != "" || local.Content().Name != "retail" {
		t.Fatalf("a configless package gained a config or recommendations: %+v", local.Metadata)
	}
	selected, ok, err := lib.Select(local.ID, []string{base})
	if !ok || err != nil || selected.HasConfig() || selected.Controls != "" || selected.MinimumGameplay != "" {
		t.Fatalf("Select = %+v, %v, %v", selected.Metadata, ok, err)
	}
	if kept := ResolveProfileDefaults([]string{base}, selected); !reflect.DeepEqual(kept, selected) {
		t.Fatalf("ResolveProfileDefaults changed the mod: %+v", kept.Metadata)
	}
}
