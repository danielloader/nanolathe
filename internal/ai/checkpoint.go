package ai

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CollectCheckpointReferences adds only the retained Factory allocation.
// Groups and rallyTargets are raw handles, never allocation graph edges.
// No initializer, profile application or planner runs during capture
// (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.19).
func (m *Manager) CollectCheckpointReferences(c *CheckpointContext) (int, error) {
	if err := m.validateCheckpoint(c); err != nil {
		return 0, err
	}
	_, known := c.Units.Allocations.Find(m.Factory)
	if _, err := c.Units.Allocations.Add(m.Factory); err != nil {
		return 0, err
	}
	if !known {
		return 1, nil
	}
	return 0, nil
}

// WriteCheckpoint writes the existing manager state in source-field lexical
// order. All stored task deadlines, groups, option inputs, placement/rally
// coordinates, engagement latches and nested Strategic/Profile values remain.
// Factory is a table-1 edge; raw group and rally handles use u32. Terrain has
// presence and must be the already-collected singleton in World; capture never
// reads it. Rules use closed stateless tags; Planner also admits the registered
// Modern witness. Admitted bindings and Ext write actual presence at their
// original logical field positions (DESIGN_MULTIPLAYER §16.3.71, §16.3.75).
//
// Exclusions from §16.3.5: broadcastWalk, hostileWalk, rallyWalk and classifyWalk
// are overwritten traversal scratch; strategicTypes and strategicTypesFor are
// the immutable catalog-key cache. Shared and ResumeGenerator are the explicit
// Modern worker/restart exception, not fields to inspect or join. Application
// history and the controller fragment are separate U5/U6 work (§16.3.19).
func (m *Manager) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	if e == nil {
		return aiCheckpointError("ai.encoder", "a checkpoint encoder")
	}
	e.Field("ai.Manager")
	if err := m.validateCheckpoint(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("ai.Manager.BattleSeed")
	e.U32(m.BattleSeed)
	e.Field("ai.Manager.CanPursueAir")
	e.Bool(m.canPursueAir != nil)
	e.Field("ai.Manager.Catalog")
	e.Bool(m.Catalog != nil)
	if err := m.Community.WriteCheckpoint(e); err != nil {
		return err
	}
	e.Field("ai.Manager.ConstructionRules")
	rulesKind, err := construction.CheckpointRulesKind(m.ConstructionRules)
	e.Fail(err)
	e.U8(rulesKind)
	e.Field("ai.Manager.Controller")
	e.U8(uint8(m.Controller))
	e.Field("ai.Manager.ControllerParams")
	e.String(m.ControllerParams)
	e.Field("ai.Manager.Deadlines")
	for _, v := range m.Deadlines {
		e.U32(v)
	}
	e.Field("ai.Manager.Ext")
	e.Bool(m.Ext != nil)
	e.Field("ai.Manager.Factory")
	id, known := c.Units.Allocations.Find(m.Factory)
	if !known {
		e.Fail(aiCheckpointError("ai.Manager.Factory", "a discovered allocation reference"))
		return e.Err()
	}
	e.U16(1)
	e.U32(uint32(id))
	writeCheckpointHandles(e, m.GroupConstruction, "ai.Manager.GroupConstruction")
	writeCheckpointHandles(e, m.GroupExplore, "ai.Manager.GroupExplore")
	writeCheckpointHandles(e, m.GroupNull, "ai.Manager.GroupNull")
	writeCheckpointHandles(e, m.GroupRally, "ai.Manager.GroupRally")
	writeCheckpointHandles(e, m.GroupRegroupA, "ai.Manager.GroupRegroupA")
	writeCheckpointHandles(e, m.GroupRegroupB, "ai.Manager.GroupRegroupB")
	writeCheckpointHandles(e, m.GroupResource, "ai.Manager.GroupResource")
	writeCheckpointHandles(e, m.GroupWaveA, "ai.Manager.GroupWaveA")
	writeCheckpointHandles(e, m.GroupWaveB, "ai.Manager.GroupWaveB")
	e.Field("ai.Manager.IsAlliance")
	e.Bool(m.isAlliance != nil)
	e.Field("ai.Manager.JammerSuppresses")
	e.Bool(m.jammerSuppresses != nil)
	e.Field("ai.Manager.MissionGateFlag")
	e.I32(m.MissionGateFlag)
	e.Field("ai.Manager.OrderBinding")
	e.Bool(m.OrderBinding != nil)
	e.Field("ai.Manager.OriginX")
	e.I64(int64(m.OriginX))
	e.Field("ai.Manager.OriginZ")
	e.I64(int64(m.OriginZ))
	e.Field("ai.Manager.Passive")
	e.Bool(m.Passive)
	e.Field("ai.Manager.Planner")
	plannerKind, err := m.checkpointPlannerKind(c)
	e.Fail(err)
	e.U8(plannerKind)
	e.Field("ai.Manager.Player")
	e.U8(m.Player)
	e.Field("ai.Manager.Profile")
	e.Bool(m.Profile != nil)
	if m.Profile != nil {
		if err := m.Profile.writeCheckpoint(e, c); err != nil {
			return err
		}
	}
	e.Field("ai.Manager.QueueBuildTyped")
	e.Bool(m.queueBuildTyped != nil)
	e.Field("ai.Manager.RNG")
	e.Bool(m.RNG != nil)
	e.Field("ai.Manager.RallyProbeKnown")
	e.Bool(m.rallyProbeKnown != nil)
	e.Field("ai.Manager.RallyShotTimeAdmits")
	e.Bool(m.rallyShotTimeAdmits != nil)
	e.Field("ai.Manager.RallyVisible")
	e.Bool(m.rallyVisible != nil)
	e.Field("ai.Manager.StartOwners")
	e.Bool(m.StartOwners != nil)
	if m.StartOwners != nil {
		e.Count(len(m.StartOwners))
		for _, v := range m.StartOwners {
			e.I8(v)
		}
	}
	e.Field("ai.Manager.StartPositions")
	e.Count(len(m.StartPositions))
	for _, v := range m.StartPositions {
		e.I32(v[0])
		e.I32(v[1])
	}
	m.Strategic.writeCheckpoint(e)
	e.Field("ai.Manager.SurfaceMetal")
	e.I32(m.SurfaceMetal)
	e.Field("ai.Manager.Survival")
	e.Bool(m.Survival != nil)
	e.Field("ai.Manager.Terrain")
	e.Bool(m.Terrain != nil)
	e.Field("ai.Manager.UnitVisible")
	e.Bool(m.unitVisible != nil)
	e.Field("ai.Manager.WeaponMaintenance")
	e.Bool(m.weaponMaintenance != nil)
	e.Field("ai.Manager.countdown")
	e.U8(m.countdown)
	e.Field("ai.Manager.modernWaveAir")
	e.Bool(m.modernWaveAir)
	e.Field("ai.Manager.rallyBestScore")
	e.I32(m.rallyBestScore)
	e.Field("ai.Manager.rallyBestX")
	e.I64(int64(m.rallyBestX))
	e.Field("ai.Manager.rallyBestY")
	e.I64(int64(m.rallyBestY))
	e.Field("ai.Manager.rallyBestZ")
	e.I64(int64(m.rallyBestZ))
	e.Field("ai.Manager.rallyDriftX")
	e.I64(int64(m.rallyDriftX))
	e.Field("ai.Manager.rallyDriftY")
	e.I64(int64(m.rallyDriftY))
	e.Field("ai.Manager.rallyDriftZ")
	e.I64(int64(m.rallyDriftZ))
	e.Field("ai.Manager.rallyInitialized")
	e.Bool(m.rallyInitialized)
	e.Field("ai.Manager.rallyProbeX")
	e.I64(int64(m.rallyProbeX))
	e.Field("ai.Manager.rallyProbeY")
	e.I64(int64(m.rallyProbeY))
	e.Field("ai.Manager.rallyProbeZ")
	e.I64(int64(m.rallyProbeZ))
	writeCheckpointHandles(e, m.rallyTargets, "ai.Manager.rallyTargets")
	e.Field("ai.Manager.unitLossDeadline")
	e.U32(m.unitLossDeadline)
	e.Field("ai.Manager.waveAEngaged")
	e.Bool(m.waveAEngaged)
	e.Field("ai.Manager.waveBEngaged")
	e.Bool(m.waveBEngaged)
	return e.Err()
}

