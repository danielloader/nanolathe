package client

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestPreviewModelScopeClosesWeaponsCorpsesAndSceneProducers(t *testing.T) {
	fs, cat, terrain, wreck, source := modelTextureBindingFixture(t)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"burn", "transport", "meteor"} {
		if err := os.WriteFile(filepath.Join(filepath.Dir(source), name+".3do"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A directory mount keeps its discovered file set. Mount the completed
	// authored fixture afresh, as a content reload would.
	fs = vfs.New()
	t.Cleanup(func() { fs.Close() })
	if err := fs.MountDirectory(filepath.Dir(filepath.Dir(source)), 1); err != nil {
		t.Fatal(err)
	}
	weapon := func(id int32, model string) *content.WeaponDef {
		return &content.WeaponDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: model}, ID: id, Name: model, Model: model}
	}
	cat.Weapons["burn"] = weapon(9, "burn")
	cat.Weapons["transport"] = weapon(10, "transport")
	cat.Weapons["meteor"] = weapon(11, "meteor")
	cat.Weapons["unrelated"] = weapon(12, "missing")
	cat.Units["alpha"].Weapon1Def = cat.Weapons["weapontwo"]
	cat.Units["alpha"].TransportedSelfDestructAsDef = cat.Weapons["transport"]
	wreck.BurnWeapon = "burn"
	cat.Units["beta"].ObjectName = "missing"
	cat.Units["beta"].Corpse = "unused_corpse"
	cat.Features["unused_corpse"] = &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "unused_corpse"}, Object: "missing"}
	assets := NewPreviewModelTextureAssets(fs)
	r, err := NewPreviewModelTextureRegistry(assets, cat, terrain, len(terrain.FeatureDefs), []string{"alpha"}, "meteor")
	if err != nil {
		t.Fatal(err)
	}
	if r.unitModel("alpha", 1, "shared") == nil || r.unitModel("beta", 2, "missing") != nil {
		t.Fatal("scope did not isolate the authored unit roots")
	}
	for _, id := range []int32{8, 9, 10, 11} {
		if _, ok := r.projectileByID[id]; !ok {
			t.Fatalf("scope omitted linked weapon or independent producer %d", id)
		}
	}
	if got := r.projectileByID[8].id; got != "7" {
		t.Fatalf("later matching weapon chose load %s, want first full-catalog load 7", got)
	}
	if _, ok := r.projectileByID[12]; ok {
		t.Fatal("scope prepared an unrelated weapon")
	}
	if r.featurePrepared["corpse"] == nil || r.featurePrepared["wreck2"] == nil || r.featurePrepared["unused_corpse"] != nil {
		t.Fatal("scope omitted a reachable corpse successor or prepared an unrelated corpse")
	}
	if _, err := NewModelTextureRegistry(fs, cat, terrain, len(terrain.FeatureDefs)); err == nil {
		t.Fatal("full battle control accepted missing named models")
	}
}

func TestPreviewModelAssetsShareGeometryAndPixelsButKeepCursorsIndependent(t *testing.T) {
	fs, cat, terrain, _, _ := modelTextureBindingFixture(t)
	assets := NewPreviewModelTextureAssets(fs)
	first, err := NewPreviewModelTextureRegistry(assets, cat, terrain, len(terrain.FeatureDefs), nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPreviewModelTextureRegistry(assets, cat.Clone(), terrain, len(terrain.FeatureDefs), nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b := first.unitModel("alpha", 1, "shared"), second.unitModel("alpha", 1, "shared")
	other := first.unitModel("beta", 2, "shared")
	if a == nil || b == nil || other == nil || a == b || a == other || a.compiled == b.compiled || a.compiled == other.compiled {
		t.Fatal("decoded geometry shared a mutable loaded-model identity")
	}
	if &a.compiled.Pieces[0] != &b.compiled.Pieces[0] || &a.compiled.Pieces[0] != &other.compiled.Pieces[0] {
		t.Fatal("preview loads did not share immutable decoded piece arrays")
	}
	ref, _ := first.resolve("tenprimary")
	twinRef, _ := second.resolve("tenprimary")
	if ref.entry != twinRef.entry || ref.frame != twinRef.frame {
		t.Fatal("paired registries decoded the same immutable texture bank twice")
	}
	first.StepPhase7()
	first.StepPhase7()
	if got := first.animatedFrame(a.compiled, 0, 0, ref); got != ref.entry.Frames[2].Frame {
		t.Fatal("first registry did not advance its primitive")
	}
	if got := second.animatedFrame(b.compiled, 0, 0, twinRef); got != twinRef.entry.Frames[0].Frame {
		t.Fatal("first registry advanced the second preview's cursor")
	}
}

func TestPreviewModelScopeKeepsNilFullAndEmptyUnitPreparationDistinct(t *testing.T) {
	fs, cat, terrain, _, _ := modelTextureBindingFixture(t)
	assets := NewPreviewModelTextureAssets(fs)
	empty, err := NewPreviewModelTextureRegistry(assets, cat, terrain, len(terrain.FeatureDefs), []string{})
	if err != nil {
		t.Fatal(err)
	}
	full, err := NewPreviewModelTextureRegistry(assets, cat, terrain, len(terrain.FeatureDefs), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.unitByID) != 0 || len(empty.projectileByID) != 0 || empty.featurePrepared["corpse"] != nil {
		t.Fatal("empty unit scope prepared catalog units or their dependencies")
	}
	if len(full.unitByID) == 0 || len(full.projectileByID) == 0 || full.featurePrepared["corpse"] == nil {
		t.Fatal("nil unit scope did not retain full preparation")
	}
	if empty.featurePrepared["stamped"] == nil || empty.featurePrepared["wreck"] == nil {
		t.Fatal("empty unit scope dropped terrain features or their successors")
	}
	if _, err := NewPreviewModelTextureRegistry(assets, cat, terrain, 0, []string{"missing"}); err == nil {
		t.Fatal("scope accepted an unknown requested unit")
	}
}
