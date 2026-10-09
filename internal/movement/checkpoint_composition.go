package movement

import (
	"errors"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// These independent slot proofs retain the actual receiver at installation.
// They cannot transfer authority to a copied System (§16.3.69).
type checkpointCompositionProof struct {
	system    *System
	authority *checkpoint.BindingAuthority
}

func (p checkpointCompositionProof) matches(s *System, authority *checkpoint.BindingAuthority) bool {
	return s != nil && p.system == s && p.authority.Matches(authority)
}

// SetDamageWithCheckpointBinding preserves the ordinary installation and
// attests only that slot. Session supplies its private authority (§16.3.69).
func (s *System) SetDamageWithCheckpointBinding(fn func(uint32, combat.DamageInput) combat.DamageResult, authority *checkpoint.BindingAuthority) {
	s.SetDamage(fn)
	if fn != nil && authority != nil {
		s.checkpointDamage = checkpointCompositionProof{s, authority}
	}
}

// SetProductFootprintWithCheckpointBinding preserves the ordinary installation
// and attests only that slot; it never resolves a product (§16.3.69).
func (s *System) SetProductFootprintWithCheckpointBinding(fn func(uint32) (int32, int32, bool), authority *checkpoint.BindingAuthority) {
	s.SetProductFootprint(fn)
	if fn != nil && authority != nil {
		s.checkpointProductFootprint = checkpointCompositionProof{s, authority}
	}
}

type checkpointCompositionBindings struct {
	system    *System
	keys      *content.CheckpointKeys
	authority *checkpoint.BindingAuthority
}

// SetCompositionBindings borrows the exact constructor owner and shared keys.
// It validates immutable classes and callback installation proofs before
// storing anything. Repeats revalidate the same tuple, without invoking a
// callback, looking up content or rebuilding classes (DESIGN_MULTIPLAYER §16.3.69).
func (c *CheckpointContext) SetCompositionBindings(s *System, keys *content.CheckpointKeys, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || keys == nil || authority == nil {
		return movementCheckpointError("movement.compositionBindings", errors.New("missing context, system, keys or authority"))
	}
	binding := checkpointCompositionBindings{s, keys, authority}
	if c.system != nil && c.system != s || c.compositionBindings.authority != nil && c.compositionBindings != binding {
		return movementCheckpointError("movement.compositionBindings", errors.New("conflicting composition binding registration"))
	}
	if err := binding.validate(s, c); err != nil {
		return movementCheckpointError("movement.compositionBindings", err)
	}
	c.system, c.compositionBindings = s, binding
	return nil
}

func (b checkpointCompositionBindings) validate(s *System, c *CheckpointContext) error {
	if s == nil || s != b.system || b.keys == nil || b.authority == nil || c == nil || c.Orders == nil || c.Orders.Units == nil || c.Orders.Units.Keys != b.keys {
		return errors.New("composition system or shared keys differ")
	}
	if !s.hasCheckpointOrderHandlerSource() || !s.checkpointOrderHandlers.authority.Matches(b.authority) {
		return errors.New("unattested movement composition constructor owner or authority")
	}
	for _, authority := range []*checkpoint.BindingAuthority{c.terrainBindings.authority, c.pathBindings.authority, c.auxiliaryBindings.authority} {
		if authority != nil && !authority.Matches(b.authority) {
			return errors.New("composition authority differs from registered movement binding")
		}
	}
	if err := b.keys.ValidateMovementClasses(s.Classes); err != nil {
		return err
	}
	if s.damage != nil {
		if !s.checkpointDamage.matches(s, b.authority) {
			return errors.New("unattested movement damage binding")
		}
	} else if s.checkpointDamage != (checkpointCompositionProof{}) {
		return errors.New("retained movement damage proof without callback")
	}
	if s.productFootprint != nil {
		if !s.checkpointProductFootprint.matches(s, b.authority) {
			return errors.New("unattested movement product footprint binding")
		}
	} else if s.checkpointProductFootprint != (checkpointCompositionProof{}) {
		return errors.New("retained movement product footprint proof without callback")
	}
	return nil
}

func (s *System) validateCheckpointComposition(c *CheckpointContext) error {
	if c.compositionBindings.authority != nil {
		return c.compositionBindings.validate(s, c)
	}
	if s.Classes != nil || s.damage != nil || s.productFootprint != nil || s.checkpointDamage != (checkpointCompositionProof{}) || s.checkpointProductFootprint != (checkpointCompositionProof{}) {
		return errors.New("unattested movement composition binding")
	}
	return nil
}
