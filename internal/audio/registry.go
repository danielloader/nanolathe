package audio

import (
	"fmt"
	"strings"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

// AliasID is the identity used by authored sound references. Zero names the
// first registration and is also returned when the table is full; only 0xffff
// is the lookup-miss sentinel [03 §8.3].
type AliasID uint16

const (
	MissingAlias  AliasID = 0xffff
	aliasCapacity         = 256
	maxAliases            = aliasCapacity - 1
)

// Alias is one ordered sound registration. Names are retained at 32 bytes
// [03 §8.3]. Path retains host metadata; registration probes the original
// authored path before storing it. Playback never reopens this path.
type Alias struct {
	ID     AliasID
	Name   string
	Path   string
	Probed bool
}

// Registry owns alias identity and the service-lifetime decoded samples.
// Registration order is observable: duplicate names return the first ID and
// failed probes still consume a slot.  There is no runtime eviction [03
// §8.2–§8.3].
type Registry struct {
	fs    vfs.FSOps
	cache *SampleCache
	slots [aliasCapacity]Alias
	// Retained authored paths are identity, independent of resolved file paths
	// and named cache keys. Each anonymous slot owns its sample [03 §8.3].
	paths   [aliasCapacity]string
	samples [aliasCapacity]*Sample
	count   int
}

// NewRegistry creates the alias table with its own session sample cache,
// resolving files through fs [02 "Sound aliases"].
func NewRegistry(fs vfs.FSOps) *Registry {
	return &Registry{fs: fs, cache: NewCache(fs)}
}

// Cache is the registry's decoded-sample store.
func (r *Registry) Cache() *SampleCache {
	if r == nil {
		return nil
	}
	return r.cache
}

// SetFS updates the resolver for an existing registry without replacing its
// ordered alias identities or decoded samples.
func (r *Registry) SetFS(fs vfs.FSOps) {
	if r == nil || fs == nil {
		return
	}
	r.fs = fs
	if r.cache != nil {
		r.cache.SetFS(fs)
	}
}

// SetCache retains an externally supplied cache while keeping registry and
// cache ownership in the audio package.
func (r *Registry) SetCache(cache *SampleCache) {
	if r != nil && cache != nil {
		r.cache = cache
		if r.fs != nil {
			cache.SetFS(r.fs)
		}
	}
}

// TODO(question): full-field retail names need a producer/termination trace
// before named-lookup equivalence is known [03 §8.3]. The host safely bounds
// both registration and lookup to 32 bytes; it never reads past the string.
func aliasName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

func aliasPath(s string) string {
	// Retain the existing host metadata bound; this is neither a retry path
	// nor the retail anonymous-registration comparison [03 §8.3].
	if len(s) > 64 {
		return s[:64]
	}
	return s
}

func aliasEqual(a, b string) bool { return strings.EqualFold(aliasName(a), aliasName(b)) }

// Register adds an alias and probes canonical sounds/ candidates immediately.
// The probe result is retained even when the VFS has no matching file.
func (r *Registry) Register(name string) AliasID {
	return r.register(name, "", false)
}

// RegisterAnonymousPath registers a weapon's static path without a name.
// It can reuse a named registration by path, including a failed probe [03 §8.3].
func (r *Registry) RegisterAnonymousPath(path string) AliasID {
	return r.register("", path, true)
}

func retainedSoundPath(path string) string {
	if len(path) > 32 {
		return path[:32]
	}
	return path
}

func (r *Registry) register(name, soundPath string, anonymous bool) AliasID {
	if r == nil {
		return MissingAlias
	}
	name = aliasName(name)
	if (!anonymous && name == "") || (anonymous && soundPath == "") {
		return MissingAlias
	}
	path := soundPath
	if path == "" {
		path = name
	}
	key := retainedSoundPath(path)
	for i := 0; i < r.count; i++ {
		if anonymous {
			if strings.EqualFold(r.paths[i], key) {
				return r.slots[i].ID
			}
		} else if r.slots[i].Name != "" && aliasEqual(r.slots[i].Name, name) {
			return r.slots[i].ID
		}
	}
	if r.count >= maxAliases {
		// Retail returns the first real registration, not a miss [03 §8.3].
		return 0
	}
	id := AliasID(r.count)
	// Retain path metadata even when probing fails; the failed registration
	// still consumes its identity [03 §8.3].
	a := Alias{ID: id, Name: name, Path: aliasPath("sounds/" + name)}
	// Probe loading is part of registration identity.  A missing sample is
	// intentionally not an error to the caller: later playback degrades to
	// silence while the slot remains occupied.
	if r.fs != nil {
		candidates := canonicalAliasPaths(name)
		if soundPath != "" {
			a.Path = aliasPath(soundPath)
			if anonymous {
				candidates = authoredSoundPaths(soundPath)
			} else {
				candidates = append(authoredSoundPaths(soundPath), candidates...)
			}
		}
		if data, p, err := r.cache.resolveCandidates(candidates); err == nil {
			a.Path = aliasPath(p.LogicalPath)
			label := name
			if anonymous {
				label = path
			}
			if sample, decodeErr := Decode(label, data); decodeErr == nil {
				sample.Provenance = p
				r.samples[r.count] = sample
				if !anonymous {
					r.cache.putSample(name, sample)
				}
				a.Probed = true
			}
		}
	}
	if anonymous && !a.Probed {
		a.Path = aliasPath(soundPath)
	}
	r.paths[r.count] = key
	r.slots[r.count] = a
	r.count++
	return id
}

// RegisterPath registers an authored alias whose sound value is a separate
// path. Named duplicates compare names; anonymous registrations can later
// reuse this identity by its retained authored path [03 §8.3].
func (r *Registry) RegisterPath(name, soundPath string) AliasID {
	return r.register(name, soundPath, false)
}

// Lookup returns the id registered for an alias name, MissingAlias when the
// name is not registered. The host bounds names to 32 bytes (see aliasName).
func (r *Registry) Lookup(name string) AliasID {
	if r == nil {
		return MissingAlias
	}
	name = aliasName(name)
	for i := 0; i < r.count; i++ {
		if r.slots[i].Name != "" && aliasEqual(r.slots[i].Name, name) {
			return r.slots[i].ID
		}
	}
	return MissingAlias
}

// Count is the number of registered aliases.
func (r *Registry) Count() int {
	if r == nil {
		return 0
	}
	return r.count
}

// Entry returns the registration for an id, including zero when registered.
func (r *Registry) Entry(id AliasID) (Alias, bool) {
	if r == nil || int(id) >= r.count {
		return Alias{}, false
	}
	return r.slots[id], true
}

// Load returns the registration's cached sample without retrying a failed
// probe [03 §8.3]. Explicit shared-cache sample supply remains a host API.
func (r *Registry) Load(id AliasID) (*Sample, error) {
	if r == nil || id == MissingAlias {
		return nil, fmt.Errorf("audio: missing alias %d", id)
	}
	a, ok := r.Entry(id)
	if !ok {
		return nil, fmt.Errorf("audio: missing alias %d", id)
	}
	if a.Name != "" {
		// Explicit named PCM injection remains a host API. Anonymous slots
		// never borrow a sample merely because its cache name matches a path.
		if s, ok := r.cache.Get(a.Name); ok {
			return s, nil
		}
	}
	if s := r.samples[id]; s != nil {
		return s, nil
	}
	return nil, fmt.Errorf("nanolathe: load sound alias: logical path %s, providers searched [], expected registered sample", a.Path)
}

func canonicalAliasPaths(name string) []string {
	clean := strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	hasWav := strings.HasSuffix(strings.ToLower(clean), ".wav")
	paths := []string{"sounds/" + clean}
	if !hasWav {
		paths = append(paths, "sounds/"+clean+".wav")
	}
	paths = append(paths, clean)
	if !hasWav {
		paths = append(paths, clean+".wav")
	}
	return paths
}

// authoredSoundPaths is the candidate list for an alias's authored `sound`
// value. Registration probes that value "through the VFS and the WAV decode
// path with the `sounds/` prefix and the canonical candidate tries"
// [03 §8.3 "Alias registration"], and stock `allsound.tdf` authors bare stems
// (`sound=butmain1`) whose files live under `sounds/`.
//
// Correction (WU-19-131): this returned only the unprefixed forms, so every
// alias registered from `allsound.tdf` probed a path no install carries, kept
// the failed path, and stayed silent for the session — which muted every
// interface cue (button clicks, `oktobuild`, `addbuild`, the panel detents) and
// every feature/weapon sound that resolves through an authored alias rather
// than through the alias NAME. The prefixed forms come first because that is
// the order the registrar tries them in; a value that already carries the
// prefix is not prefixed twice.
func authoredSoundPaths(path string) []string {
	clean := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	if clean == "" {
		return nil
	}
	hasWav := strings.HasSuffix(strings.ToLower(clean), ".wav")
	var paths []string
	if !strings.HasPrefix(strings.ToLower(clean), "sounds/") {
		paths = append(paths, "sounds/"+clean)
		if !hasWav {
			paths = append(paths, "sounds/"+clean+".wav")
		}
	}
	paths = append(paths, clean)
	if !hasWav {
		paths = append(paths, clean+".wav")
	}
	return paths
}

// Aliases returns a copy of every registration in registration order [I1].
func (r *Registry) Aliases() []Alias {
	if r == nil || r.count == 0 {
		return nil
	}
	out := make([]Alias, r.count)
	copy(out, r.slots[:r.count])
	return out
}
