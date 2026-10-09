package movement

import (
	"errors"
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func checkpointMovementPayload(s *System, p GoalPayload) error {
	var owner *System
	switch v := p.(type) {
	case nil:
		return nil
	case *airMarker:
		if v == nil {
			return errors.New("typed-nil air marker")
		}
		owner = v.sys
	case *airVelocityMarker:
		if v == nil {
			return errors.New("typed-nil air velocity marker")
		}
		owner = v.sys
	default:
		return fmt.Errorf("unsupported goal payload %T", p)
	}
	if owner != nil && owner != s {
		return errors.New("goal payload system alias differs")
	}
	return nil
}

func collectMovementPayload(c *CheckpointContext, s *System, p GoalPayload) (int, error) {
	if err := checkpointMovementPayload(s, p); err != nil {
		return 0, err
	}
	var u *units.Unit
	switch v := p.(type) {
	case *airMarker:
		u = v.unit
	case *airVelocityMarker:
		u = v.unit
	}
	return movementCheckpointAdd(&c.Orders.Units.Allocations, u)
}

// Marker: altOffset, attachPiece, flags, goal, heading, radial, radius, sys,
// target, unit. Velocity: commanded, pos, savedAux, savedFlags, savedTrailing,
// steer, sys, unit, vel. Fixed fields keep i64; raw target keeps u32.
func writeMovementPayload(e *checkpoint.Encoder, c *CheckpointContext, s *System, payload GoalPayload, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	if err := checkpointMovementPayload(s, payload); err != nil {
		m.fail("", err)
		return
	}
	switch v := payload.(type) {
	case *airMarker:
		e.U8(1)
		m.i16("altOffset", v.altOffset)
		m.u16("attachPiece", v.attachPiece)
		m.u16("flags", v.flags)
		m.vec("goal", v.goal)
		m.u16("heading", v.heading)
		m.i64("radial", int64(v.radial))
		m.u16("radius", v.radius)
		m.boolean("sys", v.sys != nil)
		m.u32("target", uint32(v.target))
		m.unit("unit", v.unit)
	case *airVelocityMarker:
		e.U8(2)
		m.u16("commanded", v.commanded)
		m.vec("pos", v.pos)
		m.u16("savedAux", v.savedAux)
		m.u16("savedFlags", v.savedFlags)
		m.u16("savedTrailing", v.savedTrailing)
		m.boolean("steer", v.steer)
		m.boolean("sys", v.sys != nil)
		m.unit("unit", v.unit)
		m.vec("vel", v.vel)
	default:
		m.fail("", errors.New("absent payload table record"))
	}
}
