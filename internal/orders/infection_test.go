package orders

import (
	"math"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func infectionFixture() (*Queue, *units.Unit, *units.Unit) {
	q, u, target := workFixture()
	q.Binding().Rules = &ModernRules{}
	u.Def.Builder, u.Def.CanCapture, u.Def.CanMove, u.Def.CanAttack = false, false, true, true
	u.Def.Category = "KBOT\tNaNoLaThE_InFeCtOr\nWEAPON"
	u.Def.NanolatheInfector = content.CategoryHasToken(u.Def.Category, content.NanolatheInfectorCategory)
	u.Flags |= units.ArmedStatus
	target.Def.BMCode, target.Def.CanMove, target.Def.Builder = 1, true, false
	target.Owner = 1
	target.AllocationSerial = 1<<40 | 7
	return q, u, target
}

func issueInfection(q *Queue, u, target *units.Unit) {
	id := Resolve(13, u, target, nil)
	q.Push(id, NewNodeForOrder(id, target.Handle, target.X, target.Y, target.Z, 1, u.Handle, false))
}

func TestModernInfectionAdmissionAndAttackFallback(t *testing.T) {
	q, u, target := infectionFixture()
	if policy := q.Binding().rules().Infection(u.Def); policy.DurationTicks != 60 || policy.Range != numeric.FixedFromInt(96) {
		t.Fatalf("prototype policy = %+v", policy)
	}
	for _, code := range []int{3, 13} {
		if got := Resolve(code, u, target, nil); got != Lookup("Capture") {
			t.Fatalf("command %d = %d, want Capture", code, got)
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Queue, *units.Unit, *units.Unit)
	}{
		{"commander", func(_ *Queue, _, v *units.Unit) { v.Def.Commander = true }},
		{"builder", func(_ *Queue, _, v *units.Unit) { v.Def.Builder = true }},
		{"air builder", func(_ *Queue, _, v *units.Unit) { v.Def.Builder, v.Def.CanFly = true, true }},
		{"building", func(_ *Queue, _, v *units.Unit) { v.Def.BMCode = 0 }},
		{"immobile", func(_ *Queue, _, v *units.Unit) { v.Def.CanMove = false }},
		{"nanoframe", func(_ *Queue, _, v *units.Unit) { v.Remaining = .5 }},
		{"dead", func(_ *Queue, _, v *units.Unit) { v.Alive = false }},
		{"dying", func(_ *Queue, _, v *units.Unit) { v.Dying = true }},
		{"carried", func(_ *Queue, _, v *units.Unit) { v.Attachment.Carrier = 9 }},
		{"ally", func(q *Queue, _, _ *units.Unit) {
			q.Binding().SetHostility(func(*units.Unit, *units.Unit) bool { return false })
		}},
		{"actor dying", func(_ *Queue, u, _ *units.Unit) { u.Dying = true }},
		{"actor unfinished", func(_ *Queue, u, _ *units.Unit) { u.Remaining = .5 }},
		{"uncompiled category", func(_ *Queue, u, _ *units.Unit) { u.Def.NanolatheInfector = false }},
		{"strict", func(q *Queue, _, _ *units.Unit) { q.Binding().Rules = StrictRules{} }},
		{"community", func(q *Queue, _, _ *units.Unit) { q.Binding().Rules = CommunityRules{} }},
		{"unbound", func(q *Queue, _, _ *units.Unit) { q.Binding().Rules = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q, u, v := infectionFixture()
			tc.mutate(q, u, v)
			if InfectionTarget(u, v) || Resolve(13, u, v, nil) != 0 {
				t.Fatal("ineligible infection admitted")
			}
		})
	}
	// Existing capture capability does not grant immunity to this policy.
	target.Def.CanCapture = true
	if !InfectionTarget(u, target) {
		t.Fatal("noncommander capturable mobile rejected")
	}
	target.Def.Commander = true
	if Resolve(3, u, target, nil) == Lookup("Capture") {
		t.Fatal("commander attack did not fall back")
	}
}

func TestModernInfectionThreeDimensionalReach(t *testing.T) {
	_, u, target := infectionFixture()
	u.X, u.Y, u.Z = 0, 0, 0
	target.X, target.Y, target.Z = 0, numeric.FixedFromInt(96), 0
	target.Def.CanFly = true
	if !InfectionInRange(u, target) {
		t.Fatal("inclusive vertical boundary refused")
	}
	target.Y++
	if InfectionInRange(u, target) {
		t.Fatal("aircraft above reach admitted")
	}
	target.X, target.Y = numeric.FixedFromInt(70), numeric.FixedFromInt(70)
	if InfectionInRange(u, target) {
		t.Fatal("diagonal outside sphere admitted")
	}
	target.X, target.Y, target.Z = numeric.FixedFromInt(32), numeric.FixedFromInt(64), numeric.FixedFromInt(64)
	if !InfectionInRange(u, target) {
		t.Fatal("three-dimensional sphere boundary refused")
	}
	target.Z++
	if InfectionInRange(u, target) {
		t.Fatal("fraction outside sphere admitted")
	}
	u.X, target.X = numeric.Fixed(math.MinInt64), numeric.Fixed(math.MaxInt64)
	if InfectionInRange(u, target) {
		t.Fatal("overflowing coordinate difference admitted")
	}
}

func TestModernInfectionRequiresUninterruptedSpray(t *testing.T) {
	for _, reset := range []bool{false, true} {
		q, u, target := infectionFixture()
		sprays, transfers, installs := 0, 0, 0
		beforeBuckets, beforeSim := *q.Binding().Economy.UnitBuckets(u.Handle), *q.Binding().SimRNG
		var capturedAt uint32
		q.Binding().Presentation = NewPresentationAdapter(PresentationAdapterConfig{Nanolathe: func(*units.Unit, *Node, uint32) bool { sprays++; return true }})
		q.Binding().Work.SetCapture(func(*units.Unit, *Node, uint32) bool { transfers++; return true })
		q.Binding().Movement.SetInstallRectangle(func(RectangleGoalRequest) bool { installs++; return true })
		issueInfection(q, u, target)
		for tick := uint32(1); tick <= 100; tick++ {
			// Motion itself does not interrupt a spray while still in range.
			target.MoveTier = 1
			target.X = u.X + numeric.FixedFromInt(int64(tick%3))
			if reset && tick == 31 {
				target.X = u.X + numeric.FixedFromInt(200)
			}
			q.Pump(u, tick)
			if transfers != 0 {
				capturedAt = tick
				break
			}
		}
		want := uint32(62) // deferred StartBuilding, then 60 full ticks
		if reset {
			want = 93 // return to reach at 32, script visit at 33, +60
		}
		if capturedAt != want || transfers != 1 || q.LenPrimary() != 0 {
			t.Fatalf("reset=%t capture tick=%d transfers=%d queue=%d, want %d", reset, capturedAt, transfers, q.LenPrimary(), want)
		}
		if (!reset && (sprays != 30 || installs != 1)) || (reset && (sprays != 45 || installs != 3)) {
			t.Fatalf("reset=%t sprays=%d installs=%d", reset, sprays, installs)
		}
		if *q.Binding().Economy.UnitBuckets(u.Handle) != beforeBuckets || *q.Binding().SimRNG != beforeSim {
			t.Fatal("infection charged resources or drew simulation randomness")
		}
	}
}

func TestModernInfectionStanceIdentityAndTransferRefusal(t *testing.T) {
	for _, reason := range []string{"stance", "identity", "alliance", "death", "builder", "loaded", "capacity"} {
		t.Run(reason, func(t *testing.T) {
			q, u, target := infectionFixture()
			calls, sprays := 0, 0
			q.Binding().Work.SetCapture(func(*units.Unit, *Node, uint32) bool { calls++; return false })
			q.Binding().Presentation = NewPresentationAdapter(PresentationAdapterConfig{Nanolathe: func(*units.Unit, *Node, uint32) bool { sprays++; return true }})
			issueInfection(q, u, target)
			q.Pump(u, 1)
			switch reason {
			case "stance":
				u.InBuildStance = false
			case "identity":
				target.AllocationSerial++
			case "alliance":
				q.Binding().SetHostility(func(*units.Unit, *units.Unit) bool { return false })
			case "death":
				target.Dying = true
			case "builder":
				target.Def.Builder = true
			case "loaded":
				// Loading into a transport mid-attempt ends it untransferred.
				target.Attachment.Carrier = 9
			}
			for tick := uint32(2); tick <= 70; tick++ {
				q.Pump(u, tick)
			}
			wantCalls := 0
			if reason == "capacity" {
				wantCalls = 1
			}
			if calls != wantCalls || target.Owner != 1 || !target.Alive {
				t.Fatalf("calls=%d, victim owner=%d alive=%t", calls, target.Owner, target.Alive)
			}
			if reason != "capacity" && sprays != 0 {
				t.Fatal("invalid attempt emitted spray")
			}
			if reason != "stance" && q.LenPrimary() != 0 {
				t.Fatal("terminal attempt retained")
			}
		})
	}
}

func TestInfectionStrictBypassAndActiveModeSwitch(t *testing.T) {
	for _, active := range []bool{false, true} {
		q, u, target := infectionFixture()
		calls := 0
		q.Binding().Presentation = NewPresentationAdapter(PresentationAdapterConfig{Nanolathe: func(*units.Unit, *Node, uint32) bool { calls++; return true }})
		q.Binding().Work.SetCapture(func(*units.Unit, *Node, uint32) bool { calls++; return true })
		if active {
			issueInfection(q, u, target)
			q.Pump(u, 1)
			q.Pump(u, 2)
		}
		q.Binding().Rules = StrictRules{}
		beforeCalls, sim, crt := calls, *q.Binding().SimRNG, *rng.Global.Crt
		beforeBuckets := *q.Binding().Economy.UnitBuckets(u.Handle)
		if Resolve(13, u, target, nil) != 0 || Resolve(3, u, target, nil) == Lookup("Capture") {
			t.Fatal("Strict selected infection")
		}
		for tick := uint32(3); tick <= 80; tick++ {
			q.Pump(u, tick)
		}
		if calls != beforeCalls || *q.Binding().SimRNG != sim || *rng.Global.Crt != crt ||
			*q.Binding().Economy.UnitBuckets(u.Handle) != beforeBuckets || q.LenPrimary() != 0 || target.Owner != 1 {
			t.Fatalf("active=%t Strict changed infection effects, random streams, resources or victim", active)
		}
	}
}

func TestModernInfectionSuspensionDoesNotCountAsSpray(t *testing.T) {
	q, u, target := infectionFixture()
	transfers := 0
	q.Binding().Work.SetCapture(func(*units.Unit, *Node, uint32) bool { transfers++; return true })
	issueInfection(q, u, target)
	q.Pump(u, 1)
	q.Pump(u, 2)
	q.Pump(u, 100)
	if transfers != 0 || q.Head().Phase != infectionApproach {
		t.Fatal("suspended interval completed infection")
	}
	for tick := uint32(101); tick <= 162; tick++ {
		q.Pump(u, tick)
	}
	if transfers != 1 {
		t.Fatal("fresh uninterrupted interval did not complete")
	}
}

func TestModernInfectionTakesWeaponsEvenWithWorkingAutonomy(t *testing.T) {
	q, u, target := infectionFixture()
	q.Binding().Community = community.Features{WorkingWeaponsAutonomous: true}
	primeWorkSlots(u)
	issueInfection(q, u, target)
	q.Pump(u, 1)
	for i := range u.Slots {
		if u.Slots[i].IsAutonomous() || u.Slots[i].Target.Kind != units.TargetNone {
			t.Fatalf("slot %d retained autonomy or target during infection", i)
		}
	}
	q.Binding().Rules = StrictRules{}
	q.Pump(u, 2)
	for i := range u.Slots {
		if !u.Slots[i].IsAutonomous() {
			t.Fatalf("slot %d not returned on cancellation", i)
		}
	}
}

func TestModernInfectionPursuesDuringSprayWithoutRestartingStance(t *testing.T) {
	q, u, target := infectionFixture()
	scripted, vm := cbUnit(cbProgram("StartBuilding", "StopBuilding"))
	u.ScriptState = scripted.ScriptState
	installs, releases := 0, 0
	var goal RectangleGoalRequest
	q.Binding().Movement.SetInstallRectangle(func(req RectangleGoalRequest) bool {
		installs++
		goal = req
		return true
	})
	q.Binding().Movement.SetRelease(func(*Node) bool { releases++; return true })
	issueInfection(q, u, target)
	n := q.Head()
	policy := q.Binding().rules().Infection(u.Def)
	for tick := uint32(1); tick <= 2; tick++ {
		if code := infectionCapture(u, n, policy, 0, tick); code != 2 {
			t.Fatalf("setup returned %d", code)
		}
	}
	if installs != 1 || releases != 0 || goal.Node != n || n.Phase != infectionSpraying {
		t.Fatal("initial target in reach did not keep a pursuit goal through spray entry")
	}
	started := n.Param3
	for tick := uint32(3); tick <= 5; tick++ {
		target.X += numeric.FixedFromInt(16)
		if code := infectionCapture(u, n, policy, gateNoRoute, tick); code != 2 {
			t.Fatalf("in-range refresh returned %d", code)
		}
		if n.Param3 != started || n.Phase != infectionSpraying || !u.InBuildStance {
			t.Fatal("in-range movement refresh restarted progress or stance")
		}
		if goal.CellX != footprintAnchorCell(target.X, target.Def.FootprintX) {
			t.Fatal("pursuit goal did not follow target footprint")
		}
	}
	if installs != 4 || releases != 0 || len(startedArgs(vm)) != 1 {
		t.Fatalf("installs=%d releases=%d callbacks=%d, want four goals and one StartBuilding", installs, releases, len(startedArgs(vm)))
	}
	target.X++ // unchanged target cell: keep the existing route
	if code := infectionCapture(u, n, policy, 0, 6); code != 2 || installs != 4 {
		t.Fatal("subcell motion replaced pursuit goal")
	}
}

func TestModernInfectionPursuitFailureOnlyCancelsOutsideReach(t *testing.T) {
	for _, inRange := range []bool{false, true} {
		q, u, target := infectionFixture()
		if !inRange {
			target.X += numeric.FixedFromInt(200)
		}
		q.Binding().Movement.SetInstallRectangle(func(RectangleGoalRequest) bool { return false })
		issueInfection(q, u, target)
		n := q.Head()
		code := infectionCapture(u, n, q.Binding().rules().Infection(u.Def), gateNoRoute, 1)
		if (inRange && (code != 2 || n.Phase != infectionStance)) || (!inRange && code != 8) {
			t.Fatalf("inRange=%t failed pursuit returned %d, phase=%d", inRange, code, n.Phase)
		}
	}
}

func TestModernInfectionDirectCaptureCannotTakeBuilder(t *testing.T) {
	q, u, target := infectionFixture()
	target.Def.Builder = true
	target.Def.UnitName = "arbitrary_mod_constructor"
	q.Binding().World = NewWorldQueryAdapter(WorldQueryAdapterConfig{SeaLevel: func() uint8 { return 0 }})
	transfers := 0
	q.Binding().Work.SetCapture(func(*units.Unit, *Node, uint32) bool { transfers++; return true })
	// Bypass command resolution; the order itself must revalidate immunity.
	q.Push(Lookup("Capture"), NewNodeForOrder(Lookup("Capture"), target.Handle, target.X, target.Y, target.Z, 1, u.Handle, false))
	sim, crt := *q.Binding().SimRNG, *rng.Global.Crt
	buckets := *q.Binding().Economy.UnitBuckets(u.Handle)
	for tick := uint32(1); tick < 70; tick++ {
		q.Pump(u, tick)
	}
	if transfers != 0 || target.Owner != 1 || q.LenPrimary() != 0 || *q.Binding().SimRNG != sim || *rng.Global.Crt != crt || *q.Binding().Economy.UnitBuckets(u.Handle) != buckets {
		t.Fatal("direct Capture bypassed constructor immunity or changed RNG/resources")
	}
	if got := Resolve(3, u, target, nil); got == 0 || got == Lookup("Capture") {
		t.Fatal("constructor immunity suppressed ordinary attack")
	}
}
