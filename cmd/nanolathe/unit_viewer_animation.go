package main

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

type unitViewerAction string

const (
	unitViewerIdle    unitViewerAction = "Idle"
	unitViewerWalking unitViewerAction = "Walk"
	unitViewerAiming  unitViewerAction = "Aim"
	unitViewerFiring  unitViewerAction = "Fire"

	// These are bounded host preview preferences, not battle rules
	// (DESIGN_DEVELOPER_TOOLS §7). COB keeps its own 30 Hz arithmetic [04 §4.6].
	unitViewerScriptBudget = 4096
	unitViewerTickRate     = 30
	unitViewerMaxTicks     = 5
	unitViewerAimTimeout   = 10 * unitViewerTickRate
	unitViewerAimHeading   = uint16(65536 / 8)  // 45 degrees
	unitViewerAimPitch     = uint16(65536 / 24) // about 15 degrees
)

// Availability describes authored entry points, not a promise that a callback
// will return. A Fire preview needs the selected weapon's complete aim/fire
// pair; an absent optional callback must not become a fabricated animation.
func unitViewerAnimationAvailable(def *content.UnitDef, action unitViewerAction, weapon int) bool {
	if def == nil || def.Script == nil {
		return false
	}
	switch action {
	case unitViewerIdle:
		return unitViewerHasCallback(def, "Create")
	case unitViewerWalking:
		return unitViewerHasCallback(def, "StartMoving")
	case unitViewerAiming, unitViewerFiring:
		if content.IsWeaponInactive(unitViewerWeapon(def, weapon)) {
			return false
		}
		return unitViewerHasCallback(def, unitViewerWeaponCallback("Aim", weapon)) &&
			(action != unitViewerFiring || unitViewerHasCallback(def, unitViewerWeaponCallback("Fire", weapon)))
	default:
		return false
	}
}

func unitViewerHasCallback(def *content.UnitDef, name string) bool {
	if def == nil || def.Script == nil {
		return false
	}
	_, ok := def.Script.Scripts[name]
	return ok
}

func unitViewerWeapon(def *content.UnitDef, weapon int) *content.WeaponDef {
	if def == nil || weapon < 1 || weapon > 3 {
		return nil
	}
	return [3]*content.WeaponDef{def.Weapon1Def, def.Weapon2Def, def.Weapon3Def}[weapon-1]
}

func unitViewerWeaponCallback(prefix string, weapon int) string {
	if weapon < 1 || weapon > 3 {
		return ""
	}
	return prefix + [3]string{"Primary", "Secondary", "Tertiary"}[weapon-1]
}

func unitViewerUnavailableReason(def *content.UnitDef, action unitViewerAction, weapon int) string {
	name := "Create"
	switch action {
	case unitViewerIdle:
	case unitViewerWalking:
		name = "StartMoving"
	case unitViewerAiming, unitViewerFiring:
		if content.IsWeaponInactive(unitViewerWeapon(def, weapon)) {
			return fmt.Sprintf("weapon %d is inactive or missing", weapon)
		}
		name = unitViewerWeaponCallback("Aim", weapon)
		if action == unitViewerFiring && unitViewerHasCallback(def, name) {
			name = unitViewerWeaponCallback("Fire", weapon)
		}
	default:
		return "unknown animation"
	}
	return "no " + name + " callback"
}

func (m *unitViewerModel) setAnimation(action unitViewerAction, weapon int) {
	if action == "" {
		action = unitViewerIdle
	}
	if weapon == 0 {
		weapon = 1
	}
	// Explicitly choosing the current action replays it, including a stopped
	// aim or fire preview (DESIGN_DEVELOPER_TOOLS §7).
	m.action, m.weapon = action, weapon
	if m.geometry != nil {
		m.anim = newUnitViewerAnimation(m.def, m.geometry, action, weapon)
		m.poses, m.poseNote = m.anim.poses(), m.anim.note
	}
	m.key = unitViewerModelKey{}
}

// Fit once from Create, before a requested action can move the limbs. Changing
// actions preserves that fit; the existing atlas bound still limits each draw.
func (m *unitViewerModel) loadAnimation(def *content.UnitDef, mdl *model.Model) {
	m.def, m.geometry = def, mdl
	if m.action == "" {
		m.action = unitViewerIdle
	}
	if m.weapon == 0 {
		m.weapon = 1
	}
	m.anim = newUnitViewerAnimation(def, mdl, unitViewerIdle, 1)
	m.poses, m.poseNote = m.anim.poses(), m.anim.note
	states := make([]model.PieceState, len(mdl.Pieces))
	for _, pose := range m.poses {
		states[pose.Index] = model.PieceState{RotX: pose.RotX, RotY: pose.RotY, RotZ: pose.RotZ, Trans: [3]numeric.Fixed{pose.Tx, pose.Ty, pose.Tz}, Hidden: pose.Hidden}
	}
	m.fitCreatePose(states)
	if m.action != unitViewerIdle || m.weapon != 1 {
		m.anim = newUnitViewerAnimation(def, mdl, m.action, m.weapon)
		m.poses, m.poseNote = m.anim.poses(), m.anim.note
	}
}

