package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	contentprofiles "github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// authorRenamedInstall publishes a loose install whose families sit under TA
// Zero's directory names, without a version-specific archive filename.
// Every byte is authored here; none is copied from a content set.
func authorRenamedInstall(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"ZGameDat/moveinfo.tdf": "[CLASS0]\n{\nName=TANK3;\nFootprintX=3;\nFootprintZ=3;\nMinWaterDepth=0;\nMaxWaterDepth=0;\nMaxSlope=15;\n}\n",
		"ZGameDat/sidedata.tdf": "[SIDE0]\n{\nname=ARM;\ncommander=ARMCOM;\nfont=scratch.fnt;\n" + authoredSideAnchors() + "}\n",
		"ZGameDat/allsound.tdf": "[PROBECUE]\n{\nsound=probe;\n}\n",
		"ZUnits/armcom.fbi":     "[UNITINFO]\n{\nUnitName=ARMCOM;\n}\n",
		"ZWeapon/scratch.tdf":   "[SCRATCHGUN]\n{\nid=1;\n}\n",
		"ZGui/probe.gui":        "[GADGET0]\n{\n[COMMON]\n{\nid=0;\n}\n}\n",
		"ZUnitPic/armcom.pcx":   "authored placeholder\n",
		"ZBuildMenu/probe.tdf":  "[MENU]\n{\n}\n",
		"ZI/default.txt":        "plan 0\n",
	}
	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// authoredSideAnchors writes the thirty mandatory anchors CompileSides needs,
// each one distinct so a compile failure is about the table and not a value.
func authoredSideAnchors() string {
	names := []string{
		"LOGO", "ENERGYBAR", "ENERGYNUM", "ENERGYMAX", "ENERGY0",
		"METALBAR", "METALNUM", "METALMAX", "METAL0", "TOTALUNITS",
		"TOTALTIME", "ENERGYPRODUCED", "ENERGYCONSUMED", "METALPRODUCED",
		"METALCONSUMED", "LOGO2", "UNITNAME", "DAMAGEBAR", "UNITMETALMAKE",
		"UNITMETALUSE", "UNITENERGYMAKE", "UNITENERGYUSE", "MISSIONTEXT",
		"UNITNAME2", "DAMAGEBAR2", "NAME", "DESCRIPTION", "RELOAD1",
		"RELOAD2", "RELOAD3",
	}
	var b strings.Builder
	for i, name := range names {
		b.WriteString("[" + name + "]\n{\nx1=1;\ny1=1;\nx2=2;\ny2=2;\nindex=")
		b.WriteByte(byte('0' + i%10))
		b.WriteString(";\n}\n")
	}
	return b.String()
}

// shippedConfigPath is one of the repository's authored mod configs.
func shippedConfigPath(t *testing.T, dir string) string {
	t.Helper()
	return testsupport.ModConfigPath(t, dir)
}

