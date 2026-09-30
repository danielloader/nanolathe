package community

import (
	"fmt"
)

// Mainline names the one feature table the engine carries: the tdraw build
// profile its maintainers ship with every feature enabled, which Community
// 3.9 and Modern start from for the base game (community-patch-engine.md
// sections 3.1 and 4.1; DESIGN_COMMUNITY_PATCH §3.3, D2). The word is the
// build profile's name in the pinned source, not a mod selector.
//
// The engine carries no other table. A mod declares the complete value its
// content was authored for in its own nanolathe-mod.json
// (`rules.communityFeatures`, read by ParseFeatures), and a session receives
// it as a content source whose Base replaces the mainline value
// (docs/DESIGN_MODS_MUTATORS.md §4.2).
const Mainline = "prota"

// mainline is the Mainline table: the effective compile-time matrix and the
// shipped simulation preference defaults of that build profile
// (community-patch-engine.md sections 3.1 and 4.1).
var mainline = Features{
	ConstructionKickout:      true,
	GuardingBuildersHold:     true,
	PatrollingBuilderFilters: true,
	ReclaimToggleKeepsBuild:  true,
	StructureRotation:        true,
	AreaDamageOverflow:       true,
	AreaDamageDedupCap:       true,
	GridClaimTieBreak:        true,
	TransportedExplosions:    true,
	AntinukeCircularCoverage: true,
	AlliedJammingIgnored:     true,
	ResurrectionFinalization: true,
	WeaponTargetKeys:         true,
	Veterancy:                true,
	SchemaUnits:              true,
	ScriptPorts:              true,
	MexSnap:                  true,
	WreckSnap:                true,

	RepairRate: RepairRate{
		RepairMultiplier:   1,
		SelfHealMultiplier: 1,
	},
	OffMapAircraftMarginTiles: 1,
	ProjectileCapacity:        3000,
	ExplosionCapacity:         3000,
	DebrisCapacity:            1000,
	SfxLimit:                  20480,
	PathStepAllowance:         66650,
	UnitLimit:                 1500,
	MexSnapRadius:             3,
	WreckSnapRadius:           1,
	MexSnapRadiusMax:          3,
	WreckSnapRadiusMax:        1,
}

// Table returns the named feature table. Only Mainline exists, so a source's
// `table` key means "start again from the mainline table", discarding every
// earlier source (DESIGN_COMMUNITY_PATCH §3.2).
func Table(name string) (Features, error) {
	if name != Mainline {
		return Features{}, featureError(
			"resolve community features",
			fmt.Sprintf("<table %q>", name),
			"embedded community tables",
			Mainline+" (a mod's own table is rules.communityFeatures in its nanolathe-mod.json)",
			nil,
		)
	}
	return mainline, nil
}
