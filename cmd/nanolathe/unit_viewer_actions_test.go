package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Independently authored instructions beyond the animation fixtures' set.
const (
	viewerPopLocal = 0x10023002
	viewerAdd      = 0x10031000
	viewerMul      = 0x10033000
	viewerSet      = 0x10082000
)

// viewerTrace appends digit k to static 0, so a fixture's callbacks record
// the order in which they actually ran.
func viewerTrace(k uint32, then ...uint32) []uint32 {
	code := []uint32{viewerStatic, 0, viewerPush, 10, viewerMul, viewerPush, k, viewerAdd, viewerPopStatic, 0}
	return append(append(code, then...), viewerPush, 0, viewerReturn)
}

// viewerReport answers static 0 through a synchronous query's first cell.
var viewerReport = viewerScriptFixture{"Report", []uint32{viewerStatic, 0, viewerPopLocal, 0, viewerPush, 0, viewerReturn}}

func viewerTraced(a *unitViewerAnimation) int32 {
	return a.script.bridge.Query("Report", [4]int32{}).QueryValue()
}

func TestUnitViewerCompletedUnitCallbackOrder(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", viewerTrace(1)},
		viewerScriptFixture{"QueryPrimary", viewerTrace(2)},
		// AimFrom leaves its -1 seed, so the slot falls back to Query*.
		viewerScriptFixture{"AimFromPrimary", viewerTrace(3)},
		viewerScriptFixture{"SetMaxReloadTime", viewerTrace(4)},
		viewerScriptFixture{"SetSpeed", viewerTrace(5, viewerStatic, 1, viewerLocal, 0, viewerAdd, viewerPopStatic, 1)},
		viewerScriptFixture{"Activate", viewerTrace(6)},
		viewerScriptFixture{"SetDirection", viewerTrace(7, viewerLocal, 0, viewerMoveNow, 2, 0)},
		viewerReport,
		viewerScriptFixture{"ReportArgs", []uint32{viewerStatic, 1, viewerPopLocal, 0, viewerPush, 0, viewerReturn}},
	)
	def.Script.Statics = 2
	def.ExtractsMetal, def.WindGenerator, def.ActivateWhenBuilt = 0.001, 30, true
	def.FootprintX, def.FootprintZ = 3, 3
	a := unitViewerPlay(def, mdl, unitViewerIdle, 1)
	// Create and the synchronous slot queries run at creation; the deferred
	// reload, extractor and activation starts wait for the first drain
	// [04 R-CB-01 §4].
	if got := viewerTraced(a); got != 1232 || !a.activated() {
		t.Fatalf("creation ran %d (activated %t), want Create, Query, AimFrom, Query fallback", got, a.activated())
	}
	unitViewerAdvance(a, 1)
	// The wind pair is the first tick's general-update producer, after the
	// creation starts in slot order [04 R-CB-01 §5].
	if got := viewerTraced(a); got != 123245675 {
		t.Fatalf("completed-unit order = %d, want 123245675", got)
	}
	// Extractor: 9 cells of (127 + 1); wind: 1050 << 4.
	if got := a.script.bridge.Query("ReportArgs", [4]int32{}).QueryValue(); got != 9*128+1050*16 || a.poses()[2].Tx != 8192 {
		t.Fatalf("SetSpeed sum %d / SetDirection %d do not carry the preview inputs", got, a.poses()[2].Tx)
	}
	unitViewerAdvance(a, 30)
	if got := viewerTraced(a); got != 123245675 {
		t.Fatalf("a creation-time or per-change callback repeated: %d", got)
	}
}

