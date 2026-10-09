package ai

import (
	"reflect"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointModernPlanner is an opaque, generated assertion for the registered
// stateless Modern step. Its identity and assertion are metadata, never bytes
// (DESIGN_MULTIPLAYER §16.3.44, §16.3.75).
type CheckpointModernPlanner struct {
	witness *checkpointModernPlannerWitness
}

type checkpointModernPlannerWitness struct {
	matches func(Planner) bool
}

// NewCheckpointModernPlanner checks the concrete zero-size value at registration
// only. Capture uses a type assertion and never invokes planner/marker methods.
func NewCheckpointModernPlanner[T ModernAIStep](step T) CheckpointModernPlanner {
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Struct || typ.Size() != 0 || typ != reflect.TypeOf(step) {
		panic(aiCheckpointError("ai.ModernAIStep", "a concrete zero-size struct type"))
	}
	return CheckpointModernPlanner{&checkpointModernPlannerWitness{matches: func(p Planner) bool {
		_, ok := p.(T)
		return ok
	}}}
}

func (w CheckpointModernPlanner) Matches(p Planner) bool {
	return w.witness != nil && w.witness.matches(p)
}

// CheckpointControllerOwner is implemented by the reviewed concrete controller.
// Implementing it alone grants no admission: a source must narrow to its exact
// generated pointer type before dispatch (DESIGN_MULTIPLAYER §16.3.75).
type CheckpointControllerOwner interface {
	ControllerCheckpointProvider
	EnableCheckpointApplications(checkpoint.Identity, *content.CheckpointKeys) error
	ValidateCheckpointBindings(*Manager, *CheckpointContext) error
}

// CheckpointControllerSource seals generated type adapters, accepting no
// application callbacks. Copies preserve source identity; another factory call
// produces a distinct source even for the same concrete type.
type CheckpointControllerSource struct {
	adapters *checkpointControllerAdapters
}

type checkpointControllerAdapters struct {
	owner   func(any) CheckpointControllerOwner
	matches func(any, any) bool
}

func NewCheckpointControllerSource[T any, P interface {
	*T
	CheckpointControllerOwner
}]() CheckpointControllerSource {
	return CheckpointControllerSource{&checkpointControllerAdapters{
		owner: func(v any) CheckpointControllerOwner {
			p, ok := v.(P)
			if !ok || p == nil {
				return nil
			}
			return p
		},
		matches: func(actual, expected any) bool {
			a, ok := actual.(P)
			b, expectedOK := expected.(P)
			return ok && expectedOK && a != nil && a == b
		},
	}}
}

func (s CheckpointControllerSource) Valid() bool { return s.adapters != nil }

func (s CheckpointControllerSource) controller(m *Manager) (CheckpointControllerOwner, error) {
	if !s.Valid() || m == nil {
		return nil, aiCheckpointError("ai.Manager.Ext", "a manager and generated controller source")
	}
	owner := s.adapters.owner(m.Ext)
	if owner == nil {
		return nil, aiCheckpointError("ai.Manager.Ext", "the present concrete controller pointer")
	}
	return owner, nil
}

// EnableApplications is an entry-time operation. An absent controller uses the
// existing manager lifecycle; a present one uses only the closed concrete owner.
func (s CheckpointControllerSource) EnableApplications(m *Manager, identity checkpoint.Identity, keys *content.CheckpointKeys) error {
	if !s.Valid() || m == nil || keys == nil || m.Controller != ControllerModern {
		return aiCheckpointError("ai.ModernApplications", "a Modern manager, admitted keys and generated controller source")
	}
	if m.Ext == nil {
		return m.EnableCheckpointApplications(identity, keys)
	}
	owner, err := s.controller(m)
	if err != nil {
		return err
	}
	return owner.EnableCheckpointApplications(identity, keys)
}

// WriteCheckpoint preflights direct aliases and concrete controller bindings
// before emitting the separate controller fragment. No provider is discovered
// by interface capability and no producer or worker is consulted.
func (s CheckpointControllerSource) WriteCheckpoint(m *Manager, e *checkpoint.Encoder, c *CheckpointContext) error {
	if e == nil {
		return aiCheckpointError("ai.controller.encoder", "a checkpoint encoder")
	}
	if c == nil || c.modern == nil || c.modern.source != s {
		e.Fail(aiCheckpointError("ai.modern.bindings", "the exact registered controller source"))
		return e.Err()
	}
	if err := c.ValidateModernManager(m); err != nil {
		e.Fail(err)
		return e.Err()
	}
	owner, err := s.controller(m)
	if err == nil {
		err = owner.ValidateCheckpointBindings(m, c)
	}
	if err != nil {
		e.Fail(err)
		return e.Err()
	}
	return owner.WriteControllerCheckpoint(e, c)
}

// AppendSummary only narrows the concrete owner. The owner's selected summary
// must remain independent of full binding validation, tables and worker state.
func (s CheckpointControllerSource) AppendSummary(m *Manager, summary *checkpoint.Summary) error {
	if summary == nil {
		return aiCheckpointError("ai.controller.summary", "a checkpoint summary")
	}
	owner, err := s.controller(m)
	if err != nil {
		return err
	}
	return owner.AppendControllerCheckpointSummary(summary)
}

// This one-manager tuple retains expectations rather than refreshing them at
// capture. The planner is a zero-size value accepted by the opaque witness;
// owner comparisons narrow to the source's pointer type before equality. None
// of these proof values is serialized (DESIGN_MULTIPLAYER §16.3.75).
type checkpointModernBindings struct {
	manager   *Manager
	authority *checkpoint.BindingAuthority
	planner   Planner
	witness   CheckpointModernPlanner
	source    CheckpointControllerSource
	owner     any
	history   *ApplicationHistory
	keys      *content.CheckpointKeys
}

func (b *checkpointModernBindings) same(next *checkpointModernBindings) bool {
	if b.manager != next.manager || b.authority != next.authority || b.witness != next.witness ||
		b.source != next.source || b.history != next.history || b.keys != next.keys {
		return false
	}
	// No interface equality: even a zero-size struct can be noncomparable.
	if !b.witness.Matches(b.planner) || !b.witness.Matches(next.planner) {
		return false
	}
	if b.owner == nil || next.owner == nil {
		return b.owner == nil && next.owner == nil
	}
	return b.source.Valid() && b.source.adapters.matches(b.owner, next.owner)
}

// SetModernBindings may precede Classic binding registration. The staged tuple
// is visible to the concrete validator but is published only after success.
func (c *CheckpointContext) SetModernBindings(m *Manager, planner Planner, witness CheckpointModernPlanner, source CheckpointControllerSource, authority *checkpoint.BindingAuthority) error {
	if c == nil || m == nil || authority == nil || !source.Valid() {
		return aiCheckpointError("ai.modern.bindings", "a context, manager, generated source and authority")
	}
	next := checkpointModernBindings{m, authority, planner, witness, source, m.Ext, m.checkpointHistory, m.checkpointKeys}
	if c.modern != nil && !c.modern.same(&next) {
		return aiCheckpointError("ai.modern.bindings", "the same registered Modern manager tuple")
	}
	candidate := *c
	candidate.modern = &next
	if err := candidate.validateModernBindings(m); err != nil {
		return err
	}
	if c.modern == nil {
		c.modern = &next
	}
	return nil
}

// ValidateModernManager checks direct aliases only. The concrete controller's
// validator calls this method, so it must not dispatch back to that controller.
func (c *CheckpointContext) ValidateModernManager(m *Manager) error {
	if c == nil || c.modern == nil || m == nil {
		return aiCheckpointError("ai.modern.bindings", "an exact registered Modern manager")
	}
	b := c.modern
	if b.manager != m || b.authority == nil || m.Controller != ControllerModern || m.Player >= 10 {
		return aiCheckpointError("ai.modern.bindings", "the exact Modern manager, player and authority")
	}
	if c.bindings != nil && (c.bindings.manager != m || !b.authority.Matches(c.bindings.authority)) {
		return aiCheckpointError("ai.modern.bindings", "the Classic registration's manager and authority")
	}
	if !b.witness.Matches(b.planner) || !b.witness.Matches(m.Planner) {
		return aiCheckpointError("ai.Manager.Planner", "the registered zero-size Modern step")
	}
	if c.Units == nil || c.World == nil || b.keys == nil || b.keys != c.Units.Keys || b.keys != c.World.Keys || b.keys != m.checkpointKeys {
		return aiCheckpointError("ai.modern.keys", "the original shared admitted keys")
	}
	if b.history == nil || b.history != m.checkpointHistory {
		return aiCheckpointError("ai.modern.history", "the original enabled application history")
	}
	state, err := b.history.Snapshot()
	if err != nil {
		return err
	}
	if !state.Enabled || state.Player != m.Player || state.Kind != 2 {
		return aiCheckpointError("ai.modern.history", "successful inactive history for this Modern player")
	}
	if !b.source.Valid() || (b.owner == nil && m.Ext != nil) ||
		(b.owner != nil && !b.source.adapters.matches(m.Ext, b.owner)) {
		return aiCheckpointError("ai.Manager.Ext", "the original optional concrete controller pointer")
	}
	return nil
}

func (c *CheckpointContext) validateModernBindings(m *Manager) error {
	if err := c.ValidateModernManager(m); err != nil {
		return err
	}
	if m.Ext == nil {
		return nil
	}
	owner, err := c.modern.source.controller(m)
	if err != nil {
		return err
	}
	return owner.ValidateCheckpointBindings(m, c)
}
