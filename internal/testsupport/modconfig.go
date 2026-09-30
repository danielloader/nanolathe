package testsupport

import (
	"os"
	"path/filepath"
	"testing"
)

// ModConfigPath returns the path of the repository's authored config for one
// mod release, `modconfigs/<dir>/nanolathe-mod.json` — the file Nanolathe's
// hosted zip of that mod carries (docs/DESIGN_MODS_MUTATORS.md §4.2). The
// engine embeds no mod's config, so a test that needs one reads it from the
// repository, found by walking up from the test's directory to go.mod.
func ModConfigPath(t testing.TB, dir string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate the repository for mod config %s: %v", dir, err)
	}
	for current := wd; ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			path := filepath.Join(current, "modconfigs", dir, "nanolathe-mod.json")
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("mod config %s: %v", dir, err)
			}
			return path
		}
		if parent := filepath.Dir(current); parent == current {
			t.Fatalf("mod config %s: no go.mod above %s", dir, wd)
		}
	}
}
