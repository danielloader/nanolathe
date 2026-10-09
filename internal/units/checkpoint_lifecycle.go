package units

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// Lifecycle proof is attached to one private world/slot installation. It is
// never inferred from function addresses (DESIGN_MULTIPLAYER §16.3.46).
type checkpointLifecycleProof struct {
	owner     *World
	authority *checkpoint.BindingAuthority
}

// DeathHook returns the current callback without transferring checkpoint proof.
func (w *World) DeathHook() DeathHook { return w.onDeath }

// SetDeathHook installs an ordinary callback and clears this slot's proof.
func (w *World) SetDeathHook(fn DeathHook) {
	w.onDeath = fn
	w.checkpointLifecycle[0] = checkpointLifecycleProof{}
}

// SetDeathHookWithCheckpointBinding installs a reviewed canonical binding.
// The session keeps its authority private; capture still checks exact ownership.
func (w *World) SetDeathHookWithCheckpointBinding(fn DeathHook, authority *checkpoint.BindingAuthority) {
	w.SetDeathHook(fn)
	if fn != nil && authority != nil {
		w.checkpointLifecycle[0] = checkpointLifecycleProof{owner: w, authority: authority}
	}
}

// DeathExtraHook returns the current callback without transferring checkpoint proof.
func (w *World) DeathExtraHook() DeathHook { return w.onDeathExtra }

// SetDeathExtraHook installs an ordinary callback and clears this slot's proof.
func (w *World) SetDeathExtraHook(fn DeathHook) {
	w.onDeathExtra = fn
	w.checkpointLifecycle[1] = checkpointLifecycleProof{}
}

// SetDeathExtraHookWithCheckpointBinding installs a reviewed canonical binding.
// The session keeps its authority private; capture still checks exact ownership.
func (w *World) SetDeathExtraHookWithCheckpointBinding(fn DeathHook, authority *checkpoint.BindingAuthority) {
	w.SetDeathExtraHook(fn)
	if fn != nil && authority != nil {
		w.checkpointLifecycle[1] = checkpointLifecycleProof{owner: w, authority: authority}
	}
}

// CreateHook returns the current callback without transferring checkpoint proof.
func (w *World) CreateHook() CreateHook { return w.onCreate }

// SetCreateHook installs an ordinary callback and clears this slot's proof.
func (w *World) SetCreateHook(fn CreateHook) {
	w.onCreate = fn
	w.checkpointLifecycle[2] = checkpointLifecycleProof{}
}

// SetCreateHookWithCheckpointBinding installs a reviewed canonical binding.
// The session keeps its authority private; capture still checks exact ownership.
func (w *World) SetCreateHookWithCheckpointBinding(fn CreateHook, authority *checkpoint.BindingAuthority) {
	w.SetCreateHook(fn)
	if fn != nil && authority != nil {
		w.checkpointLifecycle[2] = checkpointLifecycleProof{owner: w, authority: authority}
	}
}

// CaptureHook returns the current callback without transferring checkpoint proof.
func (w *World) CaptureHook() CaptureHook { return w.onCapture }

// SetCaptureHook installs an ordinary callback and clears this slot's proof.
func (w *World) SetCaptureHook(fn CaptureHook) {
	w.onCapture = fn
	w.checkpointLifecycle[3] = checkpointLifecycleProof{}
}

// SetCaptureHookWithCheckpointBinding installs a reviewed canonical binding.
// The session keeps its authority private; capture still checks exact ownership.
func (w *World) SetCaptureHookWithCheckpointBinding(fn CaptureHook, authority *checkpoint.BindingAuthority) {
	w.SetCaptureHook(fn)
	if fn != nil && authority != nil {
		w.checkpointLifecycle[3] = checkpointLifecycleProof{owner: w, authority: authority}
	}
}

// SetLifecycleBindings records the expected world for this capture only.
func (c *CheckpointContext) SetLifecycleBindings(w *World, authority *checkpoint.BindingAuthority) error {
	if c == nil || w == nil || authority == nil {
		return unitCheckpointError("units.lifecycle", "a context, exact world and binding authority")
	}
	if c.lifecycleWorld != nil && (c.lifecycleWorld != w || !c.lifecycleAuthority.Matches(authority)) {
		return unitCheckpointError("units.lifecycle", "one consistent world and authority")
	}
	if c.worldBindings.world != nil && (c.worldBindings.world != w || !c.worldBindings.authority.Matches(authority)) {
		return unitCheckpointError("units.lifecycle", "the same world and authority as the world bindings")
	}
	c.lifecycleWorld, c.lifecycleAuthority = w, authority
	return nil
}

func (w *World) validateCheckpointLifecycle(c *CheckpointContext) error {
	if c.lifecycleWorld != nil && c.lifecycleWorld != w {
		return unitCheckpointError("units.lifecycle", "the registered world")
	}
	present := [4]bool{w.onDeath != nil, w.onDeathExtra != nil, w.onCreate != nil, w.onCapture != nil}
	names := [4]string{"OnDeath", "OnDeathExtra", "OnCreate", "OnCapture"}
	for i, set := range present {
		proof := w.checkpointLifecycle[i]
		if set && (c.lifecycleWorld != w || proof.owner != w || !proof.authority.Matches(c.lifecycleAuthority)) {
			return unitCheckpointError("units.World."+names[i], "the exact world's attested lifecycle binding")
		}
	}
	return nil
}