func (m *Manager) validateCheckpoint(c *CheckpointContext) error {
	if m == nil {
		return aiCheckpointError("ai.Manager", "a present manager")
	}
	if c == nil || c.Units == nil || c.Units.Keys == nil || c.World == nil || c.World.Keys != c.Units.Keys {
		return aiCheckpointError("ai.context", "unit and world contexts sharing admitted content keys")
	}
	if m.Terrain != nil && m.Terrain != c.World.Terrain {
		return aiCheckpointError("ai.Manager.Terrain", "the collected singleton terrain")
	}
	if _, err := construction.CheckpointRulesKind(m.ConstructionRules); err != nil {
		return fmt.Errorf("%w: %w", aiCheckpointError("ai.Manager.ConstructionRules", "reviewed stateless rules"), err)
	}
	if _, err := m.checkpointPlannerKind(c); err != nil {
		return err
	}
	if c.modern != nil {
		if err := c.validateModernBindings(m); err != nil {
			return err
		}
	} else if m.Ext != nil {
		return aiCheckpointError("ai.Manager.Ext", "an absent or U6-attested binding (TODO(M3-U6))")
	}
	if err := m.validateCheckpointBindings(c); err != nil {
		return err
	}
	if m.Profile != nil {
		if _, err := m.Profile.checkpointRecordIDs(c); err != nil {
			return err
		}
	}
	for i, v := range m.Strategic.MetalSpots {
		bits := math.Float32bits(v.Metal)
		if bits&0x7f800000 == 0x7f800000 && bits&0x007fffff != 0 {
			return aiCheckpointError(fmt.Sprintf("ai.Manager.Strategic.MetalSpots[%d].Metal", i), "non-NaN binary32")
		}
	}
	return nil
}

func writeCheckpointHandles(e *checkpoint.Encoder, row []pool.Handle, path string) {
	e.Field(path)
	e.Count(len(row))
	for _, v := range row {
		e.U32(uint32(v))
	}
}

func aiCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: AI checkpoint failed: logical path %s, providers searched [ai], expected %s", path, expected)
}
