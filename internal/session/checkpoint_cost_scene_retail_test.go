//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// busy-modern-classic-v1 is an authored cost fixture, not a skirmish policy:
// 67 extra records per existing seat, six factory queues and 88 ground moves.
// Every record, queue and binding comes from the ordinary admitted services.
func checkpointCostBusyBattle(t *testing.T) *Session {
	t.Helper()
	s := checkpointLifecycleBattle(t)
	var commanders [2]*units.Unit
	for _, u := range s.Units.IterSliced() {
		if u != nil && u.Alive && u.Def != nil && u.Def.Commander && int(u.Owner) < len(commanders) {
			commanders[u.Owner] = u
		}
	}
	for owner, commander := range commanders {
		if commander == nil || commanders[1-owner] == nil {
			t.Fatal("busy cost scene requires both ordinary commanders")
		}
		t.Logf("busy placement owner=%d commander=(%d,%d) radius=1536 pitch=144", owner, commander.X>>16, commander.Z>>16)
		for _, row := range []struct {
			names   [2]string
			count   int
			product [2]string
		}{
			{[2]string{"armlab", "corlab"}, 3, [2]string{"armpw", "corak"}},
			{[2]string{"armsolar", "corsolar"}, 12, [2]string{}},
			{[2]string{"armmakr", "cormakr"}, 4, [2]string{}},
			{[2]string{"armmstor", "cormstor"}, 2, [2]string{}},
			{[2]string{"armestor", "corestor"}, 2, [2]string{}},
			{[2]string{"armpw", "corak"}, 24, [2]string{}},
			{[2]string{"armflash", "corraid"}, 20, [2]string{}},
		} {
			def, ok := s.Catalog.Unit(row.names[owner])
			if !ok || def == nil {
				t.Fatalf("busy cost scene lacks %s", row.names[owner])
			}
			for range row.count {
				x, z := checkpointCostSite(t, s, def, commander)
				h, err := s.Units.Create(def, uint8(owner), x, s.World.HeightAt(x, z), z)
				if err != nil {
					t.Fatalf("busy cost allocation %s owner %d: %v", def.UnitName, owner, err)
				}
				u := s.Units.Unit(h)
				if u == nil {
					t.Fatal("busy cost allocation returned no unit")
				}
				s.Movement.EnsureUnit(u)
				if row.product[owner] != "" || def.CanMove {
					s.BindStagedOrderQueue(u)
				}
				if row.product[owner] != "" {
					if err := construction.QueueFactoryBuild(u, row.product[owner], 20, s.Catalog); err != nil {
						t.Fatal(err)
					}
				} else if def.CanMove {
					x, z := checkpointCostSite(t, s, def, commanders[1-owner])
					q := orders.QueueOfUnit(u)
					id := orders.Lookup("Move_Ground")
					if q == nil || id == 0 {
						t.Fatal("busy cost unit lacks ordinary move queue")
					}
					q.Push(id, orders.NewNodeForOrder(id, 0, x, s.World.HeightAt(x, z), z, s.Clock.GlobalTick, u.Handle, false))
					if q.Head() == nil {
						t.Fatal("busy cost move insertion refused")
					}
				}
			}
		}
	}
	return s
}

// Pick the first clear land footprint on a fixed Z/X lattice around the
// existing commander. This setup-only search draws no RNG and edits no terrain.
func checkpointCostSite(t *testing.T, s *Session, def *content.UnitDef, centre *units.Unit) (numeric.Fixed, numeric.Fixed) {
	t.Helper()
	profile := movement.NewScratchProfile(def)
	if class := s.Catalog.Movement[content.CanonicalKey(def.MovementClass)]; class != nil {
		profile = movement.NewProfile(class)
	}
	fx, fz := world.FootprintForUnit(s.Catalog, def)
	extent, err := world.NewFootprintExtent(fx, fz)
	if err != nil {
		t.Fatal(err)
	}
	cx, cz := int32(centre.X>>16), int32(centre.Z>>16)
	for z := max(int32(128), cz-1536); z <= min(s.World.CellH*16-128, cz+1536); z += 144 {
		for x := max(int32(128), cx-1536); x <= min(s.World.CellW*16-128, cx+1536); x += 144 {
			px, pz := numeric.FixedFromInt(int64(x)), numeric.FixedFromInt(int64(z))
			anchor, err := world.SnapFootprintAnchor(px, pz, extent)
			if err != nil {
				t.Fatal(err)
			}
			cell := movement.Cell{X: anchor.CellX(), Z: anchor.CellZ()}
			if s.World.HeightAt(px, pz) > numeric.FixedFromInt(int64(s.World.SeaLevel)) &&
				s.Movement.Grid.RectOnMap(cell, int16(fx), int16(fz)) &&
				!s.Movement.Grid.FootprintOccupied(cell, int16(fx), int16(fz), -1) &&
				profile.IsPassableFootprint(s.World, cell.X, cell.Z) {
				return px, pz
			}
		}
	}
	t.Fatalf("busy cost scene has no clear %s footprint around owner %d", def.UnitName, centre.Owner)
	return 0, 0
}

type checkpointCostCensus struct {
	Live, Moving, Routes, Searches, Nanoframes, Orders, Builds [2]int
	Projectiles, Effects, Fragments, Strips                    int
}

func checkpointCostCount(s *Session) checkpointCostCensus {
	var out checkpointCostCensus
	for _, u := range s.Units.IterSliced() {
		if u == nil || !u.Alive || u.Dying || int(u.Owner) >= len(out.Live) {
			continue
		}
		owner := u.Owner
		out.Live[owner]++
		if u.Move.Speed != 0 || u.Move.VelX != 0 || u.Move.VelZ != 0 {
			out.Moving[owner]++
		}
		if u.Remaining > 0 {
			out.Nanoframes[owner]++
		}
		if int(u.Handle) < len(s.Movement.Routes) && s.Movement.Routes[u.Handle] != nil && s.Movement.Routes[u.Handle].Active {
			out.Routes[owner]++
		}
		if s.Path.HasRequest(u.Handle) {
			out.Searches[owner]++
		}
		if q := orders.QueueOfUnit(u); q != nil {
			for _, rows := range [2][]*orders.Node{q.Primary(), q.Secondary()} {
				for _, n := range rows {
					if n != nil {
						out.Orders[owner]++
						if n.BuildDefKey != "" {
							out.Builds[owner]++
						}
					}
				}
			}
		}
	}
	published := s.Snapshot.Current()
	out.Projectiles, out.Effects, out.Fragments, out.Strips = len(published.Projectiles), len(published.Effects), len(published.Fragments), len(published.Strips)
	return out
}
