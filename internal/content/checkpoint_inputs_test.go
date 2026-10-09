package content

import (
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// The graph includes holes, unused records and links outside the name tables.
// They are deliberate authored inputs, not a canonical reconstruction of them.
func checkpointInputsFixture(t *testing.T) *SimulationInputs {
	t.Helper()
	f := newFrozenFixture(t)
	c := f.catalog(t)
	u := c.Units["testunit"]
	u.BuildCostMetal, u.EnergyMake = 1, 1
	u.VeterancyThresholds = []uint32{4, 9}
	u.UnitMask = CategoryMask{words: []uint64{2, 1}}
	u.Unknown = map[string]string{"authored": "value"}
	c.unitRecords = append(c.unitRecords, nil)
	c.Units["absent"] = nil
	w := &WeaponDef{DefinitionHeader: DefinitionHeader{CanonicalKey: "pulse", Hash: "original"}, ID: 7, Range: 19,
		Damage: map[string]int32{"default": 13, "custom": 21}, damageOrder: []string{"custom", "default"}, Unknown: map[string]string{"authored": "weapon"}}
	c.Weapons = map[string]*WeaponDef{"pulse": w, "absent": nil}
	c.weaponRecords = []*WeaponDef{nil, w, nil}
	c.weaponByID = map[int32]*WeaponDef{7: w, 8: nil}
	u.Weapon1Def = &WeaponDef{DefinitionHeader: DefinitionHeader{CanonicalKey: "linked"}, Range: 9}
	foreign := &FeatureDef{DefinitionHeader: DefinitionHeader{CanonicalKey: "successor"}, Damage: 3}
	foreign.FeatureDeadDef = c.Features["tree1"]
	c.Features["tree1"].FeatureDeadDef = foreign
	c.Features["tree1"].SeqNameDie = "missing"
	c.Features["absent"] = nil
	c.Movement = map[string]*MovementClass{"walk": {DefinitionHeader: DefinitionHeader{CanonicalKey: "walk", Hash: "original"}, FootprintX: 1, FootprintZ: 2, MaxWaterDepth: 3, MinWaterDepth: -4, MaxSlope: 5, BadSlope: 6, MaxWaterSlope: 7, BadWaterSlope: 8}, "absent": nil}
	c.Sides = []*SideDef{{DefinitionHeader: DefinitionHeader{CanonicalKey: "side", Hash: "original"}, Commander: "testunit", Anchors: map[string]Rect{"RAW": {X1: 4, Y1: 3, X2: 2, Y2: 1}}}, nil}
	c.Categories = &CategoryRegistry{entries: []CategoryEntry{{Name: "all", Membership: CategoryMask{words: []uint64{6}}}}, byName: map[string]int{"all": 0}}
	c.BuildMenus = map[string]*BuildMenuPage{"testunit": {Builder: "testunit", Buttons: []string{"otherunit"}, AuthoredButtons: []string{"otherunit", "testunit"}, BaseButtonCount: 1}, "absent": nil}
	c.DownloadPlacements = []DownloadMenuPlacement{{Builder: "testunit", Product: "otherunit", Menu: 2, Button: 7, BuilderResolved: true, ProductResolved: true, Provenance: Provenance{LogicalPath: "download/fixture.tdf"}, FileOrder: 2, ItemOrder: 3}}
	c.Meteor = &MeteorDefaults{MeteorDensity: 1, MeteorInterval: 2, MeteorDuration: 3}
	c.Sight = &SightShapes{Shapes: []SightShape{{W: 2, H: 1, AnchorX: 1, Opaque: []bool{true, false}}}}
	c.LOS = &LOSTables{DefinitionHeader: DefinitionHeader{Hash: "original"}, NumTables: 1, Tables: []LOSTable{{TableNum: 1, NumLines: 1, Lines: [][]int32{{1, 2, 3}, nil, {7, 8}}}, {TableNum: 9, Lines: [][]int32{{9, 10}}}}}
	c.SurvivalRoster = &SurvivalRoster{Units: []SurvivalRosterEntry{{Unit: "testunit", Tier: 1}}, IncludeBuildTree: true}
	otaBytes := []byte("[GlobalHeader]{ gravity=112; tidalstrength=1; [Schema 0]{type=Network 1; meteordensity=1;} }")
	f.write(t, "maps/test.ota", otaBytes)
	tnt := make([]byte, 64)
	binary.LittleEndian.PutUint32(tnt, 0x2000)
	f.write(t, "maps/test.tnt", tnt)
	ota, err := formats.LoadOTA(otaBytes)
	if err != nil {
		t.Fatal(err)
	}
	c.Maps = map[string]*MapHeader{"test": compileMapHeader("maps/test.ota", "maps/test.tnt", Provenance{}, Provenance{}, ota, tntHeaderLite{Version: 0x2000})}
	f.fs = mountFrozenFixture(t, f.root)
	in := f.freeze(t, SimulationInputRequest{Catalog: c, MapTNT: "maps/test.tnt"})
	if err := in.ValidateCheckpointInputs(); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestCheckpointInputsRejectMutationFromFreeze(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SimulationInputs)
	}{
		{"unit deletion", func(in *SimulationInputs) { delete(in.catalog.Units, "testunit") }},
		{"unit foreign equal pointer", func(in *SimulationInputs) { u := *in.catalog.Units["testunit"]; in.catalog.Units["testunit"] = &u }},
		{"unit nil row deletion", func(in *SimulationInputs) { delete(in.catalog.Units, "absent") }},
		{"unit index poisoned", func(in *SimulationInputs) { in.catalog.Units["testunit"] = in.catalog.Units["otherunit"] }},
		{"unit trailing hole", func(in *SimulationInputs) { in.catalog.unitRecords = in.catalog.unitRecords[:2] }},
		{"unit record ordering", func(in *SimulationInputs) { slices.Reverse(in.catalog.unitRecords) }},
		{"unit extension", func(in *SimulationInputs) { in.catalog.Units["testunit"].NanolatheInfector = true }},
		{"unit float32 below rounding", func(in *SimulationInputs) { in.catalog.Units["testunit"].BuildCostMetal = math.Nextafter32(1, 2) }},
		{"unit float64 below rounding", func(in *SimulationInputs) { in.catalog.Units["testunit"].EnergyMake = math.Nextafter(1, 2) }},
		{"unit negative zero", func(in *SimulationInputs) { in.catalog.Units["testunit"].EnergyUse = math.Copysign(0, -1) }},
		{"unit NaN", func(in *SimulationInputs) { in.catalog.Units["testunit"].EnergyMake = math.NaN() }},
		{"unit mask backing", func(in *SimulationInputs) { in.catalog.Units["testunit"].UnitMask.words[1]++ }},
		{"unit veteran backing", func(in *SimulationInputs) { in.catalog.Units["testunit"].VeterancyThresholds[1]++ }},
		{"unit unknown backing", func(in *SimulationInputs) { in.catalog.Units["testunit"].Unknown["authored"] = "changed" }},
		{"unit resolved weapon", func(in *SimulationInputs) { in.catalog.Units["testunit"].Weapon1Def = in.catalog.Weapons["pulse"] }},
		{"linked weapon outside index", func(in *SimulationInputs) { in.catalog.Units["testunit"].Weapon1Def.Range++ }},
		{"program code", func(in *SimulationInputs) { in.catalog.Units["testunit"].Script.Code[0]++ }},
		{"program index", func(in *SimulationInputs) { in.catalog.Units["testunit"].Script.Scripts["create"] = 42 }},
		{"program ordered entries", func(in *SimulationInputs) { in.catalog.Units["testunit"].Script.ScriptsByID[0]++ }},
		{"weapon deletion", func(in *SimulationInputs) { delete(in.catalog.Weapons, "pulse") }},
		{"weapon nil", func(in *SimulationInputs) { in.catalog.Weapons["pulse"] = nil }},
		{"weapon foreign equal pointer", func(in *SimulationInputs) { w := *in.catalog.Weapons["pulse"]; in.catalog.Weapons["pulse"] = &w }},
		{"weapon record hole", func(in *SimulationInputs) { in.catalog.weaponRecords[2] = in.catalog.Weapons["pulse"] }},
		{"weapon ID index", func(in *SimulationInputs) { in.catalog.weaponByID[7] = nil }},
		{"weapon damage", func(in *SimulationInputs) { in.catalog.Weapons["pulse"].Damage["custom"]++ }},
		{"weapon damage order", func(in *SimulationInputs) { slices.Reverse(in.catalog.Weapons["pulse"].damageOrder) }},
		{"weapon stale hash", func(in *SimulationInputs) { in.catalog.Weapons["pulse"].Range++ }},
		{"feature deletion", func(in *SimulationInputs) { delete(in.catalog.Features, "rock") }},
		{"feature foreign equal pointer", func(in *SimulationInputs) { f := *in.catalog.Features["rock"]; in.catalog.Features["rock"] = &f }},
		{"feature stale hash", func(in *SimulationInputs) { in.catalog.Features["tree1"].SpreadChance++ }},
		{"linked feature outside index", func(in *SimulationInputs) { in.catalog.Features["tree1"].FeatureDeadDef.Damage++ }},
		{"feature resolved link", func(in *SimulationInputs) { in.catalog.Features["tree1"].FeatureDeadDef.FeatureDeadDef = nil }},
		{"movement stale hash", func(in *SimulationInputs) { in.catalog.Movement["walk"].MaxSlope++ }},
		{"side stale hash", func(in *SimulationInputs) { in.catalog.Sides[0].Commander = "otherunit" }},
		{"side raw anchor", func(in *SimulationInputs) {
			in.catalog.Sides[0].Anchors["raw"] = in.catalog.Sides[0].Anchors["RAW"]
			delete(in.catalog.Sides[0].Anchors, "RAW")
		}},
		{"side anchor value", func(in *SimulationInputs) { in.catalog.Sides[0].Anchors["RAW"] = Rect{} }},
		{"category index poisoned", func(in *SimulationInputs) { in.catalog.Categories.byName["all"] = 1 }},
		{"category mask backing", func(in *SimulationInputs) { in.catalog.Categories.entries[0].Membership.words[0]++ }},
		{"limits", func(in *SimulationInputs) { in.catalog.Limits.Units++ }},
		{"build final buttons", func(in *SimulationInputs) { in.catalog.BuildMenus["testunit"].Buttons[0] = "testunit" }},
		{"build authored buttons", func(in *SimulationInputs) { in.catalog.BuildMenus["testunit"].AuthoredButtons[1] = "otherunit" }},
		{"download position", func(in *SimulationInputs) { in.catalog.DownloadPlacements[0].Button++ }},
		{"download logical path", func(in *SimulationInputs) { in.catalog.DownloadPlacements[0].Provenance.LogicalPath = "different" }},
		{"meteor float bits", func(in *SimulationInputs) { in.catalog.Meteor.MeteorDensity = math.Nextafter32(1, 2) }},
		{"sight backing", func(in *SimulationInputs) { in.catalog.Sight.Shapes[0].Opaque[1] = true }},
		{"LOS unused line", func(in *SimulationInputs) { in.catalog.LOS.Tables[0].Lines[2][0]++ }},
		{"LOS unused table", func(in *SimulationInputs) { in.catalog.LOS.Tables[1].Lines[0][0]++ }},
		{"roster tier", func(in *SimulationInputs) { in.catalog.SurvivalRoster.Units[0].Tier++ }},
		{"roster tree", func(in *SimulationInputs) { in.catalog.SurvivalRoster.IncludeBuildTree = false }},
		{"model deletion", func(in *SimulationInputs) { delete(in.models, "objects3d/fixture.3do") }},
		{"model foreign equal pointer", func(in *SimulationInputs) {
			m := *in.models["objects3d/fixture.3do"]
			in.models["objects3d/fixture.3do"] = &m
		}},
		{"model vertex", func(in *SimulationInputs) { in.models["objects3d/fixture.3do"].Pieces[0].Vertices[0][1]++ }},
		{"model hierarchy", func(in *SimulationInputs) { in.models["objects3d/fixture.3do"].Pieces[0].Parent++ }},
		{"model top", func(in *SimulationInputs) { in.modelTops["objects3d/fixture.3do"]++ }},
		{"sequence deletion", func(in *SimulationInputs) { delete(in.simArt.sequences, "trees|treeburn") }},
		{"sequence known miss deletion", func(in *SimulationInputs) { delete(in.simArt.sequences, "trees|missing") }},
		{"sequence visits", func(in *SimulationInputs) { in.simArt.sequences["trees|treeburn"].visits++ }},
		{"sequence delay", func(in *SimulationInputs) { in.simArt.sequences["trees|treeburn"].frames[0].delay++ }},
		{"sequence geometry", func(in *SimulationInputs) { in.simArt.sequences["trees|treeburn"].frames[0].xoff++ }},
		{"effect hold", func(in *SimulationInputs) { in.simArt.effects["fx|smoke 1"][0]++ }},
		{"effect deletion", func(in *SimulationInputs) { delete(in.simArt.effects, "fx|smoke 1") }},
		{"map deletion", func(in *SimulationInputs) { delete(in.catalog.Maps, "test") }},
		{"map foreign equal pointer", func(in *SimulationInputs) { m := *in.catalog.Maps["test"]; in.catalog.Maps["test"] = &m }},
		{"map stale hash", func(in *SimulationInputs) { in.catalog.Maps["test"].Gravity++ }},
		{"map float bits", func(in *SimulationInputs) { in.catalog.Maps["test"].TidalStrength = math.Nextafter(1, 2) }},
		{"schema float bits", func(in *SimulationInputs) { in.catalog.Maps["test"].Schemas[0].MeteorDensity = math.Nextafter(1, 2) }},
		{"manifest deletion", func(in *SimulationInputs) { in.manifest = in.manifest[1:] }},
	}
	for _, afterKeys := range []bool{false, true} {
		phase := "before-first-keys"
		if afterKeys {
			phase = "after-keys"
		}
		t.Run(phase, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					in := checkpointInputsFixture(t)
					if afterKeys {
						if _, err := in.CheckpointKeys(); err != nil {
							t.Fatal(err)
						}
					}
					tt.mutate(in)
					for n := 0; n < 2; n++ {
						if err := in.ValidateCheckpointInputs(); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint reference failed: logical path ") {
							t.Fatalf("mutation not refused in owner error shape: %v", err)
						}
						if _, err := in.CheckpointKeys(); err == nil {
							t.Fatal("keys accepted changed freeze-time input")
						}
					}
				})
			}
		})
	}
}

