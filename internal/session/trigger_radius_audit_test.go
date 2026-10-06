package session

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/triggers"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func radiusAuditSession(t *testing.T) *Session {
	t.Helper()
	s := newLoopTestSession(t, 0)
	// Production binds this world in the unit phase before the phase-5 poll.
	s.Movement.BindWorld(s.Units)
	for i := range s.World.Plot {
		s.World.Plot[i].SetHeight(0)
		s.World.Plot[i].SetMinHeight(0)
		s.World.Plot[i].SetMaxHeight(0)
	}
	return s
}

func radiusAuditUnit(t *testing.T, s *Session, def *content.UnitDef, owner uint8, x, z int32) *units.Unit {
	t.Helper()
	def.Script = fixtureCOBProgram()
	h, err := s.Units.Create(def, owner, numeric.FixedFromInt(int64(x)), 0, numeric.FixedFromInt(int64(z)))
	if err != nil {
		t.Fatal(err)
	}
	u := s.Units.Unit(h)
	s.Movement.EnsureUnit(u)
	return u
}

// Authored radii cross the stored-word boundary before sector clamping. The
// production adapter must retain the query's candidate set [08 R-TRIG-01 §5].
func TestMissionRadiusUsesSpatialBounds(t *testing.T) {
	for _, tc := range []struct {
		name, typ            string
		cx, cz, radius, x, z int32
		want                 bool
	}{
		{"inside", "AIR", 100, 200, 10, 105, 200, true},
		{"outside", "AIR", 100, 200, 4, 105, 200, false},
		{"wrong type", "OTHER", 100, 200, 10, 105, 200, false},
		{"negative stored radius", "ANYTYPE", 64, 64, 32768, 64, 64, true},
		{"wrapped upper bounds", "ANYTYPE", 64, 64, 32767, 256, 256, false},
		{"off map bucket", "ANYTYPE", 64, 64, 200, -16, 64, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := radiusAuditSession(t)
			def := &content.UnitDef{UnitName: "AIR", BMCode: 1, CanFly: true, MaxDamage: 100, FootprintX: 1, FootprintZ: 1}
			radiusAuditUnit(t, s, def, 0, tc.x, tc.z)
			tr, ok := triggers.ParseCondition("MoveUnitToRadius", fmt.Sprintf("%s,%d,%d,%d", tc.typ, tc.cx, tc.cz, tc.radius))
			if !ok {
				t.Fatal("authored condition rejected")
			}
			simBefore, crtBefore := rng.Global.Sim.Draws(), rng.Global.Crt.Draws()
			if got := tr.Poll(s.missionTriggerContext(0)); got != tc.want {
				t.Fatalf("radius completion=%v, want %v", got, tc.want)
			}
			if tr.CenterX != tc.cx<<16 || tr.CenterZ != tc.cz<<16 {
				t.Fatalf("flat-map centre=%d,%d", tr.CenterX, tr.CenterZ)
			}
			if rng.Global.Sim.Draws() != simBefore || rng.Global.Crt.Draws() != crtBefore {
				t.Fatal("radius poll consumed RNG")
			}
		})
	}
}

// The airbase bit is authored; attachment uses the real commit. Radius scans
// exclude parked aircraft, while the owner-slice annihilation predicate still
// counts eligible cargo [08 R-TRIG-01 §3, §5].
func TestMissionRadiusExcludesAttachedAircraftUntilDetach(t *testing.T) {
	s := radiusAuditSession(t)
	base := radiusAuditUnit(t, s, &content.UnitDef{UnitName: "BASE", IsAirBase: true, MaxDamage: 100, FootprintX: 1, FootprintZ: 1}, 1, 80, 80)
	air := radiusAuditUnit(t, s, &content.UnitDef{UnitName: "AIR", BMCode: 1, CanFly: true, MaxDamage: 100, FootprintX: 1, FootprintZ: 1}, 0, 80, 80)
	if !movement.AttachCargo(s.Units, base.Handle, air.Handle, -1) {
		t.Fatal("attach failed")
	}
	s.Movement.SyncCarriedMotion(s.Units)
	c := s.missionTriggerContext(0)
	tr := triggers.New(triggers.KindMoveUnitToRadius, "AIR", 80, 80, 20)
	if tr.Poll(c) {
		t.Fatal("parked aircraft satisfied radius condition")
	}
	if triggers.New(triggers.KindAllUnitsKilled, "").Poll(c) {
		t.Fatal("airbase cargo disappeared from annihilation predicate")
	}
	if tr.Celebrated {
		t.Fatal("excluded cargo celebrated")
	}
	if _, ok := movement.DetachTakeoff(s.Units, air.Handle); !ok {
		t.Fatal("detach failed")
	}
	if !tr.Poll(c) {
		t.Fatal("detached aircraft did not satisfy radius condition")
	}
	if !tr.Celebrated {
		t.Fatal("detached aircraft completion did not celebrate")
	}
}

func TestMissionRadiusContinuesQueryAfterCompletion(t *testing.T) {
	s := radiusAuditSession(t)
	def := &content.UnitDef{UnitName: "AIR", BMCode: 1, CanFly: true, MaxDamage: 100, FootprintX: 1, FootprintZ: 1}
	radiusAuditUnit(t, s, def, 0, 64, 64)
	radiusAuditUnit(t, s, def, 0, 96, 96)
	c := s.missionTriggerContext(0)
	query := c.VisitRadiusUnits
	visits, cues := 0, 0
	c.VisitRadiusUnits = func(x, z, r int32, visit func(*units.Unit)) {
		query(x, z, r, func(u *units.Unit) { visits++; visit(u) })
	}
	c.Celebrate = func() { cues++ }
	tr := triggers.New(triggers.KindMoveUnitToRadius, "AIR", 80, 80, 40)
	for i := 0; i < 2; i++ {
		if !tr.Poll(c) {
			t.Fatal("matching query did not complete")
		}
	}
	if visits != 4 || cues != 1 {
		t.Fatalf("visits/cues=%d/%d, want4/1", visits, cues)
	}
}