func TestUnitViewerMobileBuildWaitsForStanceAndQueriesEachStep(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		// Record the bearing argument, then raise the stance after a sleep.
		viewerScriptFixture{"StartBuilding", []uint32{
			viewerLocal, 0, viewerMoveNow, 0, 0, viewerPush, 100, viewerSleep,
			viewerPush, 5, viewerPush, 1, viewerSet, viewerPush, 0, viewerReturn,
		}},
		viewerScriptFixture{"QueryNanoPiece", viewerTrace(1, viewerPush, 2, viewerPopLocal, 0)},
		viewerScriptFixture{"StopBuilding", viewerTrace(9)},
		viewerReport,
	)
	def.Builder, def.BMCode = true, 1
	a := unitViewerPlay(def, mdl, unitViewerBuilding, 1)
	var queries int32
	for range 12 {
		unitViewerAdvance(a, 1)
		if !a.building() {
			if viewerTraced(a) != 0 || a.nanoPiece() != -1 {
				t.Fatal("nano query ran before the build stance [05 R-P0-06 §1]")
			}
			continue
		}
		queries = queries*10 + 1
		if viewerTraced(a) != queries || a.nanoPiece() != 2 || a.padPiece() != -1 {
			t.Fatalf("work step did not query the nano piece exactly once: %d, nano %d", viewerTraced(a), a.nanoPiece())
		}
	}
	if queries == 0 || a.poses()[0].Tx != numeric.Fixed(unitViewerBuildHeading) {
		t.Fatalf("stance never reached or bearing %d not carried as StartBuilding's first argument", a.poses()[0].Tx)
	}
	a.toggleBuild()
	unitViewerAdvance(a, 2)
	if a.building() || a.nanoPiece() != -1 || viewerTraced(a) != queries*10+9 {
		t.Fatalf("stop did not end work with StopBuilding: %d", viewerTraced(a))
	}
}

func TestUnitViewerFactoryBuildEdgeOrder(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		viewerScriptFixture{"Activate", viewerTrace(1, viewerPush, 5, viewerPush, 1, viewerSet)},
		viewerScriptFixture{"QueryBuildInfo", viewerTrace(2, viewerPush, 1, viewerPopLocal, 0)},
		viewerScriptFixture{"StartBuilding", viewerTrace(3)},
		viewerScriptFixture{"QueryNanoPiece", viewerTrace(4, viewerPush, 2, viewerPopLocal, 0)},
		viewerScriptFixture{"StopBuilding", viewerTrace(5)},
		viewerScriptFixture{"Deactivate", viewerTrace(6)},
		viewerReport,
	)
	def.Builder, def.BMCode = true, 0
	a := unitViewerPlay(def, mdl, unitViewerBuilding, 1)
	unitViewerAdvance(a, 3)
	// Activation opens the factory; once the stance is set the pad query and
	// the building edge precede the first work step's nano query
	// [05 "Factory production lifecycle"].
	if got := viewerTraced(a); got != 12434 || !a.building() || a.padPiece() != 1 || a.nanoPiece() != 2 {
		t.Fatalf("factory sequence %d, pad %d, nano %d", got, a.padPiece(), a.nanoPiece())
	}
	a.toggleBuild()
	unitViewerAdvance(a, 2)
	if got := viewerTraced(a); got != 1243456 || a.building() || a.activated() {
		t.Fatalf("factory stop %d, want the building edge then activation lowered", got)
	}
}

func TestUnitViewerHitArguments(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		viewerScriptFixture{"HitByWeapon", []uint32{viewerLocal, 0, viewerMoveNow, 0, 0, viewerLocal, 1, viewerMoveNow, 1, 0, viewerPush, 0, viewerReturn}},
		viewerScriptFixture{"TakeDamage", []uint32{viewerLocal, 0, viewerMoveNow, 2, 0, viewerPush, 4, viewerRead, viewerMoveNow, 2, 1, viewerPush, 0, viewerReturn}},
	)
	def.MaxDamage = 100
	a := unitViewerPlay(def, mdl, unitViewerHit, 1)
	unitViewerAdvance(a, 1)
	p := a.poses()
	// A hit from straight ahead is direction byte 0x80: cos and sin at radius
	// 400 through the shared table are -400 and 0 [04 R-CB-01 §2]. TakeDamage
	// carries the clamped post-hit percentage and the health port agrees.
	x, z := cob.HitByWeaponArgs(unitViewerHitDirection)
	if x != -400 || z != 0 || p[0].Tx != numeric.Fixed(x) || p[1].Tx != numeric.Fixed(z) || p[2].Tx != 50 || p[2].Ty != 50 {
		t.Fatalf("hit arguments (%d, %d), TakeDamage %d, health port %d", p[0].Tx, p[1].Tx, p[2].Tx, p[2].Ty)
	}
}

