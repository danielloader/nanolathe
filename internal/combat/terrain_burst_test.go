package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func terrainBurstWeapon() *content.WeaponDef {
	w := modernTerrainWeapon()
	w.Burst, w.BurstRate = 3, 5
	return w
}

// Nanolathe Modern policy, DESIGN_WEAPONS_PROJECTILES §2.3.1. A rejected
// anchor must stop before the retail effects of [06 §4.1][06 §4.4].
func TestModernBurstTerrainRootRefusesBeforeCallbacksAndRNG(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules Rules
		block bool
	}{{"modern", &ModernRules{}, true}, {"strict", StrictRules{}, false}, {"community", CommunityRules{}, false}, {"unbound", nil, false}} {
		t.Run(tc.name, func(t *testing.T) {
			_, terrain := newContactFixture(t)
			terrain.PlotAt(4, 1).SetMinHeight(32)
			muzzle, aim := modernTerrainPoints()
			weapon := terrainBurstWeapon()
			weapon.Turret, weapon.Accuracy = true, 128
			weapon.SoundStart, weapon.StartSmoke = "burst-start", true
			slot := Slot{Weapon: weapon, DesiredYaw: 1234, DesiredPitch: 2345}
			priorSlot := slot
			r := rng.NewSimulation(9)
			priorRNG := r
			script, events := &scriptRecorder{}, &eventRecorder{}
			queries := 0
			shot := ShotQuery{Tick: 10, Terrain: terrain}
			ports := FirePorts{Origin: Vec3{X: muzzle.X, Y: numeric.FixedFromInt(64), Z: muzzle.Z},
				MuzzlePiece: func(int) int32 { queries++; return 3 },
				MuzzleWorld: func(int32) (Vec3, bool) { return muzzle, true },
				RNG:         &r, Script: script, Events: events, ShooterHealth: 100, ShooterMaxHealth: 100}
			if tc.block {
				ports.Shot = &shot
			}
			svc := Service{Rules: tc.rules}
			_, fired := TryFire(&svc, &slot, 0, Target{Kind: TargetPoint, X: aim.X, Y: aim.Y, Z: aim.Z}, 10, ports)
			if fired == tc.block || queries != 1 {
				t.Fatalf("fired=%v queries=%d, want fired=%v and one muzzle query", fired, queries, !tc.block)
			}
			if tc.block {
				if !shot.Blocked || r != priorRNG || svc.Count() != 0 || len(script.calls) != 0 || len(events.sounds) != 0 || len(events.smoke) != 0 {
					t.Fatalf("blocked root leaked effects: blocked=%v draws=%d count=%d script=%v events=%+v", shot.Blocked, r.Draws(), svc.Count(), script.calls, events)
				}
				if slot.DesiredYaw != priorSlot.DesiredYaw || slot.DesiredPitch != priorSlot.DesiredPitch {
					t.Fatal("rejected spread changed the launch angles")
				}
			} else if r.Draws() != 2 || svc.Count() != 1 || svc.Records[0].BurstRemaining != weapon.Burst ||
				len(script.calls) != 2 || script.calls[0] != "FirePrimary" || script.calls[1] != "RockUnit" || len(events.sounds) != 1 || len(events.smoke) != 1 {
				t.Fatalf("retail bypass changed: draws=%d count=%d script=%v events=%+v", r.Draws(), svc.Count(), script.calls, events)
			}
		})
	}
}

