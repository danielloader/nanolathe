package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestRegistryUsesAuthoredPathAndRetainsFailedIdentity(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "camps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "camps", "voice.wav"), buildRIFF(1, 11025, 8, []byte{128, 129}), 0644); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	r := NewRegistry(fs)
	id := r.RegisterPath("VOICE", "camps/voice.wav")
	if id != 0 || r.Count() != 1 {
		t.Fatalf("registration id=%d count=%d", id, r.Count())
	}
	s, err := r.Load(id)
	if err != nil || s == nil {
		t.Fatalf("authored path load sample=%v err=%v", s, err)
	}
	if s.Provenance.LogicalPath != "camps/voice.wav" {
		t.Fatalf("authored path provenance=%+v", s.Provenance)
	}
	missing := r.RegisterPath("MISSING", "camps/no-file.wav")
	if missing != 1 || r.Count() != 2 {
		t.Fatalf("failed probe must consume identity id=%d count=%d", missing, r.Count())
	}
	if _, err := r.Load(missing); err == nil {
		t.Fatal("missing registered path should remain silent/error on load")
	}
}

// A full table returns its first real sound, while duplicate registration still
// resolves the existing slot before the capacity check [03 §8.3].
func TestRegistryFirstDuplicateAndFullIdentity(t *testing.T) {
	r := NewRegistry(nil)
	if _, ok := r.Entry(0); ok {
		t.Fatal("empty registry has a first entry")
	}
	for i := 0; i < maxAliases; i++ {
		if got := r.Register(fmt.Sprintf("sound%d", i)); got != AliasID(i) {
			t.Fatalf("registration %d = %d", i, got)
		}
	}
	if got := r.Register("SOUND254"); got != 254 || r.Count() != maxAliases {
		t.Fatalf("full-table duplicate = %d, count=%d", got, r.Count())
	}
	if got := r.Register("overflow"); got != 0 || r.Count() != maxAliases {
		t.Fatalf("overflow = %d, count=%d", got, r.Count())
	}
	if got := r.Lookup("sound0"); got != 0 {
		t.Fatalf("first lookup = %d", got)
	}
	if first, ok := r.Entry(0); !ok || first.Name != "sound0" {
		t.Fatalf("first registration replaced: %+v, present=%t", first, ok)
	}
	if got := r.Lookup("overflow"); got != MissingAlias {
		t.Fatalf("overflow was registered: %d", got)
	}
	if _, ok := r.Entry(255); ok {
		t.Fatal("reserved last slot became a registration")
	}
	if _, err := r.Load(MissingAlias); err == nil {
		t.Fatal("missing sentinel loaded a sample")
	}
	aliases := r.Aliases()
	if len(aliases) != r.Count() {
		t.Fatalf("exported aliases=%d, count=%d", len(aliases), r.Count())
	}
	for i, alias := range aliases {
		entry, ok := r.Entry(AliasID(i))
		if !ok || alias.ID != AliasID(i) || alias != entry {
			t.Fatalf("exported alias %d = %+v, entry=%+v present=%t", i, alias, entry, ok)
		}
	}
}

// TestAuthoredSoundPathsCarryTheSoundsPrefix locks the alias registrar's probe
// candidates [03 §8.3 "Alias registration"]: the authored `sound` value is
// probed "with the `sounds/` prefix and the canonical candidate tries". Stock
// `allsound.tdf` authors bare stems, so dropping the prefix silences every
// alias-registered cue.
func TestAuthoredSoundPathsCarryTheSoundsPrefix(t *testing.T) {
	got := authoredSoundPaths("butmain1")
	if len(got) == 0 || got[0] != "sounds/butmain1" {
		t.Fatalf("authoredSoundPaths(butmain1) = %v, want the sounds/-prefixed form first", got)
	}
	if !containsPath(got, "sounds/butmain1.wav") {
		t.Fatalf("authoredSoundPaths(butmain1) = %v, want the prefixed .wav candidate", got)
	}
	if !containsPath(got, "butmain1") {
		t.Fatalf("authoredSoundPaths(butmain1) = %v, want the unprefixed candidate retained", got)
	}
	// An authored value that already carries the prefix is not prefixed twice.
	for _, p := range authoredSoundPaths("sounds/explode.wav") {
		if p == "sounds/sounds/explode.wav" {
			t.Fatalf("authoredSoundPaths doubled the prefix: %v", authoredSoundPaths("sounds/explode.wav"))
		}
	}
}

func containsPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

// A failed static registration keeps its identity and failed result even when
// its file later becomes available. Direct-path opens are independent [03 §8.3].
func TestRegistryFailedProbeDoesNotRetry(t *testing.T) {
	root := t.TempDir()
	mount := func() *vfs.FS {
		t.Helper()
		fs := vfs.New()
		if err := fs.MountDirectory(root, 1); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { fs.Close() })
		return fs
	}
	r := NewRegistry(mount())
	id := r.RegisterPath("late", "camps/arriving.wav")
	before, ok := r.Entry(id)
	if !ok || before.Probed {
		t.Fatalf("missing file registration = %+v, present=%v", before, ok)
	}
	if err := os.MkdirAll(filepath.Join(root, "camps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "camps", "arriving.wav"), buildRIFF(1, 11025, 8, []byte{128, 129}), 0644); err != nil {
		t.Fatal(err)
	}
	// Rebind to a fresh directory index so the newly authored file is visible.
	r.SetFS(mount())
	if sample, err := r.Cache().LoadPath("camps/arriving.wav"); err != nil || sample == nil {
		t.Fatalf("independent direct-path load = %v, err=%v", sample, err)
	}
	if got := r.RegisterPath("LATE", "camps/arriving.wav"); got != id {
		t.Fatalf("duplicate identity = %d, want %d", got, id)
	}
	if sample, err := r.Load(r.Lookup("late")); err == nil || sample != nil {
		t.Fatalf("failed alias retried: sample=%v err=%v", sample, err)
	}
	if after, _ := r.Entry(id); after != before || r.Count() != 1 {
		t.Fatalf("failed registration changed: before=%+v after=%+v count=%d", before, after, r.Count())
	}
}

// Supplying decoded PCM through the shared cache is an explicit host API;
// removing implicit file retries must not remove that existing injection seam.
func TestRegistryKeepsExplicitSharedCacheSamples(t *testing.T) {
	r := NewRegistry(nil)
	id := r.Register("supplied")
	cache := NewCache(nil)
	_, err := cache.Put("supplied", []byte{128, 129})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := cache.Get("supplied")
	r.SetCache(cache)
	if got, err := r.Load(id); err != nil || got != want {
		t.Fatalf("shared sample = %v, err=%v", got, err)
	}
}
