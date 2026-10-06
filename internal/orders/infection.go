package orders

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// InfectionPolicy is the original takeover attack selected by the bound rule
// set. Zero disables it. Range is a 16.16 centre-to-centre distance, bounded to
// an unsigned 32-bit fixed-point word. See DESIGN_UNITS_ORDERS_COB "Modern infection".
type InfectionPolicy struct {
	DurationTicks uint32
	Range         numeric.Fixed
}

// Infection reads the catalog's compiled NANOLATHE_INFECTOR flag; nothing is
// tokenized per query.
func (*ModernRules) Infection(def *content.UnitDef) InfectionPolicy {
	if def == nil || def.BMCode != 1 || !def.CanMove || !def.NanolatheInfector {
		return InfectionPolicy{}
	}
	return InfectionPolicy{DurationTicks: 60, Range: numeric.FixedFromInt(96)}
}

// unitInfection is the bound policy of a live unit's own definition.
func unitInfection(u *units.Unit) InfectionPolicy {
	if u == nil {
		return InfectionPolicy{}
	}
	return rulesOfUnit(u).Infection(u.Def)
}

// InfectionTarget checks the bound policy and live target admission. Captured
// hosts retain their definitions, so ownership alone never grants this attack.
// A unit with a carrier (transport cargo, or docked on a pad) is not a target:
// the replacement transfer has no carried form, and loading during a spray
// ends the attempt like any other loss of eligibility.
func InfectionTarget(actor, target *units.Unit) bool {
	if actor == nil || !actor.Alive || actor.Dying || actor.Remaining != 0 ||
		target == nil || !target.Alive || target.Dying || target.Def == nil || target.Attachment.Carrier != 0 ||
		target.Remaining != 0 || target.Def.BMCode != 1 || !target.Def.CanMove || target.Def.Commander || target.Def.Builder {
		return false
	}
	p := unitInfection(actor)
	return p.DurationTicks != 0 && p.Range > 0 && isHostile(actor, target)
}

// InfectionThreat reports a live takeover capability through the unit's bound
// orders policy. It is observation data for Modern targeting, not a second
// policy selector. Captured ordinary hosts and disabled modes answer false.
func InfectionThreat(u *units.Unit) bool {
	if u == nil || !u.Alive || u.Dying || u.Stunned || u.Remaining != 0 {
		return false
	}
	policy := unitInfection(u)
	return policy.DurationTicks != 0 && policy.Range > 0
}

// InfectionInRange is the bounded three-dimensional centre reach test. Bound
// each unsigned difference before squaring and subtract from the squared
// radius, avoiding overflow both in coordinate subtraction and accumulation.
func InfectionInRange(actor, target *units.Unit) bool {
	if !InfectionTarget(actor, target) {
		return false
	}
	r := unitInfection(actor).Range
	if r <= 0 || uint64(r) > uint64(^uint32(0)) {
		return false
	}
	left := uint64(r) * uint64(r)
	for _, pair := range [3][2]numeric.Fixed{{actor.X, target.X}, {actor.Y, target.Y}, {actor.Z, target.Z}} {
		a, b := pair[0], pair[1]
		if a < b {
			a, b = b, a
		}
		d := uint64(a) - uint64(b)
		if d > uint64(r) || d*d > left {
			return false
		}
		left -= d * d
	}
	return true
}

// The high phase range tags infection records independently of the current
// rules. A mode switch therefore cancels them rather than interpreting their
// saved words as retail capture progress. Param1/2 retain the target's full
// allocation serial; Param3 is the uninterrupted spray's starting tick. The
// unused capture CachedX/Y pair stores the last checked tick, so a suspended
// queue cannot finish an interval during which it emitted no spray.
const (
	infectionApproach uint8 = 128 + iota
	infectionFollowing
	infectionStance
	infectionSpraying
)

func infectionCapture(u *units.Unit, n *Node, policy InfectionPolicy, satisfied, tick uint32) Code {
	if policy.DurationTicks == 0 {
		return 8
	}
	target := lookupTarget(u, n.Target)
	if !InfectionTarget(u, target) || satisfied&pendTargetGone != 0 {
		return 8
	}
	if n.Phase < infectionApproach {
		n.Param1, n.Param2 = uint32(target.AllocationSerial), uint32(target.AllocationSerial>>32)
		n.Param3 = 0
		n.Phase = infectionApproach
		captionClearText(u, n, "Infecting")
		// Infection is the attack itself: do not let the Community work-fire
		// option hand ordinary weapons back to autonomy during the takeover.
		releaseAllWeaponSlots(u)
		clearWeaponTargetsUnconditional(u)
	} else if uint64(n.Param1)|uint64(n.Param2)<<32 != target.AllocationSerial {
		return 8
	}
	inRange := InfectionInRange(u, target)
	if !inRange && n.Phase >= infectionStance {
		emitStopBuilding(u, n)
		n.Phase, n.Param3 = infectionApproach, 0
	}
	// Pursuit remains bound during the spray. Follow changed target cells, but
	// keep the script stance and uninterrupted timer when refreshing in range.
	// Keeping an unchanged goal avoids restarting a live route every tick.
	moved := n.GoalX>>20 != target.X>>20 || n.GoalY>>20 != target.Y>>20 || n.GoalZ>>20 != target.Z>>20
	if n.Phase == infectionApproach || moved {
		if installWorkGoal(u, n, target.X, target.Y, target.Z) {
			n.GoalX, n.GoalY, n.GoalZ = target.X, target.Y, target.Z
		} else if !inRange {
			return 8
		}
		if n.Phase == infectionApproach {
			n.Phase = infectionFollowing
		}
	} else if !inRange && satisfied&gateNoRoute != 0 {
		return 8
	}
	if !inRange {
		n.DynamicGate = pendTargetGone | gateNoRoute
		return deadlineHold(n, tick, 1)
	}
	if n.Phase < infectionStance {
		EmitStartBuilding(u, n, target.X, target.Z)
		n.Phase = infectionStance
		// StartBuilding is deferred; let the script establish its stance before
		// starting the interval, even when a previous work item left it set.
		return deadlineHold(n, tick, 1)
	}
	started := false
	if n.Phase == infectionStance {
		if !u.InBuildStance {
			return deadlineHold(n, tick, 1)
		}
		n.Phase, n.Param3 = infectionSpraying, tick
		started = true
		workStatus(u, statusWorking, "")
	}
	lastTick := uint32(uint16(n.CachedX)) | uint32(uint16(n.CachedY))<<16
	if !u.InBuildStance || (!started && tick != lastTick && tick-lastTick != 1) {
		emitStopBuilding(u, n)
		n.Phase, n.Param3 = infectionApproach, 0
		return deadlineHold(n, tick, 1)
	}
	if !started && tick == lastTick {
		return deadlineHold(n, tick, 1)
	}
	n.CachedX, n.CachedY = int16(tick), int16(tick>>16)
	elapsed := tick - n.Param3
	if elapsed >= policy.DurationTicks {
		// The existing port owns capacity refusal, replacement, cleanup and
		// notification. Refusal ends this attempt without touching the victim.
		if ok, _ := boundCapture(QueueForUnit(u), u, n, tick); ok {
			workStatus(u, statusCapture, "")
		}
		return 5
	}
	if elapsed%2 == 0 {
		emitNanolathe(u, n, tick)
		stampNanolatheActive(u, tick, nanolatheStampCapture)
	}
	n.DynamicGate = pendTargetGone
	return deadlineHold(n, tick, 1)
}
