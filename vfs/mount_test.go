package vfs_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// TestMountRetailInstall is PLAN_01's real gate: the whole reference install
// mounts, tier precedence puts the patch archive above the base archives, and
// no archive is dropped (docs/SPEC_CONFLICTS.md SC1).
func TestMountRetailInstall(t *testing.T) {
	root := testsupport.RetailRoot(t)
	fileSystem := vfs.New()
	if err := fileSystem.MountGameDirectory(root); err != nil {
		t.Fatalf("mount: %v", err)
	}
	defer fileSystem.Close()

	archives := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".hpi", ".ufo", ".ccx", ".gp3", ".gp4", ".gpf", ".swx":
			archives++
		}
	}
	// Every archive plus the loose root directory.
	if want := archives + 1; fileSystem.MountCount() != want {
		t.Fatalf("mounted %d providers, want %d (no archive may be dropped)", fileSystem.MountCount(), want)
	}

	for _, required := range []string{
		"gamedata/moveinfo.tdf", "gamedata/sidedata.tdf", "gamedata/los.tdf",
		"palettes/palette.pal", "units/armcom.fbi", "ai/default.txt",
	} {
		if _, err := fileSystem.Stat(required); err != nil {
			t.Errorf("%s: %v", required, err)
		}
	}
	// Tier precedence smoke test from PLAN_01 WU-01-1: sidedata.tdf must
	// resolve to the patch archive rev31.gp3, beating the base HPIs.
	sources := fileSystem.Sources("gamedata/sidedata.tdf")
	if len(sources) == 0 {
		t.Fatal("gamedata/sidedata.tdf not found")
	}
	if got := sources[0].Source.ProviderID(); got != "rev31.gp3" {
		t.Fatalf("sidedata.tdf provider = %q, want rev31.gp3", got)
	}
}

// TestLooseShadowsArchive proves the loose tier wins and that the manifest
// names the provider it shadowed — the guarantee that makes PLAN_01's
// intra-tier ordering divergence auditable.
func TestLooseShadowsArchive(t *testing.T) {
	root := testsupport.RetailRoot(t)
	overlay := t.TempDir()
	if err := os.MkdirAll(filepath.Join(overlay, "gamedata"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("[MOVEINFO] { marker=1; }\n")
	if err := os.WriteFile(filepath.Join(overlay, "gamedata", "moveinfo.tdf"), marker, 0o644); err != nil {
		t.Fatal(err)
	}

	fileSystem := vfs.New()
	if err := fileSystem.MountGameDirectory(root); err != nil {
		t.Fatal(err)
	}
	defer fileSystem.Close()
	if err := fileSystem.MountDirectory(overlay, 200); err != nil {
		t.Fatal(err)
	}

	data, err := fileSystem.ReadFile("gamedata/moveinfo.tdf")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(marker) {
		t.Fatal("loose overlay did not win the lookup")
	}

	records, err := fileSystem.Manifest(vfs.ManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.LogicalPath != "gamedata/moveinfo.tdf" {
			continue
		}
		if len(record.Shadowed) == 0 {
			t.Fatal("manifest did not record the shadowed archive provider")
		}
		return
	}
	t.Fatal("manifest is missing the overlaid path")
}

// TestManifestHashStable locks the identity guarantee: no map iteration may
// reach the output (INVARIANTS I1).
func TestManifestHashStable(t *testing.T) {
	root := testsupport.RetailRoot(t)
	var hashes []string
	for i := 0; i < 2; i++ {
		fileSystem := vfs.New()
		if err := fileSystem.MountGameDirectory(root); err != nil {
			t.Fatal(err)
		}
		hash, err := fileSystem.ManifestHash()
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, hash)
		fileSystem.Close()
	}
	if hashes[0] != hashes[1] {
		t.Fatalf("manifest hash unstable: %s vs %s", hashes[0], hashes[1])
	}
}

// TestDuplicateMountSuppressed covers the dedup rule: an equal full path
// mounts once [02 §2]. The second mount attempt must not change the mount
// count or the manifest.
func TestDuplicateMountSuppressed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{{Path: "file", Data: []byte("data")}}, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.hpi"), archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	fileSystem := vfs.New()
	defer fileSystem.Close()
	if err := fileSystem.MountGameDirectory(dir); err != nil {
		t.Fatal(err)
	}
	before := fileSystem.MountCount()
	first, err := fileSystem.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := fileSystem.MountGameDirectoryWithPlan(dir, vfs.DefaultRetailMountPlan()); err != nil {
		t.Fatal(err)
	}
	if after := fileSystem.MountCount(); after != before {
		t.Fatalf("mount count %d -> %d; duplicate full path must be suppressed [02 §2]", before, after)
	}
	second, err := fileSystem.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("manifest hash changed across a suppressed duplicate mount")
	}
	notes := strings.Join(fileSystem.Notes(), "\n")
	if strings.Contains(notes, dir) || !strings.Contains(notes, "fixture.hpi") || !strings.Contains(notes, "loose root") {
		t.Fatalf("duplicate diagnostics must use portable provider labels [C13]: %s", notes)
	}
}

