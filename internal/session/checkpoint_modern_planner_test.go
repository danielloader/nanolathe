package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"testing"
)

type checkpointModernStep struct{}

func (checkpointModernStep) Step(*ai.Manager, uint32, *units.World, *economy.Service) {
	panic("witness invoked think")
}
func (checkpointModernStep) ControlsModernAI(*ai.Manager) bool { panic("witness invoked marker") }

type checkpointModernEmbedding struct{ checkpointModernStep }
type checkpointModernSlice []int

func (checkpointModernSlice) Step(*ai.Manager, uint32, *units.World, *economy.Service) {
	panic("witness invoked foreign think")
}
func (checkpointModernSlice) ControlsModernAI(*ai.Manager) bool {
	panic("witness invoked foreign marker")
}

func TestCheckpointModernPlannerWitness(t *testing.T) {
	step := checkpointModernStep{}
	s := &Session{modernAI: step, modernAICheckpointWitness: ai.NewCheckpointModernPlanner(step)}
	if !s.checkpointModernPlannerMatches(step) {
		t.Fatal("exact concrete planner refused")
	}
	for _, other := range []ai.Planner{nil, (*checkpointModernStep)(nil), &checkpointModernStep{}, checkpointModernEmbedding{}, checkpointModernSlice{1}, ai.ModernPlanner{}} {
		if s.checkpointModernPlannerMatches(other) {
			t.Fatalf("replacement %T accepted", other)
		}
	}
	s.modernAI = checkpointModernEmbedding{}
	if s.checkpointModernPlannerMatches(step) {
		t.Fatal("foreign retained session planner accepted")
	}
	if (*Session)(nil).checkpointModernPlannerMatches(step) || (&Session{modernAI: step}).checkpointModernPlannerMatches(step) {
		t.Fatal("absent witness accepted")
	}
}

func TestCheckpointModernPlannerRegistrationRefusals(t *testing.T) {
	// The package test build deliberately uses ordinary registration. Resolving
	// it preserves gameplay but cannot create checkpoint authority.
	s := &Session{}
	if err := s.resolveModernAI(0); err != nil {
		t.Fatal(err)
	}
	if s.modernAI == nil || s.modernAICheckpointWitness != (ai.CheckpointModernPlanner{}) || s.checkpointModernPlannerMatches(s.modernAI) {
		t.Fatal("ordinary registration acquired a witness")
	}
	for _, tc := range []struct {
		name string
		run  func()
	}{
		{"interface instantiation", func() { ai.NewCheckpointModernPlanner[ai.ModernAIStep](checkpointModernStep{}) }},
		{"nil interface", func() { ai.NewCheckpointModernPlanner[ai.ModernAIStep](nil) }},
		{"typed nil", func() { ai.NewCheckpointModernPlanner((*checkpointModernStep)(nil)) }},
		{"pointer", func() { ai.NewCheckpointModernPlanner(&checkpointModernStep{}) }},
		{"noncomparable", func() { ai.NewCheckpointModernPlanner(checkpointModernSlice{}) }},
		{"duplicate", func() {
			RegisterModernAIWithCheckpointBinding(checkpointModernStep{}, ai.NewCheckpointControllerSource[checkpointSourceFixture, *checkpointSourceFixture]())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("unsupported registration accepted")
				}
			}()
			tc.run()
		})
	}
	// A failed attested duplicate must not replace either half of the slot.
	after := &Session{}
	if err := after.resolveModernAI(0); err != nil {
		t.Fatal(err)
	}
	if _, ok := after.modernAI.(testModernAIStep); !ok || after.modernAICheckpointWitness != (ai.CheckpointModernPlanner{}) {
		t.Fatal("failed registration changed the slot")
	}
}

// Never invoked: only provides a concrete generated source for duplicate
// registration refusal, without importing the controller implementation.
type checkpointSourceFixture struct{ ai.CheckpointControllerOwner }
