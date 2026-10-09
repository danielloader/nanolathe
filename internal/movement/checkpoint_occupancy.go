package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func validateMovementGrid(s *System, c *CheckpointContext, g *OccupancyGrid) error {
	if g.inOverlapScan {
		return errors.New("occupancy overlap scan is active")
	}
	if g.plot != nil && g.plot != s.Terrain {
		return errors.New("occupancy terrain alias differs")
	}
	if g.overlap != nil {
		if v, ok := g.overlap.(*System); !ok || v == nil || v != s {
			return errors.New("unsupported occupancy overlap binding")
		}
	}
	if err := g.validateCheckpointCallbacks(s, c); err != nil {
		return err
	}
	// Each plane is independently lazy: resizing preserves nil storage and
	// only a later writer allocates it (DESIGN_MULTIPLAYER §16.3.61).
	size := int64(g.planeW) * int64(g.planeH)
	if g.planeW < 0 || g.planeH < 0 || g.cells != nil && int64(len(g.cells)) != size || g.air != nil && int64(len(g.air)) != size {
		return errors.New("occupancy plane dimensions differ")
	}
	ground, air := 0, 0
	for _, v := range g.cells {
		if v != 0 {
			ground++
		}
	}
	for _, v := range g.air {
		if v != 0 {
			air++
		}
	}
	if ground != g.cellCount || air != g.airCount {
		return errors.New("occupancy counts differ from cells")
	}
	return nil
}

// Identity-plus-one is storage: each logical occupant is presence then i64,
// retaining identity zero distinctly from absence (DESIGN_MULTIPLAYER §16.3.5).
func writeMovementOccupant(e *checkpoint.Encoder, v int32, p string) {
	e.Field(p)
	e.Bool(v != 0)
	if v != 0 {
		e.I64(int64(v) - 1)
	}
}

// Occupancy fields: air, cells, claimConflict, linkSeq, links, offMapHead,
// overlap, ownerState, planeH, planeW, plot, sectorH, sectorHead, sectorW.
// Counts are validated derivations; rev/scan/unfiled are excluded scratch.
func writeMovementGrid(e *checkpoint.Encoder, c *CheckpointContext, s *System, g *OccupancyGrid, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	if err := validateMovementGrid(s, c, g); err != nil {
		m.fail("", err)
		return
	}
	for _, row := range []struct {
		name   string
		values []int32
	}{{"air", g.air}, {"cells", g.cells}} {
		m.count(row.name, len(row.values))
		prefix := p + "." + row.name
		for i, v := range row.values {
			e.FieldIndex(prefix, i, "")
			e.Bool(v != 0)
			if v != 0 {
				e.I64(int64(v) - 1)
			}
		}
	}
	m.boolean("claimConflict", g.claimConflict != nil)
	m.u64("linkSeq", g.linkSeq)
	m.count("links", len(g.links))
	linkPrefix := p + ".links"
	for i, v := range g.links {
		e.FieldIndex(linkPrefix, i, ".linked")
		e.Bool(v.linked)
		e.FieldIndex(linkPrefix, i, ".next")
		e.Bool(v.next != 0)
		if v.next != 0 {
			e.I64(int64(v.next) - 1)
		}
		e.FieldIndex(linkPrefix, i, ".offMap")
		e.Bool(v.offMap)
		e.FieldIndex(linkPrefix, i, ".prev")
		e.Bool(v.prev != 0)
		if v.prev != 0 {
			e.I64(int64(v.prev) - 1)
		}
		e.FieldIndex(linkPrefix, i, ".sx")
		e.I32(v.sx)
		e.FieldIndex(linkPrefix, i, ".sz")
		e.I32(v.sz)
	}
	writeMovementOccupant(e, g.offMapHead, p+".offMapHead")
	m.boolean("overlap", g.overlap != nil)
	m.boolean("ownerState", g.ownerState != nil)
	m.i32("planeH", g.planeH)
	m.i32("planeW", g.planeW)
	m.boolean("plot", g.plot != nil)
	m.i32("sectorH", g.sectorH)
	m.count("sectorHead", len(g.sectorHead))
	sectorPrefix := p + ".sectorHead"
	for i, v := range g.sectorHead {
		e.FieldIndex(sectorPrefix, i, "")
		e.Bool(v != 0)
		if v != 0 {
			e.I64(int64(v) - 1)
		}
	}
	m.i32("sectorW", g.sectorW)
}

// Air-sector identity is nil 0, immutable record 1 + zero-based u32 index,
// or sentinel 2; current position never reconstructs the selected record.
func writeMovementAirSector(e *checkpoint.Encoder, s *System, v *airSector, p string) {
	e.Field(p)
	if v == nil {
		e.U8(0)
		return
	}
	if s.AirSectors != nil {
		if v == &s.AirSectors.sentinel {
			e.U8(2)
			return
		}
		for i := range s.AirSectors.records {
			if v == &s.AirSectors.records[i] {
				e.U8(1)
				e.U32(uint32(i))
				return
			}
		}
	}
	e.Fail(errors.New("selected air sector has no admitted grid identity"))
}