func TestModernBurstTerrainRootPreservesResourcesAndAmmo(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, stockpile := range []bool{false, true} {
			name := "strict/cost"
			if modern {
				name = "modern/cost"
			}
			if stockpile {
				name += "/stockpile"
			}
			t.Run(name, func(t *testing.T) {
				w, terrain, shooter, target := newTestWorldAndUnits(t)
				muzzle, aim := modernTerrainPoints()
				shooter.X, shooter.Y, shooter.Z = muzzle.X, muzzle.Y, muzzle.Z
				target.X, target.Y, target.Z = aim.X, aim.Y, aim.Z
				terrain.PlotAt(4, 1).SetMinHeight(32)
				weapon := terrainBurstWeapon()
				weapon.EnergyPerShot, weapon.MetalPerShot, weapon.ReloadTime = 100, 5, 30
				weapon.Stockpile = stockpile
				shooter.InstallWeapon(0, weapon)
				slot := shooter.SlotAt(0)
				slot.Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
				slot.Ammo = 3
				beforeTarget, beforePending := slot.Target, shooter.Pending
				var econ economy.Service
				econ.Players[shooter.Owner].Stock[economy.Energy] = 500
				econ.Players[shooter.Owner].Stock[economy.Metal] = 50
				svc := Service{Rules: rulesForModern(modern)}
				var sum UnitStepSummary
				svc.firePreparedSlot(shooter, slot, 0, &slotPrep{weapon: weapon, tgtPos: aim}, 10, terrain, &econ, nil, w, nil, &sum)
				wantFired, wantReload, wantAmmo := 1, int32(30), int32(3)
				wantEnergy, wantMetal := float32(500), float32(50)
				if modern {
					wantFired, wantReload = 0, 0
				} else if stockpile {
					wantAmmo, wantReload = 2, 0
				} else {
					wantEnergy, wantMetal = 400, 45
				}
				if sum.Fired != wantFired || slot.Reload != wantReload || slot.Ammo != wantAmmo ||
					econ.Players[shooter.Owner].Stock[economy.Energy] != wantEnergy || econ.Players[shooter.Owner].Stock[economy.Metal] != wantMetal {
					t.Fatalf("fired=%d reload=%d ammo=%d stock=%v", sum.Fired, slot.Reload, slot.Ammo, econ.Players[shooter.Owner].Stock)
				}
				if modern && (slot.Target != beforeTarget || shooter.Pending != beforePending) {
					t.Fatal("terrain gate changed retained target or order feedback")
				}
			})
		}
	}
}

// A pending pellet copies the actual parked template after muzzle refresh,
// rather than solving another shot from the current aim [06 §4.3].
func TestModernBurstTerrainUsesTemplateOriginAndVelocity(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pos     Vec3
		vel     Vec3
		ridgeX  int32
		ridgeZ  int32
		floor   uint8
		blocked bool
	}{
		{name: "inherited spray crosses ridge", pos: Vec3{X: cellCentre(1), Y: numeric.FixedFromInt(16), Z: cellCentre(1)}, vel: Vec3{X: numeric.FixedFromInt(16), Z: numeric.FixedFromInt(16)}, ridgeX: 2, ridgeZ: 2, floor: 32, blocked: true},
		{name: "refreshed muzzle crosses ridge", pos: Vec3{X: cellCentre(1), Y: numeric.FixedFromInt(16), Z: cellCentre(2)}, vel: Vec3{X: numeric.FixedFromInt(16)}, ridgeX: 2, ridgeZ: 2, floor: 32, blocked: true},
		{name: "inherited level flight clears downward aim", pos: Vec3{X: cellCentre(1), Y: numeric.FixedFromInt(64), Z: cellCentre(1)}, vel: Vec3{X: numeric.FixedFromInt(16)}, ridgeX: 4, ridgeZ: 1, floor: 56},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, terrain := newContactFixture(t)
			terrain.PlotAt(tc.ridgeX, tc.ridgeZ).SetMinHeight(tc.floor)
			_, aim := modernTerrainPoints()
			weapon := terrainBurstWeapon()
			anchor := Projectile{Pos: tc.pos, StartPos: Vec3{Y: numeric.FixedFromInt(100)}, TargetPos: aim,
				Velocity: tc.vel, Speed: numeric.FixedFromInt(16), StoredPlanarDistance: numeric.FixedFromInt(176),
				BurstRemaining: 2, ExpiryTick: 11}
			before := anchor
			q := ShotQuery{Launch: Slot{Weapon: weapon}, Muzzle: tc.pos, Aim: aim, Tick: 10, Terrain: terrain, Burst: &anchor}
			if admitted := (&ModernRules{}).AdmitShot(&q); admitted == tc.blocked || q.Blocked != tc.blocked {
				t.Fatalf("admitted=%v blocked=%v, want blocked=%v", admitted, q.Blocked, tc.blocked)
			}
			if anchor != before {
				t.Fatal("terrain preview mutated the live template")
			}
		})
	}
}