func TestCheckpointMovementClassesActualMembershipAndValues(t *testing.T) {
	in := checkpointInputsFixture(t)
	keys, err := in.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	clonedMap := maps.Clone(in.catalog.Movement)
	if err := keys.ValidateMovementClasses(clonedMap); err != nil {
		t.Fatal(err)
	}
	m := clonedMap["walk"]
	for _, field := range []*int32{&m.FootprintX, &m.FootprintZ, &m.MaxWaterDepth, &m.MinWaterDepth, &m.MaxSlope, &m.BadSlope, &m.MaxWaterSlope, &m.BadWaterSlope} {
		*field++
		if keys.ValidateMovementClasses(clonedMap) == nil {
			t.Fatal("changed class value accepted")
		}
		*field--
	}
	foreign := *m
	clonedMap["walk"] = &foreign
	if keys.ValidateMovementClasses(clonedMap) == nil {
		t.Fatal("same-value foreign class accepted")
	}
	clonedMap["walk"] = m
	delete(clonedMap, "absent")
	if keys.ValidateMovementClasses(clonedMap) == nil {
		t.Fatal("removed nil row accepted")
	}
	clonedMap["absent"] = nil
	clonedMap["extra"] = nil
	if keys.ValidateMovementClasses(clonedMap) == nil {
		t.Fatal("additional nil row accepted")
	}
	delete(clonedMap, "extra")
	if err := keys.ValidateMovementClasses(clonedMap); err != nil {
		t.Fatal(err)
	}
	if err := in.ValidateCheckpointInputs(); err != nil {
		t.Fatal("class validation changed original:", err)
	}
}

