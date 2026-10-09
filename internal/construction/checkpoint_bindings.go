package construction

import (
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

const (
	checkpointCRTRandom = iota
	checkpointSpecialSecondState
	checkpointModelForFactory
	checkpointModelForUnit
)

// The four private slots retain exact installation owner/authority, not a
// function address. Their logical presence bytes keep the original names and
// positions. Ordinary replacement clears only its slot (§16.3.67).
type checkpointCallbackProof struct {
	owner     *Service
	authority *checkpoint.BindingAuthority
}

func (s *Service) SetCRTRandomWithCheckpointBinding(fn func(uint32) uint32, authority *checkpoint.BindingAuthority) {
	s.SetCRTRandom(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointCRTRandom] = checkpointCallbackProof{s, authority}
	}
}

func (s *Service) SetIsSpecialSecondStateWithCheckpointBinding(fn func(uint8) bool, authority *checkpoint.BindingAuthority) {
	s.SetIsSpecialSecondState(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointSpecialSecondState] = checkpointCallbackProof{s, authority}
	}
}

func (s *Service) SetModelForFactoryWithCheckpointBinding(fn func(*units.Unit) *model.Model, authority *checkpoint.BindingAuthority) {
	s.SetModelForFactory(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointModelForFactory] = checkpointCallbackProof{s, authority}
	}
}

func (s *Service) SetModelForUnitWithCheckpointBinding(fn func(*units.Unit) *model.Model, authority *checkpoint.BindingAuthority) {
	s.SetModelForUnit(fn)
	if fn != nil && authority != nil {
		s.checkpointCallbacks[checkpointModelForUnit] = checkpointCallbackProof{s, authority}
	}
}

// These are capture-local expectations, never wire data. Keeping the original
// constructor source and catalog pointer prevents a repeat from refreshing an
// old capture after either input object is overwritten. Frozen input semantics
// are validated once by the root; this owner only reads the catalog alias.
type checkpointConstructionBindings struct {
	service   *Service
	inputs    *content.SimulationInputs
	catalog   *content.Catalog
	world     *units.World
	economy   *economy.Service
	combat    *combat.Service
	movement  *movement.System
	orders    *orders.QueueBinding
	authority *checkpoint.BindingAuthority
	source    *orders.CheckpointHandlerSource
}

// SetBindings admits one exact constructor and owner tuple. Every check
// precedes assignment, including repeat validation; capture never calls this
// method to repair a missing registration (DESIGN_MULTIPLAYER §16.3.67).
func (c *CheckpointContext) SetBindings(s *Service, inputs *content.SimulationInputs, w *units.World, econ *economy.Service, damage *combat.Service, move *movement.System, binding *orders.QueueBinding, authority *checkpoint.BindingAuthority) error {
	if c == nil || c.World == nil || s == nil || inputs == nil || authority == nil {
		return constructionCheckpointError("construction.bindings", "a context with terrain, service, frozen inputs and authority")
	}
	next := checkpointConstructionBindings{s, inputs, inputs.Catalog(), w, econ, damage, move, binding, authority, s.checkpointHandlers.source}
	if c.bindings != nil && *c.bindings != next {
		return constructionCheckpointError("construction.bindings", "the same registered owner tuple")
	}
	if s.Terrain != c.World.Terrain {
		return constructionCheckpointError("construction.Service.Terrain", "the collected terrain identity")
	}
	if err := next.validate(s); err != nil {
		return err
	}
	// Sink registration may precede or follow this tuple. Once supplied, its
	// expectation is checked here too; absence of registration is not admission.
	if c.presentation != nil && !c.presentation.matches(s.Presentation) {
		return constructionCheckpointError("construction.Service.Presentation", "the registered concrete presentation sink")
	}
	if c.bindings == nil {
		c.bindings = &next
	}
	return nil
}