// Clone expiry uses timer first, otherwise stored distance plus one cell over
// scalar speed. The first motion is the emission tick plus one, and equality
// with expiry retires without sampling [06 §4.3][06 §6.3].
func TestModernBurstTerrainCloneLifetimeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		timer    int32
		distance numeric.Fixed
		speed    int64
		rangeMax int32
		root     bool
		blocked  bool
	}{
		{name: "root timer expires before first motion", timer: 1, root: true},
		{name: "root timer permits first motion", timer: 2, root: true, blocked: true},
		{name: "clone timer expires before first motion", timer: 1, distance: numeric.FixedFromInt(176)},
		{name: "clone timer permits first motion", timer: 2, blocked: true},
		{name: "clone distance expires before first motion"},
		{name: "clone distance permits first motion", distance: numeric.FixedFromInt(16), blocked: true},
		{name: "stored scalar speed overrides weapon velocity", distance: numeric.FixedFromInt(16), speed: 32},
		{name: "root range expiry does not limit clone flight", rangeMax: 16, root: true, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, terrain := newContactFixture(t)
			terrain.PlotAt(2, 1).SetMinHeight(32)
			muzzle, aim := modernTerrainPoints()
			weapon := terrainBurstWeapon()
			weapon.WeaponTimer = tc.timer
			if tc.rangeMax != 0 {
				weapon.Range = tc.rangeMax
			}
			anchor := Projectile{Pos: muzzle, TargetPos: aim, Velocity: Vec3{X: numeric.FixedFromInt(16)},
				Speed: numeric.FixedFromInt(16), StoredPlanarDistance: tc.distance, BurstRemaining: 2, ExpiryTick: 10}
			if tc.speed != 0 {
				anchor.Speed = numeric.FixedFromInt(tc.speed)
			}
			q := ShotQuery{Launch: Slot{Weapon: weapon}, Muzzle: muzzle, Aim: aim, Tick: 10, Terrain: terrain}
			if !tc.root {
				q.Burst = &anchor
			}
			if admitted := (&ModernRules{}).AdmitShot(&q); admitted == tc.blocked || q.Blocked != tc.blocked {
				t.Fatalf("admitted=%v blocked=%v, want blocked=%v", admitted, q.Blocked, tc.blocked)
			}
		})
	}
}

// These independently rounded points reproduce a descending Brawler shot into
// The Pass's hillside. The installed weapon and map remain asset inputs, and
// the admission answer is Nanolathe Modern policy, not a retail parity claim.
func TestModernBurstTerrainInstalledBrawlerThePassRetail(t *testing.T) {
	catalog, fs := retailcat.Shared(t)
	brawler := catalog.Units["armbrawl"]
	if brawler == nil || brawler.Weapon1Def == nil {
		t.Fatal("installed Brawler primary weapon missing")
	}
	weapon := brawler.Weapon1Def
	if weapon.Burst != 4 || weapon.RandomDecay != 0 || liveCreationFamilyForWeapon(weapon) != CreationOrdinary || MotionFamilyForWeapon(weapon) != MotionDirect {
		t.Fatal("installed Brawler no longer exercises ordinary direct burst admission")
	}
	terrain, err := world.Load(fs, catalog, "the pass")
	if err != nil {
		t.Fatalf("load The Pass: %v", err)
	}
	muzzle := Vec3{X: numeric.FixedFromInt(1986), Y: numeric.FixedFromInt(272), Z: numeric.FixedFromInt(703)}
	aim := Vec3{X: numeric.FixedFromInt(2173), Y: numeric.FixedFromInt(1), Z: numeric.FixedFromInt(737)}
	if got := modernTerrainAdmission(Slot{Weapon: weapon}, muzzle, aim, 10, terrain, nil, nil); got != terrainShotBlocked {
		t.Fatalf("installed Brawler hillside admission=%v, want blocked", got)
	}
}

