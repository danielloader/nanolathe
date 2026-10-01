package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/film"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// An authored replacement catalog has no retail identities, and deliberately
// retains Arm/Core prefixes on replacement names. Selection must use capabilities
// and SIDEDATA/build-tree membership rather than interpreting those names twice.
func nlRenamedCatalog() *content.Catalog {
	laser := &content.WeaponDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "laser"}, ID: 1, DamageDefault: 100, Range: 600, ReloadTime: 30, Turret: true}
	cat := &content.Catalog{
		Units:      map[string]*content.UnitDef{},
		Movement:   map[string]*content.MovementClass{"ground": {FootprintX: 2, FootprintZ: 2, MinWaterDepth: -10000, MaxWaterDepth: 0, MaxSlope: 255}},
		BuildMenus: map[string]*content.BuildMenuPage{},
		Weapons:    map[string]*content.WeaponDef{"laser": laser},
		Features:   map[string]*content.FeatureDef{"wreck": {Metal: 50, Reclaimable: true}},
		Sides:      []*content.SideDef{{Name: "FIRST", Commander: "FirstLeader"}, {Name: "SECOND", Commander: "SecondLeader"}, {Name: "THIRD", Commander: "ThirdLeader"}},
	}
	add := func(name, side string, mobile bool) *content.UnitDef {
		u := &content.UnitDef{UnitName: name, Name: name, Side: side, FootprintX: 2, FootprintZ: 2, MaxDamage: 500, BuildCostMetal: 100, BuildTime: 900, SightDistance: 200, MinWaterDepth: -10000, MaxWaterDepth: 0, MaxSlope: 255}
		if mobile {
			u.BMCode, u.CanMove, u.MaxVelocity, u.MovementClass = 1, true, 65536, "ground"
		}
		cat.Units[content.CanonicalKey(name)] = u
		return u
	}
	for _, side := range cat.Sides {
		add(side.Commander, side.Name, true).Commander = true
	}
	for _, p := range []struct{ name, side string }{{"ArmReplacement", "FIRST"}, {"CoreReplacement", "SECOND"}, {"OtherReplacement", "THIRD"}} {
		u := add(p.name, p.side, true)
		u.Weapon1Def, u.Corpse = laser, "wreck"
		u.Weapon1 = "laser"
	}
	// The human constructor uses a distinct authored side tag; the commander's
	// actual visible build tree establishes its faction membership.
	builder := add("CoreHumanBuilder", "FIRST_HUMAN", true)
	builder.Builder, builder.WorkerTime, builder.BuildDistance = true, 60, 100
	tower := add("ArmAuthoredTower", "FIRST_HUMAN", false)
	tower.Weapon1Def = laser
	tower.Weapon1 = "laser"
	add("UnauthorizedRadar", "FIRST", false).RadarDistance = 1000
	add("FirstPower", "FIRST", false).EnergyMake = 20
	add("SecondPower", "SECOND", false).EnergyMake = 20
	cat.BuildMenus["corehumanbuilder"] = &content.BuildMenuPage{Buttons: []string{"ArmAuthoredTower"}}
	cat.BuildMenus["firstleader"] = &content.BuildMenuPage{Buttons: []string{"CoreHumanBuilder"}, BaseButtonCount: 1}
	cat.DownloadPlacements = []content.DownloadMenuPlacement{{Builder: "CoreHumanBuilder", Product: "ArmAuthoredTower", Menu: 2, Button: 1, BuilderResolved: true, ProductResolved: true}}
	return cat
}

