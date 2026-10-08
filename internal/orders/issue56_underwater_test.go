package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Direct contextual, move and repair commands share the signed whole-height
// water gate. LOS's byte-masked height and fractional parts must not decide
// whether a non-amphibious aircraft can assist [04 R-ORD-01 §7].
func TestConstructorWaterAdmissionUsesFullHeight(t *testing.T) {
	for _, tc := range []struct {
		name   string
		y      numeric.Fixed
		height int32
		want   bool
	}{
		{"below surface", numeric.FixedFromInt(33), 6 << 16, false},
		{"at surface", numeric.FixedFromInt(34), 6 << 16, true},
		{"fractions floor separately", numeric.FixedFromInt(33) + 65535, 7<<16 - 1, false},
		{"full height exceeds LOS byte", numeric.FixedFromInt(-220), 260 << 16, true},
		{"negative Y floors before addition", numeric.FixedFromInt(-220) - 1, 260 << 16, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, actor, target := vtolWorkFixture()
			actor.Def.CanMove = true
			actor.Def.MaxWaterDepth = 10000
			target.Remaining = 0.5
			target.Y = tc.y
			target.Def.ModelTopFixed = tc.height
			target.Def.ModelTop = tc.height >> 16 & 255
			setTestSeaLevel(actor, 40)
			for _, code := range []int{1, 2, 8} {
				want := "VTOL_HelpBuild"
				if !tc.want {
					want = "VTOL_Move"
					if code == 8 {
						want = ""
					}
				}
				if got := DescriptorFor(Resolve(code, actor, target, nil)).Name; got != want {
					t.Fatalf("code %d = %q, want %q [04 R-ORD-01 §7]", code, got, want)
				}
			}
		})
	}
}

// The VTOL scan keeps submerged candidates and makes its ordinary pick, then
// applies nano-reach before either branch. The explicit unfinished-target
// spawn bypasses issuer additions, not water admission [04 R-ORD-01 §7].
func TestVTOLPatrolChecksPickedRepairAdmission(t *testing.T) {
	for _, mode := range []struct {
		name  string
		rules Rules
	}{{"strict", StrictRules{}}, {"modern", &ModernRules{}}} {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				y         int32
				remaining float32
				health    int32
				amphib    bool
				want      string
			}{
				{"submerged nanoframe", 33, 0.5, 50, false, ""},
				{"submerged damaged unit", 33, 0, 50, false, ""},
				{"surface nanoframe", 34, 0.5, 50, false, "VTOL_HelpBuild"},
				{"surface damaged unit", 34, 0, 50, false, "VTOL_RepairUnit"},
				{"amphibious flyer", 33, 0.5, 50, true, "VTOL_HelpBuild"},
				{"full health nanoframe", 34, 0.5, 100, false, ""},
			} {
				t.Run(tc.name, func(t *testing.T) {
					q, actor, target := vtolWorkFixture()
					actor.Def.SightDistance = 300
					actor.Def.Amphibious = tc.amphib
					target.Y = numeric.FixedFromInt(int64(tc.y))
					target.Def.ModelTop, target.Def.ModelTopFixed = 6, 6<<16
					target.Remaining, target.Health = tc.remaining, tc.health
					b := q.Binding()
					b.Rules = mode.rules
					b.Resources = func(uint8) (ResourceView, bool) {
						return ResourceView{Stock: [2]float32{100, 100}, Capacity: [2]float32{100, 100}}, true
					}
					b.World = &WorldQueryAdapter{
						SeaLevel:    func() uint8 { return 40 },
						ForEachUnit: func(visit func(pool.Handle, *units.Unit) bool) { visit(target.Handle, target) },
						ForEachUnitInRadius: func(_, _, _ numeric.Fixed, visit func(pool.Handle, *units.Unit) bool) {
							visit(target.Handle, target)
						},
					}
					if candidates := scanRepairCandidates(actor, 300); len(candidates) != 1 || candidates[0] != target {
						t.Fatal("the visitor must keep this candidate until the post-pick admission")
					}
					n := &Node{ID: Lookup("VTOL_RepairPatrol"), Owner: actor.Handle, Phase: 1, Deadline: -1}
					var retained *Node
					if mode.name == "modern" {
						// Modern work is bounded to its retained assignment. Give this
						// water-admission fixture a waypoint at the offered target.
						n.GoalX, n.GoalZ = target.X, target.Z
						q.primary, retained = []*Node{n}, n
						actor.Def.Builder = true
					}
					code := vtolRepairPatrolHandler(actor, n, 0, 100)
					if tc.want == "" {
						if code != 2 || q.Head() != retained {
							t.Fatalf("rejected pick: code %d head %v, want feature-work hold with no order", code, q.Head())
						}
					} else if head := q.Head(); head == nil || DescriptorFor(head.ID).Name != tc.want || head.Target != target.Handle {
						t.Fatalf("admitted pick: head %v, want %s on the chosen target", head, tc.want)
					}
					if b.SimRNG.Draws() != 0 {
						t.Fatalf("single candidate consumed %d draws, want zero", b.SimRNG.Draws())
					}
				})
			}
		})
	}
}

