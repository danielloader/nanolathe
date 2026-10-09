package movement

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Profile fields: BadSlope, BadWaterSlope, FootPrintX, FootPrintZ, MaxSlope, MaxWaterDepth, MaxWaterSlope, MinWaterDepth.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementProfile(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *Profile, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u8("BadSlope", v.BadSlope)
	m.u8("BadWaterSlope", v.BadWaterSlope)
	m.i16("FootPrintX", v.FootPrintX)
	m.i16("FootPrintZ", v.FootPrintZ)
	m.u8("MaxSlope", v.MaxSlope)
	m.i32("MaxWaterDepth", v.MaxWaterDepth)
	m.u8("MaxWaterSlope", v.MaxWaterSlope)
	m.i32("MinWaterDepth", v.MinWaterDepth)
}

// Route fields: Active, Count, Dirty, LastRequestTick, Points, Status, WantsRepath, firstHold, firstPending.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementRoute(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *Route, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.boolean("Active", v.Active)
	m.u8("Count", v.Count)
	m.boolean("Dirty", v.Dirty)
	m.u32("LastRequestTick", v.LastRequestTick)
	for i, x := range v.Points {
		m.cell(fmt.Sprintf("Points[%d]", i), Cell(x))
	}
	m.u32("Status", uint32(v.Status))
	m.boolean("WantsRepath", v.WantsRepath)
	m.u32("firstHold", v.firstHold)
	m.boolean("firstPending", v.firstPending)
}

// SteerState fields: Acceleration, BrakeRate, DefFlags, Dirty, Heading, HeightWord, MaxVelocity, PendingHeading, SeaLevel, Speed, TurnRate, X, Z.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementSteerState(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *SteerState, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.i32("Acceleration", v.Acceleration)
	m.i32("BrakeRate", v.BrakeRate)
	m.u32("DefFlags", v.DefFlags)
	m.boolean("Dirty", v.Dirty)
	m.u16("Heading", v.Heading)
	m.i16("HeightWord", v.HeightWord)
	m.i32("MaxVelocity", v.MaxVelocity)
	m.u16("PendingHeading", v.PendingHeading)
	m.u8("SeaLevel", v.SeaLevel)
	m.i32("Speed", v.Speed)
	m.i32("TurnRate", v.TurnRate)
	m.i32("X", v.X)
	m.i32("Z", v.Z)
}

// CollisionState fields: Blocked, BlockerID, Building, CachedAnchor, CachedMode, Dirty, Filing, FootPrintX, FootPrintZ, HasStamp, Heading, ID, LastProposalTick, LastStampTick, LeanX, LeanY, LeanZ, MaxVelocity, Mode, OldAnchor, SavedStateByte, Speed, StampedAnchor, StampedPlane, TurnResidual, VX, VY, VZ, X, Y, Yard, YardOpen, Z, airOffMap, airSector, halfBiasSet, halfBiasX, halfBiasZ.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementCollisionState(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *CollisionState, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.boolean("Blocked", v.Blocked)
	m.i64("BlockerID", int64(v.BlockerID))
	m.boolean("Building", v.Building)
	m.cell("CachedAnchor", v.CachedAnchor)
	m.u8("CachedMode", v.CachedMode)
	m.boolean("Dirty", v.Dirty)
	writeMovementSectorFiling(e, c, s, &v.Filing, p+".Filing")
	m.i16("FootPrintX", v.FootPrintX)
	m.i16("FootPrintZ", v.FootPrintZ)
	m.boolean("HasStamp", v.HasStamp)
	m.u16("Heading", v.Heading)
	m.i64("ID", int64(v.ID))
	m.u32("LastProposalTick", v.LastProposalTick)
	m.u32("LastStampTick", v.LastStampTick)
	m.i32("LeanX", v.LeanX)
	m.i32("LeanY", v.LeanY)
	m.i32("LeanZ", v.LeanZ)
	m.i32("MaxVelocity", v.MaxVelocity)
	m.u8("Mode", v.Mode)
	m.cell("OldAnchor", v.OldAnchor)
	m.u8("SavedStateByte", v.SavedStateByte)
	m.i32("Speed", v.Speed)
	m.cell("StampedAnchor", v.StampedAnchor)
	m.u8("StampedPlane", uint8(v.StampedPlane))
	m.i16("TurnResidual", v.TurnResidual)
	m.i32("VX", v.VX)
	m.i32("VY", v.VY)
	m.i32("VZ", v.VZ)
	m.i32("X", v.X)
	m.i32("Y", v.Y)
	m.count("Yard", len(v.Yard))
	for _, x := range v.Yard {
		e.U8(uint8(x))
	}
	m.boolean("YardOpen", v.YardOpen)
	m.i32("Z", v.Z)
	m.boolean("airOffMap", v.airOffMap)
	writeMovementAirSector(e, s, v.airSector, p+".airSector")
	m.boolean("halfBiasSet", v.halfBiasSet)
	m.i32("halfBiasX", v.halfBiasX)
	m.i32("halfBiasZ", v.halfBiasZ)
}