// TestWindowedMountAppliesTheContentProfileTable is the contract this unit
// exists for: the graphical command's one mount boundary applies the running
// config's content section and hands every content reader the view that
// carries its directory table, so a loader asking for a retail directory
// reaches the tree the content set actually ships
// (docs/DESIGN_CONTENT_VFS.md §5 "Content profiles"). TA Zero's config is
// named with --mod-config, as a manual stack names one.
func TestWindowedMountAppliesTheContentProfileTable(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	config := shippedConfigPath(t, "ta-zero-alpha5-20241224")
	cs, err := openContent(Options{Roots: []string{authorRenamedInstall(t)}, ModConfig: config})
	if err != nil {
		t.Fatalf("mount renamed install: %v", err)
	}
	defer cs.Close()

	if cs.profile != "ta-zero" || cs.configPath != config || cs.config == nil || cs.modNotice != "" {
		t.Fatalf("mounted config = %q from %q (notice %q)", cs.profile, cs.configPath, cs.modNotice)
	}
	// The config's rules ride with the mount, apart from its content: the
	// Community table as the content source, and the controls preset a
	// manual stack is offered (docs/DESIGN_MODS_MUTATORS.md §4.3).
	if len(cs.gameplayFeatures) != 1 || cs.gameplayFeatures[0].Base == nil || cs.gameplayFeatures[0].Base.AIBuilderPlacementLimit != 127 || cs.profileControls != "zero" {
		t.Fatalf("config rules = %+v, controls %q", cs.gameplayFeatures, cs.profileControls)
	}
	if cs.fs == vfs.FSOps(cs.unmappedMount) {
		t.Fatal("a profile with a directory table left the mount unwrapped")
	}

	// The catalog family: the compiler asks for the retail name and the view
	// answers from the renamed tree. Provenance stays retail-named, because
	// the catalog hash and every diagnostic are built from these paths.
	sides, err := content.CompileSides(cs.fs)
	if err != nil {
		t.Fatalf("compile sides through the profile view: %v", err)
	}
	if len(sides) != 1 || sides[0].Name != "ARM" {
		t.Fatalf("sides = %+v", sides)
	}
	info, err := cs.fs.Stat("gamedata/sidedata.tdf")
	if err != nil {
		t.Fatalf("stat through the profile view: %v", err)
	}
	if info.Path != "gamedata/sidedata.tdf" {
		t.Fatalf("provenance path = %q, want the retail name", info.Path)
	}

	// The sound family reads the same renamed gamedata tree.
	aliases, _, err := content.CompileSoundAliasesOrdered(cs.fs)
	if err != nil {
		t.Fatalf("compile sound aliases through the profile view: %v", err)
	}
	if _, ok := aliases["probecue"]; !ok {
		t.Fatalf("sound aliases = %v, want the authored cue", aliases)
	}

	// The GUI family goes through the content set's own loader, which is the
	// only path the front end and the HUD use. A parse verdict is not the
	// point; reaching the authored file instead of a missing one is.
	if _, err := cs.loadGUI("guis/probe.gui"); errors.Is(err, vfs.ErrNotFound) {
		t.Fatalf("GUI loader missed the renamed tree: %v", err)
	}

	// The unit, weapon, build-menu, picture and AI families resolve the same
	// way, which is what makes the compiler's enumeration profile-agnostic.
	for _, logical := range []string{
		"units/armcom.fbi", "weapons/scratch.tdf",
		"download/probe.tdf", "unitpics/armcom.pcx", "ai/default.txt",
	} {
		if _, err := cs.fs.Stat(logical); err != nil {
			t.Fatalf("stat %s through the profile view: %v", logical, err)
		}
		if _, err := cs.unmappedMount.Stat(logical); err == nil {
			t.Fatalf("%s resolved on the raw mount, so the fixture does not prove the table", logical)
		}
	}
}

