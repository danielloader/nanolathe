package world

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// FootprintRestampOwner supplies the method captured by the terrain installer.
// Movement narrows this opaque owner to its exact System before registration;
// world never invokes it to establish identity (DESIGN_MULTIPLAYER §16.3.57).
type FootprintRestampOwner interface {
	NoteFeatureFootprint(int32, int32, int16, int16)
}

// One immutable receipt covers the two private movement ports. Either ordinary
// setter discards it. A fresh admitted installation gets a fresh receipt even
// when its owner and authority have not changed, so old captures fail closed.
type checkpointMovementReceipt struct {
	terrain   *Terrain
	authority *checkpoint.BindingAuthority
	owner     FootprintRestampOwner
}

// SetClassRestampOwnerWithCheckpointBinding extracts the owner's method itself,
// rather than trusting an independently supplied callback and claimed owner.
// Nil authority preserves ordinary installation without proof; nil owner clears
// the callback without proof. Neither installation invokes an owner method.
func (t *Terrain) SetClassRestampOwnerWithCheckpointBinding(owner FootprintRestampOwner, authority *checkpoint.BindingAuthority) {
	if t == nil {
		return
	}
	if owner == nil {
		t.SetClassRestamp(nil)
		return
	}
	t.SetClassRestamp(owner.NoteFeatureFootprint)
	if authority != nil {
		t.checkpointMovement = &checkpointMovementReceipt{terrain: t, authority: authority, owner: owner}
	}
}

// CheckpointMovementOwner returns installation metadata only. Concrete owner
// and occupancy-adapter admission belongs to movement, without calling either
// interface. A copied terrain cannot reuse the original terrain's receipt.
func (t *Terrain) CheckpointMovementOwner(authority *checkpoint.BindingAuthority) (FootprintRestampOwner, bool) {
	if t == nil {
		return nil, false
	}
	r := t.checkpointMovement
	if r == nil || r.terrain != t || !r.authority.Matches(authority) {
		return nil, false
	}
	return r.owner, true
}

// SetMovementBindings registers the current installation after movement checks
// its concrete owners. Validation precedes both context writes; conflicts leave
// the existing terrain singleton and receipt unchanged (§16.3.57).
func (c *CheckpointContext) SetMovementBindings(t *Terrain, authority *checkpoint.BindingAuthority) error {
	if c == nil || t == nil || authority == nil {
		return worldCheckpointError("world.context.movementBindings", "a context, exact terrain and binding authority")
	}
	if c.Terrain != nil && c.Terrain != t {
		return worldCheckpointError("world.context.Terrain", "the registered terrain identity")
	}
	if _, ok := t.CheckpointMovementOwner(authority); !ok {
		return worldCheckpointError("world.context.movementBindings", "the exact terrain's current attested movement installation")
	}
	if c.movementBinding != nil && c.movementBinding != t.checkpointMovement {
		return worldCheckpointError("world.context.movementBindings", "one consistent terrain, authority and installation receipt")
	}
	c.Terrain, c.movementBinding = t, t.checkpointMovement
	return nil
}

func (t *Terrain) validateCheckpointMovementBindings(c *CheckpointContext) error {
	if r := c.movementBinding; r != nil {
		if c.Terrain != t || r.terrain != t || r != t.checkpointMovement || r.authority == nil {
			return worldCheckpointError("world.context.movementBindings", "the registered terrain's unchanged movement installation")
		}
		return nil
	}
	if t.classRestamp != nil {
		return worldCheckpointError("world.Terrain.ClassRestamp", "an absent or attested movement binding")
	}
	if t.movers != nil {
		return worldCheckpointError("world.Terrain.Movers", "an absent or attested movement binding")
	}
	return nil
}
