package ai

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Strategic retains stored maps and battle-entry metal spots without refresh.
// setupDrawsReady gates future constructor/placement work and is retained;
// setupDraws, negRegionW, negRegionH, posRegionW and posRegionH are the excluded
// draw ledger and constructor intermediates. countsWalk is refresh scratch. Bound callbacks
// and Catalog write validated presence (§16.3.5, §16.3.19, §16.3.71).
// ClassVector is C0,C1,C2; PlacementRegion is CellH,CellW,OffsetX,OffsetZ.
func (s *Strategic) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("ai.Manager.Strategic.BuildCapable")
	e.I32(s.BuildCapable)
	e.Field("ai.Manager.Strategic.Catalog")
	e.Bool(s.Catalog != nil)
	e.Field("ai.Manager.Strategic.CenterX")
	e.I64(int64(s.CenterX))
	e.Field("ai.Manager.Strategic.CenterY")
	e.I64(int64(s.CenterY))
	e.Field("ai.Manager.Strategic.CenterZ")
	e.I64(int64(s.CenterZ))
	e.Field("ai.Manager.Strategic.ClassVectors")
	e.Count(len(s.ClassVectors))
	for _, key := range checkpointSortedKeys(s.ClassVectors) {
		e.String(key)
		v := s.ClassVectors[key]
		e.I8(v.C0)
		e.I8(v.C1)
		e.I8(v.C2)
	}
	e.Field("ai.Manager.Strategic.Counts")
	e.Count(len(s.Counts))
	for _, key := range checkpointSortedKeys(s.Counts) {
		e.String(key)
		v := s.Counts[key]
		e.I32(v)
	}
	e.Field("ai.Manager.Strategic.InitVectors")
	e.Count(len(s.InitVectors))
	for _, key := range checkpointSortedKeys(s.InitVectors) {
		e.String(key)
		v := s.InitVectors[key]
		e.I8(v)
	}
	writeCheckpointRegion(e, s.LandRegion, "ai.Manager.Strategic.LandRegion")
	e.Field("ai.Manager.Strategic.LastRefreshTick")
	e.U32(s.LastRefreshTick)
	e.Field("ai.Manager.Strategic.MetalSpots")
	e.Count(len(s.MetalSpots))
	for i, v := range s.MetalSpots {
		path := fmt.Sprintf("ai.Manager.Strategic.MetalSpots[%d]", i)
		e.Field(path + ".CellX")
		e.I16(v.CellX)
		e.Field(path + ".CellZ")
		e.I16(v.CellZ)
		e.Field(path + ".Metal")
		e.F32(v.Metal)
	}
	e.Field("ai.Manager.Strategic.Radius")
	e.I32(s.Radius)
	e.Field("ai.Manager.Strategic.SingleVectors")
	e.Count(len(s.SingleVectors))
	for _, key := range checkpointSortedKeys(s.SingleVectors) {
		e.String(key)
		v := s.SingleVectors[key]
		e.I8(v)
	}
	writeCheckpointRegion(e, s.WaterRegion, "ai.Manager.Strategic.WaterRegion")
	e.Field("ai.Manager.Strategic.energyEnvironment")
	e.Bool(s.energyEnvironment != nil)
	e.Field("ai.Manager.Strategic.liveUnitCount")
	e.U16(s.liveUnitCount)
	e.Field("ai.Manager.Strategic.maxWind")
	e.I32(s.maxWind)
	e.Field("ai.Manager.Strategic.maxWindBound")
	e.Bool(s.maxWindBound)
	e.Field("ai.Manager.Strategic.rebuildRegistry")
	e.Bool(s.rebuildRegistry != nil)
	e.Field("ai.Manager.Strategic.setupDrawsReady")
	e.Bool(s.setupDrawsReady)
	e.Field("ai.Manager.Strategic.unitLimit")
	e.U16(s.unitLimit)
	e.Field("ai.Manager.Strategic.unitLimitBound")
	e.Bool(s.unitLimitBound)
}

func writeCheckpointRegion(e *checkpoint.Encoder, v PlacementRegion, path string) {
	e.Field(path + ".CellH")
	e.I16(v.CellH)
	e.Field(path + ".CellW")
	e.I16(v.CellW)
	e.Field(path + ".OffsetX")
	e.I16(v.OffsetX)
	e.Field(path + ".OffsetZ")
	e.I16(v.OffsetZ)
}
