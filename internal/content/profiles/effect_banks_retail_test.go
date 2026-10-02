//go:build retail

package profiles_test

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/effects"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
)

// The stock holds the simulation times the fixed effect pool with are the
// asset census of [06 R-WFX-01 §1]: every frame of each listed entry holds
// for this many advances. Weapon explosion art is admitted one-shot: effect
// holders clear the entry's loop byte, so the pool never wraps it.
func TestStockEffectHoldsMatchTheCensus(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	art := content.CompileSimArt(fs, catalog)
	if diagnostics := art.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("stock effect banks did not all compile: %v", diagnostics)
	}
	for _, census := range []struct {
		bank, entry string
		hold        int32
	}{
		{"fx", "Explosion", 2}, {"fx", "Explode2", 2}, {"fx", "Explode3", 2}, {"fx", "Nuke1", 2},
		{"fx", "Explode4", 3}, {"fx", "Explode5", 3}, {"fx", "H2oBoom2", 3}, {"fx", "H2o", 3},
		{"fx", "lavasplash", 3}, {"fx", "lavasplashlg", 3},
		{"fx", "h2oboom1", 2}, {"fx", "lavasplashsm", 2},
		{"commboom", "CommBoom", 3}, {"empboom", "EMPboom", 2}, {"tronboom", "Tronboom", 2},
	} {
		holds, ok := art.EffectEntryHolds(census.bank, census.entry)
		if !ok || len(holds) == 0 {
			t.Fatalf("%s/%s has no compiled holds", census.bank, census.entry)
		}
		for i, hold := range holds {
			if hold != census.hold {
				t.Fatalf("%s/%s frame %d holds %d, want the census's %d", census.bank, census.entry, i, hold, census.hold)
			}
		}
	}
	// Every pair a stock weapon names resolves, and no frame holds for less
	// than one advance: a frame with hold h is shown for max(h, 1).
	for key, weapon := range catalog.Weapons {
		for _, pair := range [...][2]string{
			{weapon.ExplosionGaf, weapon.ExplosionArt},
			{weapon.WaterExplosionGaf, weapon.WaterExplosionArt},
			{weapon.LavaExplosionGaf, weapon.LavaExplosionArt},
		} {
			if pair[0] == "" || pair[1] == "" {
				continue
			}
			holds, ok := art.EffectEntryHolds(pair[0], pair[1])
			if !ok {
				t.Fatalf("weapon %s names %s/%s, which has no compiled holds", key, pair[0], pair[1])
			}
			if slices.Min(holds) < 1 {
				t.Fatalf("weapon %s art %s/%s holds %v; every hold is at least 1", key, pair[0], pair[1], holds)
			}
		}
	}

	// The pool admits the stock Explosion with those holds and without its
	// authored loop flag, whatever the event carried.
	pool := effects.NewFixedEffectPool(0)
	service := effects.NewEffectServiceWithPool(pool.Cap(), pool, art)
	if !service.Admit(1, frame.Event{Kind: frame.KindExplosion, AssetID: "fx", Graphic: "Explosion", LoopA: true, Tick: 1}) {
		t.Fatal("the pool refused the stock Explosion")
	}
	views := service.Snapshot()
	want, _ := art.EffectEntryHolds("fx", "Explosion")
	if len(views) != 1 || views[0].LoopA || !views[0].ActiveA || !slices.Equal(views[0].DurationsA, want) {
		t.Fatalf("admitted Explosion view = %+v, want a one-shot active player with holds %v", views, want)
	}
}

// The audited mods' large banks exceed the eager decoder's pixel budget but
// not the shared effect-bank policy, so their weapon explosions are timed in
// every host, as the client draws them. Each content set skips unless its
// roots are configured (NANOLATHE_MOD_ROOTS_<NAME>).
func TestModEffectBanksCompileUnderTheSharedPolicy(t *testing.T) {
	for _, set := range []struct {
		name, config string
		pairs        [][2]string
	}{
		{"zero", "ta-zero-alpha5-20241224", [][2]string{{"ModFX", "Death_G8"}, {"ModFX", "Weapon_Cannon1"}, {"ModFX", "Water_Splash1"}}},
		{"escalation", "escalation-10.2.0", [][2]string{
			{"esc_nuke_a_02", "esc_nuke_a_02"}, {"esc_nuke_x_01", "esc_nuke_x_01"}, {"esc_weap_x_01", "esc_weap_x_01"},
		}},
	} {
		t.Run(set.name, func(t *testing.T) {
			fs := mountWithMod(t, set.name)
			view := modContent(t, fs, set.config).Layout().Apply(fs)
			// Only the banks under test are named, so the check reads them and
			// the default bank rather than compiling the whole content set.
			weapons := make(map[string]*content.WeaponDef, len(set.pairs))
			for _, pair := range set.pairs {
				weapons[pair[0]+"/"+pair[1]] = &content.WeaponDef{ExplosionGaf: pair[0], ExplosionArt: pair[1]}
			}
			art := content.CompileSimArt(view, &content.Catalog{Weapons: weapons})
			if diagnostics := art.Diagnostics(); len(diagnostics) != 0 {
				t.Fatalf("%s effect banks were refused: %v", set.name, diagnostics)
			}
			for _, pair := range set.pairs {
				if holds, ok := art.EffectEntryHolds(pair[0], pair[1]); !ok || slices.Min(holds) < 1 {
					t.Fatalf("%s/%s holds = %v, ok=%v", pair[0], pair[1], holds, ok)
				}
			}
		})
	}
}
