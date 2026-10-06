//go:build retail

package aikit

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The configured Modern brain requests ammunition through BUILDWEAPON in every
// gameplay mode. Production still takes authored time and pays the ordinary
// resource ledger [06 §11.1]; the policy only decides what to request
// (docs/DESIGN_SESSIONS_AI_SAVE.md "Modern AI computer player").
func TestModernAIStockpilesPaidRetailAmmunition(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, controller := range []ai.Controller{ai.ControllerModern, ai.ControllerClassic} {
			t.Run(string(mode)+"/"+controller.String(), func(t *testing.T) {
				s := stockpileBattle(t, mode, controller)
				silo := stockpileUnit(t, s, "ARMSILO", 1, 600, 600)
				protector := stockpileUnit(t, s, "ARMAMD", 1, 850, 600)
				// Leave just the carriers in the computer's economy so its resource
				// ledger measures ammunition alone, with no builder spending.
				s.Units.ForEachPlayerSliceLive(1, func(u *units.Unit) {
					if u.Def.Commander {
						s.Units.Destroy(u.Handle, units.DeathReclaimed)
					}
				})
				stepBattle(s, 2)
				carriers := []*units.Unit{silo, protector}
				want := []int32{1, 3}
				p := &s.Econ.Players[1]
				before := p.TotalConsumed
				seenWork := [2]bool{}
				deadline := 600
				for i, u := range carriers {
					slot := u.SlotAt(0)
					if slot == nil || slot.Weapon == nil || !slot.Weapon.Stockpile || slot.Weapon.ReloadTime <= 5 {
						t.Fatalf("%s has no ordinary stockpile production fixture", u.Def.UnitName)
					}
					if slot.Ammo != 0 {
						t.Fatalf("%s began with ammo %d", u.Def.UnitName, slot.Ammo)
					}
					deadline = max(deadline, int(slot.Weapon.ReloadTime)*int(want[i])+600)
				}
				if !protector.SlotAt(0).Weapon.Interceptor {
					t.Fatal("ARMAMD does not carry an interceptor")
				}
				if controller == ai.ControllerClassic {
					// The negative control covers many planner dispatches; only
					// the positive case needs a whole authored production cycle.
					deadline = 300
				}
				for tick := 0; tick < deadline; tick++ {
					stockpileSolvent(s)
					stepBattle(s, 1)
					for i, u := range carriers {
						if q := orders.QueueForUnit(u); q != nil {
							for _, n := range q.Secondary() {
								if n.ID == orders.Lookup("BuildWeapon") && n.Param3 > 0 {
									seenWork[i] = true
								}
							}
						}
						if u.SlotAt(0).Ammo > 0 && !seenWork[i] {
							t.Fatalf("%s gained ammo without observed paid production", u.Def.UnitName)
						}
					}
					if silo.SlotAt(0).Ammo == 1 && protector.SlotAt(0).Ammo == 3 {
						// Let outstanding settlement and another AI observation pass.
						for i := 0; i < 90; i++ {
							stockpileSolvent(s)
							stepBattle(s, 1)
						}
						break
					}
				}
				if s.State != session.StateBattle {
					t.Fatalf("fixture left battle: %v", s.State)
				}
				var wantEnergy, wantMetal float64
				for i, u := range carriers {
					if controller == ai.ControllerClassic {
						want[i] = 0
					}
					slot := u.SlotAt(0)
					if slot.Ammo != want[i] {
						t.Errorf("%s ammo = %d, want %d (production observed %v)", u.Def.UnitName, slot.Ammo, want[i], seenWork[i])
					}
					if q := orders.QueueForUnit(u); q != nil && q.LenSecondary() != 0 {
						t.Errorf("%s kept ammunition queued after its reserve was reached", u.Def.UnitName)
					}
					wantEnergy += float64(want[i]) * float64(slot.Weapon.EnergyPerShot)
					wantMetal += float64(want[i]) * float64(slot.Weapon.MetalPerShot)
				}
				// Cumulative truncated costs telescope to the authored whole-round
				// cost [06 §11.1]. No manual stockpile command or ammo write occurs.
				if energy, metal := p.TotalConsumed[economy.Energy]-before[economy.Energy], p.TotalConsumed[economy.Metal]-before[economy.Metal]; energy != wantEnergy || metal != wantMetal {
					t.Errorf("ammunition charged energy/metal %v/%v, want %v/%v", energy, metal, wantEnergy, wantMetal)
				}
				if controller == ai.ControllerModern && !t.Failed() {
					stockpileLaunch(t, s, silo)
				}
			})
		}
	}
}

