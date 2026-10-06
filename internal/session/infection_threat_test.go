package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

func TestInfectionThreatCompositionRebindAndConstructorDamage(t *testing.T) {
	s, actor, victim := infectionSession(t, gameplay.Modern)
	s.bindOrderQueue(actor) // the director binds every hunter before issuing work
	if s.Combat.InfectionThreat == nil {
		t.Fatal("missing existing-policy observation binding")
	}
	sim, crt, stock := *s.SimRNG(), *s.CrtRNG(), s.Econ.Players[1].Stock
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		s.SetGameplay(mode)
		if s.Combat.InfectionThreat(actor) != (mode == gameplay.Modern) || s.Combat.InfectionThreat(victim) {
			t.Fatalf("mode=%v actor=%v victim=%v alive=%v remaining=%v actorQueue=%v rules=%T", mode, s.Combat.InfectionThreat(actor), s.Combat.InfectionThreat(victim), actor.Alive, actor.Remaining, orders.QueueOfUnit(actor), s.Build.OrderBinding.Rules)
		}
	}
	for _, state := range []string{"stunned", "unfinished", "dying", "dead"} {
		actor.Stunned, actor.Remaining, actor.Dying, actor.Alive = false, 0, false, true
		switch state {
		case "stunned":
			actor.Stunned = true
		case "unfinished":
			actor.Remaining = .5
		case "dying":
			actor.Dying = true
		case "dead":
			actor.Alive = false
		}
		if s.Combat.InfectionThreat(actor) {
			t.Fatalf("inactive %s infector received threat preference", state)
		}
	}
	actor.Stunned, actor.Remaining, actor.Dying, actor.Alive = false, 0, false, true
	if *s.SimRNG() != sim || *s.CrtRNG() != crt || s.Econ.Players[1].Stock != stock {
		t.Fatal("capability observation spent RNG or resources")
	}
	victim.Def.Builder = true
	if orders.InfectionTarget(actor, victim) {
		t.Fatal("constructor admitted")
	}
	before := victim.Health
	s.Combat.AcceptDamage(s.Units, 20, combat.DamageInput{Victim: victim.Handle, Attacker: actor.Handle, Kind: combat.KindNoReaction, Nominal: 1})
	if victim.Health >= before {
		t.Fatal("constructor infection immunity suppressed ordinary damage")
	}
}
