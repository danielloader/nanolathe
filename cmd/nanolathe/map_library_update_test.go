package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/maplibrary"
	"github.com/nanolathe-gg/nanolathe/internal/modfetch"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// updateFixtureTerrain is a 2x2 terrain whose first cell places feature, or
// no feature when it is empty.
func updateFixtureTerrain(feature string) []byte {
	terrain := make([]byte, 64+2+16+1024)
	for _, field := range []struct {
		offset int
		value  uint32
	}{{0, 0x2000}, {4, 2}, {8, 2}, {12, 64}, {16, 66}, {20, 82}, {24, 1}} {
		binary.LittleEndian.PutUint32(terrain[field.offset:], field.value)
	}
	for i := range 4 {
		binary.LittleEndian.PutUint16(terrain[67+i*4:], 0xffff)
	}
	if feature != "" {
		binary.LittleEndian.PutUint32(terrain[28:], 1)
		binary.LittleEndian.PutUint32(terrain[32:], uint32(len(terrain)))
		binary.LittleEndian.PutUint16(terrain[67:], 0)
		record := make([]byte, 132)
		copy(record[4:], feature)
		terrain = append(terrain, record...)
	}
	return terrain
}

type mapCatalogueFixture struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex
	files  map[string][]byte
	gets   atomic.Int32
}

