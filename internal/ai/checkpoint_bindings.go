package ai

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

const (
	checkpointCanPursueAir = iota
	checkpointIsAlliance
	checkpointJammerSuppresses
	checkpointQueueBuildTyped
	checkpointRallyProbeKnown
	checkpointRallyShotTimeAdmits
	checkpointRallyVisible
	checkpointUnitVisible
	checkpointWeaponMaintenance
)

// Proof records are installation metadata, never bytes. Each private function
// keeps its own exact owner and authority; a copied Manager or Strategic must
// be installed independently (DESIGN_MULTIPLAYER §16.3.71).
type checkpointManagerCallbackProof struct {
	owner     *Manager
	authority *checkpoint.BindingAuthority
}

type checkpointStrategicCallbackProof struct {
	owner     *Strategic
	authority *checkpoint.BindingAuthority
}

func (m *Manager) stampCheckpointCallback(slot int, present bool, authority *checkpoint.BindingAuthority) {
	if present && authority != nil {
		m.checkpointCallbacks[slot] = checkpointManagerCallbackProof{m, authority}
	}
}

// This capture-local tuple supplies no gameplay state and emits no bytes.
// Catalog is retained separately so overwriting inputs cannot refresh an old
// expectation. Root owns frozen-input validation and the original session slot;
// it also proves the Session captured by each canonical callback (§16.3.71).
type checkpointManagerBindings struct {
	manager   *Manager
	inputs    *content.SimulationInputs
	catalog   *content.Catalog
	rng       *rng.Simulation
	orders    *orders.QueueBinding
	survival  *SurvivalInfo
	authority *checkpoint.BindingAuthority
}

// SetBindings retains one exact owner tuple, after validating it without
// callbacks, profile application, content preparation or graph discovery.
// Repeats revalidate; failures leave the previous context unchanged (§16.3.71).
func (c *CheckpointContext) SetBindings(m *Manager, inputs *content.SimulationInputs, stream *rng.Simulation, binding *orders.QueueBinding, survival *SurvivalInfo, authority *checkpoint.BindingAuthority) error {
	if c == nil || m == nil || inputs == nil || authority == nil {
		return aiCheckpointError("ai.bindings", "a context, manager, frozen inputs and authority")
	}
	if c.modern != nil && (c.modern.manager != m || !authority.Matches(c.modern.authority)) {
		return aiCheckpointError("ai.bindings", "the Modern registration's manager and authority")
	}
	next := checkpointManagerBindings{m, inputs, inputs.Catalog(), stream, binding, survival, authority}
	if c.bindings != nil && *c.bindings != next {
		return aiCheckpointError("ai.bindings", "the same registered manager and binding tuple")
	}
	candidate := *c
	candidate.bindings = &next
	if err := m.validateCheckpoint(&candidate); err != nil {
		return err
	}
	if c.bindings == nil {
		c.bindings = &next
	}
	return nil
}

type checkpointCallbackState struct {
	field   string
	present bool
}

func (m *Manager) checkpointCallbackStates() [9]checkpointCallbackState {
	return [9]checkpointCallbackState{
		{"CanPursueAir", m.canPursueAir != nil},
		{"IsAlliance", m.isAlliance != nil},
		{"JammerSuppresses", m.jammerSuppresses != nil},
		{"QueueBuildTyped", m.queueBuildTyped != nil},
		{"RallyProbeKnown", m.rallyProbeKnown != nil},
		{"RallyShotTimeAdmits", m.rallyShotTimeAdmits != nil},
		{"RallyVisible", m.rallyVisible != nil},
		{"UnitVisible", m.unitVisible != nil},
		{"WeaponMaintenance", m.weaponMaintenance != nil},
	}
}

func (m *Manager) validateCheckpointBindings(c *CheckpointContext) error {
	b := c.bindings
	if b != nil && b.manager != m {
		return aiCheckpointError("ai.bindings", "the exact registered manager")
	}
	// Only the aliases below change from refused bindings to presence tags.
	// Profile's mutable values remain fully serialized; its memo may be absent
	// after SetDifficulty. Survival data is written once by the scenario owner.
	for _, edge := range [...]struct {
		field string
		valid bool
	}{
		{"Catalog", b == nil && m.Catalog == nil || b != nil && b.inputs.Catalog() == b.catalog && m.Catalog == b.catalog},
		{"OrderBinding", b == nil && m.OrderBinding == nil || b != nil && m.OrderBinding == b.orders},
		{"RNG", b == nil && m.RNG == nil || b != nil && m.RNG == b.rng},
		{"Survival", b == nil && m.Survival == nil || b != nil && m.Survival == b.survival},
		{"Strategic.Catalog", b == nil && m.Strategic.Catalog == nil || b != nil && m.Strategic.Catalog == b.catalog},
		{"Profile.appliedCatalog", m.Profile == nil || m.Profile.appliedCatalog == nil || b != nil && m.Profile.appliedCatalog == b.catalog},
	} {
		if !edge.valid {
			return aiCheckpointError("ai.Manager."+edge.field, "an absent fixture binding or the registered owner alias")
		}
	}
	for i, v := range m.checkpointCallbackStates() {
		p := m.checkpointCallbacks[i]
		if v.present && (b == nil || p.owner != m || !p.authority.Matches(b.authority)) {
			return aiCheckpointError("ai.Manager."+v.field, "the exact callback installation owner and authority")
		}
	}
	for i, v := range [...]checkpointCallbackState{
		{"energyEnvironment", m.Strategic.energyEnvironment != nil},
		{"rebuildRegistry", m.Strategic.rebuildRegistry != nil},
	} {
		p := m.Strategic.checkpointCallbacks[i]
		if v.present && (b == nil || p.owner != &m.Strategic || !p.authority.Matches(b.authority)) {
			return aiCheckpointError("ai.Manager.Strategic."+v.field, "the exact callback installation owner and authority")
		}
	}
	return nil
}