func TestModernBurstTerrainUncertainFamiliesRemainAdmitted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*content.WeaponDef)
	}{
		{"random decay", func(w *content.WeaponDef) { w.RandomDecay = 10 }},
		{"signed negative count", func(w *content.WeaponDef) { w.Burst = -1 }},
		{"self propelled", func(w *content.WeaponDef) { w.SelfProp = true }},
		{"vertical", func(w *content.WeaponDef) { w.VLaunch, w.SelfProp = true, true }},
		{"ballistic", func(w *content.WeaponDef) { w.LineOfSight, w.Ballistic = false, true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, terrain := newContactFixture(t)
			terrain.PlotAt(4, 1).SetMinHeight(32)
			muzzle, aim := modernTerrainPoints()
			weapon := terrainBurstWeapon()
			tc.change(weapon)
			if got := modernTerrainAdmission(Slot{Weapon: weapon}, muzzle, aim, 10, terrain, nil, nil); got != terrainShotUnknown {
				t.Fatalf("admission=%v, want unknown", got)
			}
		})
	}
}

// Refusal happens after the established refresh and before count/deadline,
// allocation, clone sound and spray. The root's existing charge stays paid.
func TestModernBurstTerrainCancelsDuePelletBeforeEffects(t *testing.T) {
	for _, modern := range []bool{false, true} {
		name := "strict"
		if modern {
			name = "modern"
		}
		t.Run(name, func(t *testing.T) {
			w, terrain, shooter, target := newTestWorldAndUnits(t)
			muzzle, aim := modernTerrainPoints()
			muzzle.Y, aim.Y = numeric.FixedFromInt(64), numeric.FixedFromInt(64)
			shooter.X, shooter.Y, shooter.Z = muzzle.X, muzzle.Y, muzzle.Z
			target.X, target.Y, target.Z = aim.X, aim.Y, aim.Z
			terrain.PlotAt(4, 1).SetMinHeight(32)
			weapon := terrainBurstWeapon()
			weapon.EnergyPerShot, weapon.MetalPerShot, weapon.ReloadTime = 100, 5, 30
			weapon.SprayAngle, weapon.SoundTrigger, weapon.SoundStart = 100, true, "burst-start"
			shooter.InstallWeapon(0, weapon)
			slot := shooter.SlotAt(0)
			slot.Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
			var econ economy.Service
			econ.Players[shooter.Owner].Stock[economy.Energy] = 500
			econ.Players[shooter.Owner].Stock[economy.Metal] = 50
			svc := Service{Rules: rulesForModern(modern)}
			var events []Event
			svc.SetEvents(func(e Event) { events = append(events, e) })
			r := rng.NewSimulation(9)
			var sum UnitStepSummary
			svc.firePreparedSlot(shooter, slot, 0, &slotPrep{weapon: weapon, tgtPos: aim}, 10, terrain, &econ, &r, w, nil, &sum)
			if sum.Fired != 1 || svc.Count() != 1 {
				t.Fatal("clear root did not launch")
			}
			beforeRNG, beforeEvents := r, len(events)
			beforeStock, beforeReload, beforeAmmo := econ.Players[shooter.Owner].Stock, slot.Reload, slot.Ammo
			beforeDeadline, beforeVelocity := svc.Records[0].BurstDeadline, svc.Records[0].Velocity
			refreshed := muzzle
			refreshed.Y = numeric.FixedFromInt(16)
			refreshes := 0
			n := svc.advanceBurstAt(0, 15, &r, func(int32) (*content.WeaponDef, bool) { return weapon, true },
				func(pool.Handle, int16) (Vec3, bool) { refreshes++; return refreshed, true }, w, terrain)
			if refreshes != 1 || svc.Records[0].Pos != refreshed {
				t.Fatal("due admission did not follow the live muzzle refresh")
			}
			if modern {
				if n != 0 || svc.Count() != 1 || !svc.Slots.IsDead(1) || svc.Records[0].BurstRemaining != 0 ||
					svc.Records[0].BurstDeadline != beforeDeadline || svc.Records[0].Velocity != beforeVelocity || r != beforeRNG || len(events) != beforeEvents {
					t.Fatalf("canceled pellet leaked effects: clones=%d count=%d anchor=%+v draws=%d events=%d", n, svc.Count(), svc.Records[0], r.Draws(), len(events))
				}
			} else {
				beforeRNG.Uint32n(uint32(weapon.SprayAngle))
				if n != 1 || svc.Count() != 2 || svc.Records[0].BurstRemaining != 2 ||
					svc.Records[0].BurstDeadline != beforeDeadline+5 || r != beforeRNG || len(events) != beforeEvents+1 {
					t.Fatalf("strict due pellet changed: clones=%d count=%d draws=%d events=%d", n, svc.Count(), r.Draws(), len(events))
				}
			}
			if econ.Players[shooter.Owner].Stock != beforeStock || slot.Reload != beforeReload || slot.Ammo != beforeAmmo ||
				beforeStock[economy.Energy] != 400 || beforeStock[economy.Metal] != 45 {
				t.Fatal("pellet admission changed the existing root charge or slot state")
			}
		})
	}
}

