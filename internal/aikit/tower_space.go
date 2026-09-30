package aikit

import (
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// BuildExact retains a defensive feature's planned line and gaps. A blocked
// site fails normally; it gains no special placement admission (§16.8).
func (k *Kit) BuildExact(builder pool.Handle, product *UnitInfo, x, z int32, queued bool) {
	var one [1]pool.Handle
	one[0] = builder
	k.push(Command{Kind: CmdBuild, Product: product, X: x, Z: z, Spot: -1, Keep: true, Exact: true, Queued: queued}, one[:])
}

func (e *executor) exactSite(info *UnitInfo, x, z int32, w *units.World, tick uint32) (int32, int32, bool) {
	p := e.placement(info)
	if !p.ok {
		return 0, 0, false
	}
	cx, cz := x/16-p.footX/2, z/16-p.footZ/2
	fx, fz := p.footX, p.footZ
	if info.Def != nil && info.Def.IsFeature && info.FinishedFeature != nil {
		fx = max(fx, info.FinishedFeature.FootprintX)
		fz = max(fz, info.FinishedFeature.FootprintZ)
	}
	e.prepareLanes()
	e.prepareTowerSpace()
	if !p.mobile && e.blocksLane(cx, cz, fx, fz) || !info.Role.Has(RoleExtractor) && e.overlapsSpot(cx, cz, fx, fz) || e.blocksAllyTower(cx, cz, fx, fz) || !e.validAt(p, cx, cz) || e.guardRefuses(info, cx, cz, fx, fz, w, tick) {
		return 0, 0, false
	}
	e.guardPlaced(cx, cz, fx, fz)
	return cx, cz, true
}

// Every building the Modern AI places keeps space around allied towers,
// including a short outward firing lane. This is a player decision in every
// rule set, not a placement privilege (docs/DESIGN_SURVIVAL.md §16.8).
const (
	alliedTowerMargin = 128
	alliedFireLane    = 256
	alliedFireWidth   = 64
)

type towerSpace struct {
	box        rowBld // world units, half-open
	x, z       int32
	dx, dz     int64 // outward direction, length 1000
	end, width int64 // projected lane bounds, world units ×1000
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// prepareTowerSpace uses only allied structures in the fair observation,
// including nanoframes. Run before both the row and extractor site searches.
func (e *executor) prepareTowerSpace() {
	e.allyTowers = e.allyTowers[:0]
	if e.obs == nil || e.mapInfo == nil {
		return
	}
	hx, hz := e.mapInfo.HomeX, e.mapInfo.HomeZ
	if e.m != nil && e.m.Survival != nil {
		hx, hz = e.m.Survival.CentreX, e.m.Survival.CentreZ
	}
	for i := range e.obs.Allies {
		u := &e.obs.Allies[i]
		info := u.Info
		if info == nil || !info.Role.Has(RoleDefense) || info.Role.Has(RoleMobile) {
			continue
		}
		fx, fz := max(info.FootX, 1)*8, max(info.FootZ, 1)*8
		v := towerSpace{x: u.X, z: u.Z, box: rowBld{x0: u.X - fx - alliedTowerMargin, z0: u.Z - fz - alliedTowerMargin, x1: u.X + fx + alliedTowerMargin, z1: u.Z + fz + alliedTowerMargin}}
		vx, vz := int64(u.X)-int64(hx), int64(u.Z)-int64(hz)
		if dist := ISqrt64(vx*vx + vz*vz); dist > 0 {
			v.dx, v.dz = vx*1000/dist, vz*1000/dist
			v.end = abs64(v.dx)*int64(fx) + abs64(v.dz)*int64(fz) + alliedFireLane*1000
			v.width = abs64(v.dz)*int64(fx) + abs64(v.dx)*int64(fz) + alliedFireWidth*1000
		}
		e.allyTowers = append(e.allyTowers, v)
	}
}

// blocksAllyTower tests the entire proposed footprint. Projection bounds
// conservatively reserve a diagonal lane too; equality leaves the edge open.
func (e *executor) blocksAllyTower(cx, cz, fx, fz int32) bool {
	c := rowBld{x0: cx * 16, z0: cz * 16, x1: (cx + fx) * 16, z1: (cz + fz) * 16}
	for i := range e.allyTowers {
		v := &e.allyTowers[i]
		if intersects(&c, &v.box) {
			return true
		}
		if v.dx == 0 && v.dz == 0 {
			continue
		}
		x, z := int64(c.x0+c.x1)/2-int64(v.x), int64(c.z0+c.z1)/2-int64(v.z)
		rx, rz := int64(fx)*8, int64(fz)*8
		along, across := x*v.dx+z*v.dz, -x*v.dz+z*v.dx
		ar := abs64(v.dx)*rx + abs64(v.dz)*rz
		cr := abs64(v.dz)*rx + abs64(v.dx)*rz
		if along+ar > 0 && along-ar < v.end && across+cr > -v.width && across-cr < v.width {
			return true
		}
	}
	return false
}