func TestNLPreviewRenamedRosterDeterministic(t *testing.T) {
	cat := nlRenamedCatalog()
	first := &nlStage{roster: newNLRoster(cat, nil)}
	second := &nlStage{roster: newNLRoster(cat.Clone(), nil)}
	for _, preferred := range []string{"armflash", "corraid", "armck", "armllt", "armsolar", "corsolar"} {
		a, b := first.name(preferred), second.name(preferred)
		if a == "" || a != b {
			t.Fatalf("%s selected %q and %q", preferred, a, b)
		}
		if got := first.name(a); got != a {
			t.Fatalf("resolved %q changed to %q on a second pass", a, got)
		}
	}
	if first.roster.factions != [2]string{"FIRST", "SECOND"} {
		t.Fatal(first.roster.factions)
	}
	if first.name("armflash") != "armreplacement" || first.name("corraid") != "corereplacement" {
		t.Fatal("faction roster changed")
	}
	if first.name("armck") != "corehumanbuilder" {
		t.Fatal("visible human build tree was ignored")
	}
	if !slices.Equal(first.assetUnitNames(), second.assetUnitNames()) {
		t.Fatal("asset roots changed")
	}
}

func TestNLPreviewBuildProductAndAssetClosure(t *testing.T) {
	cat := nlRenamedCatalog()
	st := &nlStage{roster: newNLRoster(cat, nil)}
	builder, _ := cat.Unit(st.name("armck"))
	product := st.buildProduct(builder, "armrad")
	if product != "armauthoredtower" {
		t.Fatalf("queued %q outside the builder's authored products", product)
	}
	if got := st.name(product); got != product {
		t.Fatalf("product resolved twice: %s", got)
	}
	// Declare every possible event variant before the first scheduled creation.
	scheduled := st.names([]string{"armflash", "corraid", "armflash"})
	st.requireUnit("FirstLeader")
	st.placement = st.name("armllt")
	roots := st.assetUnitNames()
	want := []string{"armauthoredtower", "armreplacement", "corehumanbuilder", "corereplacement", "firstleader"}
	if !slices.Equal(roots, want) {
		t.Fatalf("asset closure %v, want %v", roots, want)
	}
	for _, name := range scheduled {
		if !slices.Contains(roots, name) {
			t.Fatalf("scheduled %s outside closure", name)
		}
	}
	if (&nlStage{}).assetUnitNames() != nil {
		t.Fatal("an undeclared event closure must retain full preparation")
	}
}

