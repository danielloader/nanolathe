package install

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func fixtureHost(t *testing.T, platform string) host {
	t.Helper()
	base := t.TempDir()
	return host{home: filepath.Join(base, "home"), cwd: filepath.Join(base, "cwd"), executable: filepath.Join(base, "bin", "nanolathe"), platform: platform, getenv: func(string) string { return "" }}
}

func marker(t *testing.T, root, name string) string {
	t.Helper()
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), []byte("authored discovery marker, not an archive"), 0644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestOverridesPreserveOrderAndDoNotDiscover(t *testing.T) {
	h := fixtureHost(t, "linux")
	h.getenv = func(string) string { return "/environment" }
	explicit := []string{"second", "first", "second"}
	got, err := resolve(explicit, h)
	if err != nil || !reflect.DeepEqual(got, explicit) {
		t.Fatalf("resolve = %v, %v", got, err)
	}
	got[0] = "changed"
	if explicit[0] != "second" {
		t.Fatal("result aliases caller slice")
	}
	got, err = resolve(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{"/environment"}) {
		t.Fatalf("environment = %v, %v", got, err)
	}
	if _, err = resolve([]string{""}, h); err == nil {
		t.Fatal("empty override accepted")
	}
}

func TestDiscoveryCollectsRootsAndDeduplicatesAliases(t *testing.T) {
	h := fixtureHost(t, "linux")
	first := marker(t, filepath.Join(h.home, "Games", "custom name"), "ToTaLa1.HpI")
	second := marker(t, filepath.Join(h.home, "TotalAnnihilation"), "totala1.hpi")
	// The cwd alias is encountered later and must not move or repeat the first root.
	if err := os.Symlink(first, h.cwd); err != nil {
		t.Logf("symlink alias unavailable on this host: %v", err)
	}
	got, err := candidates(nil, h)
	want := []string{first, second}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, %v; want %v", got, err, want)
	}
	again, err := candidates(nil, h)
	if err != nil || !reflect.DeepEqual(got, again) {
		t.Fatalf("unstable result: %v, %v", again, err)
	}
}

func TestDiscoverySteamCustomLibraryAndProton(t *testing.T) {
	h := fixtureHost(t, "linux")
	steam := filepath.Join(h.home, ".local/share/Steam")
	library := filepath.Join(t.TempDir(), "custom library")
	first := marker(t, filepath.Join(steam, "steamapps/common/Renamed TA"), "totala1.hpi")
	second := marker(t, filepath.Join(library, "steamapps/common/Total Annihilation"), "TOTALA1.HPI")
	third := marker(t, filepath.Join(library, "steamapps/compatdata/fixture/pfx/drive_c/cavedog/totala"), "totala1.hpi")
	portable := marker(t, h.cwd, "totala1.hpi")
	home := marker(t, filepath.Join(h.home, "TotalAnnihilation"), "totala1.hpi")
	config := `"libraryfolders" { "0" { "path" ` + strconv.Quote(steam) + ` } "1" { "path" ` + strconv.Quote(library) + ` "apps" { "123" "999" } } }`
	if err := os.WriteFile(filepath.Join(steam, "steamapps/libraryfolders.vdf"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := candidates(nil, h)
	want := []string{first, second, third, portable, home}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, %v; want %v", got, err, want)
	}
}

func TestDiscoveryWineAndCrossOver(t *testing.T) {
	h := fixtureHost(t, "darwin")
	wine := filepath.Join(t.TempDir(), "custom prefix")
	h.getenv = func(key string) string {
		if key == "WINEPREFIX" {
			return wine
		}
		return ""
	}
	first := marker(t, filepath.Join(h.home, "Library/Application Support/CrossOver/Bottles/TA/drive_c/GOG Games/Total Annihilation"), "totala1.hpi")
	second := marker(t, filepath.Join(wine, "drive_c/Program Files (x86)/CAVEDOG/TOTALA"), "totala1.hpi")
	got, err := candidates(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{first, second}) {
		t.Fatalf("candidates = %v, %v", got, err)
	}
}

func TestDiscoveryMacAppWrappers(t *testing.T) {
	for _, layout := range []string{
		"Total Annihilation Commander Pack.app/Contents/Resources/drive_c/Program Files/GOG.com/Total Annihilation",
		"Total Annihilation Commander Pack/Total Annihilation.app/drive_c/GOG Games/Total Annihilation",
		"Total Annihilation Commander Pack/Contents/Resources/c_drive/Program Files/GOG.com/Total Annihilation",
		"Renamed.app/Contents/Resources/game/Total Annihilation.app/Contents/Resources/drive_c/Program Files/GOG.com/Total Annihilation",
		"Renamed.app/Contents/Resources/game/Total Annihilation.app/drive_c/Program Files/GOG.com/Total Annihilation",
		"Renamed.app/Contents/Resources/game/c_drive/program files/gog.com/total annihilation",
	} {
		t.Run(layout, func(t *testing.T) {
			for _, location := range []string{"system", "user"} {
				t.Run(location, func(t *testing.T) {
					h := fixtureHost(t, "darwin")
					applications := filepath.Join(t.TempDir(), "Applications")
					h.systemRoots = []string{applications}
					if location == "user" {
						applications = filepath.Join(h.home, "Applications")
					}
					root := marker(t, filepath.Join(applications, layout), "ToTaLa1.HpI")
					got, err := candidates(nil, h)
					if err != nil || !reflect.DeepEqual(got, []string{root}) {
						t.Fatalf("candidates = %v, %v; want %v", got, err, root)
					}
				})
			}
		})
	}
}

func TestDiscoveryMacAppAliasesAndPrecedence(t *testing.T) {
	h := fixtureHost(t, "darwin")
	applications := filepath.Join(t.TempDir(), "Applications")
	h.systemRoots = []string{applications}
	app := filepath.Join(applications, "Total Annihilation Commander Pack.app")
	first := marker(t, filepath.Join(app, "Contents/Resources/drive_c/GOG Games/Total Annihilation"), "totala1.hpi")
	if err := os.Symlink(filepath.Join(app, "Contents/Resources/drive_c"), filepath.Join(app, "drive_c")); err != nil {
		t.Skipf("symlink alias unavailable on this host: %v", err)
	}
	second := marker(t, filepath.Join(h.home, "Applications/TA.app/drive_c/Cavedog/TotalA"), "totala1.hpi")
	portable := marker(t, h.cwd, "totala1.hpi")
	want := []string{first, second, portable}
	for range 2 {
		got, err := candidates(nil, h)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("candidates = %v, %v; want %v", got, err, want)
		}
	}
}

