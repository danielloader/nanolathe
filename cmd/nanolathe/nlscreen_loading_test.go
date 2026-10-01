package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func authoredNLPreviewContent(t *testing.T, name string, health int) *contentSet {
	t.Helper()
	unit := "[UNITINFO]{UnitName=" + name + ";objectname=SCRATCH;MaxDamage="
	unit += strconv.Itoa(health) + ";FootprintX=1;FootprintZ=1;weapon1=SCRATCHGUN;}"
	files := []vfs.ArchiveFile{
		{Path: "units/" + name + ".fbi", Data: []byte(unit)},
		{Path: "objects3d/scratch.3do", Data: authoredScratchModel(t)},
		{Path: "gamedata/moveinfo.tdf", Data: []byte("[CLASS0]{Name=TANK;FootprintX=1;FootprintZ=1;}")},
		{Path: "gamedata/sidedata.tdf", Data: []byte("[SIDE0]{name=Scratch;commander=" + name + ";font=scratch.fnt;" + authoredSideAnchors() + "}")},
		{Path: "gamedata/los.tdf", Data: []byte("[TABLEINFO]{numtables=1;}[TABLE1]{numlines=1;line1=1, 0, 1;}")},
		{Path: "anims/vismasks.gaf", Data: authoredVisMaskGAF(t)},
		{Path: "ai/default.txt", Data: []byte("plan 0\n")},
		{Path: "weapons/scratch.tdf", Data: []byte("[SCRATCHGUN]{id=1;name=Scratch Gun;[DAMAGE]{default=10;}}")},
		{Path: "features/notes.txt", Data: []byte("authored fixture: no features\n")},
		{Path: "download/notes.txt", Data: []byte("authored fixture: no placements\n")},
		{Path: "guis/notes.txt", Data: []byte("authored fixture: no build menus\n")},
		{Path: "maps/notes.txt", Data: []byte("authored fixture: no maps\n")},
	}
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, files, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "authored.hpi")
	if err := os.WriteFile(path, archive.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if _, err := fs.MountArchive(path, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return &contentSet{fs: fs, unmappedMount: fs}
}

func TestNLPreviewCatalogIsScopedToMountedContentAndConcurrentReaders(t *testing.T) {
	first := authoredNLPreviewContent(t, "first", 100)
	second := authoredNLPreviewContent(t, "second", 300)
	const readers = 4
	var cats [readers]*content.Catalog
	var errs [readers]error
	var done sync.WaitGroup
	for i := range cats {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			cats[i], errs[i] = first.nlPreviewCatalog()
		}(i)
	}
	done.Wait()
	for i := range cats {
		if errs[i] != nil || cats[i] == nil || cats[i] != cats[0] {
			t.Fatalf("reader %d saw an incomplete or repeated compile: %p, %v", i, cats[i], errs[i])
		}
	}
	other, err := second.nlPreviewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if other == cats[0] || other.Hash == cats[0].Hash || other.Units["first"] != nil || cats[0].Units["second"] != nil {
		t.Fatal("catalog crossed mounted content sets")
	}
	clone := cats[0].Clone()
	mutators, err := content.ParseMutators(map[string]string{"health": "2", "damage": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := clone.ApplyMutators(mutators); err != nil {
		t.Fatal(err)
	}
	if clone.Units["first"].MaxDamage != 200 || clone.Units["first"].Weapon1Def.DamageDefault != 30 {
		t.Fatal("preview clone did not apply mutators to its linked records")
	}
	if cats[0].Units["first"].MaxDamage != 100 || cats[0].Units["first"].Weapon1Def.DamageDefault != 10 {
		t.Fatal("preview mutators changed the cached authored catalog")
	}
	assets := first.nlPreviewModelAssets()
	again := first.nlPreviewModelAssets()
	if assets != again || assets == second.nlPreviewModelAssets() {
		t.Fatal("decoded preview assets were repeated or crossed content sets")
	}
}

func TestNLPreviewCatalogMemoizesCompileFailure(t *testing.T) {
	var absent *contentSet
	if _, err := absent.nlPreviewCatalog(); err == nil {
		t.Fatal("absent content compiled a preview catalog")
	}
	set := &contentSet{}
	_, first := set.nlPreviewCatalog()
	_, second := set.nlPreviewCatalog()
	if first == nil || first != second {
		t.Fatal("one content set retried or lost its catalog failure")
	}
}

func TestNLPreviewSessionCatalogsKeepAuthoredValuesAndMutatorsIsolated(t *testing.T) {
	opts, cs := openNLTestContent(t)
	base, err := cs.nlPreviewCatalog()
	if err != nil {
		t.Fatal(err)
	}
	health := base.Units["armstump"].MaxDamage
	staged, _, err := stageNLSession(opts, cs, nlPresets["armor"], gameplay.Modern, "health=2")
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := stageNLSession(opts, cs, nlPresets["armor"], gameplay.Strict31, "")
	if err != nil {
		t.Fatal(err)
	}
	if staged.s.Catalog == base || plain.s.Catalog == base || staged.s.Catalog == plain.s.Catalog {
		t.Fatal("staged battles share a catalog with another scene or the authored cache")
	}
	if base.Units["armstump"].MaxDamage != health || plain.s.Catalog.Units["armstump"].MaxDamage != health || staged.s.Catalog.Units["armstump"].MaxDamage != health*2 {
		t.Fatal("mutator staging contaminated the authored catalog or a later strict preview")
	}
}
