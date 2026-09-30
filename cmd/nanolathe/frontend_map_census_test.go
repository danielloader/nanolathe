package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Issue #50: a player's skirmish list lacked one archive's maps and the run
// log said only how many maps were listed. The startup report must name the
// providers that supplied the list, an archive the mount rejected, and a map
// file the census could not read — without changing which maps are listed.
// The three archives are authored here; none is retail data.
func TestFrontendStartupReportNamesMissingMapCauses(t *testing.T) {
	const network = "[GlobalHeader] { numplayers=0; SCHEMACOUNT=1; [Schema 0] { Type=Network 1; } }"
	archive := func(t *testing.T, path string, compress bool) []byte {
		t.Helper()
		var buf bytes.Buffer
		files := []vfs.ArchiveFile{{Path: path, Data: []byte(network)}}
		if err := vfs.WriteArchive(&buf, files, vfs.ArchiveWriteOptions{Compress: compress}); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	root := t.TempDir()
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(root, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("good.ccx", archive(t, "maps/Good Map.ota", false))
	// A trailer one byte short fails the container gate, so discovery skips
	// the whole archive and only a mount note records it [02 §2].
	rejected := archive(t, "maps/Rejected Map.ota", false)
	write("rejected.ccx", rejected[:len(rejected)-1])
	// A payload byte changed after the chunk checksum was taken leaves the
	// archive mountable but its map file unreadable.
	damaged := archive(t, "maps/Damaged Map.ota", true)
	chunk := bytes.Index(damaged, []byte("SQSH"))
	if chunk < 0 {
		t.Fatal("authored compressed archive has no SQSH chunk")
	}
	damaged[chunk+19] ^= 0xff
	write("damaged.ccx", damaged)

	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountGameDirectory(root); err != nil {
		t.Fatal(err)
	}
	census, err := censusSkirmishMaps(fs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(census.names, "|") != "Good Map" {
		t.Fatalf("listed maps %q, want only the readable one", census.names)
	}
	shell := &gameShell{cs: &contentSet{unmappedMount: fs}, maps: census.names, mapCensus: census}
	var report bytes.Buffer
	writeContentStartupReport(&report, shell.cs, 0)
	writeFrontendStartupReport(&report, shell)
	got := report.String()
	for _, want := range []string{
		"nanolathe: content mount: archive rejected: logical path rejected.ccx, providers searched [rejected.ccx]",
		"nanolathe: skirmish map census skipped a map: logical path maps/Damaged Map.ota, providers searched [damaged.ccx], expected a readable OTA file: ",
		"nanolathe: maps: 1 skirmish maps (good.ccx 1); skipped=1\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("startup report lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, root) {
		t.Errorf("startup report names a host path (DESIGN_CONTENT_VFS C13):\n%s", got)
	}
}
