package combat

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Every Projectile field is retained below in lexical order, including the
// shadow Dead, raw State69, orientation and cached floor word. Residual rows
// survive reservation and stale raw links can address them [06 §5.1–§5.2]
// [06 R-DMG-01 §13–§14]. The floor and visual words are deliberately conservative
// U0 inclusions. Raw handles use u32; WeaponID remains its raw i32, never a key.
func writeCheckpointProjectile(e *checkpoint.Encoder, p *Projectile, path string) {
	e.FieldChild(path, "AutomaticAttackBurst")
	e.Bool(p.AutomaticAttackBurst)
	e.FieldChild(path, "BeamLatch")
	e.Bool(p.BeamLatch)
	e.FieldChild(path, "BurstDeadline")
	e.U32(p.BurstDeadline)
	e.FieldChild(path, "BurstRemaining")
	e.I32(p.BurstRemaining)
	e.FieldChild(path, "CacheCellX")
	e.I32(p.CacheCellX)
	e.FieldChild(path, "CacheCellZ")
	e.I32(p.CacheCellZ)
	e.FieldChild(path, "CachedFloorHeight")
	e.I16(p.CachedFloorHeight)
	e.FieldChild(path, "CreationTick")
	e.U32(p.CreationTick)
	e.FieldChild(path, "Dead")
	e.Bool(p.Dead)
	e.FieldChild(path, "ExpiryTick")
	e.U32(p.ExpiryTick)
	e.FieldChild(path, "GroundAttackBurst")
	e.Bool(p.GroundAttackBurst)
	e.FieldChild(path, "MeteorPitch")
	e.U16(uint16(p.MeteorPitch))
	e.FieldChild(path, "MuzzlePiece")
	e.I16(p.MuzzlePiece)
	e.FieldChild(path, "OldMarker")
	e.I16(p.OldMarker)
	e.FieldChild(path, "OrderedBurst")
	e.Bool(p.OrderedBurst)
	e.FieldChild(path, "Pitch")
	e.U16(uint16(p.Pitch))
	writeCheckpointVec3(e, p.Pos, path+".Pos")
	e.FieldChild(path, "PropellerYaw")
	e.U16(uint16(p.PropellerYaw))
	e.FieldChild(path, "Roll")
	e.U16(uint16(p.Roll))
	e.FieldChild(path, "Shooter")
	e.U32(uint32(p.Shooter))
	e.FieldChild(path, "ShooterSide")
	e.U8(p.ShooterSide)
	e.FieldChild(path, "SmokeDeadline")
	e.U32(p.SmokeDeadline)
	e.FieldChild(path, "Speed")
	e.I64(int64(p.Speed))
	writeCheckpointVec3(e, p.StartPos, path+".StartPos")
	e.FieldChild(path, "State69")
	e.U8(p.State69)
	e.FieldChild(path, "StoredPlanarDistance")
	e.I64(int64(p.StoredPlanarDistance))
	writeCheckpointVec3(e, p.TargetPos, path+".TargetPos")
	e.FieldChild(path, "TargetProjectile")
	e.U32(uint32(p.TargetProjectile))
	e.FieldChild(path, "TargetUnit")
	e.U32(uint32(p.TargetUnit))
	e.FieldChild(path, "TwoPhase")
	e.Bool(p.TwoPhase)
	writeCheckpointVec3(e, p.Velocity, path+".Velocity")
	e.FieldChild(path, "WeaponID")
	e.I32(p.WeaponID)
	e.FieldChild(path, "Yaw")
	e.U16(uint16(p.Yaw))
}

func writeCheckpointVec3(e *checkpoint.Encoder, v Vec3, path string) {
	e.FieldChild(path, "X")
	e.I64(int64(v.X))
	e.FieldChild(path, "Y")
	e.I64(int64(v.Y))
	e.FieldChild(path, "Z")
	e.I64(int64(v.Z))
}

func writeCheckpointUnitRef(e *checkpoint.Encoder, c *CheckpointContext, u *units.Unit, path string) {
	e.Field(path)
	id, known := c.Units.Allocations.Find(u)
	if !known {
		e.Fail(combatCheckpointError(path, "a discovered allocation reference"))
		return
	}
	e.U16(1)
	e.U32(uint32(id))
}

// hasTick, lastTick, passengers are retained; pending is checked empty before
// writing. Passenger handle, health, typeID, unit preserve the captured snapshot
// even when today's allocation disagrees. Capture never invokes Tick (CP-DMG-3).
func (s *TransportDeathState) writeCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) {
	e.Field("combat.Service.TransportDeaths.hasTick")
	e.Bool(s.hasTick)
	e.Field("combat.Service.TransportDeaths.lastTick")
	e.U32(s.lastTick)
	e.Field("combat.Service.TransportDeaths.passengers")
	e.Count(len(s.passengers))
	for i, row := range s.passengers {
		path := fmt.Sprintf("combat.Service.TransportDeaths.passengers[%d]", i)
		e.FieldChild(path, "handle")
		e.U32(uint32(row.handle))
		e.FieldChild(path, "health")
		e.I32(row.health)
		e.FieldChild(path, "typeID")
		e.U32(row.typeID)
		writeCheckpointUnitRef(e, c, row.unit, path+".unit")
	}
}

// Incoming fields: beamInvalid, motion (heading, mode, speed, velocity), shooter,
// target, weapon. Actual weapon pointers have presence plus admitted identity;
// nil allocation references use table 1 with object zero (§16.3.6).
func writeCheckpointIncoming(e *checkpoint.Encoder, c *CheckpointContext, row incomingShot, path string) {
	e.FieldChild(path, "beamInvalid")
	e.Bool(row.beamInvalid)
	e.FieldChild(path, "motion.heading")
	e.U16(row.motion.heading)
	e.FieldChild(path, "motion.mode")
	e.U8(row.motion.mode)
	e.FieldChild(path, "motion.speed")
	e.I64(int64(row.motion.speed))
	writeCheckpointVec3(e, row.motion.velocity, path+".motion.velocity")
	writeCheckpointUnitRef(e, c, row.shooter, path+".shooter")
	writeCheckpointUnitRef(e, c, row.target, path+".target")
	e.FieldChild(path, "weapon")
	e.Bool(row.weapon != nil)
	if row.weapon != nil {
		ref, err := c.Units.Keys.Weapon(row.weapon)
		if err != nil {
			e.Fail(err)
			return
		}
		e.Definition(ref)
	}
}

func writeCheckpointHandles(e *checkpoint.Encoder, row []pool.Handle, path string) {
	e.Field(path)
	e.Count(len(row))
	for _, h := range row {
		e.U32(uint32(h))
	}
}