func TestUnitViewerKilledSeverityAndCorpseChain(t *testing.T) {
	heap := &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "heap"}, Object: "heap", Metal: 7}
	wreck := &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "wreck"}, Object: "wreck", Metal: 15, FeatureDead: "heap", FeatureDeadDef: heap}
	features := map[string]*content.FeatureDef{"wreck": wreck, "heap": heap}
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		// Record the severity cell, explode-free, and choose depth 2.
		viewerScriptFixture{"Killed", []uint32{viewerLocal, 0, viewerMoveNow, 0, 0, viewerPush, 2, viewerPopLocal, 1, viewerHide, 1, viewerPush, 0, viewerReturn}},
	)
	def.Corpse = "Wreck"
	for i, severity := range unitViewerSeverities {
		a := newUnitViewerAnimation(def, mdl, unitViewerAnimationOptions{action: unitViewerDeath, severity: severity, features: features})
		if a.death.done {
			t.Fatal("death ran before the first preview tick")
		}
		unitViewerAdvance(a, 1)
		p := a.poses()
		// Cell 0 carries the preview severity into the local query
		// [04 R-CB-01 §7]; the second cell is the corpse depth.
		if !a.death.done || !a.stopped || p[0].Tx != numeric.Fixed(severity) || !p[1].Hidden || a.death.depth != 2 || a.death.corpse != heap {
			t.Fatalf("severity %d (option %d): seeded %d, depth %d, corpse %v", severity, i, p[0].Tx, a.death.depth, a.death.corpse)
		}
		if a.update(1.0/unitViewerTickRate) || a.wreckFeature() != nil {
			t.Fatal("the script ran after death, or Death drew a wreck")
		}
	}
	w := newUnitViewerAnimation(def, mdl, unitViewerAnimationOptions{action: unitViewerWreck, features: features})
	if w.wreckFeature() != heap {
		t.Fatal("Wreck did not show the corpse its own Death selects")
	}
	// No Killed body: the query writes nothing and the battle's sanctioned
	// substitute depth 1 selects the authored corpse [04 R-CB-01 §9].
	delete(def.Script.Scripts, "Killed")
	w = newUnitViewerAnimation(def, mdl, unitViewerAnimationOptions{action: unitViewerWreck, features: features})
	if w.death.queried || w.death.depth != 1 || w.wreckFeature() != wreck {
		t.Fatalf("absent Killed gave depth %d, corpse %v", w.death.depth, w.wreckFeature())
	}
}

func TestUnitViewerMoveTiersFollowTheSpeedRamp(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		viewerScriptFixture{"StartMoving", viewerTrace(1)},
		viewerScriptFixture{"MoveRate1", viewerTrace(2)},
		viewerScriptFixture{"MoveRate2", viewerTrace(3)},
		viewerScriptFixture{"MoveRate3", viewerTrace(4)},
		viewerReport,
	)
	def.BMCode, def.MaxVelocity, def.Acceleration, def.MoveRate1, def.MoveRate2 = 1, 4<<16, 1<<16, 2<<16, 3<<16
	a := unitViewerPlay(def, mdl, unitViewerMoving, 1)
	// Speeds 1, 2, 3, 4 classify inclusively as tiers 1, 1, 2, 3 [04 §5.2].
	for i, want := range []int32{12, 12, 123, 1234, 1234} {
		unitViewerAdvance(a, 1)
		if got := viewerTraced(a); got != want {
			t.Fatalf("tick %d: tier callbacks %d, want %d", i+1, got, want)
		}
	}
}

