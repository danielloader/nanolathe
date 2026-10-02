package combat

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestFiringPositionPhysicalBlockAndTranslatedBurst(t *testing.T) {
	s, w, terrain, shooter, target, weapon := modernCombatFixture(t)
	weapon.Turret = false
	weapon.Burst = 3
	slot := shooter.SlotAt(0)
	slot.Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
	slot.Flags &^= units.SlotFlagAutonomous
	h, err := w.Create(contactDef(40<<16), shooter.Owner, cellCentre(3), 0, cellCentre(1))
	if err != nil {
		t.Fatal(err)
	}
	stampGroundRect(terrain, 3, 1, 1, 1, h)
	econ := &economy.Service{}
	econ.Players[0].Stock[economy.Energy], econ.Players[0].Stock[economy.Metal] = 100, 100
	random := rng.NewSimulation(77)
	randomBefore, resources := random, econ.Players[0]
	if got := s.StepWeaponsForUnit(shooter, 10, w, nil, terrain, econ, nil, &random, nil); got.Fired != 0 {
		t.Fatal("blocked shot fired")
	}
	if !s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("physical refusal was not observed")
	}
	if random != randomBefore || econ.Players[0] != resources {
		t.Fatal("refusal changed RNG/resources")
	}
	shooterBefore, targetBefore := *shooter, *target
	plotBefore := append(terrain.Plot[:0:0], terrain.Plot...)
	count := s.Count()
	s.Events = func(Event) { t.Fatal("candidate emitted event") }
	if s.FiringPositionClear(shooter, target, 10, shooter.X, shooter.Y, shooter.Z, w, terrain) {
		t.Fatal("original blocked position was clear")
	}
	if !s.FiringPositionClear(shooter, target, 10, shooter.X, shooter.Y, cellCentre(4), w, terrain) {
		t.Fatal("translated burst launch was not clear")
	}
	if !reflect.DeepEqual(*shooter, shooterBefore) || !reflect.DeepEqual(*target, targetBefore) ||
		!reflect.DeepEqual(terrain.Plot, plotBefore) || random != randomBefore || econ.Players[0] != resources || s.Count() != count {
		t.Fatal("candidate preview mutated live state")
	}
	if s.FiringPositionClear(shooter, target, 10, numeric.FixedFromInt(2000), shooter.Y, shooter.Z, w, terrain) {
		t.Fatal("candidate bypassed physical range gate")
	}
	if s.FiringPositionClear(shooter, target, 10, shooter.X, numeric.FixedFromInt(-100), shooter.Z, w, terrain) {
		t.Fatal("candidate bypassed shooter medium gate")
	}
	if s.FiringPositionBlocked(shooter, target, 11) {
		t.Fatal("observation leaked across tick")
	}
	copyTarget, copyShooter := *target, *shooter
	if s.FiringPositionBlocked(shooter, &copyTarget, 10) || s.FiringPositionBlocked(&copyShooter, target, 10) {
		t.Fatal("observation accepted reused identity")
	}
	slot.Target = units.Target{}
	if s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("observation survived target replacement")
	}
	slot.Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
	target.Dying = true
	if s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("dying target retained observation")
	}
	target.Dying = false
	s.StepWeaponsForUnit(target, 10, w, nil, terrain, econ, nil, &random, nil)
	if s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("observation survived another unit visit")
	}

	s.Events = nil
	s.Rules = StrictRules{}
	if got := s.StepWeaponsForUnit(shooter, 10, w, nil, terrain, econ, nil, &random, nil); got.Fired != 1 {
		t.Fatalf("Strict did not preserve ordinary launch: %+v", got)
	}
	if s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("Strict exposed Modern observation")
	}
}

func TestFiringPositionCoveredHoldIsNotPhysicalBlock(t *testing.T) {
	s, w, terrain, shooter, target, weapon := modernCombatFixture(t)
	weapon.Turret, weapon.DamageDefault = false, 100
	random := rng.NewSimulation(77)
	if _, ok := modernLaunch(t, s, w, terrain, shooter, target, weapon, &random); !ok {
		t.Fatal("initial shot refused")
	}
	shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
	shooter.SlotAt(0).Flags &^= units.SlotFlagAutonomous
	if got := s.StepWeaponsForUnit(shooter, 10, w, nil, terrain, nil, nil, &random, nil); got.Fired != 0 {
		t.Fatal("covered target fired again")
	}
	if s.FiringPositionBlocked(shooter, target, 10) {
		t.Fatal("coverage requested reposition")
	}
}

type firingPositionTestRules struct {
	ModernRules
	blockedWeapon *content.WeaponDef
}

func (r *firingPositionTestRules) AdmitShot(q *ShotQuery) bool {
	q.Blocked = q.Launch.Weapon == r.blockedWeapon
	return !q.Blocked
}