func TestCheckpointInputsExclusionsSemanticProgramAndPurity(t *testing.T) {
	in := checkpointInputsFixture(t)
	beforeManifest, beforeDigest := in.Manifest(), in.Digest()
	u := in.catalog.Units["testunit"]
	p := *u.Script
	p.Code, p.Scripts, p.Pieces, p.ScriptsByID = slices.Clone(p.Code), maps.Clone(p.Scripts), slices.Clone(p.Pieces), slices.Clone(p.ScriptsByID)
	u.Script = &p // Program identity remains semantic, not pointer identity.
	u.Provenance = Provenance{LogicalPath: "host-only", ProviderID: "other", MountOrder: 99}
	in.catalog.Warnings = []string{"host-only"}
	in.catalog.weaponDuplicates = []WeaponDuplicate{{}}
	in.catalog.Sounds = map[string]*SoundCategory{"host-only": {}}
	in.catalog.Aliases = map[string]*SoundAlias{"host-only": {}}
	in.catalog.Maps["unselected"] = &MapHeader{Gravity: 999}
	in.catalog.sortedModels, in.catalog.modelIndex = []string{"unused cache"}, map[string]int{"unused cache": 42}
	in.catalog.SurvivalRoster.AttackerSkin = "host-only"
	in.catalog.DownloadPlacements[0].Provenance.ProviderID = "other"
	in.catalog.DownloadPlacements[0].Provenance.MountOrder++
	in.simArt.compiled, in.simArt.requests, in.simArt.sources = false, [32]byte{1}, nil
	in.simArt.diagnostics = []EffectBankDiagnostic{{Reason: "host-only"}}
	in.view = struct{ vfs.FSOps }{} // Every filesystem call panics.
	for n := 0; n < 3; n++ {
		if err := in.ValidateCheckpointInputs(); err != nil {
			t.Fatal(err)
		}
		if _, err := in.CheckpointKeys(); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(beforeManifest, in.Manifest()) || beforeDigest != in.Digest() {
		t.Fatal("capture changed M2 identity")
	}
	if u.Script != &p || in.catalog.modelIndex["unused cache"] != 42 || in.simArt.compiled || in.simArt.sources != nil {
		t.Fatal("capture repaired excluded caches or pointer identity")
	}
	if allocs := testing.AllocsPerRun(10, func() {
		if err := in.ValidateCheckpointInputs(); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("validation allocated: %v", allocs)
	}
}

func TestCheckpointInputModelSnapshotDetachesEveryNestedSlice(t *testing.T) {
	m := &model.Model{Name: "authored", Root: 0, Hash: [32]byte{17}, Pieces: []model.Piece{{Name: "root", Children: []int{1}, Vertices: [][3]numeric.Fixed{{2, 0, 0}}, Primitives: []model.Primitive{{VertexIndices: []uint16{0}, TextureName: "skin", SourceIndex: 3}}}}}
	s := snapshotCheckpointInputModel(m)
	for _, mutate := range []func(){
		func() { m.Pieces[0].Children[0]++ },
		func() { m.Pieces[0].Vertices[0][0]++ },
		func() { m.Pieces[0].Primitives[0].VertexIndices[0]++ },
	} {
		mutate()
		if sameCheckpointInputModel(m, &s) {
			t.Fatal("shared model backing storage")
		}
		*m = snapshotCheckpointInputModel(&s)
	}
	if !sameCheckpointInputModel(m, &s) {
		t.Fatal("restored model refused")
	}
}

func TestCheckpointInputsMissingAdmissionAndEmptyClassContainer(t *testing.T) {
	for _, in := range []*SimulationInputs{nil, {}, {catalog: &Catalog{}, view: struct{ vfs.FSOps }{}}} {
		if in.ValidateCheckpointInputs() == nil {
			t.Fatal("missing freeze accepted")
		}
	}
	for _, k := range []*CheckpointKeys{nil, {}} {
		if k.ValidateMovementClasses(nil) == nil {
			t.Fatal("missing admission accepted")
		}
	}
	in := newFrozenFixture(t).freeze(t, SimulationInputRequest{})
	keys, err := in.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	if in.catalog.Movement != nil {
		t.Fatal("fixture should have absent movement map")
	}
	if err := keys.ValidateMovementClasses(map[string]*MovementClass{}); err != nil {
		t.Fatal("empty membership rejected:", err)
	}
	if err := keys.ValidateMovementClasses(map[string]*MovementClass{"absent": nil}); err == nil {
		t.Fatal("nil entry confused with absent key")
	}
}

func TestCheckpointInputsRetainNilShapesAndFailureOrder(t *testing.T) {
	in := checkpointInputsFixture(t)
	// A present empty nested row differs from its physical nil hole.
	in.catalog.LOS.Tables[0].Lines[1] = []int32{}
	if err := in.ValidateCheckpointInputs(); err == nil {
		t.Fatal("nil line shape lost")
	}
	in.catalog.LOS.Tables[0].Lines[1] = nil
	if err := in.ValidateCheckpointInputs(); err != nil {
		t.Fatal(err)
	}
	// Frozen raw key order, rather than later map iteration, determines the
	// first failure. Record ordering precedes the independent name index.
	in.catalog.Units["otherunit"].NanolatheInfector = true
	in.catalog.Units["testunit"].NanolatheInfector = true
	for n := 0; n < 8; n++ {
		err := in.ValidateCheckpointInputs()
		if err == nil || !strings.Contains(err.Error(), "logical path catalog.unit[0],") {
			t.Fatalf("unstable first failure: %v", err)
		}
	}
}