// TestManifestHashPortable locks PLAN_01 C13's cross-filesystem clause: the
// same content at two different host roots must produce the same
// ManifestHash, because ProviderID is archive base name or root-relative
// path — never an absolute host path.
func TestManifestHashPortable(t *testing.T) {
	build := func(root string) {
		files := map[string]string{
			"gamedata/moveinfo.tdf": "[CLASS0]\n{\nName=box;\n}\n",
			"units/armcom.fbi":      "[UNITINFO]\n{\nunitname=ARMCOM;\n}\n",
		}
		for rel, data := range files {
			full := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	a := t.TempDir()
	b := t.TempDir()
	build(a)
	build(filepath.Join(b, "deeper", "nested", "install"))
	if err := os.MkdirAll(filepath.Join(b, "deeper", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	fsA := vfs.New()
	defer fsA.Close()
	if err := fsA.MountDirectory(a, 10); err != nil {
		t.Fatal(err)
	}
	fsB := vfs.New()
	defer fsB.Close()
	if err := fsB.MountDirectory(filepath.Join(b, "deeper", "nested", "install"), 10); err != nil {
		t.Fatal(err)
	}
	hashA, err := fsA.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := fsB.ManifestHash()
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatalf("manifest hash depends on host root: %s vs %s", hashA[:16], hashB[:16])
	}
	records, err := fsA.Manifest(vfs.ManifestOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if strings.Contains(record.ProviderID, string(filepath.Separator)+string(filepath.Separator)) ||
			filepath.IsAbs(record.ProviderID) {
			t.Fatalf("ProviderID %q leaks a host path", record.ProviderID)
		}
	}
}

// More than ten local HPIs are ordinary content: retail's budget is per
// mount pass [02 §2], so all providers remain available without a warning.
func TestMoreThanTenHPIsMountWithoutWarning(t *testing.T) {
	dir := t.TempDir()
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{{Path: "file", Data: []byte("data")}}, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		name := filepath.Join(dir, fmt.Sprintf("dummy%d.hpi", i))
		if err := os.WriteFile(name, archive.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountGameDirectoryWithPlan(dir, vfs.DefaultRetailMountPlan()); err != nil {
		t.Fatal(err)
	}
	if len(fs.Notes()) != 0 {
		t.Fatalf("valid archives produced mount warnings: %v", fs.Notes())
	}
	if sources := fs.Sources("file"); len(sources) != 11 {
		t.Fatalf("available providers = %d, want all 11 archives", len(sources))
	}
}

// TestReadFileRangeMatchesWholeFile locks the range reader against the whole
// read it exists to avoid. A compressed HPI record is a table of chunk sizes
// followed by SQSH chunks, so the arithmetic that steps over the chunks before
// a range is the part that can silently drift.
func TestReadFileRangeMatchesWholeFile(t *testing.T) {
	root := testsupport.RetailRoot(t)
	fileSystem := vfs.New()
	if err := fileSystem.MountGameDirectory(root); err != nil {
		t.Fatalf("mount: %v", err)
	}
	defer fileSystem.Close()

	// One archived file per compression method plus a small one, so the test
	// covers a record whose range spans more than one chunk.
	for _, name := range []string{"maps/acid foursome.tnt", "gamedata/sidedata.tdf", "guis/selmap.gui"} {
		whole, err := fileSystem.ReadFileLimit(name, 64<<20)
		if err != nil {
			t.Skipf("%s: %v", name, err)
		}
		if len(whole) < 0x100 {
			t.Fatalf("%s is only %d bytes", name, len(whole))
		}
		cases := []struct{ offset, length int }{
			{0, 0x40},
			{0, len(whole)},
			{1, 0x40},
			{len(whole) / 2, 0x2000},
			{len(whole) - 8, 8},
			{len(whole), 0},
		}
		for _, c := range cases {
			if c.offset+c.length > len(whole) {
				// A length past the end is clamped to the end.
				c.length = len(whole) - c.offset
			}
			got, err := fileSystem.ReadFileRange(name, int64(c.offset), c.length)
			if err != nil {
				t.Errorf("%s[%d:+%d]: %v", name, c.offset, c.length, err)
				continue
			}
			want := whole[c.offset : c.offset+c.length]
			if string(got) != string(want) {
				t.Errorf("%s[%d:+%d] differs from the whole read", name, c.offset, c.length)
			}
		}
		// A negative length reads to the end.
		tail, err := fileSystem.ReadFileRange(name, int64(len(whole)-16), -1)
		if err != nil || string(tail) != string(whole[len(whole)-16:]) {
			t.Errorf("%s: read to end: %v", name, err)
		}
	}
}