// TestRetailMountKeepsTheConcreteOverlay locks the no-change half: an install
// without a config mounts the base game's profile, whose table is empty, and
// an empty table returns the mounted overlay itself — so a retail run reads
// exactly what it read before content profiles existed.
func TestRetailMountKeepsTheConcreteOverlay(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "gamedata"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"moveinfo.tdf", "sidedata.tdf"} {
		if err := os.WriteFile(filepath.Join(root, "gamedata", name), []byte("[TEST] { value=base; }"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cs, err := openContent(Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if cs.profile != contentprofiles.RetailName {
		t.Fatalf("resolved profile = %q, want %s", cs.profile, contentprofiles.RetailName)
	}
	if cs.fs != vfs.FSOps(cs.unmappedMount) {
		t.Fatal("an empty directory table wrapped the mounted overlay")
	}
	if cs.config != nil || cs.gameplayFeatures != nil || cs.modNotice != "" {
		t.Fatalf("the base game mounted a config %+v, sources %v, notice %q", cs.config, cs.gameplayFeatures, cs.modNotice)
	}
}

// TestConcreteMountFamiliesAreNeverRedirected guards the readers this mount
// boundary still hands the concrete overlay: the presentation model cache
// (objects3d, textures, anims) and the OTA map census (maps). The menu's TNT
// preview uses the layout view.
// Both are typed on the overlay rather than on a read view, so they read those
// families unmapped — correct only while no profile renames them. A profile
// that did would need those two surfaces widened to a read view first, so this
// test turns that into a failure rather than into missing art.
func TestConcreteMountFamiliesAreNeverRedirected(t *testing.T) {
	concrete := []string{"objects3d", "textures", "anims", "maps"}
	for _, dir := range []string{"prota-4.8", "escalation-10.2.0", "ta-zero-alpha5-20241224", "mayhem-11.3.0"} {
		meta, err := modlibrary.ReadConfigFile(shippedConfigPath(t, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range meta.Content().Layout().Names() {
			for _, family := range concrete {
				if strings.EqualFold(row[0], family) {
					t.Fatalf("config %s redirects %s, which the windowed command still reads through the concrete mount", dir, family)
				}
			}
		}
	}
}

// With no mod, the config comes from --mod-config, else the saved
// contentProfile preference, else nowhere: nothing detects a layout, a
// saved name of a removed built-in profile is ignored with the notice, and
// mounting never writes the preference (docs/DESIGN_MODS_MUTATORS.md §4.3).
func TestContentConfigPrecedenceWithoutAMod(t *testing.T) {
	root := authorRenamedInstall(t)
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv(settings.EnvPath, settingsPath)
	// Without a config a renamed layout is not recognised: the required
	// products are under ZGameDat, so the plain mount cannot start.
	if cs, err := openContent(Options{Root: root}); err == nil {
		cs.Close()
		t.Fatal("a renamed install mounted without a config, so something detected its layout")
	} else if !strings.Contains(err.Error(), "gamedata/moveinfo.tdf") {
		t.Fatalf("plain mount = %v, want the missing product named", err)
	}
	if _, err := os.Stat(settingsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mounting wrote settings: %v", err)
	}
	zero := shippedConfigPath(t, "ta-zero-alpha5-20241224")
	stored := settings.Defaults()
	stored.ContentProfile = zero
	if err := stored.Save(); err != nil {
		t.Fatal(err)
	}
	cs, err := openContent(Options{Root: root})
	if err != nil {
		t.Fatalf("saved config path: %v", err)
	}
	if cs.profile != "ta-zero" || cs.configPath != zero {
		t.Fatalf("saved config mounted %q from %q", cs.profile, cs.configPath)
	}
	cs.Close()

	// A removed built-in profile's name reads as no config, with the notice.
	base := t.TempDir()
	writeRetailGamedata(t, base)
	stored.ContentProfile = "zero"
	if err := stored.Save(); err != nil {
		t.Fatal(err)
	}
	cs, err = openContent(Options{Root: base})
	if err != nil {
		t.Fatal(err)
	}
	if cs.profile != contentprofiles.RetailName || !strings.Contains(cs.modNotice, `"zero" was removed`) {
		t.Fatalf("removed profile preference mounted %q with notice %q", cs.profile, cs.modNotice)
	}
	cs.Close()

	// The explicit file wins over the saved preference.
	stored.ContentProfile = filepath.Join(t.TempDir(), "absent.json")
	if err := stored.Save(); err != nil {
		t.Fatal(err)
	}
	if cs, err := openContent(Options{Root: base}); err == nil {
		cs.Close()
		t.Fatal("an unreadable saved config path mounted")
	}
	cs, err = openContent(Options{Root: root, ModConfig: zero})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if cs.profile != "ta-zero" {
		t.Fatalf("explicit config = %q", cs.profile)
	}
}

// A manual root stack names its config explicitly; without one it mounts as
// plain content and the main menu says so.
func TestManualStackWithoutAConfigShowsTheNotice(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	base, extra := t.TempDir(), t.TempDir()
	writeRetailGamedata(t, base)
	cs, err := openContent(Options{Roots: []string{base, extra}})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if !cs.manualRoots || cs.profile != contentprofiles.RetailName || !strings.Contains(cs.modNotice, "no Nanolathe config file") {
		t.Fatalf("manual stack mounted %q with notice %q", cs.profile, cs.modNotice)
	}
}

// writeRetailGamedata authors the two required retail-named products.
func writeRetailGamedata(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "gamedata"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"moveinfo.tdf", "sidedata.tdf"} {
		if err := os.WriteFile(filepath.Join(root, "gamedata", name), []byte("[TEST] { value=base; }"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestContentProfileRangePreferences(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	path := filepath.Join(t.TempDir(), "nanolathe-mod.json")
	// This authored install uses the Zero directory names; the config's
	// content section carries UI policy through the same mount boundary as
	// its layout.
	err := os.WriteFile(path, []byte(`{"schema":2,"id":"custom","name":"Custom","version":"1","content":{"layout":{"gamedata":"ZGameDat"},"presentation":{"show_ranges":true,"placement_weapon_ranges":false}}}`), 0600)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := openContent(Options{Roots: []string{authorRenamedInstall(t)}, ModConfig: path})
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if !cs.presentation.ShowRanges || cs.presentation.PlacementWeaponRanges == nil || *cs.presentation.PlacementWeaponRanges {
		t.Fatalf("config lost UI defaults: %+v", cs.presentation)
	}
}
