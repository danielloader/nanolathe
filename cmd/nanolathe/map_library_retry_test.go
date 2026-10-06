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
	"sync/atomic"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/maplibrary"
	"github.com/nanolathe-gg/nanolathe/internal/modfetch"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
)

// A dependency may publish before the map fails validation. Retrying must
// reuse that verified dependency without requiring a render-thread remount.
func TestMapRetryUsesAlreadyInstalledUnmountedDependency(t *testing.T) {
	base, _ := modFixture(t)
	// Same-path base content must still beat a dependency's fallback asset.
	writeModFixtureFile(t, base, "features/shared/base.tdf", "[base-tree]{metal=10;}")
	root, err := maplibrary.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	lib, err := maplibrary.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	terrain := make([]byte, 64+2+16+1024+132)
	for _, field := range []struct {
		offset int
		value  uint32
	}{{0, 0x2000}, {4, 2}, {8, 2}, {12, 64}, {16, 66}, {20, 82}, {24, 1}, {28, 1}, {32, 64 + 2 + 16 + 1024}} {
		binary.LittleEndian.PutUint32(terrain[field.offset:], field.value)
	}
	for i := range 4 {
		binary.LittleEndian.PutUint16(terrain[67+i*4:], 0xffff)
	}
	binary.LittleEndian.PutUint16(terrain[67:], 0)
	copy(terrain[64+2+16+1024+4:], "retry-tree")
	archive := func(id string, files map[string][]byte) []byte {
		t.Helper()
		var buf bytes.Buffer
		z := zip.NewWriter(&buf)
		files[modlibrary.MetadataFile] = []byte(fmt.Sprintf(`{"schema":1,"id":%q,"name":%q,"version":"1"}`, id, id))
		for name, data := range files {
			f, err := z.Create(name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	dependencyBytes := archive("retry-features", map[string][]byte{
		"features/retry/tree.tdf":  []byte("[retry-tree]{metal=5;}"),
		"features/shared/base.tdf": []byte("[base-tree]{metal=20;}"),
	})
	brokenBytes := archive("retry-map", map[string][]byte{"maps/retry.ota": []byte("[GlobalHeader]{}"), "maps/retry.tnt": terrain})
	validBytes := archive("retry-map", map[string][]byte{"maps/retry.ota": []byte("[GlobalHeader]{[Schema 0]{Type=Network 1;}}"), "maps/retry.tnt": terrain})
	var dependencyRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/features.zip":
			dependencyRequests.Add(1)
			_, _ = w.Write(dependencyBytes)
		case "/broken.zip":
			_, _ = w.Write(brokenBytes)
		case "/valid.zip":
			_, _ = w.Write(validBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer mapDownload.cancelNetwork()
	t.Setenv("NANOLATHE_MAP_CATALOG", server.URL+"/manifest.json")
	makeEntry := func(id, name string, data []byte) modfetch.Entry {
		return modfetch.Entry{ID: id, Name: id, Version: "1", Archive: modfetch.Archive{URL: server.URL + "/" + name, Size: int64(len(data)), SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}}
	}
	dependency := makeEntry("retry-features", "features.zip", dependencyBytes)
	broken := makeEntry("retry-map", "broken.zip", brokenBytes)
	broken.Map, broken.Requires = "maps/retry.ota", []string{dependency.ID}
	valid := makeEntry("retry-map", "valid.zip", validBytes)
	valid.Map, valid.Requires = broken.Map, broken.Requires
	g := &gameShell{cs: &contentSet{roots: []string{base}, baseRoots: []string{base}}, setup: newSkirmishMenuConfig("kept")}
	wait := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for mapDownload.view().running {
			if time.Now().After(deadline) {
				t.Fatal("map download timeout")
			}
			time.Sleep(time.Millisecond)
		}
	}
	installsBefore := mapDownload.view().installs
	t.Cleanup(func() { mapInstallsSeen = mapDownload.view().installs; mapDownload.acknowledge() })
	if err := g.startMapDownload(lib, broken, []modfetch.Entry{dependency}); err != nil {
		t.Fatal(err)
	}
	wait()
	if got := mapDownload.view(); !strings.HasPrefix(got.outcome, "Failed:") || got.installs != installsBefore {
		t.Fatalf("first request unexpectedly succeeded: %+v", got)
	}
	installed, err := lib.Installed()
	if err != nil || len(installed) != 1 || installed[0].ID != dependency.ID {
		t.Fatalf("partially installed dependency: %+v, %v", installed, err)
	}
	if len(g.cs.roots) != 1 || g.cs.roots[0] != base {
		t.Fatal("worker remounted the live shell")
	}
	if err := g.startMapDownload(lib, valid, []modfetch.Entry{dependency}); err != nil {
		t.Fatal(err)
	}
	wait()
	got := mapDownload.view()
	if got.installs != installsBefore+1 {
		t.Fatalf("retry did not reuse installed feature definitions: %s", got.outcome)
	}
	if dependencyRequests.Load() != 1 {
		t.Fatalf("dependency downloaded %d times", dependencyRequests.Load())
	}
	if len(g.cs.roots) != 1 || g.cs.roots[0] != base || g.setup.MapName != "kept" {
		t.Fatal("worker changed the live shell or selection")
	}
	mapDownload.acknowledge()
}
