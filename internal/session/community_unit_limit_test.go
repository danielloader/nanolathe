package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
)

// TestBuildTableUnitLimitDoesNotReplaceConfiguredLimit locks the precedence
// behind the configured unit limit (DESIGN_CONTENT_VFS §5,
// DESIGN_COMMUNITY_PATCH §4.1 CP-LIM-2): a build table's shipped preference
// default never replaces the player's `unitLimit` / `--unit-limit`, while a
// limit a source names explicitly — a mod profile, the settings file's
// `gameplayFeatures`, `--gameplay-feature` — still does. Zero means "the
// configured setting". Before this, the mainline table's 1500 replaced every
// configured limit outside Strict 3.1.
func TestBuildTableUnitLimitDoesNotReplaceConfiguredLimit(t *testing.T) {
	if mainline, _ := community.Table(community.Mainline); mainline.UnitLimit == 0 {
		t.Fatal("the mainline table no longer carries a unit-limit default, so this test proves nothing")
	}
	n := func(v int) *int { return &v }
	cases := []struct {
		name    string
		sources CommunitySources
		want    int
	}{
		{"no source names one", CommunitySources{}, 0},
		{"table name alone", CommunitySources{Content: []community.Overrides{{Table: "escalation"}}}, 0},
		{"mod profile names one", CommunitySources{Content: []community.Overrides{{Table: "escalation"}, {UnitLimit: n(1000)}}}, 1000},
		{"player names one", CommunitySources{Content: []community.Overrides{{UnitLimit: n(1000)}}, Player: community.Overrides{UnitLimit: n(600)}}, 600},
		{"player zero restores the setting", CommunitySources{Content: []community.Overrides{{UnitLimit: n(1000)}}, Player: community.Overrides{UnitLimit: n(0)}}, 0},
		{"later table resets", CommunitySources{Content: []community.Overrides{{UnitLimit: n(1000)}}, Player: community.Overrides{Table: "prota"}}, 0},
		{"command line wins", CommunitySources{Player: community.Overrides{UnitLimit: n(600)}, CommandLine: []community.Overrides{{UnitLimit: n(700)}}}, 700},
	}
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Community39} {
		for _, tc := range cases {
			got, err := ResolveCommunity(mode, tc.sources)
			if err != nil {
				t.Fatalf("%s/%s: %v", mode, tc.name, err)
			}
			if got.UnitLimit != tc.want {
				t.Errorf("%s/%s: UnitLimit = %d, want %d", mode, tc.name, got.UnitLimit, tc.want)
			}
		}
	}
	strict, err := ResolveCommunity(gameplay.Strict31, CommunitySources{CommandLine: []community.Overrides{{UnitLimit: n(700)}}})
	if err != nil || strict.UnitLimit != 0 {
		t.Fatalf("Strict 3.1 UnitLimit = %d, %v; want the configured setting (0)", strict.UnitLimit, err)
	}
}
