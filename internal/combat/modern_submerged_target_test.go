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

// Nanolathe Modern policy: DESIGN_WEAPONS_PROJECTILES "Modern submerged
// target release". The boundary repeats [06 §3.1]'s whole height plus model top.
func TestModernSubmergedTargetBoundary(t *testing.T) {
	weapon := &content.WeaponDef{}
	target := &units.Unit{Def: &content.UnitDef{ModelTop: 16}}
	terrain := &world.Terrain{SeaLevel: 64}
	for _, tc := range []struct {
		name           string
		y              numeric.Fixed
		water, release bool
	}{
		{"model above surface", numeric.FixedFromInt(49), false, false},
		{"model at surface", numeric.FixedFromInt(48), false, true},
		{"fraction above boundary", numeric.FixedFromInt(49) - 1, false, true},
		{"model submerged", numeric.FixedFromInt(47), false, true},
		{"water capable", numeric.FixedFromInt(47), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target.Y, weapon.WaterWeapon = tc.y, tc.water
			if got := (&ModernRules{}).ReleaseSubmergedTarget(weapon, target, terrain); got != tc.release {
				t.Fatalf("release=%v, want %v", got, tc.release)
			}
			for _, rules := range []Rules{StrictRules{}, CommunityRules{}, (&Service{}).rules()} {
				if rules.ReleaseSubmergedTarget(weapon, target, terrain) {
					t.Fatal("baseline changed retained-target admission")
				}
			}
		})
	}
}

func submergedTargetFixture(t *testing.T) (*units.World, *world.Terrain, *units.Unit, *units.Unit, *economy.Service) {
	t.Helper()
	w, terrain, shooter, target := newTestWorldAndUnits(t)
	terrain.SeaLevel = 64
	shooter.Y, target.Y = numeric.FixedFromInt(100), numeric.FixedFromInt(49)
	def := *target.Def
	def.ModelTop = 16
	target.Def = &def
	shooter.InstallWeapon(0, &content.WeaponDef{ID: 41, Range: 1000, LineOfSight: true,
		WeaponVelocity: 65536, ReloadTime: 30, DamageDefault: 10, EnergyPerShot: 7, MetalPerShot: 3,
		Tolerance: wideDriftTolerance, PitchTolerance: wideDriftTolerance})
	shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
	econ := &economy.Service{}
	econ.Players[0].Stock[economy.Energy], econ.Players[0].Stock[economy.Metal] = 100, 100
	return w, terrain, shooter, target, econ
}

func TestModernReleasesSubmergedSlotBeforeAimAndCosts(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		for _, stockpile := range []bool{false, true} {
			w, terrain, shooter, target, econ := submergedTargetFixture(t)
			slot := shooter.SlotAt(0)
			slot.Weapon.Stockpile, slot.Ammo = stockpile, 3
			if ordered {
				slot.Flags &^= units.SlotFlagAutonomous
			}
			s := &Service{Rules: &ModernRules{}}
			random, crt := rng.NewSimulation(77), rng.NewCRT(88)
			if sum := s.StepWeaponsForUnit(shooter, 1, w, nil, terrain, econ, nil, &random, &crt); sum.Fired != 1 {
				t.Fatalf("ordered=%v stockpile=%v: surface shot failed: %+v", ordered, stockpile, sum)
			}
			target.Y = numeric.FixedFromInt(48)
			slot.Reload = 2
			slot.Aim.IssueBit = true
			slot.Flags |= units.SlotFlagAimLatch
			beforeRandom, beforeCRT, beforePlayer := random, crt, econ.Players[0]
			beforeAmmo, beforePending, beforeReveal := slot.Ammo, shooter.Pending, shooter.RevealDeadline
			sum := s.StepWeaponsForUnit(shooter, 2, w, nil, terrain, econ, nil, &random, &crt)
			if sum.Fired != 0 || sum.Dispatched || s.Count() != 1 || slot.Target.Kind != units.TargetNone || !slot.IsAutonomous() || slot.Aim.IssueBit || slot.Flags&units.SlotFlagAimLatch != 0 || slot.Reload != 1 {
				t.Fatalf("ordered=%v stockpile=%v: target was not released before weapon work: %+v slot=%+v", ordered, stockpile, sum, slot)
			}
			if random != beforeRandom || crt != beforeCRT || econ.Players[0] != beforePlayer || slot.Ammo != beforeAmmo || shooter.Pending != beforePending || shooter.RevealDeadline != beforeReveal {
				t.Fatal("release changed RNG, costs, ammunition or firing effects")
			}
		}
	}
}

