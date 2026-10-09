package orders

import (
	"bytes"
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func patrolAuditDefinitions(t *testing.T) (*content.UnitDef, *content.FeatureDef) {
	t.Helper()
	var archive bytes.Buffer
	if err := vfs.WriteArchive(&archive, []vfs.ArchiveFile{
		{Path: "units/patrol.fbi", Data: []byte(`[UNITINFO]{UnitName=patrol; Version=3.1; MaxDamage=100; EnergyStorage=3; EnergyMake=40.6; SightDistance=97;}`)},
		{Path: "features/audit.tdf", Data: []byte(`[audit]{Energy=1; Metal=0; Reclaimable=1; Autoreclaimable=1; FootprintX=1; FootprintZ=1;}`)},
	}, vfs.ArchiveWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	t.Cleanup(func() { fs.Close() })
	if _, err := fs.MountArchiveReader("patrol.hpi", bytes.NewReader(archive.Bytes()), int64(archive.Len()), 0, vfs.ArchiveOptions{}); err != nil {
		t.Fatal(err)
	}
	defs, err := content.CompileUnits(fs)
	if err != nil {
		t.Fatal(err)
	}
	features, err := content.CompileFeatures(fs)
	if err != nil {
		t.Fatal(err)
	}
	def, feature := defs[content.CanonicalKey("patrol")], features[content.CanonicalKey("audit")]
	if def == nil || feature == nil || def.SightDistance != 97 || feature.Energy != 1 {
		t.Fatal("authored patrol fixture did not compile")
	}
	return def, feature
}

// A completed authored producer, a zero opening stock and the ordinary entry
// bonus reach the fractional threshold without injecting malformed state
// [04 R-ORD-01 §4][05 R-ECO-01 §2, §4, §5].
func TestRepairPatrolStoredStockBelowWorkingThreshold(t *testing.T) {
	def, _ := patrolAuditDefinitions(t)
	w := newOrdersFixtureWorld(10, nil)
	if _, err := w.Create(def, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	var econ economy.Service
	econ.Players[0].InstallStorageBonus(0, 0)
	econ.Settle(0, 30, w)
	p := &econ.Players[0]
	if p.Stock[economy.Energy] != float32(40.6) || p.Capacity[economy.Energy] != 203 {
		t.Fatalf("authored settlement: stock=%v capacity=%v", p.Stock, p.Capacity)
	}
	for _, air := range []bool{false, true} {
		actor, _, sim, q := repairPatrolRefusalFixture(t, air)
		resources := ResourceView{Stock: p.Stock, Capacity: p.Capacity}
		q.binding.SetResources(func(uint8) (ResourceView, bool) { return resources, true })
		// The next stored stock admits two candidates and one bounded draw.
		// Stance three then refuses the issue without adding a work record.
		actor.Flags = actor.Flags&^(stanceFieldMask<<stanceMoveShift) | 3<<stanceMoveShift
		scans := 0
		scan := q.binding.World.ForEachUnitInRadiusHook()
		q.binding.World.SetForEachUnitInRadius(func(x, z, radius numeric.Fixed, visit func(pool.Handle, *units.Unit) bool) {
			scans++
			scan(x, z, radius, visit)
		})
		q.binding.World.SetLookupFeature(func(int32, int32) (FeatureView, bool) { return FeatureView{}, false })
		handler := repairPatrolHandler
		if air {
			handler = vtolRepairPatrolHandler
		}
		n := &Node{Owner: actor.Handle, Phase: 1}
		if code := handler(actor, n, 0, 100); code != 2 || scans != 0 || sim.Draws() != 0 || q.LenPrimary() != 0 {
			t.Fatalf("air=%v: low stock code/scans/draws/queue=%d/%d/%d/%d", air, code, scans, sim.Draws(), q.LenPrimary())
		}
		resources.Stock[1] = math.Nextafter32(resources.Stock[1], float32(math.Inf(1)))
		stock := resources.Stock
		if code := handler(actor, n, 0, 101); code != 3 || scans != 1 || sim.Draws() != 1 || q.LenPrimary() != 0 {
			t.Fatalf("air=%v: next stored value code/scans/draws/queue=%d/%d/%d/%d", air, code, scans, sim.Draws(), q.LenPrimary())
		}
		if resources.Stock != stock {
			t.Fatal("repair selection spent resources")
		}
	}
}

func TestRepairPatrolFitDoesNotRoundAwayFeatureValue(t *testing.T) {
	_, feature := patrolAuditDefinitions(t)
	for _, stock := range []float32{16777216, 16777215} {
		actor, _, sim, q := repairPatrolRefusalFixture(t, false)
		var p economy.Player
		p.InstallStorageBonus(0, 16777216)
		resources := ResourceView{Stock: [2]float32{0, stock}, Capacity: p.StorageBonus}
		q.binding.SetResources(func(uint8) (ResourceView, bool) { return resources, true })
		q.binding.World.SetForEachUnitInRadius(func(_, _, _ numeric.Fixed, _ func(pool.Handle, *units.Unit) bool) {})
		q.binding.World.SetLookupFeature(func(int32, int32) (FeatureView, bool) {
			return FeatureView{Energy: feature.Energy, Reclaimable: feature.Reclaimable, Autoreclaimable: feature.Autoreclaimable}, true
		})
		n := &Node{Owner: actor.Handle, Phase: 1}
		code := repairPatrolHandler(actor, n, 0, 100)
		wantCode, wantQueue := Code(2), 0
		if stock == 16777215 {
			wantCode, wantQueue = 3, 1 // exact equality fits
		}
		if code != wantCode || q.LenPrimary() != wantQueue || sim.Draws() != 3 {
			t.Fatalf("stock=%v: code/queue/draws=%d/%d/%d", stock, code, q.LenPrimary(), sim.Draws())
		}
		if resources.Stock[1] != stock {
			t.Fatal("feature selection spent resources")
		}
	}
}

// NaN rows exercise the consumer branch only; no authored NaN resource producer
// is asserted. Equality and ordered strictness also lock the finite gates.
func TestPatrolResourceComparisonConsumerBoundaries(t *testing.T) {
	nan := float32(math.NaN())
	for _, tc := range []struct {
		stock, capacity float32
		want            bool
	}{{20, 100, true}, {19, 100, false}, {nan, 100, true}, {20, nan, true}} {
		if got := patrolResourceAtLeastTwenty(tc.stock, tc.capacity); got != tc.want {
			t.Fatalf("threshold(%v,%v)=%v", tc.stock, tc.capacity, got)
		}
	}
	if !resourceFits(nan, 100, 1) || !resourceFits(20, nan, 1) || !resourceFits(99, 100, 1) || resourceFits(100, 100, 1) {
		t.Fatal("fit comparison lost inclusive or unordered admission")
	}
}

func TestRepairFeatureLatticeKeepsRawHalvesAndBoundsWork(t *testing.T) {
	def, _ := patrolAuditDefinitions(t)
	for _, tc := range []struct {
		name     string
		centre   numeric.Fixed
		diameter int32
		first    numeric.Fixed
		count    int
	}{
		{"odd diameter", 64 << 16, def.SightDistance, 15<<16 | 32768, 9},
		{"fractional centre", 64<<16 | 16384, 96, 16<<16 | 16384, 9},
		// This endpoint never becomes false after retail's increment wraps.
		// The host retains its diameter-derived bound, not retail parity.
		{"wrapped endpoint host bound", numeric.Fixed(math.MaxInt32 - (48 << 16)), 96, numeric.Fixed(math.MaxInt32 - (96 << 16)), 9},
		{"negative diameter host rejection", 64 << 16, -1, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actor := &units.Unit{Handle: 1, Alive: true, X: tc.centre, Z: tc.centre}
			calls := 0
			BindQueue(actor, &Queue{binding: &QueueBinding{World: NewWorldQueryAdapter(WorldQueryAdapterConfig{
				LookupFeature: func(int32, int32) (FeatureView, bool) {
					calls++
					if calls > tc.count {
						t.Fatal("feature scan exceeded the host work bound")
					}
					return FeatureView{Energy: 1, Reclaimable: true, Autoreclaimable: true}, true
				},
			})}})
			energy, _ := scanFeatureLists(actor, tc.diameter)
			if len(energy) != tc.count {
				t.Fatalf("samples=%d want %d", len(energy), tc.count)
			}
			for i, f := range energy {
				if f.X != tc.first+numeric.Fixed((i%3)*48<<16) || f.Z != tc.first+numeric.Fixed((i/3)*48<<16) {
					t.Fatalf("sample%d=(%v,%v) has wrong fixed-point traversal", i, f.X, f.Z)
				}
			}
		})
	}
}

func TestRepairFeatureEqualValuesKeepFirstTournamentPick(t *testing.T) {
	actor := &units.Unit{Handle: 1, Alive: true, X: 64 << 16, Z: 64 << 16}
	sim := rng.SimulationFromState(1)
	BindQueue(actor, &Queue{binding: &QueueBinding{SimRNG: &sim, World: NewWorldQueryAdapter(WorldQueryAdapterConfig{
		LookupFeature: func(x, z int32) (FeatureView, bool) {
			id := uint16(0)
			if x == 4 && z == 1 {
				id = 1
			}
			if x == 1 && z == 4 {
				id = 2
			}
			return FeatureView{ID: id, Energy: 1, Reclaimable: true, Autoreclaimable: true}, id != 0
		},
	})}})
	list, _ := scanFeatureLists(actor, 96)
	got, ok := pickFeatureTournament(actor, list, false)
	if len(list) != 2 || list[0].ID != 1 || list[1].ID != 2 || !ok || got.ID != 2 || sim.Draws() != 3 {
		t.Fatalf("Z-first tie tournament: list=%v choice=%v draws=%d", list, got.ID, sim.Draws())
	}
}

func TestHealthyRepairPatrolFeatureBranchDiffersForAir(t *testing.T) {
	for _, air := range []bool{false, true} {
		actor, _, sim, q := repairPatrolRefusalFixture(t, air)
		resources := ResourceView{Stock: [2]float32{100, 100}, Capacity: [2]float32{200, 200}}
		q.binding.SetResources(func(uint8) (ResourceView, bool) { return resources, true })
		q.binding.World.SetForEachUnitInRadius(func(_, _, _ numeric.Fixed, _ func(pool.Handle, *units.Unit) bool) {})
		q.binding.World.SetLookupFeature(func(int32, int32) (FeatureView, bool) {
			return FeatureView{Energy: 1, Reclaimable: true, Autoreclaimable: true}, true
		})
		n := &Node{Owner: actor.Handle, Phase: 1}
		handler, wantCode, wantDraws, wantQueue := repairPatrolHandler, Code(2), uint64(0), 0
		if air {
			handler, wantCode, wantDraws, wantQueue = vtolRepairPatrolHandler, 3, 3, 1
		}
		if code := handler(actor, n, 0, 100); code != wantCode || sim.Draws() != wantDraws || q.LenPrimary() != wantQueue {
			t.Fatalf("air=%v: code/draws/queue=%d/%d/%d", air, code, sim.Draws(), q.LenPrimary())
		}
		if air && (q.Head().ID != Lookup("VTOL_Reclaim") || n.DynamicGate != 0) {
			t.Fatal("air reclaim did not own the next visit")
		}
		if resources.Stock != [2]float32{100, 100} {
			t.Fatal("selection paid reclaim resources")
		}
	}
}

func TestModernGuardRetainsSinglePrecisionThreshold(t *testing.T) {
	for _, modern := range []bool{false, true} {
		f := newGuardFixture(t, 1, 1)
		f.ward.X, f.ward.Z = f.guard.X, f.guard.Z
		f.guard.Def.Builder, f.guard.Def.CanReclamate = true, true
		f.guard.Def.BMCode, f.guard.Def.SightDistance = 1, 128
		b := QueueForUnit(f.guard).Binding()
		b.Rules = modeRules(modern)
		patient := &units.Unit{Handle: 3, Alive: true, Def: &content.UnitDef{MaxDamage: 100}, Health: 50, X: f.guard.X, Z: f.guard.Z}
		patient.Move.Mode, patient.Move.ModeMirror = 1, 1
		b.World = NewWorldQueryAdapter(WorldQueryAdapterConfig{ForEachUnit: func(visit func(pool.Handle, *units.Unit) bool) { visit(patient.Handle, patient) }})
		resources := ResourceView{Stock: [2]float32{0, 40.6}, Capacity: [2]float32{200, 203}}
		b.SetResources(func(uint8) (ResourceView, bool) { return resources, true })
		before := *f.sim
		if got := b.Rules.GuardWorksNearby(f.guard, guardNode(f), 100); got != modern {
			t.Fatalf("modern=%v assistance=%v", modern, got)
		}
		if *f.sim != before || resources.Stock[1] != float32(40.6) {
			t.Fatal("guard selection changed RNG/resources")
		}
	}
}