func stockpileBattle(t *testing.T, mode gameplay.Mode, controller ai.Controller) *session.Session {
	t.Helper()
	cat, fs := retailcat.Shared(t)
	const mapName = "The Pass"
	if _, ok := cat.Maps[content.CanonicalKey(mapName)]; !ok {
		t.Skipf("retail map %q is absent", mapName)
	}
	cfg := session.DirectSkirmishConfig(mapName)
	cfg.ApplyDefaults()
	cfg.Gameplay, cfg.Players[1].AI = mode, controller
	cfg.CommanderDeath = int(session.CommanderDeathContinues)
	// The launch case scouts a building and then removes that sight source,
	// distinguishing remembered ground attacks from visible acquisition.
	cfg.Mapping, cfg.LineOfSight = 1, 1
	cfg.Difficulty = 2
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 71, 71
	options := session.SkirmishEntryOptions{}
	if controller == ai.ControllerModern {
		options.AIOverrides.All = "jitter=0,style=eco"
	}
	s, err := session.NewSkirmishWithEntryOptions(fs, cat, cfg, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeControllers(s) })
	for i := 0; i < 2; i++ {
		s.Step(s.Clock.ScaledAnchor + 1)
	}
	if s.State != session.StateBattle {
		t.Fatalf("fixture entered %v, want battle", s.State)
	}
	return s
}

func stockpileUnit(t *testing.T, s *session.Session, key string, owner uint8, x, z int64) *units.Unit {
	t.Helper()
	def, ok := s.Catalog.Unit(key)
	if !ok || def == nil {
		t.Fatalf("retail fixture unit %q is absent", key)
	}
	fx, fz := numeric.FixedFromInt(x), numeric.FixedFromInt(z)
	h, err := s.Units.Create(def, owner, fx, s.World.HeightAt(fx, fz), fz)
	if err != nil {
		t.Fatal(err)
	}
	s.CompleteUnit(h)
	return s.Units.Unit(h)
}

func stockpileSolvent(s *session.Session) {
	p := &s.Econ.Players[1]
	p.Capacity = [2]float32{1e6, 1e6}
	p.Stock = p.Capacity
}

func stockpileLaunch(t *testing.T, s *session.Session, silo *units.Unit) {
	t.Helper()
	target := stockpileUnit(t, s, "ARMFUS", 0, 1600, 1600)
	scout := stockpileUnit(t, s, "ARMSOLAR", 1, 1600, 1480)
	slot := silo.SlotAt(0)
	// Hold the paid round while the brain observes the building. The scout
	// also occupies its blast area, so the deliberate policy must wait.
	slot.Reload = 10000
	for i := 0; i < 90; i++ {
		stockpileSolvent(s)
		stepBattle(s, 1)
	}
	if !s.IsUnitVisible(1, target) {
		t.Fatal("scout never exposed the enemy fusion")
	}
	if stockpileGroundOrder(silo, target) {
		t.Fatal("AI ordered a ground attack while its scout occupied the blast area")
	}
	s.Units.Destroy(scout.Handle, units.DeathReclaimed)
	ordered := false
	for tick := 0; tick < 300; tick++ {
		stockpileSolvent(s)
		stepBattle(s, 1)
		if !s.IsUnitVisible(1, target) && stockpileGroundOrder(silo, target) {
			ordered = true
			break
		}
	}
	if !ordered {
		t.Fatalf("AI never installed Suppress at its remembered fusion: visible=%v, slot=%+v", s.IsUnitVisible(1, target), slot.Target)
	}
	// Suppress is the ordinary position-only attack order [04 R-ORD-02 §1].
	// No human order was supplied, and the target is now outside our sight;
	// this proves the controller's AttackPos path before allowing the launch.
	slot.Reload = 0
	for tick := 0; tick < 600; tick++ {
		stockpileSolvent(s)
		stepBattle(s, 1)
		for i := 0; i < s.Combat.Count(); i++ {
			p := &s.Combat.Records[i]
			w, ok := s.Catalog.WeaponByID(p.WeaponID)
			if !ok || w.CanonicalKey != slot.Weapon.CanonicalKey || p.Shooter != silo.Handle {
				continue
			}
			if p.TargetPos.X != target.X || p.TargetPos.Z != target.Z {
				t.Fatalf("AI nuke target = (%v, %v), want known fusion (%v, %v)", p.TargetPos.X, p.TargetPos.Z, target.X, target.Z)
			}
			if slot.Ammo != 0 {
				t.Fatalf("AI launched a nuke without spending its produced round: ammo %d", slot.Ammo)
			}
			return
		}
	}
	t.Fatalf("AI never launched its paid nuke at the known enemy fusion: visible=%v, ammo=%d, target=%+v", s.IsUnitVisible(1, target), slot.Ammo, slot.Target)
}

func stockpileGroundOrder(silo, target *units.Unit) bool {
	if q := orders.QueueForUnit(silo); q != nil {
		for _, n := range q.Primary() {
			if n.ID == orders.Lookup("Suppress") && n.Target == 0 && n.GoalX == target.X && n.GoalZ == target.Z {
				return true
			}
		}
	}
	return false
}
