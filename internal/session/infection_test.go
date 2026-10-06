package session

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func infectionSession(t *testing.T, mode gameplay.Mode) (*Session, *units.Unit, *units.Unit) {
	t.Helper()
	s := captureSeamSession(t)
	actor := s.Catalog.Units["captureseamcaptor"]
	actor.CanCapture = false
	actor.Category = "NANOLATHE_INFECTOR"
	actor.NanolatheInfector = true
	actor.CanPatrol, actor.CanAttack = true, true
	victim := s.Catalog.Units["captureseamvictim"]
	victim.CanPatrol, victim.CanAttack = true, true
	victim.Weapon1Def = &content.WeaponDef{ID: 7}
	s.SetGameplay(mode)
	s.Survival = &survivalState{attacker: 1, team: []uint8{0}, phase: survivalActive, wave: 3}
	create := func(d *content.UnitDef, owner uint8, x int64) *units.Unit {
		h, err := s.Units.Create(d, owner, numeric.FixedFromInt(x), 0, numeric.FixedFromInt(128))
		if err != nil {
			t.Fatal(err)
		}
		s.CompleteUnit(h)
		return s.Units.Unit(h)
	}
	return s, create(actor, 1, 128), create(victim, 0, 192)
}

func TestSurvivalInfectionSelectsMobilesAndStrictKeepsPatrol(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31, gameplay.Community39} {
		t.Run(string(mode), func(t *testing.T) {
			s, actor, victim := infectionSession(t, mode)
			bdef := *victim.Def
			bdef.UnitName, bdef.BMCode, bdef.Commander = "fixturebuilding", 0, false
			h, err := s.Units.Create(&bdef, 0, actor.X+16<<16, 0, actor.Z)
			if err != nil {
				t.Fatal(err)
			}
			s.CompleteUnit(h)
			su := survivalUnit{h: actor.Handle}
			sim, crt, stock := *s.SimRNG(), *s.CrtRNG(), s.Econ.Players[1].Stock
			s.survivalSend(&su, actor, 20)
			if mode == gameplay.Modern {
				if su.target != victim.Handle {
					t.Fatal("infector preferred immune building")
				}
				for _, change := range []func(){func() { victim.Def.Builder = true }, func() { victim.Def.Builder = false; victim.Def.Commander = true }, func() { victim.Def.Commander = false; victim.Y = actor.Y + 97<<16 }, func() { victim.Y = 0; victim.Remaining = 0.5 }, func() { victim.Remaining = 0; victim.Attachment.Carrier = h }} {
					change()
					if s.survivalInfectionTarget(actor, nil, 20) != nil {
						t.Fatal("selected immune or vertically unreachable target")
					}
				}
			} else if su.target != h {
				t.Fatal("Strict/Community changed structure-first patrol")
			}
			if *s.SimRNG() != sim || *s.CrtRNG() != crt || s.Econ.Players[1].Stock != stock {
				t.Fatal("director selection spent RNG or resources")
			}
		})
	}
}

