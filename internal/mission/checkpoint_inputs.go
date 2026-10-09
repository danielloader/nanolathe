package mission

import (
	"fmt"
	"slices"

	"github.com/nanolathe-gg/nanolathe/formats"
)

// CheckpointInputs owns detached authored mission values and exact input
// identities. It is admission metadata, not a mission save or wire payload
// (DESIGN_MULTIPLAYER §16.3.74).
type CheckpointInputs struct {
	owner    *Mission
	values   checkpointMissionValues
	units    []UnitPlacement
	specials []Special
	features []FeaturePlacement
	ota      *formats.CheckpointOTAInputs
}

type checkpointMissionValues struct {
	typ                 Type
	terrainKey          string
	schema              Schema
	windBounds          WindBounds
	useOnlyPath         string
	isRestore           bool
	campaignPath        string
	campaignIndex       int
	campaignMissionName string
	difficulty          int
}

func missionCheckpointValues(m *Mission) checkpointMissionValues {
	return checkpointMissionValues{
		typ: m.Type, terrainKey: m.TerrainKey, schema: m.Schema,
		windBounds: m.WindBounds, useOnlyPath: m.UseOnlyPath, isRestore: m.IsRestore,
		campaignPath: m.CampaignPath, campaignIndex: m.CampaignIndex,
		campaignMissionName: m.CampaignMissionName, difficulty: m.Difficulty,
	}
}

// SnapshotCheckpointInputs records the current authored inputs without
// decoding or resolving them. Victory/Defeat progress and the load-order
// diagnostic are runtime/observation state, excluded by §16.3.74; all stored
// placement fields remain retained [08 "Mission object"].
func SnapshotCheckpointInputs(m *Mission) (*CheckpointInputs, error) {
	if m == nil {
		return nil, missionCheckpointInputError("Mission", "a nonnil mission")
	}
	ota, err := formats.SnapshotCheckpointOTAInputs(m.OTA)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", missionCheckpointInputError("Mission.OTA", "snapshot of authored OTA inputs"), err)
	}
	return &CheckpointInputs{
		owner: m, values: missionCheckpointValues(m), ota: ota,
		units: slices.Clone(m.Units), specials: slices.Clone(m.Specials), features: slices.Clone(m.Features),
	}, nil
}

// Validate compares only recorded values and identities. It never resolves
// mission getters or changes trigger state, and successful validation allocates
// no storage (DESIGN_MULTIPLAYER §16.3.74).
func (s *CheckpointInputs) Validate(m *Mission) error {
	if s == nil || s.owner == nil || m == nil || s.owner != m {
		return missionCheckpointInputError("Mission", "the exact snapshotted mission")
	}
	if s.values != missionCheckpointValues(m) {
		return missionCheckpointInputError("Mission", "unchanged authored scalar values")
	}
	if (s.units == nil) != (m.Units == nil) || !slices.Equal(s.units, m.Units) {
		return missionCheckpointInputError("Mission.Units", "unchanged ordered placement values and presence")
	}
	if (s.specials == nil) != (m.Specials == nil) || !slices.Equal(s.specials, m.Specials) {
		return missionCheckpointInputError("Mission.Specials", "unchanged ordered special values and presence")
	}
	if (s.features == nil) != (m.Features == nil) || !slices.Equal(s.features, m.Features) {
		return missionCheckpointInputError("Mission.Features", "unchanged ordered feature values and presence")
	}
	if err := s.ota.Validate(m.OTA); err != nil {
		return fmt.Errorf("%w: %w", missionCheckpointInputError("Mission.OTA", "unchanged authored OTA inputs"), err)
	}
	return nil
}

func missionCheckpointInputError(path, expected string) error {
	return fmt.Errorf("nanolathe: mission checkpoint input validation failed: logical path %s, providers searched [mission], expected %s", path, expected)
}