func TestSubmergedTargetStrictBypassPreservesShotEffects(t *testing.T) {
	var expectedRandom rng.Simulation
	var expectedPlayer economy.Player
	for i, rules := range []Rules{nil, StrictRules{}, CommunityRules{}} {
		w, terrain, shooter, target, econ := submergedTargetFixture(t)
		target.Y = numeric.FixedFromInt(48)
		s := &Service{Rules: rules}
		random := rng.NewSimulation(77)
		if sum := s.StepWeaponsForUnit(shooter, 1, w, nil, terrain, econ, nil, &random, nil); sum.Fired != 1 || shooter.SlotAt(0).Target.Unit != target.Handle {
			t.Fatalf("baseline %d stopped retained-target fire: %+v", i, sum)
		}
		if econ.Players[0].Stock[economy.Energy] != 93 || econ.Players[0].Stock[economy.Metal] != 97 || shooter.SlotAt(0).Reload != 30 {
			t.Fatal("baseline lost its resource or reload effects")
		}
		if i == 0 {
			expectedRandom, expectedPlayer = random, econ.Players[0]
		} else if random != expectedRandom || econ.Players[0] != expectedPlayer {
			t.Fatal("bound baseline changed unbound RNG or ledger effects")
		}
	}
}

func TestModernSubmergedReleaseAcquiresAnotherContact(t *testing.T) {
	for _, ordered := range []bool{false, true} {
		w, terrain, shooter, target, econ := submergedTargetFixture(t)
		vis := allVisibleService(terrain)
		h, err := w.Create(target.Def, 1, numeric.FixedFromInt(40), numeric.FixedFromInt(80), numeric.FixedFromInt(40))
		if err != nil {
			t.Fatal(err)
		}
		s := &Service{Rules: &ModernRules{}}
		random := rng.NewSimulation(77)
		s.rebuildTargetRegistry(30, 0, w, vis, terrain, econ)
		target.Y = numeric.FixedFromInt(48)
		if ordered {
			shooter.SlotAt(0).Flags &^= units.SlotFlagAutonomous
		}
		s.StepWeaponsForUnit(shooter, 31, w, vis, terrain, econ, nil, &random, nil)
		s.StepAutonomousForPlayer(0, w, vis, terrain, econ, nil, &random)
		if got := shooter.SlotAt(0).Target; got.Kind != units.TargetUnit || got.Unit != h {
			t.Fatalf("ordered=%v: replacement=%+v, want contact %d", ordered, got, h)
		}
		if sum := s.StepWeaponsForUnit(shooter, 32, w, vis, terrain, econ, nil, &random, nil); sum.Fired != 1 {
			t.Fatalf("replacement did not fire: %+v", sum)
		}
	}
}

func TestModernSubmergedReleaseIsPerSlotAndPreservesGroundTargets(t *testing.T) {
	w, terrain, shooter, target, econ := submergedTargetFixture(t)
	target.Y = numeric.FixedFromInt(48)
	water := *shooter.SlotAt(0).Weapon
	water.ID, water.WaterWeapon = 42, true
	shooter.InstallWeapon(1, &water)
	shooter.SlotAt(1).Target = shooter.SlotAt(0).Target
	ground := water
	ground.ID, ground.WaterWeapon = 43, false
	shooter.InstallWeapon(2, &ground)
	point := units.Target{Kind: units.TargetGround, X: target.X, Z: target.Z}
	shooter.SlotAt(2).Target = point
	s := &Service{Rules: &ModernRules{}}
	random := rng.NewSimulation(77)
	if sum := s.StepWeaponsForUnit(shooter, 1, w, nil, terrain, econ, nil, &random, nil); sum.Fired != 2 {
		t.Fatalf("water/ground shots did not continue independently: %+v", sum)
	}
	if shooter.SlotAt(0).Target.Kind != units.TargetNone || shooter.SlotAt(1).Target.Unit != target.Handle || shooter.SlotAt(2).Target != point {
		t.Fatal("release changed another slot's target")
	}
}

func TestModernSubmersionCancelsOnlyUnlaunchedBurst(t *testing.T) {
	for _, modern := range []bool{false, true} {
		for _, deadline := range []uint32{4, 100} {
			w, terrain, shooter, target, _ := submergedTargetFixture(t)
			target.Y = numeric.FixedFromInt(48)
			s := &Service{Rules: rulesForModern(modern)}
			weapon := shooter.SlotAt(0).Weapon
			weapon.BurstRate, weapon.SprayAngle, weapon.RandomDecay = 3, 500, 100
			cat := &content.Catalog{Weapons: map[string]*content.WeaponDef{"burst": weapon}}
			cat.RebuildWeaponIndex()
			pos := Vec3{X: numeric.FixedFromInt(100), Y: numeric.FixedFromInt(100), Z: numeric.FixedFromInt(100)}
			s.Reserve()
			s.Records[0] = Projectile{Shooter: shooter.Handle, TargetUnit: target.Handle, WeaponID: weapon.ID, BurstRemaining: 2, OrderedBurst: true, BurstDeadline: deadline, ExpiryTick: 1000, Speed: 65536, Pos: pos, StartPos: pos, Velocity: Vec3{X: 65536}}
			s.Reserve()
			s.Records[1] = Projectile{Shooter: shooter.Handle, TargetUnit: target.Handle, WeaponID: weapon.ID, ExpiryTick: 1000, Speed: 65536, Pos: pos, StartPos: pos, Velocity: Vec3{X: 65536}}
			random := rng.NewSimulation(77)
			before := random
			s.TickProjectiles(4, w, terrain, nil, nil, nil, nil, cat, &random, nil)
			if modern {
				if s.Count() != 1 || s.Records[0].BurstRemaining != 0 || s.Records[0].Pos.X != pos.X+65536 || random != before {
					t.Fatal("submersion did not cancel only unlaunched pellets without RNG")
				}
			} else if deadline == 4 {
				if s.Count() != 3 || s.Records[0].BurstRemaining != 1 || random == before {
					t.Fatal("Strict due burst lost its clone or spray effects")
				}
			} else if s.Count() != 2 || s.Records[0].BurstRemaining != 2 || random != before {
				t.Fatal("Strict early burst changed")
			}
		}
	}
}

