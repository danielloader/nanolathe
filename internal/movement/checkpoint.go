package movement

import (
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/path"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// CheckpointContext owns movement's capture-local graph, borrowing the shared
// lower-owner tables (DESIGN_MULTIPLAYER §16.3.6 and §16.3.14).
type CheckpointContext struct {
	Orders              *orders.CheckpointContext
	Paths               *path.CheckpointContext
	Layers              checkpoint.References[*ClassLayer]
	Payloads            checkpoint.References[GoalPayload]
	pilots              checkpoint.References[any]
	claimRows           checkpoint.References[*checkpointClaimRow]
	moveGoals           checkpoint.References[*moveGoal]
	system              *System
	pathBindings        checkpointPathBindings // capture-local aliases and proof, never payload
	terrainBindings     checkpointTerrainBindings
	auxiliaryBindings   checkpointAuxiliaryBindings
	compositionBindings checkpointCompositionBindings
}

func NewCheckpointContext(o *orders.CheckpointContext, p *path.CheckpointContext) *CheckpointContext {
	return &CheckpointContext{Orders: o, Paths: p}
}

func movementCheckpointError(p string, err error) error {
	return fmt.Errorf("nanolathe: checkpoint owner movement: logical path %s, providers searched [], expected canonical checkpoint: %w", p, err)
}

func (s *System) checkpointContext(c *CheckpointContext) error {
	if s == nil || c == nil || c.Orders == nil || c.Orders.Units == nil || c.Paths == nil {
		return movementCheckpointError("movement", errors.New("missing system or shared context"))
	}
	if c.system != nil && c.system != s {
		return movementCheckpointError("movement", errors.New("context belongs to another system"))
	}
	if c.terrainBindings.authority != nil {
		if err := c.terrainBindings.validate(s); err != nil {
			return movementCheckpointError("movement.terrainBindings", err)
		}
	}
	if c.auxiliaryBindings.authority != nil {
		if err := c.auxiliaryBindings.validate(s); err != nil {
			return movementCheckpointError("movement.auxiliaryBindings", err)
		}
	}
	if c.pathBindings.authority != nil {
		if err := c.pathBindings.validate(s); err != nil {
			return movementCheckpointError("movement.pathBindings", err)
		}
	}
	if err := s.validateCheckpointComposition(c); err != nil {
		return movementCheckpointError("movement.compositionBindings", err)
	}
	return nil
}

func (s *System) validateMovementCheckpoint(c *CheckpointContext) error {
	if s.tickStarted {
		return errors.New("unit sweep is active")
	}
	if s.checkpointSearch != nil {
		return errors.New("pilot search handoff is active")
	}
	// TODO(M3-U6): attest upper-owner and callback composition at session entry.
	// Non-nil values alone do not prove the admitted binding; capture calls none.
	if s.Terrain != nil && c.terrainBindings.authority == nil {
		return errors.New("unattested movement terrain binding")
	}
	if err := s.validateCheckpointOrderHandlers(); err != nil {
		return err
	}
	if err := s.validateCheckpointAirSectors(c); err != nil {
		return err
	}
	if _, err := CheckpointRulesKind(s.Rules); err != nil {
		return err
	}
	if s.world != nil && c.pathBindings.authority == nil && c.auxiliaryBindings.authority == nil {
		return errors.New("unattested movement world binding")
	}
	if _, err := path.CheckpointKernelKind(s.Kernel); err != nil {
		return err
	}
	if s.Grid != nil {
		if err := validateMovementGrid(s, c, s.Grid); err != nil {
			return err
		}
	}
	if s.layerRegistry != nil {
		if err := validateMovementLayers(s, c, s.layerRegistry); err != nil {
			return err
		}
	}
	if s.pathProvider != nil {
		if err := validateMovementProvider(s, s.pathProvider, c); err != nil {
			return err
		}
	}
	return nil
}

func movementCheckpointAdd[T comparable](r *checkpoint.References[T], v T) (int, error) {
	if _, ok := r.Find(v); ok {
		return 0, nil
	}
	_, err := r.Add(v)
	if err != nil {
		return 0, err
	}
	return 1, nil
}

func checkpointMovementGoal(g path.Goal) error {
	if g != nil && path.DescribeGoal(g).Unknown {
		return fmt.Errorf("unsupported or typed-nil path goal %T", g)
	}
	return nil
}

// All reference helpers validate interface variants before generic lookup;
// writers never register an object or execute a callback.
type movementCheckpointRecord struct {
	e *checkpoint.Encoder
	c *CheckpointContext
	s *System
	p string
}

func (m movementCheckpointRecord) field(f string)           { m.e.FieldChild(m.p, f) }
func (m movementCheckpointRecord) fail(f string, err error) { m.field(f); m.e.Fail(err) }
func (m movementCheckpointRecord) boolean(f string, v bool) { m.field(f); m.e.Bool(v) }
func (m movementCheckpointRecord) u8(f string, v uint8)     { m.field(f); m.e.U8(v) }
func (m movementCheckpointRecord) u16(f string, v uint16)   { m.field(f); m.e.U16(v) }
func (m movementCheckpointRecord) u32(f string, v uint32)   { m.field(f); m.e.U32(v) }
func (m movementCheckpointRecord) u64(f string, v uint64)   { m.field(f); m.e.U64(v) }
func (m movementCheckpointRecord) i8(f string, v int8)      { m.field(f); m.e.I8(v) }
func (m movementCheckpointRecord) i16(f string, v int16)    { m.field(f); m.e.I16(v) }
func (m movementCheckpointRecord) i32(f string, v int32)    { m.field(f); m.e.I32(v) }
func (m movementCheckpointRecord) i64(f string, v int64)    { m.field(f); m.e.I64(v) }
func (m movementCheckpointRecord) count(f string, n int)    { m.field(f); m.e.Count(n) }
func (m movementCheckpointRecord) cell(f string, v Cell)    { m.i32(f+".X", v.X); m.i32(f+".Z", v.Z) }
func (m movementCheckpointRecord) vec(f string, v Vec3) {
	m.i64(f+".X", int64(v.X))
	m.i64(f+".Y", int64(v.Y))
	m.i64(f+".Z", int64(v.Z))
}
func (m movementCheckpointRecord) rect(f string, v path.Rect) {
	m.cell(f+".Max", Cell(v.Max))
	m.cell(f+".Min", Cell(v.Min))
}
func (m movementCheckpointRecord) ref(f string, table uint16, id checkpoint.ObjectID, ok bool) {
	m.field(f)
	writeMovementReference(m.e, table, id, ok)
}

// The caller selects either a child or indexed diagnostic path; reference
// refusal and wire framing are identical for both.
func writeMovementReference(e *checkpoint.Encoder, table uint16, id checkpoint.ObjectID, ok bool) {
	if !ok {
		e.Fail(errors.New("undiscovered reference"))
		return
	}
	e.U16(table)
	e.U32(uint32(id))
}
func (m movementCheckpointRecord) unit(f string, v *units.Unit) {
	id, ok := m.c.Orders.Units.Allocations.Find(v)
	m.ref(f, 1, id, ok)
}
func (m movementCheckpointRecord) order(f string, v *orders.Node) {
	id, ok := m.c.Orders.Nodes.Find(v)
	m.ref(f, 3, id, ok)
}
func (m movementCheckpointRecord) ground(f string, v *moveGoal) {
	id, ok := m.c.moveGoals.Find(v)
	m.ref(f, 11, id, ok)
}
func (m movementCheckpointRecord) goal(f string, v path.Goal) {
	if err := checkpointMovementGoal(v); err != nil {
		m.fail(f, err)
		return
	}
	id, ok := m.c.Paths.Goals.Find(v)
	m.ref(f, 9, id, ok)
}
func (m movementCheckpointRecord) payload(f string, v GoalPayload) {
	if err := checkpointMovementPayload(m.s, v); err != nil {
		m.fail(f, err)
		return
	}
	id, ok := m.c.Payloads.Find(v)
	m.ref(f, 8, id, ok)
}
func (m movementCheckpointRecord) pilot(f string, v any) {
	if err := checkpointMovementPilot(v); err != nil {
		m.fail(f, err)
		return
	}
	id, ok := m.c.pilots.Find(v)
	m.ref(f, 6, id, ok)
}
func (m movementCheckpointRecord) row(f string, v *checkpointClaimRow) {
	id, ok := m.c.claimRows.Find(v)
	m.ref(f, 7, id, ok)
}

func writeMovementRows[T any](e *checkpoint.Encoder, c *CheckpointContext, s *System, p string, rows []*T, write func(*checkpoint.Encoder, *CheckpointContext, *System, *T, string)) {
	e.Field(p)
	e.Count(len(rows))
	for i, v := range rows {
		e.FieldIndex(p, i, "")
		e.Bool(v != nil)
		if v != nil {
			q := fmt.Sprintf("%s[%d]", p, i)
			write(e, c, s, v, q)
		}
	}
}
