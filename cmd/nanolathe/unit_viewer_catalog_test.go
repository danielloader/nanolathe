package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

func TestUnitViewerCatalogOrderIdentityAndIsolation(t *testing.T) {
	alpha := &content.UnitDef{UnitName: "ZETA", Name: "Alpha", UnitDefID: 6}
	first := &content.UnitDef{UnitName: "ALPHA", Name: "Scout", UnitDefID: 2}
	duplicate := &content.UnitDef{UnitName: "ALPHA", Name: "Scout", UnitDefID: 3}
	last := &content.UnitDef{UnitName: "BETA", Name: "scout", UnitDefID: 4}
	unnamed := &content.UnitDef{UnitName: "", Name: "Unknown record", UnitDefID: 1}
	cat := &content.Catalog{
		Units: map[string]*content.UnitDef{
			"alpha": first, "duplicate": duplicate, "last": last,
			"zeta": alpha, "": unnamed, "nil": nil,
		},
		// Only one listed definition is buildable. The viewer still keeps
		// the unlisted, duplicate and unnamed records.
		BuildMenus: map[string]*content.BuildMenuPage{
			"builder": {Buttons: []string{"ALPHA"}},
		},
	}
	before := cat.UnitRecords()
	definitions := make([]content.UnitDef, 0, len(before))
	for _, def := range before {
		if def != nil {
			definitions = append(definitions, *def)
		}
	}
	got := unitViewerEntries(cat)
	want := []unitViewerEntry{
		{Key: "zeta", Def: alpha},
		{Key: "alpha", Def: first},
		{Key: "alpha", Def: duplicate},
		{Key: "beta", Def: last},
		{Key: "", Def: unnamed},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("viewer entries = %+v, want %+v", got, want)
	}
	if !slices.Equal(cat.UnitRecords(), before) {
		t.Fatal("viewer reordered catalog records")
	}
	j := 0
	for _, def := range before {
		if def != nil {
			if !reflect.DeepEqual(*def, definitions[j]) {
				t.Fatal("viewer mutated a definition")
			}
			j++
		}
	}
	got[0] = unitViewerEntry{}
	if again := unitViewerEntries(cat); !slices.Equal(again, want) {
		t.Fatal("editing the viewer list changed the catalog or a later list")
	}
	if got := unitViewerEntries(nil); len(got) != 0 {
		t.Fatalf("nil catalog has entries: %+v", got)
	}
}

func TestUnitViewerSearchMatchesNameAndIDTokensWithoutReordering(t *testing.T) {
	entries := []unitViewerEntry{
		{Key: "arm_scout", Def: &content.UnitDef{UnitName: "ARMSCOUT", Name: "Swift Scout", Description: "secret phrase"}},
		{Key: "core_scout", Def: &content.UnitDef{UnitName: "CORSCOUT", Name: "Scout Tank"}},
		{Key: "arm_tank", Def: &content.UnitDef{UnitName: "ARMTANK", Name: "Heavy Tank"}},
	}
	before := slices.Clone(entries)
	for _, tc := range []struct {
		query string
		want  []unitViewerEntry
	}{
		{"  \t\n", entries},
		{"SCOUT", entries[:2]},
		{"  sWiFt\tArM  ", entries[:1]},
		{"ARMSCOUT", entries[:1]},
		{"arm_ scout", entries[:1]},
		{"TANK core", entries[1:2]},
		{"secret phrase", nil},
		{"missing", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got := filterUnitViewerEntries(entries, tc.query)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("search %q = %+v, want %+v", tc.query, got, tc.want)
			}
			if len(got) != 0 {
				got[0] = unitViewerEntry{}
			}
			if !slices.Equal(entries, before) {
				t.Fatal("filter result aliases its source entry slice")
			}
		})
	}
}

// unitViewerPairs collects the label/value/unit rows in order.
func unitViewerPairs(rows []unitViewerRow) []unitViewerStat {
	var out []unitViewerStat
	for _, r := range rows {
		if r.Kind == unitViewerRowPair {
			out = append(out, unitViewerStat{r.Label, strings.TrimSpace(r.Value + " " + r.Unit)})
		}
	}
	return out
}

type unitViewerStat struct{ Label, Value string }

