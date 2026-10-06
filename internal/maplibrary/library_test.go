package maplibrary

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func terrain(feature string) []byte {
	data := make([]byte, 64+2+16+1024)
	for _, field := range []struct {
		off   int
		value uint32
	}{{0, 0x2000}, {4, 2}, {8, 2}, {12, 64}, {16, 66}, {20, 82}, {24, 1}} {
		binary.LittleEndian.PutUint32(data[field.off:], field.value)
	}
	for i := range 4 {
		binary.LittleEndian.PutUint16(data[66+i*4+1:], 0xffff)
	}
	if feature != "" {
		binary.LittleEndian.PutUint16(data[67:], 0)
		binary.LittleEndian.PutUint32(data[28:], 1)
		binary.LittleEndian.PutUint32(data[32:], uint32(len(data)))
		record := make([]byte, 132)
		copy(record[4:], feature)
		data = append(data, record...)
	}
	return data
}

func mapFiles() map[string][]byte {
	return map[string][]byte{"maps/authored.ota": []byte("[GlobalHeader]{[Schema 0]{type=Network 1;}}"), "maps/authored.tnt": terrain("")}
}

func fixtureRoot(t *testing.T, id, name string, files map[string][]byte) (string, modlibrary.Metadata) {
	t.Helper()
	root := t.TempDir()
	meta := modlibrary.Metadata{Schema: 1, ID: id, Name: name, Version: "1"}
	raw, _ := json.Marshal(meta)
	writeFile(t, root, modlibrary.MetadataFile, raw)
	for name, data := range files {
		writeFile(t, root, name, data)
	}
	return root, meta
}
func writeFile(t *testing.T, root, name string, data []byte) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		t.Fatal(err)
	}
}
func hpi(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var out bytes.Buffer
	var entries []vfs.ArchiveFile
	for name, data := range files {
		entries = append(entries, vfs.ArchiveFile{Path: name, Data: data})
	}
	if err := vfs.WriteArchive(&out, entries, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestValidatorRefusesNonMapPayloads(t *testing.T) {
	for _, name := range []string{"units/unit.fbi", "gamedata/moveinfo.tdf", "ai/default.tdf", "scripts/tree.cob", "anims/setup.exe", "anims/vismasks.gaf", "features/policy.json", "launcher.ini", "maps/sub/map.ota"} {
		for _, container := range []bool{false, true} {
			t.Run(name+map[bool]string{false: " loose", true: " HPI"}[container], func(t *testing.T) {
				files := mapFiles()
				files[name] = []byte("forbidden")
				if container {
					files = map[string][]byte{"map.ufo": hpi(t, files)}
				}
				root, meta := fixtureRoot(t, "map", "Map", files)
				if err := Validator(nil, "maps/authored.ota")(root, meta); err == nil {
					t.Fatal("accepted forbidden payload")
				}
			})
		}
	}
}

func TestValidatorRequiresIdentityAndValidPairs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(string, *modlibrary.Metadata)
	}{
		{"schema2", func(root string, m *modlibrary.Metadata) { m.Schema = 2 }},
		{"legacy config", func(root string, m *modlibrary.Metadata) {
			m.Controls = "retail"
			raw, _ := json.Marshal(m)
			writeFile(t, root, modlibrary.MetadataFile, raw)
		}},
		{"bad archive", func(root string, m *modlibrary.Metadata) { writeFile(t, root, "invalid.ufo", []byte("bad HPI")) }},
		{"bad OTA", func(root string, m *modlibrary.Metadata) {
			writeFile(t, root, "maps/authored.ota", []byte("[Empty]{}"))
		}},
		{"bad TNT", func(root string, m *modlibrary.Metadata) { writeFile(t, root, "maps/authored.tnt", []byte("bad TNT")) }},
		{"missing TNT", func(root string, m *modlibrary.Metadata) { os.Remove(filepath.Join(root, "maps/authored.tnt")) }},
		{"not skirmish", func(root string, m *modlibrary.Metadata) {
			writeFile(t, root, "maps/authored.ota", []byte("[GlobalHeader]{[Schema 0]{type=Easy;}}"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, meta := fixtureRoot(t, "map", "Map", mapFiles())
			tc.mutate(root, &meta)
			if err := Validator(nil, "maps/authored.ota")(root, meta); err == nil {
				t.Fatal("accepted invalid map")
			}
		})
	}
	root, meta := fixtureRoot(t, "map", "Map", map[string][]byte{"map.ufo": hpi(t, mapFiles())})
	if err := Validator(nil, "maps/authored.ota")(root, meta); err != nil {
		t.Fatal(err)
	}
	if err := Validator(nil, "")(root, meta); err == nil {
		t.Fatal("dependency accepted maps")
	}
	if err := Validator(nil, "maps/absent.ota")(root, meta); err == nil {
		t.Fatal("absent map accepted")
	}
}

func TestMapFeaturesResolveDependenciesAndPreserveBase(t *testing.T) {
	base, _ := fixtureRoot(t, "base", "Base", map[string][]byte{"features/z/base.tdf": []byte("[tree]{metal=10;}")})
	root, meta := fixtureRoot(t, "map", "Map", mapFiles())
	writeFile(t, root, "maps/authored.tnt", terrain("tree"))
	if err := Validator(nil, "maps/authored.ota")(root, meta); err == nil {
		t.Fatal("missing feature accepted")
	}
	if err := Validator([]string{base}, "maps/authored.ota")(root, meta); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "features/a/override.tdf", []byte("[tree]{metal=20;}"))
	if err := ValidateRoots([]string{root}, []string{base}); err == nil {
		t.Fatal("mount-time collision admitted")
	}

	if err := Validator([]string{base}, "maps/authored.ota")(root, meta); err == nil || !strings.Contains(err.Error(), "changes an existing feature") {
		t.Fatalf("base name collision: %v", err)
	}
	// A later selected mod can introduce a collision absent at installation.
	other, _ := fixtureRoot(t, "other", "Other", map[string][]byte{"features/z/new.tdf": []byte("[new-feature]{metal=1;}")})
	clean, _ := fixtureRoot(t, "clean", "Clean", map[string][]byte{"features/a/new.tdf": []byte("[new-feature]{metal=2;}")})
	if err := ValidateRoots([]string{clean}, []string{base}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRoots([]string{clean}, []string{base, other}); err == nil {
		t.Fatal("new mod collision admitted")
	}
	// Same-path shadowing must leave the existing base feature in control.
	os.RemoveAll(filepath.Join(root, "features"))
	writeFile(t, root, "features/z/base.tdf", []byte("[tree]{metal=20;}"))
	if err := Validator([]string{base}, "maps/authored.ota")(root, meta); err != nil {
		t.Fatal(err)
	}
}

func TestRootsAreOfflineDependencyFirstAndDoNotCreate(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	if roots, skipped, err := Roots(root); err != nil || len(roots) != 0 || len(skipped) != 0 {
		t.Fatalf("absent: %v %v %v", roots, skipped, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("Roots created missing library")
	}
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	mapRoot, _ := fixtureRoot(t, "map", "A Map", mapFiles())
	dependency, _ := fixtureRoot(t, "features", "Z Features", map[string][]byte{"features/authored.tdf": []byte("[tree]{}")})
	m, err := lib.InstallDirectory(mapRoot, modlibrary.InstallOptions{Validate: Validator(nil, "maps/authored.ota")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := lib.InstallDirectory(dependency, modlibrary.InstallOptions{Validate: Validator(nil, "")})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, skipped, err := Roots(root)
		if err != nil || len(skipped) != 0 || !reflect.DeepEqual(got, []string{d.Dir, m.Dir}) {
			t.Fatalf("roots: %v %v %v", got, skipped, err)
		}
	}
	// A damaged package is left out alone and named; the library still mounts.
	writeFile(t, m.Dir, "units/after-install.fbi", []byte("bad"))
	got, skipped, err := Roots(root)
	if err != nil || !reflect.DeepEqual(got, []string{d.Dir}) || len(skipped) != 1 || skipped[0].Dir != m.Dir ||
		!strings.Contains(skipped[0].Err.Error(), "non-map content") {
		t.Fatalf("edited installed payload: roots %v skipped %+v err %v", got, skipped, err)
	}
}

// Host file-manager litter is not package content: the mount-time audit
// ignores it without letting it pose as a map, while installation of a
// hash-pinned archive stays strict.
func TestRootsIgnoreFileManagerClutter(t *testing.T) {
	root := t.TempDir()
	lib, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := fixtureRoot(t, "map", "A Map", mapFiles())
	m, err := lib.InstallDirectory(src, modlibrary.InstallOptions{Validate: Validator(nil, "maps/authored.ota")})
	if err != nil {
		t.Fatal(err)
	}
	clutter := []string{".DS_Store", "maps/.DS_Store", "Thumbs.db", "textures/Thumbs.db", "desktop.ini", "._x", "maps/._authored.ota", "features/._trees.tdf", "._map.ufo", "__MACOSX/maps/._authored.tnt"}
	for _, name := range clutter {
		writeFile(t, m.Dir, name, []byte{0, 5, 22, 7, 0, 2, 0, 0})
	}
	got, skipped, err := Roots(root)
	if err != nil || len(skipped) != 0 || !reflect.DeepEqual(got, []string{m.Dir}) {
		t.Fatalf("clutter refused the package: roots %v skipped %+v err %v", got, skipped, err)
	}
	// The clutter leaves the package a map package, not a dependency.
	_, paths, err := inspect(m.Dir, true)
	if err != nil || !reflect.DeepEqual(paths, []string{"maps/authored.ota"}) {
		t.Fatalf("clutter changed the map paths: %v %v", paths, err)
	}
	for _, name := range []string{"Thumbs.db", "maps/._authored.ota"} {
		staged, _ := fixtureRoot(t, "map", "A Map", mapFiles())
		writeFile(t, staged, name, []byte("clutter"))
		if err := Validator(nil, "maps/authored.ota")(staged, modlibrary.Metadata{Schema: 1, ID: "map", Name: "A Map", Version: "1"}); err == nil {
			t.Fatalf("installation admitted %s", name)
		}
	}
}

// One package that no longer validates against the current base and mod stack
// is left out; packages mounted before and after it are kept.
func TestSelectRootsSkipsOnlyFailingPackages(t *testing.T) {
	base, _ := fixtureRoot(t, "base", "Base", map[string][]byte{"features/z/base.tdf": []byte("[tree]{metal=10;}")})
	good, _ := fixtureRoot(t, "good", "Good", map[string][]byte{"features/m/good.tdf": []byte("[shrub]{metal=1;}")})
	collides, _ := fixtureRoot(t, "collides", "Collides", map[string][]byte{"features/a/override.tdf": []byte("[tree]{metal=20;}")})
	later, _ := fixtureRoot(t, "later", "Later", mapFiles())
	if kept, skipped := SelectRoots([]string{good, later}, []string{base}); len(skipped) != 0 || !reflect.DeepEqual(kept, []string{good, later}) {
		t.Fatalf("clean set: %v %+v", kept, skipped)
	}
	kept, skipped := SelectRoots([]string{good, collides, later}, []string{base})
	if !reflect.DeepEqual(kept, []string{good, later}) || len(skipped) != 1 || skipped[0].Dir != collides ||
		!strings.Contains(skipped[0].Err.Error(), "changes an existing feature") {
		t.Fatalf("collision: kept %v skipped %+v", kept, skipped)
	}
	if kept, skipped := SelectRoots(nil, []string{base}); kept != nil || skipped != nil {
		t.Fatalf("empty: %v %+v", kept, skipped)
	}
}

func packageZIP(t *testing.T, files map[string][]byte) (string, modlibrary.InstallOptions) {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	meta := modlibrary.Metadata{Schema: 1, ID: "map", Name: "Map", Version: "1"}
	raw, _ := json.Marshal(meta)
	files[modlibrary.MetadataFile] = raw
	for name, data := range files {
		out, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "map.zip")
	if err := os.WriteFile(file, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return file, modlibrary.InstallOptions{SHA256: hex.EncodeToString(sum[:]), Size: int64(buf.Len()), ExpectIdentity: &modlibrary.ExpectedIdentity{ID: "map", Version: "1"}, Replace: true, Validate: Validator(nil, "maps/authored.ota")}
}

func TestFailedMapReplacementPreservesInstalledBytes(t *testing.T) {
	lib, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file, opts := packageZIP(t, mapFiles())
	old, err := lib.InstallArchive(file, opts)
	if err != nil {
		t.Fatal(err)
	}
	bad := mapFiles()
	bad["maps/authored.tnt"] = []byte("broken")
	file, opts = packageZIP(t, bad)
	if _, err := lib.InstallArchive(file, opts); err == nil {
		t.Fatal("invalid replacement installed")
	}
	current, ok, err := lib.Lookup("map", "1")
	if err != nil || !ok || current.Receipt.SHA256 != old.Receipt.SHA256 {
		t.Fatalf("receipt changed: %+v %v", current, err)
	}
	got, err := os.ReadFile(filepath.Join(current.Dir, "maps/authored.tnt"))
	if err != nil || !bytes.Equal(got, terrain("")) {
		t.Fatal("old terrain lost")
	}
	staging, err := os.ReadDir(lib.StagingDir())
	if err != nil || len(staging) != 0 {
		t.Fatal("failed staging was retained")
	}
}

// Package admission follows actual placement: inert names must not turn a
// playable map into a refused package, while a placed missing feature must.
func TestValidatorChecksPlacedOTAAndReferencedTerrainFeatures(t *testing.T) {
	root, meta := fixtureRoot(t, "map", "Map", mapFiles())
	unused := terrain("missing")
	binary.LittleEndian.PutUint16(unused[67:], 0xffff)
	writeFile(t, root, "maps/authored.tnt", unused)
	if err := Validator(nil, "maps/authored.ota")(root, meta); err != nil {
		t.Fatalf("unused TNT record: %v", err)
	}
	for _, tc := range []struct {
		name, schemas string
		wantMissing   bool
	}{
		{"network placed", `[Schema 0]{type=Network 1;[features]{[anything]{Featurename=missing;XPos=0;ZPos=0;}}}`, true},
		{"network negative", `[Schema 0]{type=Network 1;[features]{[anything]{Featurename=missing;XPos=-1;ZPos=0;}}}`, false},
		{"network coordinate omitted", `[Schema 0]{type=Network 1;[features]{[anything]{Featurename=missing;ZPos=0;}}}`, false},
		{"campaign only placement", `[Schema 0]{type=Network 1;}[Schema 1]{type=Easy;[features]{[anything]{Featurename=missing;XPos=0;ZPos=0;}}}`, false},
		{"later network schema", `[Schema 0]{type=Network 1;}[Schema 1]{type=Network 2;[features]{[anything]{Featurename=missing;XPos=0;ZPos=0;}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, root, "maps/authored.ota", []byte("[GlobalHeader]{"+tc.schemas+"}"))
			err := Validator(nil, "maps/authored.ota")(root, meta)
			if (err != nil) != tc.wantMissing {
				t.Fatalf("validation = %v; want missing %v", err, tc.wantMissing)
			}
			if err != nil && !strings.Contains(err.Error(), "map feature is missing") {
				t.Fatal(err)
			}
		})
	}
	writeFile(t, root, "features/missing.tdf", []byte("[missing]{}"))
	if err := Validator(nil, "maps/authored.ota")(root, meta); err != nil {
		t.Fatalf("supplied OTA feature: %v", err)
	}
}

// A catalogue install resolves features only from the base stack and its own
// declared dependencies. Another installed map package can neither complete
// it nor have its existing feature definitions changed by it.
func TestInstallValidatorIgnoresUnrelatedPackages(t *testing.T) {
	other, _ := fixtureRoot(t, "other", "Other", map[string][]byte{"features/o/rock.tdf": []byte("[rock]{metal=3;}")})
	dependency, _ := fixtureRoot(t, "rocks", "Rocks", map[string][]byte{"features/d/rock.tdf": []byte("[rock]{metal=3;}")})
	root, meta := fixtureRoot(t, "map", "Map", mapFiles())
	writeFile(t, root, "maps/authored.tnt", terrain("rock"))
	if err := InstallValidator(nil, []string{other}, "maps/authored.ota")(root, meta); err == nil || !strings.Contains(err.Error(), "map feature is missing") {
		t.Fatalf("an unrelated installed package completed the map: %v", err)
	}
	if err := InstallValidator([]string{dependency}, []string{other}, "maps/authored.ota")(root, meta); err != nil {
		t.Fatalf("declared dependency: %v", err)
	}
	clean, cleanMeta := fixtureRoot(t, "clean", "Clean", mapFiles())
	writeFile(t, clean, "features/a/pebble.tdf", []byte("[pebble]{metal=9;}"))
	installed, _ := fixtureRoot(t, "installed", "Installed", map[string][]byte{"features/o/pebble.tdf": []byte("[pebble]{metal=3;}")})
	if err := InstallValidator(nil, nil, "maps/authored.ota")(clean, cleanMeta); err != nil {
		t.Fatal(err)
	}
	if err := InstallValidator(nil, []string{installed}, "maps/authored.ota")(clean, cleanMeta); err == nil || !strings.Contains(err.Error(), "changes an existing feature") {
		t.Fatalf("changed an installed package's feature: %v", err)
	}
}

// The base stack wins a shared logical path, so a package whose map it already
// supplies is refused rather than installed in its shadow.
func TestValidatorRefusesMapAlreadyInBase(t *testing.T) {
	root, meta := fixtureRoot(t, "map", "Map", mapFiles())
	base, _ := fixtureRoot(t, "base", "Base", map[string][]byte{"maps/Authored.OTA": []byte("[GlobalHeader]{[Schema 0]{type=Network 1;}}"), "maps/Authored.TNT": terrain("")})
	if err := Validator([]string{base}, "maps/authored.ota")(root, meta); !errors.Is(err, ErrMapInInstall) {
		t.Fatalf("shadowed map: %v", err)
	}
	unrelated, _ := fixtureRoot(t, "base", "Base", map[string][]byte{"maps/other.ota": []byte("[GlobalHeader]{[Schema 0]{type=Network 1;}}"), "maps/other.tnt": terrain("")})
	if err := Validator([]string{unrelated}, "maps/authored.ota")(root, meta); err != nil {
		t.Fatal(err)
	}
}
