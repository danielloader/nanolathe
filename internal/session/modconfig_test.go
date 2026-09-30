package session

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content/profiles"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// modContent is one of the repository's authored mod configs as a session
// test uses it: the config's content section, and its Community table as the
// session's content source (docs/DESIGN_MODS_MUTATORS.md §4.2). The engine
// carries no mod's config, so the package tests read them from modconfigs/.
type modContent struct {
	profiles.Profile
	sources []community.Overrides
}

// GameplaySources is the config's Community declaration.
func (m modContent) GameplaySources() []community.Overrides { return m.sources }

// unitLimit is the per-player unit limit the content set is authored for:
// its config's communityFeatures.unitLimit, which the removed content
// profile carried as limits.unit_limit.
func (m modContent) unitLimit() int {
	if len(m.sources) == 0 || m.sources[0].Base == nil {
		return 0
	}
	return m.sources[0].Base.UnitLimit
}

// loadModContent reads modconfigs/<dir>/nanolathe-mod.json through the two
// parsers that own its sections here — the content section's and the
// Community table's. The mod library, which reads the whole file, is off
// limits to an authoritative package even in its tests
// (internal/architecture). With a mounted overlay it also checks that the
// content presents every tree the config's markers name.
func loadModContent(t *testing.T, mounted vfs.FSOps, dir string) modContent {
	t.Helper()
	path := testsupport.ModConfigPath(t, dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		ID      string          `json:"id"`
		Content json.RawMessage `json:"content"`
		Rules   struct {
			CommunityFeatures json.RawMessage `json:"communityFeatures"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	profile, err := profiles.Parse(doc.Content, doc.ID, path)
	if err != nil {
		t.Fatal(err)
	}
	m := modContent{Profile: profile}
	if len(doc.Rules.CommunityFeatures) != 0 {
		features, err := community.ParseFeatures(doc.Rules.CommunityFeatures, path)
		if err != nil {
			t.Fatal(err)
		}
		m.sources = []community.Overrides{{Base: &features}}
	}
	if mounted != nil {
		if missing := profile.MissingMarkers(mounted); len(missing) != 0 {
			t.Fatalf("the mounted content lacks the %s config's trees %v", dir, missing)
		}
	}
	return m
}
