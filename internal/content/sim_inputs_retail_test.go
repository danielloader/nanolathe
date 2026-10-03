//go:build retail

package content

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

const frozenRetailMap = "ashap plateau"

func frozenRetailRequest(t *testing.T, cat *Catalog) SimulationInputRequest {
	t.Helper()
	header := cat.Maps[CanonicalKey(frozenRetailMap)]
	if header == nil {
		t.Fatalf("reference install has no %q", frozenRetailMap)
	}
	return SimulationInputRequest{Catalog: cat, MapOTA: header.LogicalOTA, MapTNT: header.LogicalTNT, AIProfile: "default"}
}

// A catalog compiled through a capture is the catalog the live mount
// compiles: Catalog.Hash and its recorded Manifest are unchanged, because the
// capture answers every loader exactly as the overlay did, retail directory
// order and byte ranges included. Frozen from the capture's own compile or
// from the shared compile of the same install, the identity is one value.
func TestCaptureCompilesTheSameCatalogRetail(t *testing.T) {
	base := compiledRetailCatalog(t)
	live := mountRetail(t)
	defer live.Close()

	sources, err := CaptureSimulationSources(live, frozenRetailMap)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	captured, err := Compile(sources.Filesystem())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("catalog compile through the capture: %v", time.Since(start).Round(time.Millisecond))
	if captured.Hash != base.Hash {
		t.Fatalf("capture compiled Catalog.Hash %s, live %s", captured.Hash, base.Hash)
	}
	if captured.Manifest != base.Manifest {
		t.Fatalf("capture recorded Manifest %q, live %q", captured.Manifest, base.Manifest)
	}

	start = time.Now()
	own, err := FreezeSimulationInputs(sources, frozenRetailRequest(t, captured))
	if err != nil {
		t.Fatalf("freeze the capture's own compile: %v", err)
	}
	t.Logf("freeze after a capture compile: %v, %d manifest entries", time.Since(start).Round(time.Millisecond), len(own.Manifest()))

	again, err := CaptureSimulationSources(live, frozenRetailMap)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	shared, err := FreezeSimulationInputs(again, frozenRetailRequest(t, base))
	if err != nil {
		t.Fatalf("freeze the shared compile of the same install: %v", err)
	}
	t.Logf("freeze of a supplied catalog: %v", time.Since(start).Round(time.Millisecond))
	if own.Digest() != shared.Digest() {
		t.Fatal("one install's content froze to two identities")
	}
	if got := shared.UncapturedLookups(); len(got) != 0 {
		t.Fatalf("freeze left uncaptured lookups: %v", got)
	}
	for _, e := range shared.Manifest() {
		if e.Family == SimulationFamilyCOB && e.Presence == SimulationInputAbsent {
			t.Fatalf("a stock unit has no admitted program: %+v", e)
		}
	}
}

// Identity is content, not provenance: a second install whose files come
// from other providers — one model shadowed by a byte-identical loose copy —
// and which carries unrelated extra files freezes to the same identity, while
// its provenance reports the different provider.
func TestFrozenIdentityIgnoresProvenanceRetail(t *testing.T) {
	base := compiledRetailCatalog(t)
	live := mountRetail(t)
	defer live.Close()
	commander := base.Units[CanonicalKey(base.Sides[0].Commander)]
	modelPath := frozenUnitModelPath(commander.ObjectName)
	data, err := live.ReadFileLimit(modelPath, 1<<22)
	if err != nil {
		t.Fatal(err)
	}
	overlay := t.TempDir()
	for logical, bytes := range map[string][]byte{modelPath: data, "notes/unrelated.txt": []byte("not content")} {
		full := filepath.Join(overlay, filepath.FromSlash(logical))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, bytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	other := vfs.New()
	defer other.Close()
	if err := other.MountGameDirectory(retailRoot(t)); err != nil {
		t.Fatal(err)
	}
	if err := other.MountDirectory(overlay, 1000); err != nil {
		t.Fatal(err)
	}

	first, _ := CaptureSimulationSources(live, frozenRetailMap)
	a, err := FreezeSimulationInputs(first, frozenRetailRequest(t, base))
	if err != nil {
		t.Fatal(err)
	}
	second, _ := CaptureSimulationSources(other, frozenRetailMap)
	b, err := FreezeSimulationInputs(second, frozenRetailRequest(t, base))
	if err != nil {
		t.Fatal(err)
	}
	if a.Digest() != b.Digest() {
		t.Fatal("a byte-identical file from another provider changed the identity")
	}
	provider := func(inputs *SimulationInputs) string {
		for _, p := range inputs.Provenance() {
			if p.Family == SimulationFamilyModel && p.Key == modelPath && len(p.Files) == 1 {
				return p.Files[0].ProviderType
			}
		}
		return ""
	}
	if provider(a) != "hpi" || provider(b) != "directory" {
		t.Fatalf("provenance providers %q and %q, want the archive and the loose copy", provider(a), provider(b))
	}
}
