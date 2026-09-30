package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// installHeadlessFixtureMod installs an authored one-file mod into the
// scratch library isolateHostFiles points at, without a content check.
func installHeadlessFixtureMod(t *testing.T, meta modlibrary.Metadata) modlibrary.Mod {
	t.Helper()
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	return installHeadlessFixtureConfig(t, meta.ID, string(data))
}

// installHeadlessFixtureConfig installs an authored one-file mod carrying
// the given nanolathe-mod.json text.
func installHeadlessFixtureConfig(t *testing.T, id, metadata string) modlibrary.Mod {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	for name, body := range map[string]string{modlibrary.MetadataFile: metadata, "units/fixture.fbi": "[UNITINFO] { }\n"} {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	root, err := modlibrary.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := modlibrary.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	mod, err := lib.InstallDirectory(dir, modlibrary.InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return mod
}

// TestParseModMountsTheInstalledMod locks --mod for the displayless command
// (docs/DESIGN_MODS_MUTATORS.md §4.3): the installed mod is mounted as the
// last root with its own config's content section and Community table,
// which beat the saved preference; a --mod-config file stands in for the
// mod's config; the settings file's saved mod is never read; `none` mounts
// nothing, taking the saved config file; a mod without a config runs as
// plain content; and a mod beside a manual root stack, an absent mod, a
// gameplay mode below the mod's minimum and the fixed benchmark scene are
// refused with the standard diagnostic.
func TestParseModMountsTheInstalledMod(t *testing.T) {
	isolateHostFiles(t)
	base := t.TempDir()
	mod := installHeadlessFixtureConfig(t, "fixture", `{"schema":2,"id":"fixture","name":"Fixture","version":"2","content":{"layout":{"weapons":"weaponF"}},"rules":{"minimumGameplay":"community-3.9","communityFeatures":{"airCorpseFall":true}}}`)
	savedConfig := filepath.Join(t.TempDir(), "saved.json")
	if err := os.WriteFile(savedConfig, []byte(`{"schema":2,"id":"saved","name":"Saved","version":"1","content":{"layout":{"units":"unitsS"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	stored := settings.Defaults()
	stored.ContentProfile, stored.Mod = savedConfig, settings.ModSelection{ID: mod.ID}
	if err := stored.Save(); err != nil {
		t.Fatal(err)
	}

	req, _, _, _, err := parse([]string{"--root", base, "--mod", "fixture"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.Roots, []string{base, mod.Dir}) || req.Root != base || req.Mod != "fixture@2" || req.Content.Name != "fixture" || req.Content.Directories["weapons"] != "weaponF" {
		t.Fatalf("--mod request roots %v root %q mod %q content %+v", req.Roots, req.Root, req.Mod, req.Content)
	}
	if len(req.ProfileFeatures) != 1 || req.ProfileFeatures[0].Base == nil || !req.ProfileFeatures[0].Base.AirCorpseFall {
		t.Fatalf("--mod community sources = %+v", req.ProfileFeatures)
	}
	// A config file named beside --mod replaces the mod's own, rules too.
	req, _, _, _, err = parse([]string{"--root", base, "--mod", "fixture@2", "--mod-config", savedConfig, "--gameplay", "strict-3.1"}, io.Discard)
	if err != nil || req.Content.Name != "saved" || req.ProfileFeatures != nil {
		t.Fatalf("an explicit config beside --mod = %+v, %v, %v", req.Content, req.ProfileFeatures, err)
	}
	for _, args := range [][]string{{"--root", base}, {"--root", base, "--mod", "none"}} {
		req, _, _, _, err := parse(args, io.Discard)
		if err != nil || !reflect.DeepEqual(req.Roots, []string{base}) || req.Mod != "" || req.Content.Name != "saved" {
			t.Fatalf("parse(%v) mounted %v, mod %q, content %q: %v", args, req.Roots, req.Mod, req.Content.Name, err)
		}
	}
	// A mod without a config is plain content: no layout, no rules.
	plain := installHeadlessFixtureMod(t, modlibrary.Metadata{Schema: 1, ID: "plain", Name: "Plain", Version: "1", ContentProfile: "prota", MinimumGameplay: "community-3.9"})
	req, _, _, _, err = parse([]string{"--root", base, "--mod", plain.ID, "--gameplay", "strict-3.1"}, io.Discard)
	if err != nil || req.Content.Name != "retail" || len(req.Content.Directories) != 0 || req.ProfileFeatures != nil {
		t.Fatalf("a configless --mod = %+v, %v, %v", req.Content, req.ProfileFeatures, err)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--root", base, "--root", t.TempDir(), "--mod", "fixture"}, "conflicts with a manual root stack"},
		{[]string{"--root", base, "--mod", "absent"}, "mod is not installed"},
		{[]string{"--root", base, "--mod", "fixture", "--gameplay", "strict-3.1"}, "below the mod's minimum"},
		{[]string{"--root", base, "--mod", "fixture", "--sim-benchmark", "/tmp/unused-benchmark"}, "simulation-cost benchmark"},
	} {
		if _, _, _, _, err := parse(tc.args, io.Discard); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: ") || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("parse(%v) = %v, want a nanolathe diagnostic containing %q", tc.args, err, tc.want)
		}
	}
	// Several roots with no mod stay a manual stack.
	if _, _, _, _, err := parse([]string{"--root", base, "--root", t.TempDir(), "--mod", "none"}, io.Discard); err != nil {
		t.Fatalf("--mod none beside a manual stack: %v", err)
	}
}