func TestFiringPositionSuccessfulOtherSlotSuppressesMovement(t *testing.T) {
	for _, successFirst := range []bool{false, true} {
		s, w, terrain, shooter, target, weapon := modernCombatFixture(t)
		weapon.Turret = false
		weapon.EnergyPerShot, weapon.MetalPerShot = 0, 0
		other := *weapon
		other.ID++
		shooter.InstallWeapon(1, &other)
		for idx := 0; idx < 2; idx++ {
			shooter.SlotAt(idx).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
			shooter.SlotAt(idx).Flags &^= units.SlotFlagAutonomous
		}
		blocked := weapon
		if successFirst {
			blocked = &other
		}
		s.Rules = &firingPositionTestRules{blockedWeapon: blocked}
		if got := s.StepWeaponsForUnit(shooter, 10, w, nil, terrain, nil, nil, nil, nil); got.Fired != 1 {
			t.Fatalf("one slot should have fired: %+v", got)
		}
		if s.FiringPositionBlocked(shooter, target, 10) || s.FiringPositionClear(shooter, target, 10, shooter.X, shooter.Y, shooter.Z, w, terrain) {
			t.Fatal("successful slot did not suppress displacement")
		}
	}
}

func TestFiringPositionBallisticCandidateResolvesNewMuzzle(t *testing.T) {
	q := obstructionFixture(t)
	q.Launch = modernBallisticSlot()
	q.Launch.DesiredYaw, q.Launch.DesiredPitch = 0, 0
	q.Terrain.Gravity = numeric.Fixed(8155)
	q.Service.ProjectileWind = &world.Wind{}
	h, err := q.World.Create(contactDef(contactModelTop), 1, q.Aim.X, 0, q.Aim.Z)
	if err != nil {
		t.Fatal(err)
	}
	q.Target = q.World.Unit(h)
	stampGroundRect(q.Terrain, 12, 1, 1, 1, h)
	q.Shooter.InstallWeapon(0, q.Launch.Weapon)
	q.Shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: h}
	q.Service.firingPosition = firingPositionObservation{shooter: q.Shooter, tick: q.Tick}
	q.Blocked = true
	q.Service.observeFiringPosition(&q, &q.Launch, 0, false)
	// The observed angles deliberately cannot reach this target. A candidate
	// must solve from its translated muzzle, not inherit the old direction.
	if !q.Service.FiringPositionClear(q.Shooter, q.Target, q.Tick, cellCentre(2), q.Shooter.Y, cellCentre(4), q.World, q.Terrain) {
		t.Fatal("candidate reused original ballistic angles")
	}
	q.Launch.Weapon.Cruise = true
	if q.Service.FiringPositionClear(q.Shooter, q.Target, q.Tick, cellCentre(2), q.Shooter.Y, cellCentre(4), q.World, q.Terrain) {
		t.Fatal("unsupported flight geometry accepted")
	}
}

func TestFiringPositionObservesRefusedTrialSpread(t *testing.T) {
	s, w, terrain, shooter, target, weapon := modernCombatFixture(t)
	weapon.Accuracy = 1024
	shooter.Move.Heading = 1234
	slot := shooter.SlotAt(0)
	slot.Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
	slot.Flags &^= units.SlotFlagAutonomous
	slot.DesiredYaw = retailYawFromGo(16384) - shooter.Move.Heading
	h, err := w.Create(contactDef(40<<16), shooter.Owner, cellCentre(3), 0, cellCentre(1))
	if err != nil {
		t.Fatal(err)
	}
	stampGroundRect(terrain, 3, 1, 1, 1, h)
	s.firingPosition = firingPositionObservation{shooter: shooter, tick: 10}
	random := rng.NewSimulation(77)
	before := random
	wantTrial := random
	bound := accuracySpreadBoundWithDivisor(weapon.Accuracy, shooter.Health, shooter.MaxHealth, s.veteranSpreadDivisor(shooter.Def, shooter.Kills))
	wantYaw := uint16(recentred(wantTrial.Uint32n(uint32(bound)), int32(bound)))
	wantPitch := uint16(recentred(wantTrial.Uint32n(uint32(bound)), int32(bound)))
	if wantYaw == 0 || wantPitch == 0 {
		t.Fatal("fixture needs nonzero spread on both axes")
	}
	if tryFireForSlot(shooter, slot, 0, 10, terrain, &random, s, w, nil, Vec3{X: target.X, Y: target.Y, Z: target.Z}) {
		t.Fatal("obstructed trial fired")
	}
	observed := s.blockedFiringPosition(shooter, target, 10)
	if observed == nil || observed.yawSpread != wantYaw || observed.pitchSpread != wantPitch {
		t.Fatalf("lost trial spread: got %+v want yaw=%d pitch=%d", observed, wantYaw, wantPitch)
	}
	if random != before || slot.DesiredYaw != retailYawFromGo(16384)-shooter.Move.Heading {
		t.Fatal("refusal committed RNG or absolute yaw")
	}
	if !s.FiringPositionClear(shooter, target, 10, shooter.X, shooter.Y, cellCentre(4), w, terrain) || random != before {
		t.Fatal("candidate redrew spread or did not clear blocker")
	}
}
