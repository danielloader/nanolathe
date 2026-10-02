package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/modfetch"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
)

func TestCatalogueCurrentRequiresVersionAndArchiveHash(t *testing.T) {
	entry := modfetch.Entry{ID: "sample", Version: "1", Archive: modfetch.Archive{SHA256: strings.Repeat("ab", 32)}}
	installed := modlibrary.Mod{Metadata: modlibrary.Metadata{ID: entry.ID, Version: entry.Version}, Receipt: modlibrary.Receipt{SHA256: strings.ToUpper(entry.Archive.SHA256)}}
	if !modInstalled([]modlibrary.Mod{installed}, entry) {
		t.Fatal("the same archive with uppercase receipt hash is offered again")
	}
	for _, field := range []string{"id", "version", "changed hash", "missing hash"} {
		t.Run(field, func(t *testing.T) {
			other := installed
			switch field {
			case "id":
				other.ID = "another"
			case "version":
				other.Version = "1+nanolathe.1"
			case "changed hash":
				other.Receipt.SHA256 = strings.Repeat("cd", 32)
			case "missing hash":
				other.Receipt.SHA256 = ""
			}
			if modInstalled([]modlibrary.Mod{other}, entry) {
				t.Fatal("catalogue package with a different identity was hidden")
			}
		})
	}
}

func TestBothModScreensRefuseUpdatingMountedVersion(t *testing.T) {
	base, lib := modFixture(t)
	entry := modfetch.Entry{ID: "sample", Version: "1", Archive: modfetch.Archive{SHA256: strings.Repeat("ab", 32)}}
	mod := modlibrary.Mod{Metadata: modlibrary.Metadata{ID: entry.ID, Version: entry.Version}, Receipt: modlibrary.Receipt{SHA256: strings.Repeat("cd", 32)}}
	g := &gameShell{cs: &contentSet{mod: &mod, baseRoots: []string{base}}}
	previousMods, previousFetch := modsUI, modsFetchUI
	t.Cleanup(func() {
		modsUI, modsFetchUI = previousMods, previousFetch
		modDownload.rememberMounted(nil)
	})
	modsUI = &modsScreen{lib: lib}
	modsFetchUI = &modsFetch{entries: []modfetch.Entry{entry}}
	g.startModDownload()
	if modDownload.view().running || !strings.Contains(modsFetchUI.status, modUpdateSwitchNotice) {
		t.Fatalf("legacy catalogue active update: %q", modsFetchUI.status)
	}
	s := newNLScreen(func() *gameShell { return g })
	s.downloadMod(entry)
	if modDownload.view().running || !strings.Contains(s.toast, modUpdateSwitchNotice) {
		t.Fatalf("Nanolathe catalogue active update: %q", s.toast)
	}
	if !modUpdateMounted(entry, &mod) || modUpdateMounted(entry, nil) {
		t.Fatal("active update row does not identify the mounted version")
	}
	mod.Version = "older"
	if modUpdateMounted(entry, &mod) {
		t.Fatal("a separate version was blocked")
	}
}

func TestModUpdateGuardAcrossAsynchronousInstall(t *testing.T) {
	var job modDownloadJob
	entry := modfetch.Entry{ID: "sample", Name: "Sample", Version: "1"}
	mod := modlibrary.Mod{Metadata: modlibrary.Metadata{ID: entry.ID, Version: entry.Version}}
	job.rememberMounted(&mod)
	mod.Version = "changed after capture"
	if err := job.allowInstall(entry); err == nil {
		t.Fatal("mounted identity was read through a mutable content pointer")
	}
	job.rememberMounted(nil)
	release := make(chan struct{})
	if !job.start(entry, func(context.Context, func(int64, int64)) error {
		<-release
		return nil
	}, func() error { return job.allowInstall(entry) }) {
		t.Fatal("job did not start")
	}
	if !job.blocksMount("sample@1") || !job.blocksMount("sample") || job.blocksMount("sample@2") || job.blocksMount("none") {
		t.Error("reload guard did not reserve exactly the updating version")
	}
	// Normally blocksMount prevents this switch. The worker must still
	// recheck the synchronized identity instead of relying on its first check.
	mod.Version = entry.Version
	job.rememberMounted(&mod)
	close(release)
	if v := waitForJob(t, &job); v.installs != 0 || !strings.Contains(v.outcome, modUpdateSwitchNotice) {
		t.Fatalf("worker ignored the active guard: %+v", v)
	}
	if job.blocksMount("sample@1") {
		t.Fatal("finished job kept its mount reservation")
	}
}

func TestModUpdateRefusesManuallyMountedDirectory(t *testing.T) {
	base, lib := modFixture(t)
	meta := modlibrary.Metadata{Schema: 1, ID: "sample", Name: "Sample", Version: "1"}
	mod := installFixtureMod(t, lib, "manual", &meta)
	alias := filepath.Join(t.TempDir(), "mod-alias")
	if err := os.Symlink(mod.Dir, alias); err != nil {
		t.Fatal(err)
	}
	entry := modfetch.Entry{ID: meta.ID, Version: meta.Version}
	for _, root := range []string{mod.Dir + string(filepath.Separator), alias} {
		g := &gameShell{cs: &contentSet{roots: []string{base, root}, baseRoots: []string{base, root}, manualRoots: true}}
		if err := g.startCatalogueDownload(lib, entry); err == nil || !strings.Contains(err.Error(), "--root") {
			t.Fatalf("manual root %s update = %v", root, err)
		}
		if modDownload.view().running {
			t.Fatal("a manually mounted update started a download")
		}
	}
	if modDirectoryMounted(mod.Dir, []string{base}) {
		t.Fatal("unrelated content root blocked the update")
	}
}
