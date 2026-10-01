package client

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

func TestBattleFeatureDefinitionsClosesEveryAdmissionRoot(t *testing.T) {
	mapTree := &content.FeatureDef{}
	missionTree := &content.FeatureDef{}
	restoredTree := &content.FeatureDef{}
	wreck := &content.FeatureDef{FeatureDead: " SECOND_WRECK "}
	secondWreck := &content.FeatureDef{}
	dead := &content.FeatureDef{}
	reclaimed := &content.FeatureDef{}
	burnt := &content.FeatureDef{}
	unrelated := &content.FeatureDef{}
	mapTree.FeatureDeadDef = dead
	missionTree.FeatureReclamateDef = reclaimed
	restoredTree.FeatureBurntDef = burnt
	dead.FeatureBurntDef = burnt
	burnt.FeatureDeadDef = mapTree // A cycle still has a finite closure.
	// A resolved pointer wins even if a synthetic fixture has a stale name.
	mapTree.FeatureDead = "unrelated"
	secondWreck.FeatureReclamate = "missing"
	cat := &content.Catalog{
		Units: map[string]*content.UnitDef{
			"future_unit": {Corpse: " WRECK "},
			"no_corpse":   {},
		},
		Features: map[string]*content.FeatureDef{
			"wreck": wreck, "second_wreck": secondWreck, "unrelated": unrelated,
		},
	}
	// Mission and restored roots need not be in the initial map table or even
	// in this fixture's catalog. Reproduction keeps its parent's definition.
	got := battleFeatureDefinitions(cat, []*content.FeatureDef{mapTree, missionTree, restoredTree, nil, mapTree})
	want := []*content.FeatureDef{mapTree, missionTree, restoredTree, wreck, dead, reclaimed, burnt, secondWreck}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("battle closure = %v, want roots and their transitive links %v", got, want)
	}
	if got := battleFeatureDefinitions(nil, nil); len(got) != 0 {
		t.Fatalf("empty battle has definitions: %v", got)
	}
}

func TestBattleFeatureWarmPreservesSequencesWithoutUnrelatedLoads(t *testing.T) {
	wreck := &content.FeatureDef{Filename: "trees", SeqNameDie: "treedie", SeqNameDieShad: "missing-shadow"}
	burnt := &content.FeatureDef{Filename: "trees", SeqNameReclamate: "treeburn"}
	tree := &content.FeatureDef{Filename: "trees", SeqNameBurn: "treeburn", FeatureBurntDef: burnt}
	unrelated := &content.FeatureDef{Filename: "unrelated", SeqNameDie: "unused"}
	cat := &content.Catalog{
		Units: map[string]*content.UnitDef{"future_unit": {Corpse: "wreck"}},
		Features: map[string]*content.FeatureDef{
			"tree": tree, "burnt": burnt, "wreck": wreck, "unrelated": unrelated,
		},
	}
	whole, scoped := newFeatureSequenceClient(t), newFeatureSequenceClient(t)
	whole.WarmFeatureSequences(cat.Features)
	scoped.WarmBattleFeatureSequences(cat, []*content.FeatureDef{tree})
	if _, seen := scoped.featureGACErr["unrelated"]; seen {
		t.Fatal("battle warm attempted the unrelated bank")
	}
	if _, seen := whole.featureGACErr["unrelated"]; !seen {
		t.Fatal("whole-catalog control did not attempt the unrelated bank")
	}
	// Remove filesystem access before reading even the first event frame:
	// prewarming includes future corpses, successors, and shadow misses.
	scoped.modelFS = nil
	for _, sequence := range []string{"treeburn", "treedie", "missing-shadow"} {
		key := featureSequenceKey("trees", sequence)
		if _, seen := scoped.featureSeqs[key]; !seen {
			t.Fatalf("battle warm omitted %s", sequence)
		}
		if !reflect.DeepEqual(scoped.featureSeqs[key], whole.featureSeqs[key]) {
			t.Fatalf("battle warm changed frame pixels, geometry or cadence for %s", sequence)
		}
		for _, visit := range []int32{-1, 0, 1, 2, 3, 4, 5, 6, 99} {
			if !reflect.DeepEqual(scoped.featureEventFrame("trees", sequence, visit), whole.featureEventFrame("trees", sequence, visit)) {
				t.Fatalf("battle warm changed %s at visit %d", sequence, visit)
			}
		}
	}
}