func (m *unitViewerModel) updateAnimation(dt float64) {
	if m.anim == nil || !m.anim.update(dt) {
		return
	}
	m.poses, m.poseNote = m.anim.poses(), m.anim.note
	// A stationary camera must still redraw a changed pose.
	m.key = unitViewerModelKey{}
}

type unitViewerAnimation struct {
	script      *unitViewerScript
	def         *content.UnitDef
	action      unitViewerAction
	weapon      int
	note        string
	carry       float64
	ticks       uint64
	nextAim     uint64
	reload      uint64
	aimStarted  uint64
	aimIdentity uint64
	aimThread   int
	aimPending  bool
	aimReturned bool
	aimReady    bool
	stopped     bool
	invalid     bool
}

func newUnitViewerAnimation(def *content.UnitDef, mdl *model.Model, action unitViewerAction, weapon int) *unitViewerAnimation {
	a := &unitViewerAnimation{def: def, action: action, weapon: weapon}
	a.script = newUnitViewerScript(def, mdl)
	if a.script == nil {
		a.stopped, a.note = true, "Authored pose / no script"
		return a
	}
	if !a.script.create() {
		a.failScript()
		return a
	}
	if !unitViewerAnimationAvailable(def, action, weapon) {
		a.stopped, a.note = true, fmt.Sprintf("%s unavailable / %s", action, unitViewerUnavailableReason(def, action, weapon))
		return a
	}
	// Authored restore threads can use the longest base reload. Its standard
	// callback follows Create and remains deferred [04 R-CB-01 §2].
	if unitViewerHasCallback(def, "SetMaxReloadTime") {
		var reload int32
		for _, w := range [3]*content.WeaponDef{def.Weapon1Def, def.Weapon2Def, def.Weapon3Def} {
			if w != nil {
				reload = max(reload, w.ReloadTime)
			}
		}
		if !a.script.bridge.SetMaxReloadTime(reload).Started {
			a.stop("reload callback unavailable")
			return a
		}
	}
	switch action {
	case unitViewerIdle:
		a.note = "Idle preview"
	case unitViewerWalking:
		if !a.script.bridge.StartMoving().Started {
			a.stop("movement callback unavailable")
		} else {
			a.note = "Walking preview"
		}
	case unitViewerAiming, unitViewerFiring:
		// The loop is a preview policy: retry only after this interval, then
		// await a fresh aim. It is not a prediction of battle firing cadence.
		a.reload = uint64(max(1, unitViewerWeapon(def, weapon).ReloadTime))
		a.startAim()
	}
	if len(a.script.vm.Diagnostics()) != 0 {
		a.failScript()
	} else if a.script.interrupted != "" {
		a.stop(a.script.interrupted + " ended without a return")
	}
	return a
}

func (a *unitViewerAnimation) poses() []frame.PieceView {
	if a.script == nil || a.invalid {
		return nil
	}
	return a.script.poses()
}

func (a *unitViewerAnimation) update(dt float64) bool {
	if a.stopped || dt <= 0 || math.IsNaN(dt) || math.IsInf(dt, 0) {
		return false
	}
	// Drop excess host elapsed time rather than accumulate catch-up work
	// after a stalled window (DESIGN_DEVELOPER_TOOLS §7).
	a.carry = min(float64(unitViewerMaxTicks), a.carry+dt*unitViewerTickRate)
	steps := int(a.carry)
	a.carry -= float64(steps)
	for i := 0; i < steps && !a.stopped; i++ {
		a.tick()
	}
	return steps != 0
}

func (a *unitViewerAnimation) tick() {
	a.ticks++
	if a.action == unitViewerFiring && !a.aimPending {
		if a.aimReady {
			// Root Fire precedes optional RockUnit [04 R-CB-01 §2]. There
			// are no projectile, burst, sound, resource or effect producers.
			if !a.script.bridge.Fire(cob.WeaponSlot(a.weapon - 1)).Started {
				a.stop("fire callback unavailable")
				return
			}
			if unitViewerHasCallback(a.def, "RockUnit") && !a.script.bridge.RockUnit(int16(unitViewerAimHeading)).Started {
				a.stop("recoil callback unavailable")
				return
			}
			a.aimReady, a.nextAim = false, a.ticks+a.reload
			a.note = fmt.Sprintf("Weapon %d / firing animation", a.weapon)
		} else if a.ticks >= a.nextAim {
			a.startAim()
		}
	}
	if a.stopped {
		return
	}
	a.script.bridge.Drain(1)
	if len(a.script.vm.Diagnostics()) != 0 {
		a.failScript()
		return
	}
	if a.script.interrupted != "" {
		a.stop(a.script.interrupted + " ended without a return")
		return
	}
	if a.aimPending {
		switch {
		case a.aimReturned:
			a.aimPending = false
			if !a.aimReady {
				a.stop("aim returned zero")
			} else {
				a.note = fmt.Sprintf("Weapon %d / aim ready", a.weapon)
			}
		case !a.script.vm.ThreadAliveAs(a.aimThread, a.aimIdentity):
			a.stop("aim ended without a return")
		case a.ticks-a.aimStarted >= unitViewerAimTimeout:
			a.stop("aim exceeded 10-second preview limit")
		}
	}
}

