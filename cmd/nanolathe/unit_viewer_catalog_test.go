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

// The viewer adopts the established cost truncation and mobile display scales
// of [07 R-HUD-03 §8], while sensor and weapon ranges retain world units. A
// compiled reload is ticks / 30 [02 "Weapon record"], not guessed shot timing.
func TestUnitViewerStatsUseCompiledValuesAndSkipInactiveWeapons(t *testing.T) {
	weapon := &content.WeaponDef{
		DefinitionHeader: content.DefinitionHeader{CanonicalKey: "beam"},
		ID:               7, Range: 301, ReloadTime: 31,
	}
	def := &content.UnitDef{
		MaxDamage: 900, BuildCostMetal: 123.75, BuildCostEnergy: 987.5, BuildTime: 4321,
		BMCode: 1, MaxVelocity: 1 << 16, Acceleration: 1 << 14, TurnRate: 600,
		SightDistance: 455, RadarDistance: 1200,
		Weapon1Def: &content.WeaponDef{ID: 0, Name: "Inactive sentinel", Range: 999},
		Weapon3Def: weapon,
	}
	before, weaponBefore := *def, *weapon
	got := unitViewerStats(def)
	want := []unitViewerStat{
		{"Health", "900"}, {"Metal cost", "123"}, {"Energy cost", "987"}, {"Build work", "4321"},
		{"Speed", "12.0 m/s"}, {"Acceleration", "3.00 m/s/s"}, {"Turn rate", "99 deg/s"},
		{"Sight range", "455 world units"}, {"Radar range", "1200 world units"},
		{"Weapon 3", "beam"}, {"W3 range", "301 world units"}, {"W3 base reload", "1.03 s"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("stats = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(*def, before) || !reflect.DeepEqual(*weapon, weaponBefore) {
		t.Fatal("stat formatting mutated the unit or linked weapon")
	}
	// All three active slots fit the sidebar's eighteen-row limit without
	// collapsing identical weapons or inventing aggregate damage figures.
	def.Weapon1Def, def.Weapon2Def = weapon, weapon
	if got := unitViewerStats(def); len(got) > 18 || !slices.Contains(got, unitViewerStat{"Weapon 2", "beam"}) {
		t.Fatalf("three-weapon stats = %+v", got)
	}
	def.BMCode, def.Weapon1Def, def.Weapon2Def, def.Weapon3Def = 0, nil, nil, nil
	for _, stat := range unitViewerStats(def) {
		if stat.Label == "Speed" || stat.Label == "Acceleration" || stat.Label == "Turn rate" || strings.HasPrefix(stat.Label, "Weapon") {
			t.Fatalf("unarmed building retained a mobile or weapon row: %+v", stat)
		}
	}
	def.DiscoveryOnly = true
	if got := unitViewerStats(def); !slices.Equal(got, []unitViewerStat{{"Stats", "Unavailable"}}) {
		t.Fatalf("unparsed gameplay fields presented as stats: %+v", got)
	}
	if got := unitViewerStats(nil); len(got) != 0 {
		t.Fatalf("nil definition stats = %+v", got)
	}
}