// SectorFiling fields: Filed, OffMap, SX, SZ, Seq.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementSectorFiling(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *SectorFiling, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.boolean("Filed", v.Filed)
	m.boolean("OffMap", v.OffMap)
	m.i32("SX", v.SX)
	m.i32("SZ", v.SZ)
	m.u64("Seq", v.Seq)
}

// FlightState fields: Acceleration, Bank, BankScale, BrakeRate, Command, Dirty, Gravity, Heading, LeanX, LeanY, LeanZ, MaxVelocity, Mode, ModeMirror, OffMap, Pitch, PitchScale, Speed, TargetHeading, TargetVX, TargetVZ, TargetX, TargetY, TargetZ, TurnRate, TurnResidual, Unit, VX, VY, VZ, X, Y, Z.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementFlightState(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *FlightState, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.i32("Acceleration", v.Acceleration)
	m.u16("Bank", v.Bank)
	m.i32("BankScale", v.BankScale)
	m.i32("BrakeRate", v.BrakeRate)
	m.boolean("Command", v.Command != nil)
	if v.Command != nil {
		writeMovementFlightCommand(e, c, s, v.Command, p+".Command")
	}
	m.boolean("Dirty", v.Dirty)
	m.i32("Gravity", v.Gravity)
	m.u16("Heading", v.Heading)
	m.i32("LeanX", v.LeanX)
	m.i32("LeanY", v.LeanY)
	m.i32("LeanZ", v.LeanZ)
	m.i32("MaxVelocity", v.MaxVelocity)
	m.u8("Mode", v.Mode)
	m.u8("ModeMirror", v.ModeMirror)
	m.boolean("OffMap", v.OffMap)
	m.u16("Pitch", v.Pitch)
	m.i32("PitchScale", v.PitchScale)
	m.i32("Speed", v.Speed)
	m.u16("TargetHeading", v.TargetHeading)
	m.i32("TargetVX", v.TargetVX)
	m.i32("TargetVZ", v.TargetVZ)
	m.i32("TargetX", v.TargetX)
	m.i32("TargetY", v.TargetY)
	m.i32("TargetZ", v.TargetZ)
	m.i32("TurnRate", v.TurnRate)
	m.i16("TurnResidual", v.TurnResidual)
	m.unit("Unit", v.Unit)
	m.i32("VX", v.VX)
	m.i32("VY", v.VY)
	m.i32("VZ", v.VZ)
	m.i32("X", v.X)
	m.i32("Y", v.Y)
	m.i32("Z", v.Z)
}

// FlightCommand fields: Heading, Payload, Pos, Unit, Vel, payloadOwner.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementFlightCommand(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *FlightCommand, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u16("Heading", v.Heading)
	m.payload("Payload", v.Payload)
	m.vec("Pos", v.Pos)
	m.unit("Unit", v.Unit)
	m.vec("Vel", v.Vel)
	m.order("payloadOwner", v.payloadOwner)
}

// activeMove fields: order, token.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementActiveMove(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *activeMove, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.order("order", v.order)
	m.u64("token", v.token)
}