// The viewer adopts the established cost truncation and mobile display scales
// of [07 R-HUD-03 §8], while sensor and weapon ranges retain world units. A
// compiled reload is ticks / 30 [02 "Weapon record"], not guessed shot timing;
// the blast radius is the halved authored diameter [06 §9.3].
func TestUnitViewerStatsUseCompiledValuesAndSkipInactiveWeapons(t *testing.T) {
	weapon := &content.WeaponDef{
		DefinitionHeader: content.DefinitionHeader{CanonicalKey: "beam"},
		ID:               7, Range: 301, ReloadTime: 30, AreaOfEffect: 49, DamageDefault: 40, Burst: 3, BurstRate: 3,
		WeaponVelocity: 65536, Turret: true,
		Damage: map[string]int32{"ARMCOM": 40, "CORCOM": 10, "corfast": 10, "armpw": 70},
	}
	def := &content.UnitDef{
		MaxDamage: 900, BuildCostMetal: 123.75, BuildCostEnergy: 987.5, BuildTime: 4321,
		BMCode: 1, MaxVelocity: 1 << 16, Acceleration: 1 << 14, TurnRate: 600,
		SightDistance: 455, RadarDistance: 1200, EnergyUse: -20,
		Weapon1Def: &content.WeaponDef{ID: 0, Name: "Inactive sentinel", Range: 999},
		Weapon3Def: weapon,
	}
	before, weaponBefore := *def, *weapon
	got := unitViewerPairs(unitViewerStatsRows(def, 264))
	want := []unitViewerStat{
		{"Health", "900"}, {"Energy cost", "987"}, {"Metal cost", "123"}, {"Build work", "4321"}, {"Energy use", "-20 /s"},
		{"Speed", "12.0 m/s"}, {"Acceleration", "3.00 m/s/s"}, {"Turn rate", "99 deg/s"},
		{"Sight", "455 wu"}, {"Radar", "1200 wu"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("stats = %+v, want %+v", got, want)
	}
	weapons := unitViewerWeaponRows(def, 264)
	got = unitViewerPairs(weapons)
	want = []unitViewerStat{
		{"Damage", "40"}, {"vs ARMPW", "70"}, {"vs 2 units", "10"}, {"Base reload", "1.00 s"},
		{"Burst", "3 shots"}, {"Burst interval", "0.10 s"}, {"Range", "301 wu"}, {"Blast radius", "24 wu"},
		{"Velocity", "30 wu/s"}, {"Nominal DPS", "120.0"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("weapon card = %+v, want %+v", got, want)
	}
	if weapons[1].Kind != unitViewerRowCard || weapons[1].Label != "beam" || weapons[1].Value != "Weapon 3" {
		t.Fatalf("weapon card title = %+v", weapons[1])
	}
	if !reflect.DeepEqual(*def, before) || !reflect.DeepEqual(*weapon, weaponBefore) {
		t.Fatal("stat formatting mutated the unit or linked weapon")
	}
	// Manual and stockpiled launches have no reload cadence to divide by.
	weapon.CommandFire = true
	if slices.ContainsFunc(unitViewerPairs(unitViewerWeaponRows(def, 264)), func(s unitViewerStat) bool { return s.Label == "Nominal DPS" }) {
		t.Fatal("command-fire weapon shows a nominal DPS")
	}
	def.BMCode, def.Weapon3Def, def.EnergyUse = 0, nil, 0
	for _, stat := range unitViewerPairs(unitViewerStatsRows(def, 264)) {
		if stat.Label == "Speed" || stat.Label == "Acceleration" || stat.Label == "Turn rate" || stat.Value == "0" {
			t.Fatalf("building retained a mobile or zero row: %+v", stat)
		}
	}
	if got := unitViewerWeaponRows(def, 264); !slices.ContainsFunc(got, func(r unitViewerRow) bool { return r.Label == "No active weapons." }) {
		t.Fatal("unarmed definition did not say so")
	}
	def.DiscoveryOnly = true
	if got := unitViewerPairs(unitViewerStatsRows(def, 264)); len(got) != 0 {
		t.Fatalf("unparsed gameplay fields presented as stats: %+v", got)
	}
	if got := unitViewerStatsRows(nil, 264); len(got) != 0 {
		t.Fatalf("nil definition stats = %+v", got)
	}
}
