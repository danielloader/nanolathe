//go:build retail

package orders

import (
	"maps"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Installed authored capabilities and pools enter the same deterministic
// selection contract; this fixture copies no asset bytes into the repository.
func TestModernBuilderWorkWithInstalledDefinitions(t *testing.T) {
	cat, _ := retailcat.Shared(t)
	var energyFeature string
	for _, key := range slices.Sorted(maps.Keys(cat.Features)) {
		f := cat.Features[key]
		if f.Reclaimable && f.Autoreclaimable && f.Energy > 0 && f.Metal == 0 {
			energyFeature = key
			break
		}
	}
	if energyFeature == "" {
		t.Fatal("install supplies no automatic energy feature")
	}
	unitDef := func(key string) *content.UnitDef {
		d, ok := cat.Unit(key)
		if !ok {
			t.Fatalf("missing installed unit %s", key)
		}
		copy := *d
		return &copy
	}
	for _, key := range []string{"armcv", "armca"} {
		t.Run(key, func(t *testing.T) {
			def := unitDef(key)
			f := newModernWorkFixture(def.CanFly)
			f.u.Def, f.u.Health, f.u.MaxHealth = def, def.MaxDamage, def.MaxDamage
			factory := modernPatient(3, 64, 0, false)
			factory.Def = unitDef("armvp")
			factory.Health, factory.MaxHealth = factory.Def.MaxDamage, factory.Def.MaxDamage
			product := modernPatient(2, 64, 0, true)
			product.Def = unitDef("armflash")
			product.Health, product.MaxHealth = 1, product.Def.MaxDamage
			f.units = []*units.Unit{factory, product}
			QueueForUnit(factory).Push(rowBuildingBuild, Node{Owner: factory.Handle, Target: product.Handle})
			QueueForUnit(factory).Head().BindTarget(product.Handle)
			before := f.sim
			f.visit(100)
			if f.q.Head().Target != product.Handle || f.q.Head().workAssignment != f.patrol || f.sim != before || product.Remaining != 1 {
				t.Fatal("installed constructor did not assist current factory frame without selection effects")
			}
			f.q.primary = f.q.primary[1:]
			product.Remaining, product.Health = 0, product.Def.MaxDamage/2
			f.visit(101)
			want := rowRepairUnit
			if def.CanFly {
				want = rowVTOLRepairUnit
			}
			if f.q.Head().ID != want || f.q.Head().Target != product.Handle {
				t.Fatal("installed constructor did not select ordinary repair")
			}
			// An installed energy-only feature and metal-bearing wreck retain their
			// authored automatic-reclaim flags and yielded resource identities.
			for _, featureKey := range []string{energyFeature, content.CanonicalKey(product.Def.Corpse)} {
				feature, ok := cat.Features[featureKey]
				if !ok {
					t.Fatalf("missing installed feature %s", featureKey)
				}
				f.q.primary = []*Node{f.patrol, f.next, f.successor}
				f.units = nil
				f.features = []FeatureView{{ID: 1, CX: 2, CZ: 0, X: numeric.Fixed(32 << 16), DefinitionKey: featureKey, Metal: feature.Metal, Energy: feature.Energy, Reclaimable: feature.Reclaimable, Autoreclaimable: feature.Autoreclaimable}}
				f.resources.Stock = [2]float32{99, 99}
				f.visit(102)
				if !feature.Reclaimable || !feature.Autoreclaimable || feature.Metal <= 0 && feature.Energy <= 0 {
					t.Fatal("fixture requires an authored automatic resource feature")
				}
				if f.q.Head() == f.patrol || f.q.Head().Target != 0 || f.q.Head().workAssignment != f.patrol || f.sim != before {
					t.Fatal("installed resource feature was not automatically reclaimed without selection draw")
				}
			}
			// The guard's ward circle is applied without changing direct assistance.
			ward := &units.Unit{Handle: 4, Alive: true, Def: unitDef("armvp"), X: numeric.Fixed(400 << 16)}
			ward.Health, ward.MaxHealth = ward.Def.MaxDamage, ward.Def.MaxDamage
			f.units = []*units.Unit{ward, product}
			product.X, product.Z = ward.X+numeric.Fixed(129<<16), 0
			f.u.X = product.X
			guard := &Node{ID: rowFollowGround, Owner: f.u.Handle, Target: ward.Handle}
			if def.CanFly {
				guard.ID = rowVTOLFollow
			}
			f.q.primary = []*Node{guard}
			f.features = nil
			if f.q.binding.rules().GuardWorksNearby(f.u, guard, 200) {
				t.Fatal("installed guard borrowed work beyond ward circle")
			}
		})
	}
}
