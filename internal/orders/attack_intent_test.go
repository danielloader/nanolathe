package orders

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Slot point storage is not enough: enemy-target bombers also use it, and a
// vanished special-attack target can resolve to Suppress. Modern obstruction
// safety uses the retained order history (DESIGN_WEAPONS_PROJECTILES §2.3.2).
func TestExplicitGroundAttackIntent(t *testing.T) {
	for _, name := range []string{"Suppress", "AirStrike", "AirToGround", "AirToGroundHover", "Attack_Chase", "Move_Ground"} {
		t.Run(name, func(t *testing.T) {
			q, u := gateFixture()
			q.Push(Lookup(name), Node{Owner: u.Handle, GoalSupplied: true})
			want := name == "Suppress" || name == "AirStrike" || name == "AirToGround" || name == "AirToGroundHover"
			if got := q.ExplicitGroundAttack(u); got != want {
				t.Fatalf("point intent=%v, want %v", got, want)
			}
			q.Head().automaticAttack = true
			if q.ExplicitGroundAttack(u) {
				t.Fatal("automatic attack received explicit ground permission")
			}
			q.Head().automaticAttack = false
			q.Head().StaticGate |= staticTargetObserver
			q.Head().Target = 7
			if q.ExplicitGroundAttack(u) {
				t.Fatal("unit-target order received ground permission")
			}
			q.Head().BindTarget(0)
			if q.ExplicitGroundAttack(u) {
				t.Fatal("lost unit target became explicit ground intent")
			}
		})
	}
}

func TestSpecialAttackKeepsTargetIntent(t *testing.T) {
	for _, friendly := range []bool{false, true} {
		q, u := gateFixture()
		u.Flags |= units.ArmedStatus
		u.Def.CanAttack, u.Def.CanDGun = true, true
		target := &units.Unit{Handle: 7, Owner: u.Owner, Alive: true, Def: u.Def}
		q.Binding().SetLookup(func(handle pool.Handle) *units.Unit {
			if friendly && handle == target.Handle {
				return target
			}
			return nil
		})
		q.Push(Lookup("AttackSpecial"), Node{Owner: u.Handle, Target: target.Handle, GoalSupplied: true})
		n := q.Head()
		if !friendly {
			n.BindTarget(0)
		}
		if code := attackSpecialHandler(u, n, 0, 1); code != 2 || n.ID != Lookup("Suppress") {
			t.Fatalf("special attack did not resolve to Suppress: code=%d id=%d", code, n.ID)
		}
		if got := q.ExplicitGroundAttack(u); got != friendly {
			t.Fatalf("friendly=%v: point intent=%v", friendly, got)
		}
	}
}

func TestAttackIntentKeepsAutomaticProducers(t *testing.T) {
	for _, producer := range []string{"explicit", "auto-engage", "danger", "guard"} {
		q, _ := gateFixture()
		id := Lookup("Attack_Chase")
		if producer == "guard" {
			id = Lookup("Guard_NoMove")
		}
		q.Push(id, Node{Target: 7, GoalSupplied: true})
		if producer == "auto-engage" {
			q.Head().automaticAttack = true
		}
		if producer == "danger" {
			q.danger.response = q.Head()
		}
		if got := q.AutomaticAttack(); got != (producer != "explicit") {
			t.Fatalf("producer=%s: automatic=%v", producer, got)
		}
	}
}
