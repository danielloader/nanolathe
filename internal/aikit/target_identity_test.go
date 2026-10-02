package aikit

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type memoryAttackBrain struct{ countBrain }

func (b *memoryAttackBrain) Think(k *Kit, o *Obs) {
	b.thinks++
	if b.thinks >= 2 && len(o.Memory) > 0 && len(o.Own) > 0 {
		k.Attack([]pool.Handle{o.Own[0].H}, o.Memory[0].H, false)
	}
}

// A remembered target keeps the observed instance across unseen slot reuse,
// even when radar detects the replacement. Reacquiring it creates exactly
// one new observed generation and makes that instance targetable (§§2–3 of
// docs/MODERN_AI_RESEARCH.md). Sync and async apply the same outcomes.
func TestRememberedTargetKeepsObservedInstance(t *testing.T) {
	for _, async := range []bool{false, true} {
		for _, radar := range []bool{false, true} {
			for _, reuse := range []string{"none", "before-issue", "during-reaction"} {
				t.Run(fmt.Sprintf("async=%t/radar=%t/%s", async, radar, reuse), func(t *testing.T) {
					b := &memoryAttackBrain{}
					f := newGenFixture(t, b)
					defer f.h.Close()
					f.h.persona.Async = async
					f.def.CanAttack = true
					actor := f.w.Unit(f.own)
					actor.Flags |= units.ArmedStatus
					if radar {
						f.def.RadarDistance = 1000
						actor.Activated = true
					}
					f.h.m.OrderBinding = &orders.QueueBinding{World: &orders.WorldQueryAdapter{SeaLevel: func() uint8 { return 0 }}}
					tick := uint32(1)
					think := func(n int) {
						t.Helper()
						for end := tick + 2*PersonaMax.ThinkEvery; b.thinks < n && tick < end; tick++ {
							f.h.Step(tick, f.w, f.econ)
							f.h.Join()
						}
						if b.thinks != n {
							t.Fatalf("got %d thinks, want %d", b.thinks, n)
						}
					}
					apply := func() {
						for due := f.h.b.due; tick <= due; tick++ {
							f.h.Step(tick, f.w, f.econ)
						}
					}
					think(1)
					old, gen := f.w.Unit(f.enemy), f.h.obs.Memory[0].Gen
					f.h.m.UnitVisible = func(uint8, *units.Unit) bool { return false }
					if reuse == "before-issue" {
						f.replace(t, f.enemy, 1, tick)
						f.replace(t, f.enemy, 1, tick) // unseen churn must not advance Gen
					}
					think(2)
					if len(f.h.obs.Memory) != 1 || f.h.obs.Memory[0].Gen != gen || f.h.ob.gen[f.enemy] != gen || f.h.b.cmds[0].target != old {
						t.Fatal("an unseen unit replaced the remembered target's identity")
					}
					if radar {
						if len(f.h.obs.Enemy) != 1 || f.h.obs.Enemy[0].H != 0 || f.h.obs.Enemy[0].Gen != 0 || f.h.obs.Enemy[0].Info != nil {
							t.Fatalf("radar disclosed identity: %+v", f.h.obs.Enemy)
						}
					} else if len(f.h.obs.Enemy) != 0 {
						t.Fatalf("unseen contacts: %+v", f.h.obs.Enemy)
					}
					if reuse == "during-reaction" {
						f.replace(t, f.enemy, 1, tick)
					}
					apply()
					want := ApplyStats{Applied: 1}
					if reuse != "none" {
						want = ApplyStats{Stale: 1}
						want.Reasons[FailTarget] = 1
					}
					if got := f.h.Stats(); got != want {
						t.Fatalf("remembered attack: %+v, want %+v", got, want)
					}
					f.h.m.UnitVisible = func(uint8, *units.Unit) bool { return true }
					think(3)
					if reuse != "none" {
						gen++
					}
					if f.h.obs.Enemy[0].Gen != gen || f.h.obs.Memory[0].Gen != gen || f.h.b.cmds[0].target != f.w.Unit(f.enemy) {
						t.Fatal("reacquired target did not take over the observed identity")
					}
					apply()
					want.Applied++
					if got := f.h.Stats(); got != want {
						t.Fatalf("reacquired attack: %+v, want %+v", got, want)
					}
				})
			}
		}
	}
}

// Replacement must not reclaim a newcomer, and factory cleanup must not
// open a different factory's exit after the reaction window (§2).
func TestConstructionCommandsRejectRecycledTargets(t *testing.T) {
	for _, kind := range []CmdKind{CmdReplace, CmdUnblock} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			builderDef := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "builder"}, UnitName: "builder", MaxDamage: 100, BMCode: 1, CanMove: true, Builder: true, CanReclamate: true}
			buildingDef := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "building"}, UnitName: "building", MaxDamage: 100}
			cat := &content.Catalog{Units: map[string]*content.UnitDef{"builder": builderDef, "building": buildingDef}}
			w := fixtureWorld(cat)
			builder, err := w.Create(builderDef, 0, 100<<16, 0, 100<<16)
			if err != nil {
				t.Fatal(err)
			}
			target, err := w.Create(buildingDef, 0, 160<<16, 0, 160<<16)
			if err != nil {
				t.Fatal(err)
			}
			old := w.Unit(target)
			queued := 0
			m := &ai.Manager{Player: 0, Catalog: cat, QueueBuildTyped: func(ai.BuildRequest) error { queued++; return nil }}
			e := &executor{m: m, table: BuildTable(cat, nil),
				mapInfo: &MapInfo{CellW: 64, CellH: 64, Spots: []MetalSpot{{X: 160, Z: 160}}, cellLo: make([]uint8, 64*64), cellHi: make([]uint8, 64*64)},
				places:  []*placeDef{{ok: true, footX: 1, footZ: 1}}}
			c := Command{Kind: kind, Target: target, target: old, Product: &UnitInfo{Index: 0, Key: "building"}, Spot: 0, count: 1}
			b := batch{actors: []pool.Handle{builder}, inst: []*units.Unit{w.Unit(builder)}}
			if kind == CmdReplace {
				if !e.exec(&c, &b, 1, w) || queued != 1 {
					t.Fatalf("valid replacement failed: %+v, queued %d", e.stats, queued)
				}
				orders.QueueOfUnit(w.Unit(builder)).PurgeUnprotected()
				e.stats, queued = ApplyStats{}, 0
			}
			w.Destroy(target, units.DeathKilled)
			w.FinalizeDeath(target, 2)
			if _, err := w.CreateWithForcedSlot(buildingDef, 0, 160<<16, 0, 160<<16, target); err != nil {
				t.Fatal(err)
			}
			if e.exec(&c, &b, 3, w) || queued != 0 || e.stats.Stale != 1 || e.stats.Reasons[FailTarget] != 1 {
				t.Fatalf("recycled target accepted: %+v, queued %d", e.stats, queued)
			}
			if q := orders.QueueOfUnit(w.Unit(builder)); q != nil && q.PrimaryLen() != 0 {
				t.Fatal("stale command changed the builder's orders")
			}
		})
	}
}