func TestNLPreviewRosterUsesAuthoredValues(t *testing.T) {
	authored := nlRenamedCatalog()
	battle := authored.Clone()
	mutators, err := content.ParseMutators(map[string]string{"health": "4", "unitSpeed": "0.25", "buildCost": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := battle.ApplyMutators(mutators); err != nil {
		t.Fatal(err)
	}
	st := &nlStage{roster: newNLRoster(authored, nil)}
	name := st.name("armflash")
	if battle.Units[name].MaxDamage == authored.Units[name].MaxDamage {
		t.Fatal("fixture did not transform battle health")
	}
	if name != "armreplacement" || authored.Units[name].MaxDamage != 500 {
		t.Fatal("selection changed authored content")
	}
}

func TestNLPreviewMissingVisualFamilyExplainsFallback(t *testing.T) {
	st := &nlStage{roster: newNLRoster(nlRenamedCatalog(), nil)}
	if name := st.name("armanac"); name != "armreplacement" {
		t.Fatalf("hovercraft fallback %q", name)
	}
	if st.previewLimit() == "" {
		t.Fatal("missing hovercraft went unexplained")
	}
	missing := &nlStage{roster: newNLRoster(nlRenamedCatalog(), nil)}
	if missing.name("armroy") != "" || strings.Contains(missing.previewLimit(), "units are shown") {
		t.Fatal("an absent naval role claimed to show compatible units")
	}
}

func TestNLPreviewProductsRetainAuthoredMembership(t *testing.T) {
	cat := nlRenamedCatalog()
	builder, _ := cat.Unit("corehumanbuilder")
	menu := cat.BuildMenus["corehumanbuilder"]
	menu.Buttons = []string{"UnauthorizedRadar"}
	menu.AuthoredButtons = []string{"ArmAuthoredTower", "ArmAuthoredTower", "missing"}
	r := newNLRoster(cat, nil)
	if got := r.products(builder); !slices.Equal(got, []string{"armauthoredtower"}) {
		t.Fatalf("preview products %v omit complete authored membership", got)
	}
	if got := construction.BuildProducts(construction.StrictRules{}, menu); !slices.Equal(got, menu.Buttons) {
		t.Fatal("preview selection changed Strict admission")
	}
	if got := construction.BuildProducts(&construction.ModernRules{}, menu); !slices.Equal(got, menu.AuthoredButtons) {
		t.Fatal("preview selection changed Modern admission")
	}
	menu.AuthoredButtons = []string{}
	if got := r.products(builder); len(got) != 0 {
		t.Fatalf("explicitly empty authored membership fell back to %v", got)
	}
	menu.AuthoredButtons = nil
	if got := r.products(builder); !slices.Equal(got, []string{"unauthorizedradar"}) {
		t.Fatalf("nil authored membership did not use legacy products: %v", got)
	}
}

func TestNLPreviewVisibleProductIncludesSlotZero(t *testing.T) {
	cat := nlRenamedCatalog()
	cat.BuildMenus["firstleader"] = &content.BuildMenuPage{}
	cat.DownloadPlacements = []content.DownloadMenuPlacement{
		{Builder: "FirstLeader", Product: "CoreHumanBuilder", Menu: 2, Button: 0, BuilderResolved: true, ProductResolved: true},
		{Builder: "FirstLeader", Product: "UnauthorizedRadar", Menu: 0, Button: 0, BuilderResolved: true, ProductResolved: true},
	}
	if got := nlVisibleProducts(cat, "firstleader"); !slices.Equal(got, []string{"corehumanbuilder"}) {
		t.Fatalf("visible slot-zero products %v", got)
	}
	if !newNLRoster(cat, nil).visible["corehumanbuilder"] {
		t.Fatal("slot-zero constructor was omitted from the visible build tree")
	}
}

func nlRosterSession(cat *content.Catalog) *session.Session {
	for _, name := range cat.SortedUnitKeys() {
		authorTestUnitScripts(cat.Units[name])
	}
	terrain := testWorldON05(256, 160)
	terrain.SeaLevel = 20
	for i := range terrain.Plot {
		terrain.Plot[i].SetHeight(60)
		terrain.Plot[i].SetMinHeight(60)
		terrain.Plot[i].SetMaxHeight(60)
	}
	return &session.Session{State: session.StateBattle, Catalog: cat, World: terrain, Units: units.NewSliced(100, cat), Clock: &clock.State{}, LocalOwner: 0, EnemyOwner: 1}
}

func TestNLPreviewDryRosterNavalFallbackIsInFrame(t *testing.T) {
	cat := nlRenamedCatalog()
	sess := nlRosterSession(cat)
	for z := int32(0); z < sess.World.CellH; z++ {
		for x := int32(0); x < sess.World.CellW/2; x++ {
			p := &sess.World.Plot[z*sess.World.CellW+x]
			p.SetHeight(0)
			p.SetMinHeight(0)
			p.SetMaxHeight(0)
		}
	}
	preset := nlPresets["naval"]
	preset.scene.Anchor = []int32{1024, 1024}
	st, err := stageNLPreviewFixture(preset, sess, cat)
	if err != nil {
		t.Fatal(err)
	}
	if st.cx < sess.World.PlayRight/2 || !strings.Contains(st.previewLimit(), "a ground battle is shown") {
		t.Fatalf("fallback anchor=%d,%d; limit=%s", st.cx, st.cz, st.previewLimit())
	}
	var inFrame [2]bool
	for _, u := range sess.Units.Iter() {
		if u != nil && u.Alive && u.Def.CanMove && wsAbs(int32(u.X.Int())-st.cx) < 260 && wsAbs(int32(u.Z.Int())-st.cz) < 200 {
			inFrame[u.Owner] = true
			if !slices.Contains(st.assetUnitNames(), content.CanonicalKey(u.Def.UnitName)) {
				t.Fatal("fallback combat unit escaped asset scope")
			}
		}
	}
	if inFrame != [2]bool{true, true} {
		t.Fatalf("fallback has no visible combat for both sides: %v", inFrame)
	}
}

func TestNLPreviewVehicleOnlyAirFallbackIsInFrame(t *testing.T) {
	cat := nlRenamedCatalog()
	for _, name := range cat.SortedUnitKeys() {
		u := cat.Units[name]
		if u.CanFly {
			delete(cat.Units, name)
			continue
		}
		if u.CanMove {
			u.Upright = false
		}
	}
	sess := nlRosterSession(cat)
	preset := nlPreset{scene: film.Scene{Kind: "battle", Roster: "air", PerSide: 6, Columns: 3, Gap: 100, Anchor: []int32{3000, 1000}}}
	st, err := stageNLPreviewFixture(preset, sess, cat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.previewLimit(), "FIRST preview faction has no combat aircraft") || !strings.Contains(st.previewLimit(), "SECOND preview faction has no combat aircraft") {
		t.Fatalf("missing-air fallback went unexplained: %s", st.previewLimit())
	}
	roots := st.assetUnitNames()
	if !slices.IsSorted(roots) || len(roots) == 0 {
		t.Fatalf("invalid fallback asset roots %v", roots)
	}
	var inFrame [2]bool
	for _, u := range sess.Units.Iter() {
		if u == nil || !u.Alive {
			continue
		}
		if u.Def.CanFly || u.Def.Upright || !u.Def.CanMove {
			t.Fatalf("fallback created incompatible %s", u.Def.UnitName)
		}
		if !slices.Contains(roots, content.CanonicalKey(u.Def.UnitName)) {
			t.Fatalf("fallback unit %s escaped asset scope", u.Def.UnitName)
		}
		if wsAbs(int32(u.X.Int())-st.cx) < 260 && wsAbs(int32(u.Z.Int())-st.cz) < 200 {
			inFrame[u.Owner] = true
		}
	}
	if inFrame != [2]bool{true, true} {
		t.Fatalf("air fallback has no visible grounded units for both sides: %v", inFrame)
	}
}

func TestNLPreviewPlacementAcceptsOrdinaryTower(t *testing.T) {
	cat := nlRenamedCatalog()
	cat.Weapons["laser"].RenderType = render.RenderTypeBaseSpriteModel
	sess := nlRosterSession(cat)
	commander, _ := cat.Unit("firstleader")
	if _, err := sess.Units.Create(commander, 0, nlFixed(3000), nlFixed(60), nlFixed(1000)); err != nil {
		t.Fatal(err)
	}
	st, err := stageNLPreviewFixture(nlPreset{scene: film.Scene{Kind: "skirmish"}}, sess, cat)
	if err != nil {
		t.Fatal(err)
	}
	if st.placementUnitName() != "armauthoredtower" || st.previewLimit() != "" {
		t.Fatalf("placement=%q; limit=%s", st.placementUnitName(), st.previewLimit())
	}
	if !slices.Contains(st.assetUnitNames(), st.placementUnitName()) {
		t.Fatal("ordinary tower ghost escaped asset scope")
	}
}

func TestNLMutatorExamplesUseActiveContent(t *testing.T) {
	cat := nlRenamedCatalog()
	stats := &nlStats{base: cat}
	for _, key := range []string{"buildSpeed", "health", "damage", "income", "unitSpeed", "sight", "salvage"} {
		stat := nlMutatorStats[key]
		names := stats.statUnitNames(stat)
		if len(names) == 0 {
			t.Errorf("%s has no active-content example", key)
		}
		for _, name := range names {
			u, ok := cat.Unit(name)
			if !ok {
				t.Fatalf("%s example %s is absent", key, name)
			}
			if _, ok := stat.value(cat, u); !ok {
				t.Fatalf("%s example %s carries no scaled value", key, name)
			}
		}
	}
}
