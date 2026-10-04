//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// Stock aircraft and ships share the repair-admission contract across direct
// commands, patrol picks and copied guard work [04 R-ORD-01 §7][04 R-ORD-02 §3].
// The named map supplies legal wet sites; no authored model or capability is
// replaced. Handler visits isolate selection from unrelated flight timing.
func TestStockConstructorsShareWaterAdmissionRetail(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern} {
		t.Run(string(mode), func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				product string
				assist  bool
			}{{"underwater energy storage", "armuwes", false}, {"floating metal maker", "armfmkr", true}} {
				t.Run(tc.name, func(t *testing.T) {
					s := aiE2ESkirmishAtMode(t, "Gods of War", 7, SkirmishDefaultDifficulty, mode)
					s.Movement.BindWorld(s.Units)
					def, ok := s.Catalog.Unit(tc.product)
					if !ok || def == nil {
						t.Skipf("stock fixture product %s is absent", tc.product)
					}
					t.Logf("fixture: sea=%d, product=%s, min-depth=%d, max-depth=%d, model-height=%d", s.World.SeaLevel, def.UnitName, def.MinWaterDepth, def.MaxWaterDepth, def.ModelTopFixed>>16)
					x, y, z := issue56WaterSite(t, s, def, tc.assist)
					h, err := s.Units.CreateNanoframe(def, s.LocalOwner, x, y, z)
					if err != nil {
						t.Fatalf("create %s nanoframe: %v", tc.product, err)
					}
					frame := s.Units.Unit(h)
					s.bindOrderQueue(frame)
					s.Movement.EnsureUnit(frame)
					top := int32(int16(frame.Y.Floor())) + int32(int16(def.ModelTopFixed>>16))
					t.Logf("%s on Gods of War: site=(%d,%d,%d), sea=%d, model-height=%d, lifted-height=%d",
						tc.product, x.Floor(), y.Floor(), z.Floor(), s.World.SeaLevel, def.ModelTopFixed>>16, top)
					p := &s.Econ.Players[s.LocalOwner]
					p.Capacity, p.Stock = [2]float32{100000, 100000}, [2]float32{100000, 100000}
					makeConstructor := func(key string) *units.Unit {
						t.Helper()
						builderDef, found := s.Catalog.Unit(key)
						if !found || builderDef == nil {
							t.Skipf("stock fixture constructor %s is absent", key)
						}
						bh, createErr := s.Units.Create(builderDef, s.LocalOwner, x+numeric.FixedFromInt(64), s.World.SeaLevelWorld(), z)
						if createErr != nil {
							t.Fatalf("create %s: %v", key, createErr)
						}
						builder := s.Units.Unit(bh)
						s.bindOrderQueue(builder)
						s.Movement.EnsureUnit(builder)
						return builder
					}
					ship := makeConstructor("armcs")
					if got := orders.DescriptorFor(orders.Resolve(8, ship, frame, nil)).Name; got != "HelpBuild" {
						t.Fatalf("stock ship assistance = %q, want HelpBuild", got)
					}
					wq := orders.QueueForUnit(ship)
					buildID := orders.Lookup("MobileBuild")
					wq.Push(buildID, orders.NewNodeForOrder(buildID, 0, x, y, z, 0, ship.Handle, false))
					wq.Head().BindTarget(frame.Handle)
					for _, path := range []string{"direct", "patrol", "guard"} {
						t.Run(path, func(t *testing.T) {
							aircraft := makeConstructor("armca")
							q := orders.QueueForUnit(aircraft)
							stock, remaining, draws := p.Stock, frame.Remaining, s.SimRNG().Draws()
							switch path {
							case "direct":
								id := orders.Resolve(8, aircraft, frame, nil)
								if (id != 0) != tc.assist {
									t.Fatalf("direct aircraft assistance = %q, want admitted=%v", orders.DescriptorFor(id).Name, tc.assist)
								}
							case "patrol":
								id := orders.Resolve(9, aircraft, nil, nil)
								n := orders.NewNodeForOrder(id, 0, x, y, z, 0, aircraft.Handle, false)
								n.Phase = 1
								code := orders.DescriptorFor(id).Handler(aircraft, &n, 0, 1)
								want := orders.Code(2)
								if tc.assist {
									want = 3
								}
								if code != want {
									t.Fatalf("patrol visit returned %d, want %d", code, want)
								}
							case "guard":
								id := orders.Resolve(7, aircraft, ship, nil)
								n := orders.NewNodeForOrder(id, ship.Handle, x, y, z, 0, aircraft.Handle, false)
								n.Phase = 2
								orders.DescriptorFor(id).Handler(aircraft, &n, 0, 1)
							}
							if path != "direct" {
								head := q.Head()
								assisting := head != nil && orders.DescriptorFor(head.ID).Name == "VTOL_HelpBuild" && head.Target == frame.Handle
								if assisting != tc.assist {
									t.Fatalf("%s aircraft assistance=%v, want %v; head=%v", path, assisting, tc.assist, head)
								}
							}
							if p.Stock != stock || frame.Remaining != remaining || s.SimRNG().Draws() != draws {
								t.Fatal("selection changed resources, construction progress or RNG")
							}
						})
					}
				})
			}
		})
	}
}

func issue56WaterSite(t *testing.T, s *Session, def *content.UnitDef, surfaced bool) (numeric.Fixed, numeric.Fixed, numeric.Fixed) {
	t.Helper()
	extent, err := world.NewFootprintExtent(def.FootprintX, def.FootprintZ)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := world.PlacementRulesForUnit(s.Catalog, def)
	if err != nil {
		t.Fatal(err)
	}
	yard, err := world.ParseYardMap(def.YardMap, int(def.FootprintX), int(def.FootprintZ))
	if err != nil {
		t.Fatal(err)
	}
	for cz := int32(20); cz < s.World.CellH-20; cz++ {
		for cx := int32(20); cx < s.World.CellW-20; cx++ {
			rect, rectErr := world.NewFootprintRect(world.NewFootprintAnchor(cx, cz), extent)
			if rectErr != nil {
				t.Fatal(rectErr)
			}
			query := world.PlacementQuery{Rect: rect, Yard: yard, Rules: rules}
			if !s.World.PlacementLegal(query) {
				continue
			}
			result, placeErr := s.World.CheckPlacement(query)
			if placeErr != nil {
				t.Fatal(placeErr)
			}
			top := result.SiteHeight + int32(int16(def.ModelTopFixed>>16))
			if (top >= int32(s.World.SeaLevel)) != surfaced {
				continue
			}
			x := world.CellToWorld(cx) + numeric.FixedFromInt(int64(def.FootprintX)*8)
			z := world.CellToWorld(cz) + numeric.FixedFromInt(int64(def.FootprintZ)*8)
			return x, numeric.FixedFromInt(int64(result.SiteHeight)), z
		}
	}
	t.Fatalf("no legal wet %s site on Gods of War with surfaced=%v", def.UnitName, surfaced)
	return 0, 0, 0
}