func newMapCatalogueFixture(t *testing.T) *mapCatalogueFixture {
	f := &mapCatalogueFixture{t: t, files: map[string][]byte{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.gets.Add(1)
		f.mu.Lock()
		data, ok := f.files[r.URL.Path]
		f.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(f.server.Close)
	t.Setenv("NANOLATHE_MAP_CATALOG", f.server.URL+"/manifest.json")
	return f
}

// publish serves a package archive under name and returns its catalogue entry.
func (f *mapCatalogueFixture) publish(id, name, mapPath string, requires []string, files map[string][]byte) modfetch.Entry {
	f.t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	files[modlibrary.MetadataFile] = []byte(fmt.Sprintf(`{"schema":1,"id":%q,"name":%q,"version":"1"}`, id, id))
	for path, data := range files {
		w, err := z.Create(path)
		if err != nil {
			f.t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			f.t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.files["/"+name] = buf.Bytes()
	f.mu.Unlock()
	return modfetch.Entry{ID: id, Name: id, Version: "1", Map: mapPath, Requires: requires,
		Archive: modfetch.Archive{URL: f.server.URL + "/" + name, Size: int64(buf.Len()), SHA256: fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))}}
}

// mapUpdateShell mounts the fixture base like a desktop start, so the map
// library remounts through the ordinary front-end path.
func mapUpdateShell(t *testing.T) (*gameShell, *modlibrary.Library, string) {
	t.Helper()
	base, _ := modFixture(t)
	writeModFixtureFile(t, base, "features/z/base.tdf", "[base-tree]{metal=10;}")
	writeModFixtureFile(t, base, "maps/Stock.ota", `[GlobalHeader]{[Schema 0]{Type=Network 1;}}`)
	writeModFixtureFile(t, base, "maps/Stock.tnt", string(updateFixtureTerrain("")))
	root, err := maplibrary.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := maplibrary.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mountContent(Options{}, []string{base}, modSelection{})
	if err != nil {
		t.Fatal(err)
	}
	g := &gameShell{cs: cs, frontend: ui.NewFrontend(modeMenuMap), setup: newSkirmishMenuConfig("kept")}
	t.Cleanup(func() {
		mapDownload.cancelNetwork()
		for mapDownload.view().running {
			time.Sleep(time.Millisecond)
		}
		mapInstallsSeen, pendingMapReplacement = mapDownload.view().installs, nil
		mapDownload.acknowledge()
		g.releaseAudio()
		g.cs.Close()
	})
	mapInstallsSeen = mapDownload.view().installs
	return g, lib, base
}

// downloadMap runs one catalogue request through the worker and the next
// front-end poll, and returns the job's outcome.
func downloadMap(t *testing.T, g *gameShell, lib *modlibrary.Library, entry modfetch.Entry, dependencies []modfetch.Entry) string {
	t.Helper()
	if err := g.startMapDownload(lib, entry, dependencies); err != nil {
		t.Fatalf("start %s: %v", entry.ID, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for mapDownload.view().running {
		if time.Now().After(deadline) {
			t.Fatal("map download timeout")
		}
		time.Sleep(time.Millisecond)
	}
	g.pollMapsFetch()
	return mapDownload.view().outcome
}

func installedSHA(t *testing.T, lib *modlibrary.Library, id string) string {
	t.Helper()
	mod, ok, err := lib.Lookup(id, "1")
	if err != nil || !ok {
		t.Fatalf("%s is not installed: %v", id, err)
	}
	return mod.Receipt.SHA256
}

// A catalogue republish of an installed id@version, of a map or of a feature
// package it requires, is offered as an update and replaced between remounts.
// A failed replacement keeps the installed package mounted.
func TestMapSameVersionRepublishUpdatesInPlace(t *testing.T) {
	g, lib, _ := mapUpdateShell(t)
	f := newMapCatalogueFixture(t)
	ota := func(title string) []byte {
		return []byte(`[GlobalHeader]{missionname=` + title + `;[Schema 0]{Type=Network 1;}}`)
	}
	dependencyV1 := f.publish("up-features", "features-v1.zip", "", nil, map[string][]byte{"features/up/tree.tdf": []byte("[up-tree]{metal=5;}")})
	mapV1 := f.publish("up-map", "map-v1.zip", "maps/up.ota", []string{"up-features"}, map[string][]byte{"maps/up.ota": ota("Up one"), "maps/up.tnt": updateFixtureTerrain("up-tree")})
	if out := downloadMap(t, g, lib, mapV1, []modfetch.Entry{dependencyV1}); strings.HasPrefix(out, "Failed") {
		t.Fatal(out)
	}

	dependencyV2 := f.publish("up-features", "features-v2.zip", "", nil, map[string][]byte{"features/up/tree.tdf": []byte("[up-tree]{metal=6;}")})
	mapV2 := f.publish("up-map", "map-v2.zip", "maps/up.ota", []string{"up-features"}, map[string][]byte{"maps/up.ota": ota("Up two"), "maps/up.tnt": updateFixtureTerrain("up-tree")})
	installed, err := lib.Installed()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		entry modfetch.Entry
		deps  []modfetch.Entry
	}{{"map", mapV2, []modfetch.Entry{dependencyV1}}, {"dependency", mapV1, []modfetch.Entry{dependencyV2}}} {
		if row := catalogueRow(tc.entry, installed, tc.deps, false, nil); row.suffix != "update available" || row.action != "Update" {
			t.Fatalf("%s republish row = %+v", tc.name, row)
		}
	}
	if row := catalogueRow(mapV1, installed, []modfetch.Entry{dependencyV1}, false, nil); row.suffix != "installed" {
		t.Fatalf("current row = %+v", row)
	}

	if out := downloadMap(t, g, lib, mapV2, []modfetch.Entry{dependencyV2}); strings.HasPrefix(out, "Failed") {
		t.Fatalf("update: %s", out)
	}
	if installedSHA(t, lib, "up-map") != mapV2.Archive.SHA256 || installedSHA(t, lib, "up-features") != dependencyV2.Archive.SHA256 {
		t.Fatal("update kept the old packages")
	}
	if got, err := g.cs.fs.ReadFileLimit("maps/up.ota", 1024); err != nil || !bytes.Equal(got, ota("Up two")) {
		t.Fatalf("mounted map after update = %q, %v", got, err)
	}

	mapBroken := f.publish("up-map", "map-broken.zip", "maps/up.ota", []string{"up-features"}, map[string][]byte{"maps/up.ota": []byte("[GlobalHeader]{}"), "maps/up.tnt": updateFixtureTerrain("up-tree")})
	if out := downloadMap(t, g, lib, mapBroken, []modfetch.Entry{dependencyV2}); !strings.HasPrefix(out, "Failed") {
		t.Fatalf("broken update outcome = %q", out)
	}
	if installedSHA(t, lib, "up-map") != mapV2.Archive.SHA256 {
		t.Fatal("failed update replaced the installed package")
	}
	if got, err := g.cs.fs.ReadFileLimit("maps/up.ota", 1024); err != nil || !bytes.Equal(got, ota("Up two")) {
		t.Fatalf("failed update left the map unmounted: %q, %v", got, err)
	}
}

// A map is validated against the base, the selected mod and its own declared
// dependencies; another installed package's features never complete it.
func TestMapInstallDoesNotBorrowUnrelatedPackageFeatures(t *testing.T) {
	g, lib, _ := mapUpdateShell(t)
	f := newMapCatalogueFixture(t)
	lender := f.publish("lender", "lender.zip", "maps/lender.ota", nil, map[string][]byte{
		"maps/lender.ota": []byte(`[GlobalHeader]{[Schema 0]{Type=Network 1;}}`), "maps/lender.tnt": updateFixtureTerrain(""),
		"features/lend/rock.tdf": []byte("[lend-rock]{metal=2;}"),
	})
	if out := downloadMap(t, g, lib, lender, nil); strings.HasPrefix(out, "Failed") {
		t.Fatal(out)
	}
	if _, err := g.cs.fs.Stat("features/lend/rock.tdf"); err != nil {
		t.Fatal("lender package is not mounted")
	}
	borrower := f.publish("borrower", "borrower.zip", "maps/borrower.ota", nil, map[string][]byte{
		"maps/borrower.ota": []byte(`[GlobalHeader]{[Schema 0]{Type=Network 1;}}`), "maps/borrower.tnt": updateFixtureTerrain("lend-rock"),
	})
	if out := downloadMap(t, g, lib, borrower, nil); !strings.HasPrefix(out, "Failed") || !strings.Contains(out, "map feature is missing") {
		t.Fatalf("borrowing map outcome = %q", out)
	}
	if _, ok, _ := lib.Lookup("borrower", "1"); ok {
		t.Fatal("borrowing map was installed")
	}
}

// A catalogue map that the base install already supplies is refused before
// any download, and its row says so.
func TestMapAlreadyInInstallIsRefused(t *testing.T) {
	g, lib, _ := mapUpdateShell(t)
	f := newMapCatalogueFixture(t)
	copyEntry := f.publish("stock-copy", "stock.zip", "maps/stock.ota", nil, map[string][]byte{
		"maps/stock.ota": []byte(`[GlobalHeader]{[Schema 0]{Type=Network 1;}}`), "maps/stock.tnt": updateFixtureTerrain(""),
	})
	err := g.startMapDownload(lib, copyEntry, nil)
	if err == nil || !strings.Contains(err.Error(), maplibrary.ErrMapInInstall.Error()) || mapDownload.view().running {
		t.Fatalf("shadowed download = %v", err)
	}
	if f.gets.Load() != 0 {
		t.Fatal("a refused map was downloaded")
	}
	inInstall := g.mapInInstall(lib.Root, copyEntry.Map)
	if row := catalogueRow(copyEntry, nil, nil, inInstall, nil); row.suffix != "in your install" || !row.blocked {
		t.Fatalf("shadowed row = %+v", row)
	}
	if g.mapInInstall(lib.Root, "maps/absent.ota") {
		t.Fatal("an absent map counted as installed")
	}
}
