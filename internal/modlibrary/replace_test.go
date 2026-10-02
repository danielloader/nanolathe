package modlibrary

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func replacementArchive(t *testing.T, body string) (string, InstallOptions) {
	t.Helper()
	meta := sampleMetadata()
	archive := writeZip(t, t.TempDir(), "sample.zip", zipItem{name: MetadataFile, body: metadataJSON(t, meta)}, zipItem{name: "units/a.fbi", body: body})
	sum, err := fileSHA256(archive)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	return archive, InstallOptions{ExpectIdentity: &ExpectedIdentity{ID: meta.ID, Version: meta.Version}, SHA256: sum, Size: info.Size(), Replace: true}
}

func assertInstalledPackage(t *testing.T, lib *Library, want Mod, body string) {
	t.Helper()
	got, ok, err := lib.Lookup(want.ID, want.Version)
	if err != nil || !ok || got.Receipt.SHA256 != want.Receipt.SHA256 {
		t.Fatalf("installed package = %+v, %v, %v; want %s", got.Receipt, ok, err, want.Receipt.SHA256)
	}
	data, err := os.ReadFile(filepath.Join(got.Dir, "units/a.fbi"))
	if err != nil || string(data) != body {
		t.Fatalf("installed content = %q, %v; want %q", data, err, body)
	}
}

// Catalogue replacement keeps the original version and updates its receipt;
// manual duplicates still refuse, and a failure before publication preserves
// the previous bytes (DESIGN_MODS_MUTATORS §5.3).
func TestCatalogueReplacementKeepsOldInstallUntilValidated(t *testing.T) {
	lib := openTestLibrary(t)
	first, _ := replacementArchive(t, "old content")
	old, err := lib.InstallArchive(first, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	archive, opts := replacementArchive(t, "new content")
	if _, err := lib.InstallArchive(archive, InstallOptions{}); !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("manual duplicate = %v", err)
	}
	for _, phase := range []string{"hash", "validation", "commit guard", "publication"} {
		t.Run(phase, func(t *testing.T) {
			attempt := opts
			stop := errors.New("refused fixture update")
			switch phase {
			case "hash":
				attempt.SHA256 = strings.Repeat("0", 64)
			case "validation":
				attempt.Validate = func(root string, _ Metadata) error {
					assertInstalledPackage(t, lib, old, "old content")
					data, err := os.ReadFile(filepath.Join(root, "units/a.fbi"))
					if err != nil || string(data) != "new content" {
						t.Fatalf("staged content = %q, %v", data, err)
					}
					return stop
				}
			case "commit guard":
				attempt.BeforeCommit = func() error { return stop }
			case "publication":
				// The source disappearing after validation forces publication
				// to fail after the old directory has moved to its backup.
				attempt.BeforeCommit = func() error {
					entries, err := os.ReadDir(lib.StagingDir())
					if err != nil {
						return err
					}
					for _, entry := range entries {
						if err := os.RemoveAll(filepath.Join(lib.StagingDir(), entry.Name())); err != nil {
							return err
						}
					}
					return nil
				}
			}
			if _, err := lib.InstallArchive(archive, attempt); err == nil {
				t.Fatal("failed update was accepted")
			}
			assertInstalledPackage(t, lib, old, "old content")
		})
	}
	updated, err := lib.InstallArchive(archive, opts)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Dir != old.Dir || updated.Version != old.Version || updated.Receipt.SHA256 != opts.SHA256 || updated.Receipt.SHA256 == old.Receipt.SHA256 {
		t.Fatalf("replacement identity = %+v", updated)
	}
	assertInstalledPackage(t, lib, updated, "new content")
	if _, err := os.Stat(lib.replacementDir(old.ID, old.Version)); !os.IsNotExist(err) {
		t.Fatalf("previous copy remained after successful replacement: %v", err)
	}
}

func TestReplacementRequiresVerifiedArchiveIdentity(t *testing.T) {
	archive, opts := replacementArchive(t, "new content")
	for _, field := range []string{"identity", "hash", "size", "directory"} {
		t.Run(field, func(t *testing.T) {
			lib := openTestLibrary(t)
			attempt := opts
			switch field {
			case "identity":
				attempt.ExpectIdentity = nil
			case "hash":
				attempt.SHA256 = ""
			case "size":
				attempt.Size = 0
			case "directory":
				attempt.SHA256, attempt.Size = "", 0
				if _, err := lib.InstallDirectory(t.TempDir(), attempt); err == nil {
					t.Fatal("directory replacement accepted")
				}
				return
			}
			if _, err := lib.InstallArchive(archive, attempt); err == nil {
				t.Fatal("unverified archive replacement accepted")
			}
			assertNothingInstalled(t, lib)
		})
	}
}

// A stopped process may leave the old directory in .replaced before or after
// publishing the new directory. Open must recover that state before deleting
// abandoned staging, and keep the backup if the target is unreadable.
func TestOpenRecoversInterruptedReplacement(t *testing.T) {
	for _, state := range []string{"before publication", "after publication", "invalid target"} {
		t.Run(state, func(t *testing.T) {
			// Build without Open so the recovery below is the first open.
			lib := &Library{Root: filepath.Join(t.TempDir(), "mods")}
			archive, _ := replacementArchive(t, "old content")
			old, err := lib.InstallArchive(archive, InstallOptions{})
			if err != nil {
				t.Fatal(err)
			}
			backup := lib.replacementDir(old.ID, old.Version)
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(old.Dir, backup); err != nil {
				t.Fatal(err)
			}
			want, body := old, "old content"
			switch state {
			case "after publication":
				archive, _ := replacementArchive(t, "new content")
				other := openTestLibrary(t)
				want, err = other.InstallArchive(archive, InstallOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(want.Dir, old.Dir); err != nil {
					t.Fatal(err)
				}
				body = "new content"
			case "invalid target":
				if err := os.Mkdir(old.Dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			staged := filepath.Join(lib.StagingDir(), "extract-abandoned")
			if err := os.Mkdir(staged, 0o755); err != nil {
				t.Fatal(err)
			}
			_, err = Open(lib.Root)
			if state == "invalid target" {
				if err == nil {
					t.Fatal("recovery discarded a backup beside an invalid target")
				}
				data, err := os.ReadFile(filepath.Join(backup, "units/a.fbi"))
				if err != nil || string(data) != "old content" {
					t.Fatalf("only previous copy lost: %q, %v", data, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertInstalledPackage(t, lib, want, body)
			if _, err := os.Stat(staged); !os.IsNotExist(err) {
				t.Fatalf("abandoned staging not cleared: %v", err)
			}
		})
	}
}