// Rejecting the picked unit must not pre-filter the list, repick, or stop the
// visit before feature pairing. Two unit candidates consume one bounded draw;
// both feature lists then consume three draws each [04 R-ORD-01 §7][I4].
func TestVTOLPatrolRejectedPickReachesFeatureWork(t *testing.T) {
	q, actor, target := vtolWorkFixture()
	actor.Def.SightDistance = 300
	target.Y, target.Remaining = numeric.FixedFromInt(20), 0.5
	other := *target
	other.Handle = 3
	b := q.Binding()
	b.Resources = func(uint8) (ResourceView, bool) {
		return ResourceView{Stock: [2]float32{0, 20}, Capacity: [2]float32{100, 100}}, true
	}
	b.World = &WorldQueryAdapter{
		SeaLevel: func() uint8 { return 40 },
		ForEachUnitInRadius: func(_, _, _ numeric.Fixed, visit func(pool.Handle, *units.Unit) bool) {
			for _, candidate := range []*units.Unit{target, &other} {
				if visit(candidate.Handle, candidate) {
					return
				}
			}
		},
		LookupFeature: func(int32, int32) (FeatureView, bool) {
			return FeatureView{Metal: 1, Energy: 1, Reclaimable: true, Autoreclaimable: true}, true
		},
	}
	n := &Node{ID: Lookup("VTOL_RepairPatrol"), Owner: actor.Handle, Phase: 1, Deadline: -1}
	if code := vtolRepairPatrolHandler(actor, n, 0, 100); code != 3 {
		t.Fatalf("feature work returned %d, want wait (3)", code)
	}
	if head := q.Head(); head == nil || DescriptorFor(head.ID).Name != "VTOL_Reclaim" {
		t.Fatalf("head %v, want feature reclaim after the rejected unit", head)
	}
	if draws := b.SimRNG.Draws(); draws != 7 {
		t.Fatalf("draws = %d, want unit pick then six feature picks [I4]", draws)
	}
}

// A flying guard joins the ward's nanoframe only when the same admission
// passes. The ship's builder status does not extend the aircraft's reach
// [04 R-ORD-02 §3]. Ground constructor reach remains depth-dependent.
func TestGuardCopiesConstructionOnlyWithinItsWaterReach(t *testing.T) {
	for _, y := range []int32{33, 34} {
		f := newAirGuardFixture(t)
		f.guard.Def.Builder, f.guard.Def.CanReclamate = true, true
		f.ward.Def.Builder = true
		frame := &units.Unit{
			Handle: 3, Alive: true, Health: 50, MaxHealth: 100, Remaining: 0.5,
			Def: &content.UnitDef{MaxDamage: 100, ModelTop: 6, ModelTopFixed: 6 << 16},
			Y:   numeric.FixedFromInt(int64(y)),
		}
		frame.Move.Mode, frame.Move.ModeMirror = 1, 1
		b := QueueForUnit(f.guard).Binding()
		wardLookup := b.Lookup
		b.Lookup = func(h pool.Handle) *units.Unit {
			if h == frame.Handle {
				return frame
			}
			return wardLookup(h)
		}
		b.World = &WorldQueryAdapter{SeaLevel: func() uint8 { return 40 }}
		wq := QueueForUnit(f.ward)
		wq.Push(rowMobileBuild, Node{Owner: f.ward.Handle})
		wq.Head().BindTarget(frame.Handle)
		if got := vtolFollowCopyWork(f.guard, airGuardNode(f), f.ward, 100); got != (y == 34) {
			t.Fatalf("model height at %d: guard copied work = %v, want %v", y+6, got, y == 34)
		}
		f.guard.Def.CanFly, f.guard.Def.MaxWaterDepth = false, 1
		if !nanoReach(f.guard, frame) {
			t.Fatal("a ground constructor admits a target at its exact authored water-depth boundary")
		}
	}
}