func TestDiscoveryMacAppSearchIsBounded(t *testing.T) {
	h := fixtureHost(t, "darwin")
	applications := filepath.Join(h.home, "Applications")
	for _, layout := range []string{
		"Unrelated/deep/TA.app/drive_c/TotalA",
		"Unrelated/TA.app/drive_c/TotalA",
		"TA.app/Contents/Resources/unrelated/drive_c/TotalA",
		"TA.app/Contents/Resources/game/Nested.app/Contents/Resources/game/Deep.app/drive_c/TotalA",
	} {
		marker(t, filepath.Join(applications, layout), "totala1.hpi")
	}
	// Empty Wine wrappers and directories named like the archive are not games.
	if err := os.MkdirAll(filepath.Join(applications, "Empty.app/drive_c/TotalA/totala1.hpi"), 0755); err != nil {
		t.Fatal(err)
	}
	got, err := candidates(nil, h)
	if got != nil || err == nil {
		t.Fatalf("candidates = %v, %v; want no installation", got, err)
	}
}

func TestDiscoveryDoesNotRecurseAndExplainsMissingInstall(t *testing.T) {
	h := fixtureHost(t, "linux")
	marker(t, filepath.Join(h.home, "Games", "unrelated", "deep install"), "totala1.hpi")
	// A directory called totala1.hpi is not an installation marker.
	fake := filepath.Join(h.home, "TotalAnnihilation", "totala1.hpi")
	if err := os.MkdirAll(fake, 0755); err != nil {
		t.Fatal(err)
	}
	got, err := candidates(nil, h)
	if got != nil || err == nil {
		t.Fatalf("candidates = %v, %v", got, err)
	}
	for _, text := range []string{"--root", "providers searched", filepath.Join(h.home, "TotalAnnihilation")} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("diagnostic misses %q: %v", text, err)
		}
	}
}

func TestSteamMetadataAcceptsBothFormsAndRejectsTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "libraryfolders.vdf")
	root := t.TempDir()
	for _, data := range []string{`"LibraryFolders" { "1" ` + strconv.Quote(root) + ` }`, "// header\n\"libraryfolders\" {\"1\" {\"path\" " + strconv.Quote(root) + "}}"} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if got := libraryPaths(path); !reflect.DeepEqual(got, []string{root}) {
			t.Fatalf("paths = %v", got)
		}
	}
	for _, data := range []string{`"path" "unterminated`, strings.Repeat(" ", (1<<20)+1)} {
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
		if got := libraryPaths(path); len(got) != 0 {
			t.Fatalf("malformed metadata returned %v", got)
		}
	}
	tokens, ok := vdfTokens(`"path" "D:\\SteamLibrary"`)
	if !ok || !reflect.DeepEqual(tokens, []string{"path", `D:\SteamLibrary`}) {
		t.Fatalf("escaped path = %v, %v", tokens, ok)
	}
}

func TestMissingHomeDoesNotAddRelativeHomeSearches(t *testing.T) {
	h := fixtureHost(t, "linux")
	h.home = ""
	_, err := candidates(nil, h)
	if err == nil {
		t.Fatal("missing installation succeeded")
	}
	cwd, getErr := os.Getwd()
	if getErr != nil {
		t.Fatal(getErr)
	}
	if strings.Contains(err.Error(), cwd+string(filepath.Separator)) {
		t.Fatalf("home-derived path fell back to working directory: %v", err)
	}
}

