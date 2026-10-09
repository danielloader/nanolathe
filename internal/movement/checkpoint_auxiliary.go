package movement

import (
	"errors"
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// BindWorldWithCheckpointBinding preserves BindWorld's ordering and installs
// the actual System observer through units' attested setter (§16.3.56, §16.3.61).
func (s *System) BindWorldWithCheckpointBinding(w *units.World, authority *checkpoint.BindingAuthority) {
	s.bindWorld(w, authority)
}

// AttachOverlapBindingWithCheckpointBinding attests only the two callbacks
// installed by the shared helper, including its actual System method value.
func (s *System) AttachOverlapBindingWithCheckpointBinding(ownerState func(uint8) uint8, authority *checkpoint.BindingAuthority) {
	s.attachOverlapBinding(ownerState, authority)
}

type checkpointAuxiliaryBindings struct {
	system    *System
	world     *units.World
	authority *checkpoint.BindingAuthority
}

// SetAuxiliaryBindings records exact owners without calling their interfaces
// or attesting existing callbacks. Path registration agrees in either order.
func (c *CheckpointContext) SetAuxiliaryBindings(s *System, w *units.World, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || authority == nil {
		return movementCheckpointError("movement.auxiliaryBindings", errors.New("missing context, system or authority"))
	}
	binding := checkpointAuxiliaryBindings{s, w, authority}
	if c.system != nil && c.system != s || c.auxiliaryBindings.authority != nil && c.auxiliaryBindings != binding {
		return movementCheckpointError("movement.auxiliaryBindings", errors.New("conflicting auxiliary binding registration"))
	}
	if c.pathBindings.authority != nil && (c.pathBindings.system != s || c.pathBindings.world != w || !c.pathBindings.authority.Matches(authority)) {
		return movementCheckpointError("movement.auxiliaryBindings", errors.New("path world or authority differs"))
	}
	if c.compositionBindings.authority != nil && !c.compositionBindings.authority.Matches(authority) {
		return movementCheckpointError("movement.auxiliaryBindings", errors.New("composition authority differs"))
	}
	if err := binding.validate(s); err != nil {
		return movementCheckpointError("movement.auxiliaryBindings", err)
	}
	c.system, c.auxiliaryBindings = s, binding
	return nil
}

func (b checkpointAuxiliaryBindings) validate(s *System) error {
	if s == nil || s != b.system || s.world != b.world || b.authority == nil {
		return errors.New("auxiliary system or world alias differs")
	}
	return nil
}

// Independent slots retain the grid and the System captured at installation.
// Copying either owner or changing overlap cannot transfer their authority.
type checkpointGridProof struct {
	grid      *OccupancyGrid
	system    *System
	authority *checkpoint.BindingAuthority
}

func (p checkpointGridProof) matches(g *OccupancyGrid, s *System, c *CheckpointContext) bool {
	return c != nil && s != nil && g != nil && s.Grid == g && p.grid == g && p.system == s &&
		c.auxiliaryBindings.system == s && s.world == c.auxiliaryBindings.world && p.authority.Matches(c.auxiliaryBindings.authority)
}

func (g *OccupancyGrid) validateCheckpointCallbacks(s *System, c *CheckpointContext) error {
	if g.ownerState == nil && g.claimConflict == nil {
		return nil
	}
	overlap, ok := g.overlap.(*System)
	if !ok || overlap == nil || overlap != s || s.Grid != g {
		return errors.New("unattested occupancy callback overlap: not the exact movement system")
	}
	if g.ownerState != nil && !g.checkpointOwnerState.matches(g, s, c) {
		return errors.New("unattested occupancy owner-state callback")
	}
	if g.claimConflict != nil && !g.checkpointClaimConflict.matches(g, s, c) {
		return errors.New("unattested occupancy claim-conflict callback")
	}
	return nil
}

// Immutable constructor metadata retains all records, including padding. The
// snapshot is never rebuilt from today's mutable plot [04 R-AIR-01 §5].
type checkpointAirSnapshot struct {
	system                      *System
	grid                        *AirSectorGrid
	terrain                     *world.Terrain
	authority                   *checkpoint.BindingAuthority
	columns, rows, cellW, cellH int32
	records                     []airSector
	sentinel                    airSector
}

func (s *System) snapshotCheckpointAirSectors(authority *checkpoint.BindingAuthority) {
	if authority == nil || s.AirSectors == nil {
		return
	}
	g := s.AirSectors
	s.checkpointAirSectors = &checkpointAirSnapshot{
		system: s, grid: g, terrain: s.Terrain, authority: authority,
		columns: g.Columns, rows: g.Rows, cellW: g.cellW, cellH: g.cellH,
		records: slices.Clone(g.records), sentinel: g.sentinel,
	}
}

func (s *System) validateCheckpointAirSectors(c *CheckpointContext) error {
	p, g := s.checkpointAirSectors, s.AirSectors
	if g == nil {
		if c.auxiliaryBindings.authority != nil && p != nil {
			return errors.New("constructor air-sector grid was removed")
		}
		return nil
	}
	if p == nil || p.system != s || p.grid != g || p.terrain != s.Terrain ||
		c.auxiliaryBindings.system != s || !p.authority.Matches(c.auxiliaryBindings.authority) {
		return errors.New("unattested air-sector constructor ownership")
	}
	if g.Columns != p.columns || g.Rows != p.rows || g.cellW != p.cellW || g.cellH != p.cellH ||
		g.sentinel != p.sentinel || !slices.Equal(g.records, p.records) {
		return errors.New("immutable air-sector snapshot changed")
	}
	return nil
}
