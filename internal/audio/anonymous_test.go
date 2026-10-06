package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func anonymousSoundFS(t *testing.T, names ...string) (*vfs.FS, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sounds"), 0755); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		if err := os.WriteFile(filepath.Join(root, "sounds", name+".wav"), buildRIFF(1, 11025, 8, []byte{128, byte(129 + i)}), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return mountAnonymousSounds(t, root), root
}

func mountAnonymousSounds(t *testing.T, root string) *vfs.FS {
	t.Helper()
	fs := vfs.New()
	if err := fs.MountDirectory(root, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

// Anonymous paths compare retained authored paths, not alias names, resolved
// provenance or empty cache names. The first match owns the sample [03 §8.3].
func TestAnonymousRegistrationSeparatesNamesAndReusesAuthoredPaths(t *testing.T) {
	fs, _ := anonymousSoundFS(t, "clang", "different", "other")
	r := NewRegistry(fs)
	named := r.RegisterPath("clang", "different")
	anonymous := r.RegisterAnonymousPath("clang")
	other := r.RegisterAnonymousPath("other")
	if named != 0 || anonymous != 1 || other != 2 || r.Lookup("clang") != named || r.Lookup("") != MissingAlias {
		t.Fatalf("identities named=%d anonymous=%d other=%d", named, anonymous, other)
	}
	namedSample, _ := r.Load(named)
	anonymousSample, _ := r.Load(anonymous)
	otherSample, _ := r.Load(other)
	if namedSample == nil || anonymousSample == nil || otherSample == nil || namedSample == anonymousSample || anonymousSample == otherSample {
		t.Fatal("distinct registrations lost sample identity")
	}
	if anonymousSample.Provenance.LogicalPath != "sounds/clang.wav" || namedSample.Provenance.LogicalPath != "sounds/different.wav" {
		t.Fatal("anonymous path was redirected by alias name")
	}
	if got := r.RegisterAnonymousPath("DIFFERENT"); got != named {
		t.Fatalf("authored path reuse = %d, want %d", got, named)
	}
	// A resolved path is not the retained authored identity.
	if got := r.RegisterAnonymousPath("sounds/different.wav"); got == named {
		t.Fatal("resolved provenance replaced authored path key")
	}
	if got := r.RegisterPath("clang", "other"); got != named {
		t.Fatal("named duplicate no longer wins by name")
	}
	if _, err := r.Cache().Put("clang", []byte{140}); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Load(anonymous); got != anonymousSample {
		t.Fatal("named PCM injection replaced unrelated anonymous sample")
	}
	injected, _ := r.Cache().Get("clang")
	if got, _ := r.Load(r.RegisterAnonymousPath("different")); got != injected {
		t.Fatal("path reuse lost named identity's explicit injection")
	}
	r.SetCache(NewCache(nil))
	if got, _ := r.Load(anonymous); got != anonymousSample {
		t.Fatal("cache replacement lost anonymous retained sample")
	}
	if got, _ := r.Load(named); got != namedSample {
		t.Fatal("cache replacement lost named registration's retained sample")
	}
}

func TestAnonymousRegistrationLongProbeAndBoundedFailedMatch(t *testing.T) {
	prefix := strings.Repeat("a", 32)
	long := prefix + "first"
	fs, root := anonymousSoundFS(t, long, prefix+"second")
	r := NewRegistry(fs)
	id := r.RegisterAnonymousPath(long)
	sample, err := r.Load(id)
	if err != nil || sample == nil || sample.Provenance.LogicalPath != "sounds/"+long+".wav" {
		t.Fatalf("full initial probe = %v, err=%v", sample, err)
	}
	if got := r.RegisterAnonymousPath(strings.ToUpper(prefix) + "second"); got != id {
		t.Fatalf("bounded case-insensitive match = %d, want %d", got, id)
	}
	missingPath := strings.Repeat("b", 32) + "missing"
	failed := r.RegisterPath("absent", missingPath)
	newPath := strings.Repeat("b", 32) + "present"
	if err := os.WriteFile(filepath.Join(root, "sounds", newPath+".wav"), buildRIFF(1, 11025, 8, []byte{130}), 0644); err != nil {
		t.Fatal(err)
	}
	r.SetFS(mountAnonymousSounds(t, root))
	if s, err := r.Cache().LoadPath("sounds/" + newPath + ".wav"); err != nil || s == nil {
		t.Fatalf("direct file = %v, err=%v", s, err)
	}
	if got := r.RegisterAnonymousPath(newPath); got != failed {
		t.Fatalf("failed named path reuse = %d, want %d", got, failed)
	}
	if got, err := r.Load(failed); got != nil || err == nil {
		t.Fatal("failed identity retried or borrowed direct-path sample")
	}
}

func TestAnonymousRegistrationSharesCapacity(t *testing.T) {
	r := NewRegistry(nil)
	first := r.RegisterAnonymousPath("first")
	named := r.RegisterPath("named", "named-path")
	for r.Count() < maxAliases {
		r.RegisterAnonymousPath(fmt.Sprintf("path%d", r.Count()))
	}
	if first != 0 || r.RegisterAnonymousPath("NAMED-PATH") != named || r.RegisterAnonymousPath("overflow") != first || r.Register("overflow-name") != first || r.Count() != maxAliases {
		t.Fatal("shared capacity, duplicate-before-capacity or first-entry overflow changed")
	}
}

func TestServiceBindsAnonymousHistoryBeforePlaybackAndRetainsFailures(t *testing.T) {
	fs, root := anonymousSoundFS(t, "first", "clang", "different")
	s := NewService(fs)
	paths := []string{"clang", "missing"}
	for i := 0; i < maxAliases; i++ {
		paths = append(paths, fmt.Sprintf("history%d", i))
	}
	paths = append(paths, "overflow")
	cat := &content.Catalog{AliasOrder: []*content.SoundAlias{{Alias: "clang", Sound: "different"}, {Alias: "first", Sound: "first"}}, WeaponSoundPaths: paths}
	s.BindCatalog(cat, nil)
	if s.Registry.Count() != maxAliases || s.weaponSounds["clang"] != 2 || s.weaponSounds["missing"] != 3 || s.weaponSounds["overflow"] != 0 {
		t.Fatalf("ordered admissions = %v, count=%d", s.weaponSounds, s.Registry.Count())
	}
	old := GlobalOutput()
	spy := &outputSpy{}
	SetGlobalOutput(spy)
	t.Cleanup(func() { SetGlobalOutput(old) })
	s.DrainEvents(1, []frame.EventView{
		{Kind: frame.EventKindAudio, Sound: "clang", AudioPositional: true, AudioAudible: true, AudioAnonymous: true},
		{Kind: frame.EventKindAudio, Sound: "clang", AudioPositional: true, AudioAudible: true},
		{Kind: frame.EventKindAudio, Sound: "overflow", AudioPositional: true, AudioAudible: true, AudioAnonymous: true},
		{Kind: frame.EventKindAudio, Sound: "first", AudioPositional: true, AudioAudible: true, AudioAnonymous: true}, // exists by name, no anonymous binding
	})
	if len(spy.plays) != 3 || spy.plays[0].sample.Provenance.LogicalPath != "sounds/clang.wav" || spy.plays[1].sample.Provenance.LogicalPath != "sounds/different.wav" || spy.plays[2].sample != spy.plays[1].sample {
		t.Fatalf("source selection or overflow playback = %+v", spy.plays)
	}
	if err := os.WriteFile(filepath.Join(root, "sounds", "missing.wav"), buildRIFF(1, 11025, 8, []byte{130}), 0644); err != nil {
		t.Fatal(err)
	}
	s.Init(mountAnonymousSounds(t, root))
	s.ResetBattleCues()
	s.BindCatalog(cat, nil)
	if s.weaponSounds["missing"] != 3 || s.Registry.Count() != maxAliases {
		t.Fatal("reload replaced registration history")
	}
	s.DrainEvents(1, []frame.EventView{{Kind: frame.EventKindAudio, Sound: "missing", AudioPositional: true, AudioAudible: true, AudioAnonymous: true}})
	if len(spy.plays) != 3 {
		t.Fatal("reload retried previously failed sample")
	}
}