func (b checkpointConstructionBindings) validate(s *Service) error {
	if s != b.service || !s.hasCheckpointHandlerSource() || !s.checkpointHandlers.authority.Matches(b.authority) || s.checkpointHandlers.source != b.source {
		return constructionCheckpointError("construction.bindings", "the original constructor owner, source and authority")
	}
	for _, edge := range [...]struct {
		field string
		valid bool
	}{
		{"Allocator", s.Allocator == nil},
		{"Catalog", b.inputs != nil && b.inputs.Catalog() == b.catalog && s.Catalog == b.catalog},
		{"Combat", s.Combat == b.combat},
		{"Economy", s.Economy == b.economy},
		{"LimitChecker", s.LimitChecker == nil},
		{"Movement", s.Movement == b.movement},
		{"OrderBinding", s.OrderBinding == b.orders},
		{"World", s.World == b.world},
		{"repairWorld", s.repairWorld == nil || s.repairWorld == b.world},
	} {
		if !edge.valid {
			return constructionCheckpointError("construction.Service."+edge.field, "the registered owner alias or supported absent hook")
		}
	}
	for i, callback := range [...]struct {
		field   string
		present bool
	}{
		{"CRTRandom", s.crtRandom != nil},
		{"IsSpecialSecondState", s.isSpecialSecondState != nil},
		{"ModelForFactory", s.modelForFactory != nil},
		{"ModelForUnit", s.modelForUnit != nil},
	} {
		p := s.checkpointCallbacks[i]
		if callback.present && (p.owner != s || !p.authority.Matches(b.authority)) {
			return constructionCheckpointError("construction.Service."+callback.field, "the exact callback installation owner and authority")
		}
	}
	return nil
}

// A matcher can be constructed only by the closed generic registration below.
// It performs no application method call or arbitrary interface comparison.
type checkpointConstructionPresentationMatch struct {
	matches func(interface{ EmitNanolathe(frame.Event) bool }) bool
}

// SetCheckpointPresentationSink records the expected concrete pointer. Root
// checks its captured Session at installation; the sink emits authoritative
// strips and is not presentation-only (DESIGN_MULTIPLAYER §16.3.67).
func SetCheckpointPresentationSink[T any, P interface {
	*T
	EmitNanolathe(frame.Event) bool
}](c *CheckpointContext, expected P) error {
	if c == nil || expected == nil {
		return constructionCheckpointError("construction.Service.Presentation", "a context and nonnil concrete presentation sink")
	}
	if c.presentation != nil {
		if !c.presentation.matches(expected) {
			return constructionCheckpointError("construction.Service.Presentation", "the same registered presentation sink")
		}
		return nil
	}
	c.presentation = &checkpointConstructionPresentationMatch{matches: func(actual interface{ EmitNanolathe(frame.Event) bool }) bool {
		p, ok := actual.(P)
		return ok && p == expected
	}}
	return nil
}

func (s *Service) validateCheckpointBindings(c *CheckpointContext) error {
	if c.bindings != nil {
		if err := c.bindings.validate(s); err != nil {
			return err
		}
	} else {
		// Preserve authored fixtures without any production binding. Allocator
		// and LimitChecker remain unsupported even after tuple registration;
		// TODO(M3-U6): a future canonical installer must precede their admission.
		for _, binding := range [...]struct {
			field   string
			present bool
		}{
			{"Allocator", s.Allocator != nil}, {"CRTRandom", s.crtRandom != nil},
			{"Catalog", s.Catalog != nil}, {"Combat", s.Combat != nil},
			{"Economy", s.Economy != nil}, {"IsSpecialSecondState", s.isSpecialSecondState != nil},
			{"LimitChecker", s.LimitChecker != nil}, {"ModelForFactory", s.modelForFactory != nil},
			{"ModelForUnit", s.modelForUnit != nil}, {"Movement", s.Movement != nil},
			{"OrderBinding", s.OrderBinding != nil}, {"Presentation", s.Presentation != nil},
			{"World", s.World != nil}, {"repairWorld", s.repairWorld != nil},
		} {
			if binding.present {
				return constructionCheckpointError("construction.Service."+binding.field, "an absent or U6-attested binding")
			}
		}
	}
	if c.presentation != nil {
		if !c.presentation.matches(s.Presentation) {
			return constructionCheckpointError("construction.Service.Presentation", "the registered concrete presentation sink")
		}
	} else if s.Presentation != nil {
		return constructionCheckpointError("construction.Service.Presentation", "a registered concrete presentation sink")
	}
	return nil
}
