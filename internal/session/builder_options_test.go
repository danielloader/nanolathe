package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
)

func TestBuilderOptionsBelongToThePlayerAndChangeAtTheBoundary(t *testing.T) {
	s := &Session{LocalOwner: 2, Clock: &clock.State{GlobalTick: 10}, Econ: &economy.Service{}, Build: &construction.Service{OrderBinding: &orders.QueueBinding{}}}
	s.Econ.Players[2] = economy.Player{Exists: true, ControllerState: 1}
	s.Econ.Players[3] = economy.Player{Exists: true, ControllerState: 2}
	preferred := orders.DefaultBuilderOptions()
	preferred.Guard[0] = orders.GuardScatter
	if err := s.initializeBuilderOptions(&preferred); err != nil {
		t.Fatal(err)
	}
	b := s.Build.OrderBinding
	if b.BuilderOptionsHook()(2) != preferred || b.BuilderOptionsHook()(3) != orders.DefaultBuilderOptions() {
		t.Fatal("human preference crossed the player boundary")
	}
	next := preferred
	next.Patrol[0] = orders.PatrolAssistOnly
	command := HumanCommand{Kind: HumanBuilderOptions, BuilderOptions: HumanBuilderOptionsCommand{Owner: 2, Options: next}}
	if err := s.EnqueueHumanCommand(command); err != nil {
		t.Fatal(err)
	}
	s.applyHumanCommands(10)
	if b.BuilderOptionsHook()(2) != preferred {
		t.Fatal("preference changed before the input boundary")
	}
	s.applyHumanCommands(11)
	if b.BuilderOptionsHook()(2) != next || b.BuilderOptionsHook()(3) != orders.DefaultBuilderOptions() {
		t.Fatal("command did not update only its player through the existing binding")
	}
	for _, owner := range []uint8{3, 10} {
		command.BuilderOptions.Owner = owner
		if err := s.EnqueueHumanCommand(command); err == nil {
			t.Fatalf("accepted foreign or invalid owner %d", owner)
		}
	}
	command.BuilderOptions.Owner = 2
	command.BuilderOptions.Options.Guard[1] = 3
	if err := s.EnqueueHumanCommand(command); err == nil {
		t.Fatal("accepted invalid option")
	}
	if err := s.initializeBuilderOptions(nil); err != nil {
		t.Fatal(err)
	}
	if b.BuilderOptionsHook()(2) != orders.DefaultBuilderOptions() {
		t.Fatal("new battle retained old preferences")
	}
}

func TestBuilderOptionsDefaultsComeFromTheBoundRules(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		s := &Session{Rules: RuleSetForMode(mode), LocalOwner: 2, Econ: &economy.Service{}}
		s.Econ.Players[2] = economy.Player{Exists: true, ControllerState: 1}
		if err := s.initializeBuilderOptions(nil); err != nil {
			t.Fatal(err)
		}
		want := orders.DefaultBuilderOptions()
		if mode == gameplay.Modern {
			want.Patrol[0] = orders.PatrolBoth
		}
		if s.builderOptionsForOwner(2) != want || s.builderOptionsForOwner(3) != want || s.builderOptionsForOwner(10) != want {
			t.Fatalf("%s did not supply the bound default %+v", mode, want)
		}
		// An explicit human preference overrides the default only for its owner.
		explicit := orders.DefaultBuilderOptions()
		if err := s.initializeBuilderOptions(&explicit); err != nil {
			t.Fatal(err)
		}
		if s.builderOptionsForOwner(2) != explicit || s.builderOptionsForOwner(3) != want {
			t.Fatalf("%s replaced an explicit preference or changed a computer", mode)
		}
	}
}