func (a *unitViewerAnimation) startAim() {
	a.aimPending, a.aimReturned, a.aimReady = true, false, false
	a.aimStarted = a.ticks
	// Only an explicit nonzero return grants readiness [04 R-CB-01 §6].
	result := a.script.bridge.Aim(cob.WeaponSlot(a.weapon-1), unitViewerAimHeading, unitViewerAimPitch, func(r cob.CallbackReturn) {
		a.aimReturned, a.aimReady = r.Explicit, r.Explicit && r.Value != 0
	})
	if !result.Started {
		a.stop("aim callback unavailable")
		return
	}
	a.aimThread, a.aimIdentity = result.Thread, a.script.vm.ThreadIdentity(result.Thread)
	a.note = fmt.Sprintf("Weapon %d / aiming", a.weapon)
}

func (a *unitViewerAnimation) stop(reason string) {
	a.stopped = true
	a.note = fmt.Sprintf("%s preview stopped / %s", a.action, reason)
}

func (a *unitViewerAnimation) failScript() {
	a.stopped, a.invalid, a.note = true, true, "Authored pose / script unavailable"
}

// This interpreter owns only detached pose/flag state. Full health, complete
// construction, unbound zero reads and random low bounds are explicit preview
// inputs, not a claim about gameplay (DESIGN_DEVELOPER_TOOLS §7).
type unitViewerScript struct {
	vm          *cob.VM
	bridge      *cob.CallbackBridge
	pieceMap    []int
	modelFlags  []uint8
	pose        []frame.PieceView
	interrupted string
}

func newUnitViewerScript(def *content.UnitDef, mdl *model.Model) *unitViewerScript {
	if def == nil || def.Script == nil || mdl == nil {
		return nil
	}
	s := &unitViewerScript{
		vm:         cob.NewPresentationVM(def.Script, unitViewerScriptBudget),
		modelFlags: units.BuildRenderPieceFlags(mdl),
		pose:       make([]frame.PieceView, len(mdl.Pieces)),
	}
	s.bridge = cob.NewCallbackBridge(s.vm)
	// Signal and malformed termination are both non-returning callbacks. Do
	// not present either as a successful preview [04 §4.2][04 §4.3].
	s.bridge.SetLifecycleSink(func(e cob.LifecycleEvent) {
		if e.Phase == "finish-abnormal" && s.interrupted == "" {
			s.interrupted = e.Name
		}
	})
	modelNames := make([]string, len(mdl.Pieces))
	for i := range mdl.Pieces {
		modelNames[i] = mdl.Pieces[i].Name
	}
	s.pieceMap = cob.LinkPieces(def.Script.Pieces, modelNames)
	flags := make([]uint8, len(s.pieceMap))
	s.vm.BindRenderFlagHandlers(func() []uint8 {
		for i, idx := range s.pieceMap {
			if idx >= 0 && idx < len(s.modelFlags) {
				flags[i] = s.modelFlags[idx]
			}
		}
		return flags
	}, func(piece int, mask uint8, set bool) bool {
		if piece >= 0 && piece < len(s.pieceMap) {
			idx := s.pieceMap[piece]
			if idx >= 0 && idx < len(s.modelFlags) {
				if set {
					s.modelFlags[idx] |= mask
				} else {
					s.modelFlags[idx] &^= mask
				}
			}
		}
		return true
	})
	s.vm.BindPortBinding(cob.Port(4), cob.PortBinding{Read: func([4]int32) int32 { return 100 }})
	s.vm.BindPortBinding(cob.Port(17), cob.PortBinding{Read: func([4]int32) int32 { return 0 }})
	return s
}

func (s *unitViewerScript) create() bool {
	if _, ok := s.vm.ScriptPC("Create"); ok && !s.bridge.Create().Started {
		return false
	}
	return len(s.vm.Diagnostics()) == 0 && s.interrupted == ""
}

func (s *unitViewerScript) poses() []frame.PieceView {
	for i := range s.pose {
		f := s.modelFlags[i]
		s.pose[i] = frame.PieceView{Index: i, Hidden: f&1 == 0, DontShade: f&4 == 0, DontShadow: f&8 == 0}
	}
	for i, state := range s.vm.Pieces {
		if i >= len(s.pieceMap) {
			continue
		}
		idx := s.pieceMap[i]
		if idx < 0 || idx >= len(s.pose) {
			continue
		}
		p := &s.pose[idx]
		p.RotX, p.RotY, p.RotZ = state.RotX, state.RotY, state.RotZ
		p.Tx, p.Ty, p.Tz = state.Trans[0], state.Trans[1], state.Trans[2]
	}
	return s.pose
}