func TestUnitViewerFlightTakeoffAndLanding(t *testing.T) {
	def, mdl := unitViewerAnimationFixture(
		viewerScriptFixture{"Create", []uint32{viewerReturn}},
		viewerScriptFixture{"Activate", viewerTrace(1)},
		viewerScriptFixture{"StartMoving", viewerTrace(2)},
		viewerScriptFixture{"MoveRate1", viewerTrace(3)},
		viewerScriptFixture{"Deactivate", viewerTrace(4)},
		viewerScriptFixture{"StopMoving", viewerTrace(5)},
		viewerReport,
	)
	def.CanFly, def.BMCode, def.CruiseAlt, def.TurnRate = true, 1, 20, 400
	def.MaxVelocity, def.Acceleration, def.BrakeRate = 4<<16, 1<<15, 8<<16
	def.MoveRate1, def.MoveRate2 = def.MaxVelocity<<1, def.MaxVelocity<<1
	a := unitViewerPlay(def, mdl, unitViewerFlying, 1)
	unitViewerAdvance(a, 1)
	// The takeoff hook is queued by the order visit; the climb's first speed
	// change wakes StartMoving then MoveRate1, whose barrier runs it first
	// [04 R-AIR-01 §6][04 §5.2].
	if got := viewerTraced(a); got != 123 || !a.activated() || !a.airborne() {
		t.Fatalf("takeoff order %d", got)
	}
	for range 600 {
		if a.fly.frozen {
			break
		}
		unitViewerAdvance(a, 1)
	}
	if !a.fly.frozen || viewerTraced(a) != 123 {
		t.Fatalf("cruise did not hold with one tier: frozen %t, %d", a.fly.frozen, viewerTraced(a))
	}
	a.toggleFlight()
	for range 600 {
		if a.fly.order == unitViewerFlightGrounded {
			break
		}
		unitViewerAdvance(a, 1)
	}
	// The landing hook precedes touchdown, where grounding zeroes the speed
	// and the classifier stops movement [04 R-AIR-01 §6].
	if got := viewerTraced(a); got != 12345 || a.activated() || a.airborne() || a.stopped {
		t.Fatalf("landing order %d (%s)", got, a.note)
	}
}

func TestUnitViewerActionRowFollowsTheUnit(t *testing.T) {
	s := unitViewerUIFixture()
	script := func(names ...string) *cob.Program {
		p := &cob.Program{Scripts: map[string]int{}}
		for _, n := range names {
			p.Scripts[n] = 0
		}
		return p
	}
	shown := func(name string) bool { return s.panel.ActiveAt(s.panel.Index(name)) }
	grey := func(name string) bool { return s.panel.Window.Gadgets[s.panel.Index(name)].GrayedOut != 0 }

	// A ground builder with an activation script and no hit callbacks.
	s.selected.Script, s.selected.BMCode, s.selected.Builder = script("StartMoving", "StartBuilding", "Activate", "Killed"), 1, true
	s.refreshControls()
	if !shown("MOVE") || s.panel.TextAt(s.panel.Index("MOVE")) != "Move" || !shown("BUILD") || grey("BUILD") ||
		!shown("POWER") || !shown("HIT") || !grey("HIT") || shown("AIM") || !grey("WRECK") {
		t.Fatal("ground builder row shows the wrong actions")
	}
	// An aircraft flies instead of moving; without Activate there is no
	// toggle, and a non-builder has no Build.
	s.selected.Script, s.selected.CanFly, s.selected.Builder, s.selected.MaxVelocity = script("HitByWeapon"), true, false, 1<<16
	s.selected.Corpse = "wreck"
	s.refreshControls()
	if s.panel.TextAt(s.panel.Index("MOVE")) != "Fly" || grey("MOVE") || shown("BUILD") || shown("POWER") || grey("HIT") || grey("WRECK") {
		t.Fatal("aircraft row shows the wrong actions")
	}
	// Visible buttons sit inside the stage row without overlapping.
	last := 0.0
	for _, b := range unitViewerActionButtons {
		if i := s.panel.Index(b.name); s.panel.ActiveAt(i) {
			r := s.panel.Window.Gadgets[i].Rect
			if float64(r.X) < last || r.X+r.W > unitViewerRowLeft+unitViewerRowWidth {
				t.Fatalf("%s overlaps or leaves the action row: %+v", b.name, r)
			}
			last = float64(r.X + r.W)
		}
	}
}
