package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// shoreTerrain is land on cells x < 16 and deep water beyond: the fixture
// class (testmove, MaxWaterDepth 10) labels only the land as a region.
func shoreTerrain() *world.Terrain {
	attrs := make([]formats.TNTAttribute, 32*32)
	for i := range attrs {
		h := uint8(120)
		if i%32 >= 16 {
			h = 0
		}
		attrs[i] = formats.TNTAttribute{Height: h, Feature: world.PlotFeatureNone}
	}
	ter := &world.Terrain{CellW: 32, CellH: 32, Plot: world.ExpandPlot(attrs, 32, 32), Version: 0x2000, SeaLevel: 100, WindMin: 100, WindMax: 2000}
	_ = ter.ApplySchema(nil, 0)
	return ter
}

func placeSurvivor(t *testing.T, s *Session, d *content.UnitDef, x int64) *units.Unit {
	t.Helper()
	h, err := s.Units.Create(d, 0, numeric.FixedFromInt(x), 0, numeric.FixedFromInt(128))
	if err != nil {
		t.Fatal(err)
	}
	s.CompleteUnit(h)
	return s.Units.Unit(h)
}

// A ground infector hunts only victims on its own static region: a nearer
// ship offshore must not pin it to the shore while a reachable mobile exists,
// and alone it leaves the infector its ordinary patrol (DESIGN_SURVIVAL
// "Modern infection hunters").
func TestSurvivalInfectionHuntSkipsWaterOnlyTargets(t *testing.T) {
	s, actor, victim := infectionSession(t, gameplay.Modern)
	s.World = shoreTerrain()
	victim.X = numeric.FixedFromInt(320) // deep water, 192 units away
	if got := s.survivalInfectionTarget(actor, nil, 20); got != nil {
		t.Fatalf("hunted an offshore ship: %v", got.Handle)
	}
	far := placeSurvivor(t, s, victim.Def, 16) // land, 112 units away
	if got := s.survivalInfectionTarget(actor, nil, 20); got != far {
		t.Fatal("nearer offshore ship hid a reachable land victim")
	}
	far.X = numeric.FixedFromInt(400)
	su := survivalUnit{h: actor.Handle}
	sim, crt := *s.SimRNG(), *s.CrtRNG()
	s.survivalSend(&su, actor, 20)
	if su.infecting || *s.SimRNG() != sim || *s.CrtRNG() != crt {
		t.Fatal("water-only victims started a hunt or spent randomness")
	}
	// One cell past the region's last cell is tolerated, two are not.
	c, home := s.survivalHuntRegion(actor)
	edge := int64(-1)
	for x := int32(0); x < 32; x++ {
		if c.regions.At(x, 8) == home {
			edge = int64(x)
		}
	}
	victim.X = numeric.FixedFromInt((edge+1)*16 + 8)
	if edge < 0 || s.survivalInfectionTarget(actor, nil, 20) != victim {
		t.Fatal("victim beside the region's edge rejected")
	}
	victim.X = numeric.FixedFromInt((edge+2)*16 + 8)
	if s.survivalInfectionTarget(actor, nil, 20) != nil {
		t.Fatal("victim two cells offshore admitted")
	}
}

// An attempt that ends with its victim still eligible shuns that victim until
// it leaves its cell or the shun lapses, so the director does not re-issue the
// same failing hunt each sweep. No draws are spent.
func TestSurvivalInfectionShunsAbandonedVictim(t *testing.T) {
	s, actor, victim := infectionSession(t, gameplay.Modern)
	other := placeSurvivor(t, s, victim.Def, 320)
	s.Survival.units = []survivalUnit{{h: actor.Handle, target: victim.Handle, infecting: true}}
	s.Survival.tuning.RetargetEvery = 90
	sim, crt := *s.SimRNG(), *s.CrtRNG()
	// The queue holds no Capture: the attempt ended without a transfer.
	s.survivalRetarget(100)
	su := s.Survival.units[0]
	if su.shun != victim.Handle || su.shunUntil != 100+survivalShunTicks || su.target != other.Handle || !su.infecting {
		t.Fatalf("abandoned victim not shunned: %+v", su)
	}
	if head := orders.QueueForUnit(actor).Head(); head == nil || head.ID != orders.Lookup("Capture") || head.Target != other.Handle {
		t.Fatal("hunter not sent to the next victim")
	}
	if *s.SimRNG() != sim || *s.CrtRNG() != crt {
		t.Fatal("shun spent randomness")
	}
	if s.survivalInfectionTarget(actor, &su, 101) != other {
		t.Fatal("shunned victim selected again")
	}
	if s.survivalInfectionTarget(actor, &su, 100+survivalShunTicks) != victim {
		t.Fatal("shun outlived its interval")
	}
	victim.X += numeric.FixedFromInt(16)
	if s.survivalInfectionTarget(actor, &su, 101) != victim {
		t.Fatal("shun outlived the victim's cell")
	}
	// A captured victim is not eligible, so a completed attempt shuns nothing.
	victim.X -= numeric.FixedFromInt(16)
	victim.Dying = true
	s.Survival.units = []survivalUnit{{h: actor.Handle, target: victim.Handle, infecting: true}}
	orders.QueueForUnit(actor).PurgeUnprotected()
	s.survivalRetarget(200)
	if s.Survival.units[0].shun != 0 {
		t.Fatal("completed capture recorded a shun")
	}
}

// A record left by a Modern attempt is inert after a rebind to Strict 3.1 or
// Community: no shun, no hunt, the ordinary patrol, and no draws.
func TestSurvivalInfectionShunBypassedOutsideModern(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39} {
		t.Run(string(mode), func(t *testing.T) {
			s, actor, victim := infectionSession(t, gameplay.Modern)
			s.SetGameplay(mode)
			s.Survival.units = []survivalUnit{{h: actor.Handle, target: victim.Handle, infecting: true}}
			sim, crt := *s.SimRNG(), *s.CrtRNG()
			s.survivalRetarget(100)
			su := s.Survival.units[0]
			if su.shun != 0 || su.infecting || su.target != victim.Handle || *s.SimRNG() != sim || *s.CrtRNG() != crt {
				t.Fatalf("%s honored Modern hunt state: %+v", mode, su)
			}
			if head := orders.QueueForUnit(actor).Head(); head == nil || head.ID == orders.Lookup("Capture") {
				t.Fatal("ordinary patrol not issued")
			}
		})
	}
}

// At its unit limit the attacker cannot receive a transfer, so infectors fight
// with their weapons instead of spraying victims they cannot take.
func TestSurvivalInfectionHuntPausesAtUnitLimit(t *testing.T) {
	s, actor, _ := infectionSession(t, gameplay.Modern)
	if s.survivalInfectionTarget(actor, nil, 20) == nil {
		t.Fatal("fixture has no victim")
	}
	for {
		if _, err := s.Units.Create(actor.Def, actor.Owner, actor.X, 0, actor.Z); err != nil {
			break
		}
	}
	if s.survivalInfectionTarget(actor, nil, 20) != nil {
		t.Fatal("hunted at the attacker's unit limit")
	}
}
