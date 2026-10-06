package movement

import (
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// VisitUnitsInRadius walks the ordinary spatial sectors intersecting the
// circle, rows before columns and each bucket from its head. Repair patrol's
// random index refers to this order, not unit-slot order [04 R-ORD-02 §4].
// Returning true stops the walk. Off-map, unfiled and attached units are
// absent: radius scans never descend cargo lists [04 R-COLL-01 §11].
func (s *System) VisitUnitsInRadius(x, z, radius numeric.Fixed, visit func(pool.Handle, *units.Unit) bool) {
	if s == nil || s.Grid == nil || visit == nil {
		return
	}
	g := s.Grid
	rx, rz, r := int32(x.Raw()), int32(z.Raw()), int32(radius.Raw())
	loX, hiX := (rx-r)>>sectorWorldShift, (rx+r)>>sectorWorldShift
	loZ, hiZ := (rz-r)>>sectorWorldShift, (rz+r)>>sectorWorldShift
	loX, hiX = max(0, min(loX, g.sectorW-1)), max(0, min(hiX, g.sectorW-1))
	loZ, hiZ = max(0, min(loZ, g.sectorH-1)), max(0, min(hiZ, g.sectorH-1))
	for sz := loZ; sz <= hiZ; sz++ {
		for sx := loX; sx <= hiX; sx++ {
			for e := g.bucketHead(sx, sz); e != 0; {
				id := int(e) - 1
				if u := s.unitFor(pool.Handle(id)); u != nil && u.Attachment.Carrier == 0 && inUnitScanRadius(rx, rz, r, u) && visit(u.Handle, u) {
					return
				}
				e = g.links[id].next
			}
		}
	}
}

// The collector squares signed raw deltas before dropping their fractional
// squares, adds the low words, then compares signed and inclusively. Flooring
// each position before subtraction changes boundary admission [04 R-ORD-02 §4].
func inUnitScanRadius(x, z, radius int32, u *units.Unit) bool {
	square := func(v int32) uint32 { return uint32((int64(v) * int64(v)) >> 32) }
	dx, dz := int32(u.X.Raw())-x, int32(u.Z.Raw())-z
	return int32(square(dx)+square(dz)) <= int32(square(radius))
}
