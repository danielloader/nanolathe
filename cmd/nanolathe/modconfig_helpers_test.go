package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
)

// modRootsConfig maps a NANOLATHE_MOD_ROOTS_<NAME> content set to the
// repository config (modconfigs/) its manual root stack names with
// --mod-config: nothing detects a mod's layout any more
// (docs/DESIGN_MODS_MUTATORS.md §4.3).
var modRootsConfig = map[string]string{
	"prota":      "prota-4.8",
	"zero":       "ta-zero-alpha5-20241224",
	"escalation": "escalation-10.2.0",
	"mayhem":     "mayhem-11.3.0",
}

// modRootsConfigPath is the --mod-config path for a NANOLATHE_MOD_ROOTS_*
// content set, by its environment name.
func modRootsConfigPath(t *testing.T, name string) string {
	t.Helper()
	dir, ok := modRootsConfig[strings.ToLower(name)]
	if !ok {
		t.Fatalf("no repository config for content set %q", name)
	}
	return testsupport.ModConfigPath(t, dir)
}

// stageModPackage copies a mod's upstream root into a scratch folder beside
// the repository config for it, the shape of Nanolathe's hosted zip
// (docs/DESIGN_MODS_MUTATORS.md §5.5), so a test can install the package the
// way a download does. Links are followed and copied as files.
func stageModPackage(t *testing.T, root, configDir string) string {
	t.Helper()
	pack := filepath.Join(t.TempDir(), filepath.Base(filepath.Clean(root)))
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(pack, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if strings.EqualFold(d.Name(), modlibrary.MetadataFile) || strings.EqualFold(d.Name(), modlibrary.ReceiptFile) {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatalf("stage %s: %v", root, err)
	}
	config, err := os.ReadFile(testsupport.ModConfigPath(t, configDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, modlibrary.MetadataFile), config, 0o644); err != nil {
		t.Fatal(err)
	}
	return pack
}
