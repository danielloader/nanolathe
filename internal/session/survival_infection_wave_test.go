package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
)

func infectionWaveFixture(t *testing.T, mode gameplay.Mode) *Session {
	t.Helper()
	s, actor, victim := infectionSession(t, mode)
	s.Survival.pool = survival.Pool{Units: []survival.Unit{
		{Key: "host", Def: victim.Def, Tier: 1, Domain: survival.Ground, Cost: 100},
		{Key: "infector", Def: actor.Def, Tier: 1, Domain: survival.Ground, Cost: 100},
	}, MaxTier: 1, Tier1Median: 100}
	s.Survival.tuning = survival.DefaultTuning(survival.PaceNormal)
	return s
}

func waveInfectors(w survival.Wave) int {
	n := 0
	for _, g := range w.Groups {
		for _, i := range g.Picks {
			if i == 1 {
				n++
			}
		}
	}
	return n
}

func TestSurvivalInfectorsWaitThenSupportOrdinaryWaves(t *testing.T) {
	s := infectionWaveFixture(t, gameplay.Modern)
	for _, pace := range []survival.Pace{survival.PaceNormal, survival.PaceRelaxed, survival.PaceRelentless} {
		s.Survival.tuning = survival.DefaultTuning(pace)
		for _, tick := range []uint32{60 * 30, survivalInfectorFrom - 1, survivalInfectorFrom, 20 * 60 * 30} {
			for seed := uint32(1); seed <= 24; seed++ {
				*s.SimRNG() = rng.NewSimulation(seed)
				wantRNG := *s.SimRNG()
				crt, stock := *s.CrtRNG(), s.Econ.Players[1].Stock
				baseline := survival.Plan(s.Survival.wave, tick, &s.Survival.pool, s.Survival.tuning, s.Survival.opts, func(_ uint16, i int) bool { return i != 1 }, &wantRNG)
				wave := s.survivalPlanWave(tick, nil)
				want := 0
				if tick >= survivalInfectorFrom && wave.Units() >= 4 {
					want = 1
				}
				if got := waveInfectors(wave); got != want {
					t.Fatalf("pace=%v tick=%d seed=%d: infectors=%d want=%d", pace, tick, seed, got, want)
				}
				if wave.Units() != baseline.Units() || wave.Budget != baseline.Budget || *s.SimRNG() != wantRNG || *s.CrtRNG() != crt || s.Econ.Players[1].Stock != stock {
					t.Fatal("support substitution changed count, budget, RNG or resources")
				}
			}
		}
	}
}

func TestSurvivalInfectorSubstitutionAdmission(t *testing.T) {
	for _, tc := range []string{"price", "tier", "domain", "entry", "small-wave"} {
		t.Run(tc, func(t *testing.T) {
			s := infectionWaveFixture(t, gameplay.Modern)
			var enter survival.Entry
			switch tc {
			case "price":
				s.Survival.pool.Units[1].Cost = 101
			case "tier":
				s.Survival.pool.Units[1].Tier = 16
				s.Survival.pool.MaxTier = 16
				s.Survival.tuning.UnlockUnits = 100
			case "domain":
				s.Survival.pool.Units[1].Domain = survival.Air
			case "entry":
				enter = func(_ uint16, i int) bool { return i != 1 }
			case "small-wave":
				s.Survival.tuning.BaseUnits = 0
				s.Survival.tuning.MaxDirections = 1
			}
			if w := s.survivalPlanWave(survivalInfectorFrom, enter); waveInfectors(w) != 0 {
				t.Fatalf("ignored %s admission", tc)
			}
		})
	}
}

func TestSurvivalInfectorLiveCapIncludesOldWavesAndRechecksSpawn(t *testing.T) {
	s := infectionWaveFixture(t, gameplay.Modern)
	def := *s.Survival.pool.Units[1].Def
	def.UnitName = "otherinfector"
	h, err := s.Units.Create(&def, s.Survival.attacker, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Neither existing attacker belongs to the current wave's handle list.
	if s.survivalLiveInfectors() != 2 {
		t.Fatal("missed old or different-definition infector")
	}
	if w := s.survivalPlanWave(survivalInfectorFrom, nil); waveInfectors(w) != 0 {
		t.Fatal("planned past live cap")
	}
	s.Survival.plan = survival.Wave{Groups: []survival.Group{{Picks: []int{1, 1, 1, 1}}}}
	before := s.Units.LiveCountForPlayer(int(s.Survival.attacker))
	s.survivalSpawn(survivalInfectorFrom)
	if s.Survival.nextG != 1 || s.Units.LiveCountForPlayer(int(s.Survival.attacker)) != before {
		t.Fatal("spawn did not drop newly capped pick")
	}
	s.Units.Unit(h).Dying = true
	if w := s.survivalPlanWave(survivalInfectorFrom, nil); waveInfectors(w) != 1 {
		t.Fatal("dying infector held admission slot")
	}
}

func TestSurvivalInfectorWavePolicyBypassPreservesPlanner(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		s := infectionWaveFixture(t, mode)
		if mode == gameplay.Modern {
			s.Survival.pool.Units[1].Def.Category, s.Survival.pool.Units[1].Def.NanolatheInfector = "ORDINARY", false
		}
		for _, tick := range []uint32{60 * 30, survivalInfectorFrom} {
			*s.SimRNG() = rng.NewSimulation(7)
			expectedRNG := *s.SimRNG()
			expected := survival.Plan(s.Survival.wave, tick, &s.Survival.pool, s.Survival.tuning, s.Survival.opts, nil, &expectedRNG)
			actual := s.survivalPlanWave(tick, nil)
			if !reflect.DeepEqual(actual, expected) || *s.SimRNG() != expectedRNG {
				t.Fatal("changed ordinary or Strict planner")
			}
		}
	}
}

func TestSurvivalInfectorSpawnRechecksModeSwitchAndSpentAttempt(t *testing.T) {
	s := infectionWaveFixture(t, gameplay.Strict31)
	// An old Strict plan can contain a whole infector signature. Even a dead
	// or failed previous pick consumes the single attempt after rebinding.
	s.Survival.plan = survival.Wave{Groups: []survival.Group{{Picks: []int{0, 1}}, {Picks: []int{1, 0}}}}
	s.SetGameplay(gameplay.Modern)
	s.Survival.nextG, s.Survival.nextP = 0, 2
	if s.survivalInfectorSpawnAllowed(survivalInfectorFrom - 1) {
		t.Fatal("mode switch bypassed opening grace")
	}
	if !s.survivalInfectorSpawnAllowed(survivalInfectorFrom) {
		t.Fatal("first eligible attempt denied")
	}
	s.Survival.nextG, s.Survival.nextP = 1, 1
	if s.survivalInfectorSpawnAllowed(survivalInfectorFrom) {
		t.Fatal("another direction bypassed spent wave attempt")
	}
}