func TestModernSubmergedSpawnerPrecedesQueriesAndRandomness(t *testing.T) {
	w, terrain, shooter, target, _ := submergedTargetFixture(t)
	target.Y = numeric.FixedFromInt(48)
	launch := Slot{Weapon: shooter.SlotAt(0).Weapon, Flags: shooter.SlotAt(0).Flags}
	random := rng.NewSimulation(77)
	before, original := random, launch
	calls := 0
	s := &Service{Rules: &ModernRules{}}
	query := ShotQuery{Service: s, World: w, Shooter: shooter, Target: target, Terrain: terrain}
	_, fired := TryFire(s, &launch, 0, Target{Kind: TargetUnit, Unit: target.Handle}, 1, FirePorts{
		Shooter: shooter, Shot: &query, RNG: &random,
		MuzzlePiece: func(int) int32 { calls++; return -1 },
		TargetWorld: func(pool.Handle) (Vec3, bool) { calls++; return Vec3{}, true },
	})
	if fired || calls != 0 || random != before || launch != original || s.Count() != 0 || shooter.RevealDeadline != 0 {
		t.Fatal("submerged direct fire performed shot work")
	}
}

func TestModernInstalledBrawlerReleasesSubmergedCrocodile(t *testing.T) {
	catalog, _ := retailcat.Shared(t)
	brawler, croc := catalog.Units["armbrawl"], catalog.Units["coramph"]
	if brawler == nil || croc == nil || brawler.Weapon1Def == nil || brawler.Weapon1Def.WaterWeapon {
		t.Fatal("installed Brawler/Crocodile fixture missing")
	}
	for _, modern := range []bool{false, true} {
		w := newCombatFixtureWorld(10, catalog)
		terrain := &world.Terrain{CellW: 100, CellH: 100, SeaLevel: 64, Plot: make([]world.PlotCell, 100*100)}
		sh, err := w.Create(brawler, 0, numeric.FixedFromInt(100), numeric.FixedFromInt(150), numeric.FixedFromInt(100))
		if err != nil {
			t.Fatal(err)
		}
		h, err := w.Create(croc, 1, numeric.FixedFromInt(200), numeric.FixedFromInt(int64(65-croc.ModelTop)), numeric.FixedFromInt(100))
		if err != nil {
			t.Fatal(err)
		}
		shooter, target := w.Unit(sh), w.Unit(h)
		s := &Service{Rules: rulesForModern(modern)}
		weapon := brawler.Weapon1Def
		shooter.Move.Heading = retailYawFromGo(uint16(YawFromDelta(target.X-shooter.X, target.Z-shooter.Z)))
		shooter.Move.Pitch = uint16(PitchFromDelta(target.X-shooter.X, target.Y-shooter.Y, target.Z-shooter.Z))
		if !s.CanEngageSlotTarget(shooter, target, 0, terrain) {
			t.Fatal("surface Crocodile did not pass installed acquisition")
		}
		random := rng.NewSimulation(77)
		if _, fired := modernLaunch(t, s, w, terrain, shooter, target, weapon, &random); !fired {
			t.Fatal("installed Brawler did not fire at surface Crocodile")
		}
		target.Y = numeric.FixedFromInt(int64(64 - croc.ModelTop))
		if s.CanEngageSlotTarget(shooter, target, 0, terrain) {
			t.Fatal("fully submerged Crocodile passed fresh acquisition")
		}
		if _, fired := modernLaunch(t, s, w, terrain, shooter, target, weapon, &random); fired == modern {
			t.Fatalf("modern=%v: installed retained-target fire=%v", modern, fired)
		}
		shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
		shooter.SlotAt(0).Flags &^= units.SlotFlagAutonomous
		s.StepWeaponsForUnit(shooter, 11, w, nil, terrain, nil, catalog, &random, nil)
		if got := shooter.SlotAt(0).Target.Kind == units.TargetNone; got != modern {
			t.Fatalf("modern=%v: installed target released=%v", modern, got)
		}
	}
}
