package combat

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

func TestModernInfectorPreferenceKeepsAdmissionAndHysteresis(t *testing.T) {
	for _, tc := range []string{"active", "disabled", "unseen", "radar", "out of range", "immune damage", "equally dangerous", "retention", "tie"} {
		t.Run(tc, func(t *testing.T) {
			s, w, terrain, shooter, ordinary, weapon := modernCombatFixture(t)
			d := *ordinary.Def
			d.BMCode, d.CanMove = 1, true
			h, err := w.Create(&d, 1, cellCentre(8), ordinary.Y, ordinary.Z)
			if err != nil {
				t.Fatal(err)
			}
			infector := w.Unit(h)
			s.SetInfectionThreat(func(u *units.Unit) bool { return u == infector })
			shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: ordinary.Handle}
			want := infector.Handle
			switch tc {
			case "disabled":
				s.SetInfectionThreat(nil)
				want = ordinary.Handle
			case "unseen", "radar":
				s.SetVisibility(func(_ visibility.PlayerID, v visibility.Target) bool { return v.X != infector.X })
				infector.Flags |= visibility.SeenBit
				want = ordinary.Handle
			case "out of range":
				infector.X = shooter.X + (2000 << 16)
				want = ordinary.Handle
			case "immune damage":
				infector.Def.DamageModifier = 0
				infector.Armored = true
				want = ordinary.Handle
			case "equally dangerous":
				ordinary.InstallWeapon(0, weapon)
				infector.InstallWeapon(0, weapon)
			case "retention":
				danger := *weapon
				danger.DamageDefault = 10000
				ordinary.InstallWeapon(0, &danger)
				want = ordinary.Handle
			case "tie":
				ordinary.X = infector.X
				shooter.SlotAt(0).Target = units.Target{}
				s.SetInfectionThreat(func(*units.Unit) bool { return true })
				want = ordinary.Handle
			}
			q := modernTargetQuery(s, w, terrain, shooter, infector, ordinary)
			q.FromSecondary = tc == "radar"
			sim := rng.NewSimulation(41)
			q.Acquisition.RNG = &sim
			beforeSim := sim
			got, ok := s.rules().SelectTarget(s, &q)
			if !ok || got != want {
				t.Fatalf("got %v/%v want %v", got, ok, want)
			}
			if sim != beforeSim {
				t.Fatal("Modern preference spent random draws")
			}
		})
	}
}

func TestInfectorBonusDoesNotOverrideManualTargetOrHoldFire(t *testing.T) {
	for _, hold := range []bool{false, true} {
		s, w, terrain, shooter, target, _ := modernCombatFixture(t)
		h, err := w.Create(target.Def, 1, cellCentre(8), target.Y, target.Z)
		if err != nil {
			t.Fatal(err)
		}
		s.SetInfectionThreat(func(u *units.Unit) bool { return u.Handle == h })
		shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
		if hold {
			shooter.Flags &^= units.StandingFieldMask << units.StandingFireShift
		} else {
			shooter.SlotAt(0).Flags &^= units.SlotFlagAutonomous
		}
		s.targets.primary[shooter.Owner] = []pool.Handle{target.Handle, h}
		for i := 0; i < 40; i++ {
			s.StepAutonomousForPlayer(shooter.Owner, w, nil, terrain, nil, nil, nil)
		}
		if shooter.SlotAt(0).Target.Unit != target.Handle {
			t.Fatal("infector stole manual target or Hold Fire work")
		}
	}
}

func TestStrictAndCommunityNeverReadInfectionPreference(t *testing.T) {
	for _, rules := range []Rules{nil, StrictRules{}, CommunityRules{}} {
		s, w, terrain, shooter, target, _ := modernCombatFixture(t)
		s.Rules = rules
		s.SetInfectionThreat(func(*units.Unit) bool { t.Fatal("disabled mode read infection threat"); return true })
		q := modernTargetQuery(s, w, terrain, shooter, target)
		sim := rng.NewSimulation(19)
		q.Acquisition.RNG = &sim
		before := sim
		got, ok := s.rules().SelectTarget(s, &q)
		after := sim
		s.SetInfectionThreat(nil)
		sim = before
		baseline, baseOK := s.rules().SelectTarget(s, &q)
		if got != baseline || ok != baseOK || sim != after {
			t.Fatal("disabled mode changed selection")
		}
	}
}

func TestHumanAndComputerAutonomousWeaponsPreferInfector(t *testing.T) {
	for _, control := range []uint8{1, 2} {
		s, w, terrain, shooter, target, _ := modernCombatFixture(t)
		h, err := w.Create(target.Def, 1, cellCentre(8), target.Y, target.Z)
		if err != nil {
			t.Fatal(err)
		}
		s.SetControlByte(func(uint8) uint8 { return control })
		s.SetInfectionThreat(func(u *units.Unit) bool { return u.Handle == h })
		shooter.SlotAt(0).Target = units.Target{Kind: units.TargetUnit, Unit: target.Handle}
		s.targets.primary[shooter.Owner] = []pool.Handle{target.Handle, h}
		for i := 0; i < 40; i++ {
			s.StepAutonomousForPlayer(shooter.Owner, w, nil, terrain, nil, nil, nil)
		}
		if shooter.SlotAt(0).Target.Unit != h {
			t.Fatalf("control=%v target=%v want=%v", control, shooter.SlotAt(0).Target.Unit, h)
		}
	}
}
