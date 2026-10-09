package effects

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// One proof covers the retained context installed together by the canonical
// session site. The sink captures that session and tick; the height method
// captures its terrain. Copies cannot transfer the exact pool's proof
// (DESIGN_MULTIPLAYER §16.3.68).
type checkpointFragmentBinding struct {
	pool      *FixedEffectPool
	terrain   *world.Terrain
	tick      uint32
	authority *checkpoint.BindingAuthority
}

type checkpointEffectBindings struct {
	service   *EffectService
	pool      *FixedEffectPool
	art       *content.SimArt
	terrain   *world.Terrain
	tick      uint32
	authority *checkpoint.BindingAuthority
}

// CheckpointFixedPool narrows the actual owner without calling any interface
// method. Nil and other implementations cannot claim the canonical pool type.
func (s *EffectService) CheckpointFixedPool() *FixedEffectPool {
	if s == nil {
		return nil
	}
	p, _ := s.owner.(*FixedEffectPool)
	return p
}

// SetFragmentStepContextWithCheckpointBinding performs the ordinary context
// installation, then records the reviewed session's captured owners. Nil
// authority preserves ordinary behavior without admitting either port.
func (p *FixedEffectPool) SetFragmentStepContextWithCheckpointBinding(ctx FragmentStepContext, terrain *world.Terrain, tick uint32, authority *checkpoint.BindingAuthority) {
	p.SetFragmentStepContext(ctx)
	if p != nil && authority != nil {
		p.checkpointFragment = checkpointFragmentBinding{p, terrain, tick, authority}
	}
}

// SetFragmentStepContextWithCheckpointBinding preserves ordinary forwarding
// for every other EffectPool implementation; only the concrete fixed pool can
// retain this proof. This introduces no additional interface method call.
func (s *EffectService) SetFragmentStepContextWithCheckpointBinding(ctx FragmentStepContext, terrain *world.Terrain, tick uint32, authority *checkpoint.BindingAuthority) {
	if p := s.CheckpointFixedPool(); p != nil {
		p.SetFragmentStepContextWithCheckpointBinding(ctx, terrain, tick, authority)
		return
	}
	s.SetFragmentStepContext(ctx)
}

// SetBindings records one exact capture tuple after validating current aliases
// and installation proof. Repeated registration revalidates; any conflict or
// refusal leaves the original context intact (DESIGN_MULTIPLAYER §16.3.68).
func (c *CheckpointContext) SetBindings(s *EffectService, art *content.SimArt, terrain *world.Terrain, tick uint32, authority *checkpoint.BindingAuthority) error {
	if c == nil || s == nil || art == nil || terrain == nil || c.Pool == nil || authority == nil {
		return effectsCheckpointError("effects.bindings", "a context, service, admitted art, terrain, pool and authority")
	}
	binding := checkpointEffectBindings{s, c.Pool, art, terrain, tick, authority}
	if c.binding.service != nil && c.binding != binding {
		return effectsCheckpointError("effects.bindings", "one consistent service, pool, art, terrain, tick and authority")
	}
	candidate := *c
	candidate.binding = binding
	if err := candidate.validateCheckpointBindings(); err != nil {
		return err
	}
	c.binding = binding
	return nil
}

func (c *CheckpointContext) validateCheckpointBindings() error {
	b := c.binding
	if b.service == nil {
		return nil
	}
	if c.Pool != b.pool {
		return effectsCheckpointError("effects.context.Pool", "the registered fixed pool identity")
	}
	if b.service.CheckpointFixedPool() != b.pool {
		return effectsCheckpointError("effects.EffectService.owner", "the registered service's exact fixed pool")
	}
	if b.service.art != b.art {
		return effectsCheckpointError("effects.EffectService.art", "the registered admitted simulation-art identity")
	}
	return b.pool.validateCheckpointFragmentBindings(c)
}

func (p *FixedEffectPool) validateCheckpointFragmentBindings(c *CheckpointContext) error {
	// This is an enable predicate, not traversal state. Its existing height
	// presence byte carries it; root owns completed-tick quiescence (§16.3.68).
	if p.fragmentStepping != (p.fragmentContext.TerrainHeight != nil) {
		return effectsCheckpointError("effects.FixedEffectPool.fragmentStepping", "coherence with retained TerrainHeight presence")
	}
	for _, port := range [...]struct {
		name    string
		present bool
	}{
		{"Impact", p.fragmentContext.Impact != nil},
		{"TerrainHeight", p.fragmentContext.TerrainHeight != nil},
	} {
		if !port.present {
			continue
		}
		b, proof := c.binding, p.checkpointFragment
		if b.service == nil || proof.pool != p || proof.terrain != b.terrain || proof.tick != b.tick || !proof.authority.Matches(b.authority) {
			return effectsCheckpointError("effects.FixedEffectPool.fragmentContext."+port.name, "the exact pool's admitted terrain, completed tick and authority")
		}
	}
	// TODO(M3-U6): this separate bounce callback has no production installer;
	// retain its refusal until a reviewed binding contract is needed.
	if p.heightAt != nil {
		return effectsCheckpointError("effects.FixedEffectPool.heightAt", "an absent or U6-attested port")
	}
	return nil
}