// A clear Modern burst keeps the retail copy-before-spray sequence, including
// the final discarded spray draw, and waits until the next phase to move.
func TestModernBurstTerrainClearPreservesCloneAndSpray(t *testing.T) {
	worldUnits, terrain := newContactFixture(t)
	muzzle, aim := modernTerrainPoints()
	weapon := terrainBurstWeapon()
	weapon.SprayAngle, weapon.SoundTrigger, weapon.SoundStart = 100, true, "burst-start"
	strict, modern := Service{Rules: StrictRules{}}, Service{Rules: &ModernRules{}}
	rStrict, rModern := rng.NewSimulation(9), rng.NewSimulation(9)
	for _, run := range []struct {
		s *Service
		r *rng.Simulation
	}{{&strict, &rStrict}, {&modern, &rModern}} {
		shot := ShotQuery{Tick: 10, Terrain: terrain}
		ports := FirePorts{Origin: muzzle, RNG: run.r}
		if previewsShot(run.s.rules()) {
			ports.Shot = &shot
		}
		if _, ok := TryFire(run.s, &Slot{Weapon: weapon}, 0, Target{Kind: TargetPoint, X: aim.X, Y: aim.Y, Z: aim.Z}, 10, ports); !ok {
			t.Fatal("clear burst root was rejected")
		}
	}
	for pellet := 0; pellet < int(weapon.Burst); pellet++ {
		tick := uint32(15 + pellet*5)
		inherited := strict.Records[0].Velocity
		for _, run := range []struct {
			s *Service
			r *rng.Simulation
		}{{&strict, &rStrict}, {&modern, &rModern}} {
			if n := run.s.advanceBurstAt(0, tick, run.r, func(int32) (*content.WeaponDef, bool) { return weapon, true }, nil, worldUnits, terrain); n != 1 {
				t.Fatalf("clear pellet %d was rejected", pellet)
			}
			clone := run.s.Records[pellet+1]
			if clone.Pos != muzzle || clone.Velocity != inherited || clone.CreationTick != tick || clone.ExpiryTick != tick+12 || clone.BurstRemaining != 0 {
				t.Fatalf("pellet %d changed copy/expiry/span: %+v", pellet, clone)
			}
		}
		if rModern != rStrict || rModern.Draws() != uint64(pellet+1) || strict.Count() != modern.Count() {
			t.Fatal("clear Modern burst changed the retail spray stream")
		}
		for i := 0; i < strict.Count(); i++ {
			if strict.Records[i] != modern.Records[i] {
				t.Fatalf("clear Modern burst changed record %d", i)
			}
		}
	}
	if !strict.Slots.IsDead(1) || !modern.Slots.IsDead(1) {
		t.Fatal("final clone did not retire the anchor silently")
	}
}