// arrivalHandle fields: border, goalX, goalZ, order, payload, threshSq.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementArrivalHandle(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *arrivalHandle, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.boolean("border", v.border != nil)
	if v.border != nil {
		m.rect("border", *v.border)
	}
	m.i32("goalX", v.goalX)
	m.i32("goalZ", v.goalZ)
	m.order("order", v.order)
	m.goal("payload", v.payload)
	m.i32("threshSq", v.threshSq)
}

// moveGoal fields: goal, order, x, z.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementMoveGoal(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *moveGoal, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.goal("goal", v.goal)
	m.order("order", v.order)
	m.i64("x", int64(v.x))
	m.i64("z", int64(v.z))
}

// recordGoal fields: air, ground, node.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementRecordGoal(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *recordGoal, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.payload("air", v.air)
	m.ground("ground", v.ground)
	m.order("node", v.node)
}

// modernClearanceRoute fields: cells, order.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementModernClearanceRoute(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *modernClearanceRoute, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.count("cells", len(v.cells))
	for i, x := range v.cells {
		m.cell(fmt.Sprintf("cells[%d]", i), x)
	}
	m.order("order", v.order)
}

// unreachableCert fields: activation, goal, order, since.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementUnreachableCert(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *unreachableCert, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u64("activation", v.activation)
	m.goal("goal", v.goal)
	m.order("order", v.order)
	m.u32("since", v.since)
}

// jamRelease fields: cooldown, limit, pocket, replan, run, until.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementJamRelease(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *jamRelease, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u32("cooldown", v.cooldown)
	m.u32("limit", v.limit)
	m.boolean("pocket", v.pocket)
	m.boolean("replan", v.replan)
	m.u16("run", v.run)
	m.u32("until", v.until)
}

// trafficState fields: ahead, goalX, goalZ, hasGoal, round, routeless, routelessFrom, side, sideUntil, steering, through.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementTrafficState(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *trafficState, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u8("ahead", v.ahead)
	m.i64("goalX", int64(v.goalX))
	m.i64("goalZ", int64(v.goalZ))
	m.boolean("hasGoal", v.hasGoal)
	m.u16("round", v.round)
	m.boolean("routeless", v.routeless)
	m.u32("routelessFrom", v.routelessFrom)
	m.i8("side", v.side)
	m.u32("sideUntil", v.sideUntil)
	m.boolean("steering", v.steering)
	m.u8("through", v.through)
}

// pocketCert fields: grants, order, since, token.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementPocketCert(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *pocketCert, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u8("grants", v.grants)
	m.order("order", v.order)
	m.u32("since", v.since)
	m.u64("token", v.token)
}

// repairLanding fields: anchor, holding, node, pad, piece, reserved, unit.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementRepairLanding(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *repairLanding, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.vec("anchor", v.anchor)
	m.boolean("holding", v.holding)
	m.order("node", v.node)
	m.unit("pad", v.pad)
	m.u16("piece", v.piece)
	m.boolean("reserved", v.reserved)
	m.unit("unit", v.unit)
}

// arriveRow fields: bestAt, bestD, exchanges, fx, fz, member, nextCheck, node, place, seen, stood, stoodAt, stoodX, stoodZ, x, z.
// DESIGN_MULTIPLAYER §16.3.5–§16.3.6; all values are read directly.
func writeMovementArriveRow(e *checkpoint.Encoder, c *CheckpointContext, s *System, v *arriveRow, p string) {
	m := movementCheckpointRecord{e, c, s, p}
	m.u32("bestAt", v.bestAt)
	m.i64("bestD", v.bestD)
	m.u8("exchanges", v.exchanges)
	m.i32("fx", v.fx)
	m.i32("fz", v.fz)
	m.u32("member", v.member)
	m.u32("nextCheck", v.nextCheck)
	m.order("node", v.node)
	m.cell("place", v.place)
	m.order("seen", v.seen)
	m.boolean("stood", v.stood)
	m.u32("stoodAt", v.stoodAt)
	m.i32("stoodX", v.stoodX)
	m.i32("stoodZ", v.stoodZ)
	m.i64("x", int64(v.x))
	m.i64("z", int64(v.z))
}