func TestSurvivalInfectionAdoptsUnchangedHostAndRefusesCapacity(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "adopt", true: "capacity"}[refuse], func(t *testing.T) {
			s, actor, victim := infectionSession(t, gameplay.Modern)
			// A remaining survivor gives the adopted host an ordinary combat patrol.
			h, err := s.Units.Create(victim.Def, 0, numeric.FixedFromInt(256), 0, victim.Z)
			if err != nil {
				t.Fatal(err)
			}
			s.CompleteUnit(h)
			victim.Health = 63
			if refuse {
				victim.Def.Limit, victim.Def.LimitEnabled = 0, true
			}
			node := orders.NewNodeForOrder(orders.Lookup("Capture"), victim.Handle, victim.X, victim.Y, victim.Z, 10, actor.Handle, false)
			stocks := [2][2]float32{s.Econ.Players[0].Stock, s.Econ.Players[1].Stock}
			ok := s.orderBinding().Work.Capture(actor, &node, 10)
			if ok == refuse {
				t.Fatalf("transfer=%v, refusal=%v", ok, refuse)
			}
			if stocks != [2][2]float32{s.Econ.Players[0].Stock, s.Econ.Players[1].Stock} {
				t.Fatal("capture debited resources")
			}
			if refuse {
				if victim.Dying || victim.Owner != 0 || len(s.Survival.units) != 0 {
					t.Fatal("refused capture changed victim or director")
				}
				return
			}
			if len(s.Survival.units) != 1 {
				t.Fatal("captured host not adopted exactly once")
			}
			repl := s.Units.Unit(s.Survival.units[0].h)
			if repl == nil || repl.Handle == victim.Handle || repl.Owner != actor.Owner || repl.Def != victim.Def || repl.Health != 63 || repl.Def.Weapon1Def.ID != 7 {
				t.Fatal("capture changed host definition, weapon or health")
			}
			if !victim.Dying || victim.Owner != 0 {
				t.Fatal("capture mutated old record instead of replacement")
			}
			if s.orderBinding().Rules.Infection(repl.Def).DurationTicks != 0 {
				t.Fatal("ordinary captured host became contagious")
			}
			if !orders.QueueForUnit(repl).HasIssuedWork() {
				t.Fatal("captured host left idle")
			}
			if !reflect.DeepEqual(s.Survival.waveUnits, []pool.Handle{repl.Handle}) || s.survivalWaveDead() {
				t.Fatal("captured host did not keep active wave alive")
			}
			s.survivalAdoptCaptured(actor, repl, 11)
			if len(s.Survival.units) != 1 || len(s.Survival.waveUnits) != 1 {
				t.Fatal("duplicate adoption")
			}
		})
	}
}

func TestInfectionNanoTagDoesNotChangeParticlesOrRandomness(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		a, actor, _ := infectionSession(t, mode)
		b, _, _ := infectionSession(t, mode)
		e := buildSegmentEvent()
		e.Source, e.Team, e.Mode, e.NanolatheBoxAtSource = actor.Handle, 1, uint8(frame.NanolatheCapture), true
		a.appendStripNanoForEvent(e)
		// Same emitter geometry but no source: only the original colour tag differs.
		e.Source = 0
		b.appendStripNanoForEvent(e)
		aa, bb := a.strips.strips[6], b.strips.strips[6]
		if len(aa) != 1 || len(bb) != 1 {
			t.Fatal("missing emitter")
		}
		if aa[0].nanoInfected != (mode == gameplay.Modern) || bb[0].nanoInfected {
			t.Fatal("infection colour escaped policy/source gate")
		}
		if !reflect.DeepEqual(aa[0].particles, bb[0].particles) || *a.CrtRNG() != *b.CrtRNG() || *a.SimRNG() != *b.SimRNG() {
			t.Fatal("colour changed particle or random state")
		}
		for _, v := range a.appendStripViews(0, nil) {
			if v.Family == frame.StripFamilyNano && v.NanoInfected != (mode == gameplay.Modern) {
				t.Fatal("publication dropped infection colour")
			}
		}
	}
}

func TestSurvivalInfectionAdoptsReusedAttackerSlot(t *testing.T) {
	s, actor, victim := infectionSession(t, gameplay.Modern)
	dead, err := s.Units.Create(victim.Def, 1, actor.X, 0, actor.Z)
	if err != nil {
		t.Fatal(err)
	}
	spare, err := s.Units.Create(victim.Def, 0, victim.X+128<<16, 0, victim.Z)
	if err != nil {
		t.Fatal(err)
	}
	s.CompleteUnit(spare)
	s.Survival.units = []survivalUnit{{h: dead, wave: 2}}
	s.Units.Destroy(dead, units.DeathKilled)
	s.Units.FinalizeDeath(dead, 5)
	if s.Units.Unit(dead) != nil && s.Units.Unit(dead).Alive {
		t.Fatal("dead slot not freed")
	}
	node := orders.NewNodeForOrder(orders.Lookup("Capture"), victim.Handle, victim.X, victim.Y, victim.Z, 10, actor.Handle, false)
	if !s.orderBinding().Work.Capture(actor, &node, 10) {
		t.Fatal("capture refused")
	}
	host := s.Units.Unit(dead)
	if host == nil || !host.Alive || host.Owner != actor.Owner {
		t.Fatal("allocator did not reuse expected dead attacker slot")
	}
	q := orders.QueueForUnit(host)
	if q == nil || !q.HasIssuedWork() {
		t.Fatalf("reused captured host %d not assigned director work; waveUnits=%v", dead, s.Survival.waveUnits)
	}
}