func TestWindowsNativeHintsUseTheSameMarkerCheck(t *testing.T) {
	h := fixtureHost(t, "windows")
	drive := t.TempDir()
	h.systemRoots = []string{drive}
	registry := marker(t, filepath.Join(t.TempDir(), "registered copy"), "totala1.hpi")
	standard := marker(t, filepath.Join(drive, "GOG Games", "custom name"), "totala1.hpi")
	h.registryRoots = []string{registry, filepath.Join(t.TempDir(), "unrelated game")}
	got, err := candidates(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{registry, standard}) {
		t.Fatalf("candidates = %v, %v", got, err)
	}
}

// fakeInstaller lays out the source installer's directory around a release
// build (tools/installer/README.md "Files and shortcuts") and returns the
// executable path. rememberedName is root.txt on Windows, game-root elsewhere.
func fakeInstaller(t *testing.T, h *host, rememberedName, remembered string) string {
	t.Helper()
	base := filepath.Join(t.TempDir(), "Nanolathe")
	release := filepath.Join(base, "releases", "alpha-fixture")
	if err := os.MkdirAll(release, 0755); err != nil {
		t.Fatal(err)
	}
	h.executable = filepath.Join(release, "nanolathe.exe")
	if err := os.WriteFile(h.executable, nil, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, rememberedName), []byte(remembered), 0644); err != nil {
		t.Fatal(err)
	}
	return base
}

// Issue #24: a Steam copy and a separately installed mod distribution that
// ships its own totala1.hpi were both detected and overlaid, so the later one
// shadowed the installation the launcher had selected. Discovery now mounts
// one installation, and a directly launched installed build uses the folder
// the launcher remembered even when discovery would prefer another.
func TestResolveMountsOneRootAndHonoursInstallerChoice(t *testing.T) {
	h := fixtureHost(t, "windows")
	drive := t.TempDir()
	h.systemRoots = []string{drive}
	steam := filepath.Join(t.TempDir(), "Steam")
	h.registrySteam = []string{steam}
	chosen := marker(t, filepath.Join(steam, "steamapps/common/Total Annihilation"), "totala1.hpi")
	// A renamed folder is still found by the one-level collection scan.
	other := marker(t, filepath.Join(drive, "Games", "ter"), "TOTALA1.HPI")
	registered := marker(t, filepath.Join(t.TempDir(), "mod install"), "totala1.hpi")
	h.registryRoots = []string{registered}

	all, err := candidates(nil, h)
	if want := []string{registered, chosen, other}; err != nil || !reflect.DeepEqual(all, want) {
		t.Fatalf("candidates = %v, %v; want %v", all, err, want)
	}
	got, err := resolve(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{registered}) {
		t.Fatalf("discovery without a remembered folder = %v, %v; want only the first candidate", got, err)
	}

	fakeInstaller(t, &h, "root.txt", "\ufeff"+chosen)
	got, err = resolve(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{chosen}) {
		t.Fatalf("installed build = %v, %v; want the remembered %s", got, err, chosen)
	}
	// Listing for the installer's own selection is unaffected by the memory.
	if all, err = candidates(nil, h); err != nil || len(all) != 3 {
		t.Fatalf("candidates with a remembered folder = %v, %v", all, err)
	}
	// Explicit and environment overrides still outrank the remembered folder.
	if got, err = resolve([]string{other}, h); err != nil || !reflect.DeepEqual(got, []string{other}) {
		t.Fatalf("explicit = %v, %v", got, err)
	}
	h.getenv = func(key string) string {
		if key == "NANOLATHE_TA_ROOT" {
			return other
		}
		return ""
	}
	if got, err = resolve(nil, h); err != nil || !reflect.DeepEqual(got, []string{other}) {
		t.Fatalf("environment = %v, %v", got, err)
	}
}

func TestInstallerRootIgnoresUnusableMemory(t *testing.T) {
	h := fixtureHost(t, "linux")
	found := marker(t, filepath.Join(h.home, "TotalAnnihilation"), "totala1.hpi")
	empty := t.TempDir()
	for _, remembered := range []string{"", "relative/folder\n", empty + "\n", filepath.Join(empty, "moved") + "\n", found + "\n" + empty + "\n"} {
		fakeInstaller(t, &h, "game-root", remembered)
		got, err := resolve(nil, h)
		if err != nil || !reflect.DeepEqual(got, []string{found}) {
			t.Fatalf("remembered %q = %v, %v; want discovery %v", remembered, got, err, found)
		}
	}
	// The Unix entry point runs the release through the base's current link.
	chosen := marker(t, filepath.Join(t.TempDir(), "chosen"), "totala1.hpi")
	base := fakeInstaller(t, &h, "game-root", chosen+"\n")
	current := filepath.Join(base, "current")
	if err := os.Symlink(filepath.Dir(h.executable), current); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	// Make the release directory itself unrecognisable, so only the link is.
	h.executable = filepath.Join(current, "nanolathe")
	if err := os.Rename(filepath.Join(base, "releases"), filepath.Join(base, "builds")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "builds", "alpha-fixture"), current); err != nil {
		t.Fatal(err)
	}
	got, err := resolve(nil, h)
	if err != nil || !reflect.DeepEqual(got, []string{chosen}) {
		t.Fatalf("current link = %v, %v; want %v", got, err, chosen)
	}
}