func TestBattleFeatureWarmIncludesRestOnlyAndShadowOnlyBanks(t *testing.T) {
	for _, def := range []*content.FeatureDef{
		{Filename: "trees", SeqName: "treeburn"},
		{Filename: "trees", SeqNameShad: "treedie"},
	} {
		c := newFeatureSequenceClient(t)
		c.WarmBattleFeatureSequences(nil, []*content.FeatureDef{def})
		if c.featureGAFs["trees"] == nil {
			t.Fatal("rest/shadow bank was left for Draw to decode")
		}
		if err := c.modelFS.Close(); err != nil {
			t.Fatal(err)
		}
		if _, _, _, _, _, ok := c.FeatureSequence("trees", "treeburn", 0); !ok {
			t.Fatal("rest bank pixels unavailable without filesystem access")
		}
	}
}

func TestPreviewFeatureWarmUsesUnitScopeAndSharesOnlyDecodedArt(t *testing.T) {
	first, second := newFeatureSequenceClient(t), newFeatureSequenceClient(t)
	assets := NewPreviewModelTextureAssets(first.modelFS)
	first.modelTextures = &ModelTextureRegistry{previewAssets: assets, previewUnitNames: []string{"product"}}
	second.modelTextures = &ModelTextureRegistry{previewAssets: assets, previewUnitNames: []string{"product"}}
	tree := &content.FeatureDef{Filename: "trees", SeqName: "treeburn"}
	wreck := &content.FeatureDef{Filename: "trees", SeqNameDie: "treedie", FeatureDead: "successor"}
	successor := &content.FeatureDef{Filename: "trees", SeqNameReclamate: "treeburn", FeatureBurntDef: wreck}
	unrelated := &content.FeatureDef{Filename: "unrelated", SeqNameDie: "unused"}
	cat := &content.Catalog{
		Units: map[string]*content.UnitDef{
			"product": {DefinitionHeader: content.DefinitionHeader{CanonicalKey: "product"}, Corpse: "wreck"},
			"unused":  {DefinitionHeader: content.DefinitionHeader{CanonicalKey: "unused"}, Corpse: "unrelated"},
		},
		Features: map[string]*content.FeatureDef{"wreck": wreck, "successor": successor, "unrelated": unrelated},
	}
	first.WarmBattleFeatureSequences(cat, []*content.FeatureDef{tree})
	second.WarmBattleFeatureSequences(cat, []*content.FeatureDef{tree})
	if _, tried := first.featureGACErr["unrelated"]; tried {
		t.Fatal("preview warmed an unrelated unit's corpse bank")
	}
	if first.featureGAFs["trees"] == nil || first.featureGAFs["trees"] != second.featureGAFs["trees"] {
		t.Fatal("paired previews decoded their shared feature pixels twice")
	}
	for _, seq := range []string{"treedie", "treeburn"} {
		key := featureSequenceKey("trees", seq)
		if first.featureSeqs[key] == nil || !reflect.DeepEqual(first.featureSeqs[key], second.featureSeqs[key]) {
			t.Fatalf("future product corpse or successor omitted %s", seq)
		}
	}
	// Each client keeps its own mutable rest cursor even when all decoded
	// frames come from one immutable bank.
	first.featureAnim = map[string]*featureAnimCursor{}
	second.featureAnim = map[string]*featureAnimCursor{}
	view := frame.FeatureView{Filename: "trees", SeqName: "treeburn", Animating: true}
	first.featureFrameFor(view, false)
	second.featureFrameFor(view, false)
	a, b := first.featureAnim["trees|treeburn"], second.featureAnim["trees|treeburn"]
	if a == nil || b == nil || a == b || a.player == b.player {
		t.Fatal("shared feature pixels aliased a mutable rest cursor")
	}
	a.player.Step()
	a.player.Step()
	a.player.Step()
	if index, _ := a.player.FrameIndex(); index == 0 {
		t.Fatal("first feature cursor did not advance in the isolation check")
	}
	if index, _ := b.player.FrameIndex(); index != 0 {
		t.Fatal("feature rest cursor advance crossed the preview boundary")
	}
}
