package session

import (
	"fmt"
	"math"
	"sort"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/survival"
	"github.com/nanolathe-gg/nanolathe/internal/triggers"
)

// Composition supplies the exact previously admitted mission. This fragment
// cannot establish immutable input provenance from current state or a name
// (DESIGN_MULTIPLAYER §16.3.38).
type checkpointScenarioContext struct {
	keys    *content.CheckpointKeys
	mission *mission.Mission
}

// writeScenarioCheckpoint follows the separate computer fragment: Mission
// presence, Defeat/Victory, communitySchema, Survival presence/payload (.38).
// It invokes no evaluator, default initializer, planner or gameplay callback.
func (s *Session) writeScenarioCheckpoint(e *checkpoint.Encoder, c *checkpointScenarioContext) error {
	e.Field("scenario")
	if err := s.validateScenarioCheckpoint(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Bool(s.Mission != nil)
	if s.Mission != nil {
		writeScenarioTriggers(e, s.Mission.Defeat, "scenario.Mission.Defeat")
		writeScenarioTriggers(e, s.Mission.Victory, "scenario.Mission.Victory")
	}
	s.communitySchema.writeCheckpoint(e)
	e.Field("scenario.Survival")
	e.Bool(s.Survival != nil)
	if s.Survival != nil {
		return s.Survival.writeCheckpoint(e, c)
	}
	return e.Err()
}

func (s *Session) validateScenarioCheckpoint(c *checkpointScenarioContext) error {
	if s == nil {
		return scenarioCheckpointError("scenario", "a present session")
	}
	if c == nil {
		return scenarioCheckpointError("scenario.context", "an admitted composition context")
	}
	if s.Mission != c.mission {
		return scenarioCheckpointError("scenario.Mission", "the exact admitted mission")
	}
	if s.communitySchema.mission != nil && s.communitySchema.mission != c.mission {
		return scenarioCheckpointError("scenario.communitySchema.mission", "the exact admitted mission")
	}
	if s.Mission != nil {
		// TODO(M3-U6): campaign/restore entry needs its own immutable admission;
		// the current scenario contract admits only ordinary skirmish entry.
		if s.Mission.Type != mission.TypeSkirmish {
			return scenarioCheckpointError("scenario.Mission.Type", "admitted skirmish entry (TODO(M3-U6): campaign/restore admission)")
		}
		if s.Mission.IsRestore {
			return scenarioCheckpointError("scenario.Mission.IsRestore", "ordinary entry (TODO(M3-U6): restore admission)")
		}
		seen := make(map[*triggers.Trigger]bool)
		for _, list := range []struct {
			path string
			rows []*triggers.Trigger
		}{
			{"scenario.Mission.Defeat", s.Mission.Defeat}, {"scenario.Mission.Victory", s.Mission.Victory},
		} {
			for i, t := range list.rows {
				if t == nil {
					continue
				}
				if seen[t] {
					return scenarioCheckpointError(fmt.Sprintf("%s[%d]", list.path, i), "a trigger with no repeated pointer alias")
				}
				seen[t] = true
			}
		}
	}
	return nil
}

func writeScenarioTriggers(e *checkpoint.Encoder, rows []*triggers.Trigger, path string) {
	e.Field(path)
	e.Count(len(rows))
	for i, t := range rows {
		e.Field(fmt.Sprintf("%s[%d]", path, i))
		e.Bool(t != nil)
		if t != nil {
			t.WriteCheckpoint(e)
		}
	}
}

// Lexical fields: active, deferredPlacements, mission, neutralOwner,
// nextDeferred, playerByStart. Diagnostics do not affect deferred work.
func (s *communitySchemaState) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("scenario.communitySchema.active")
	e.Bool(s.active)
	e.Field("scenario.communitySchema.deferredPlacements")
	e.Count(len(s.deferredPlacements))
	for _, v := range s.deferredPlacements {
		e.I64(int64(v))
	}
	e.Bool(s.mission != nil)
	e.I8(s.neutralOwner)
	e.I64(int64(s.nextDeferred))
	for _, v := range s.playerByStart {
		e.I8(v)
	}
}

// The retained lexical fields are accts, attacker, centreX, centreZ, classes,
// cleanLost, info, lastSpawn, nextG, nextP, nextRetarget, opts, phase, phaseEnd,
// plan, pool, removed, settled, startClass, startRegion, stats, survived,
// team, tuning, units, wave, wavePoints, waveUnits. Setup/report deposits,
// history and the rebuilt walk are excluded (DESIGN_MULTIPLAYER §16.3.38).
func (s *survivalState) writeCheckpoint(e *checkpoint.Encoder, c *checkpointScenarioContext) error {
	start, err := s.checkpointStartClass()
	if err != nil {
		e.Fail(err)
		return e.Err()
	}
	for i, a := range s.accts {
		path := fmt.Sprintf("scenario.Survival.accts[%d]", i)
		e.Field(path + ".Capacity")
		e.F32(a.Capacity)
		e.Field(path + ".Earned")
		e.F32(a.Earned)
		e.Field(path + ".Stock")
		e.F32(a.Stock)
	}
	e.Field("scenario.Survival.attacker")
	e.U8(s.attacker)
	e.I32(s.centreX)
	e.I32(s.centreZ)
	e.Field("scenario.Survival.classes")
	e.Count(len(s.classes))
	for _, cls := range s.classes {
		cls.writeCheckpoint(e)
	}
	e.Field("scenario.Survival.cleanLost")
	e.Bool(s.cleanLost)
	e.Field("scenario.Survival.info")
	e.Bool(s.info != nil)
	if s.info != nil {
		s.info.WriteCheckpoint(e)
	}
	e.Field("scenario.Survival.lastSpawn")
	e.U32(s.lastSpawn)
	e.I64(int64(s.nextG))
	e.I64(int64(s.nextP))
	e.U32(s.nextRetarget)
	e.Bool(s.opts.NoAir)
	e.Bool(s.opts.NoNaval)
	e.U8(uint8(s.phase))
	e.U32(s.phaseEnd)
	writeScenarioWave(e, &s.plan)
	writeScenarioPool(e, &s.pool, c.keys)
	e.Field("scenario.Survival.removed")
	keys := checkpointScenarioRemovedHandles(s.removed)
	e.Count(len(keys))
	for _, h := range keys {
		e.U32(uint32(h))
		e.I32(s.removed[h])
	}
	e.Field("scenario.Survival.settled")
	for _, v := range s.settled {
		e.U32(v)
	}
	e.U32(start)
	e.I32(s.startRegion)
	for _, v := range s.stats {
		e.I64(v.Damage)
		e.I64(v.Destroyed)
		e.I64(v.Lost)
	}
	e.I64(int64(s.survived))
	e.Field("scenario.Survival.team")
	e.Count(len(s.team))
	for _, v := range s.team {
		e.U8(v)
	}
	writeScenarioTuning(e, &s.tuning)
	e.Field("scenario.Survival.units")
	e.Count(len(s.units))
	for _, v := range s.units {
		// h, infecting, shun, shunCellX, shunCellZ, shunSerial, shunUntil, target, wave.
		e.U32(uint32(v.h))
		e.Bool(v.infecting)
		e.U32(uint32(v.shun))
		e.I32(v.shunCellX)
		e.I32(v.shunCellZ)
		e.U64(v.shunSerial)
		e.U32(v.shunUntil)
		e.U32(uint32(v.target))
		e.I64(int64(v.wave))
	}
	e.I64(int64(s.wave))
	e.I64(s.wavePoints)
	e.Field("scenario.Survival.waveUnits")
	e.Count(len(s.waveUnits))
	for _, h := range s.waveUnits {
		e.U32(uint32(h))
	}
	return e.Err()
}

func (s *survivalState) checkpointStartClass() (uint32, error) {
	if uint64(len(s.classes)) > math.MaxUint32 {
		return 0, scenarioCheckpointError("scenario.Survival.classes", "a u32 class count")
	}
	seen := make(map[*survivalClass]bool, len(s.classes))
	var start uint32
	for i, cls := range s.classes {
		if cls == nil || seen[cls] {
			return 0, scenarioCheckpointError(fmt.Sprintf("scenario.Survival.classes[%d]", i), "a unique nonnil class")
		}
		seen[cls] = true
		if cls == s.startClass {
			start = uint32(i + 1)
		}
	}
	if s.startClass != nil && start == 0 {
		return 0, scenarioCheckpointError("scenario.Survival.startClass", "an exact member of classes")
	}
	return start, nil
}

// Only keys are gathered from the map. Numeric sorting precedes every value
// read, including stale raw handles (DESIGN_MULTIPLAYER §16.3.38; I1).
func checkpointScenarioRemovedHandles(m map[pool.Handle]int32) []pool.Handle {
	keys := make([]pool.Handle, 0, len(m))
	for h := range m {
		keys = append(keys, h)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func (c *survivalClass) writeCheckpoint(e *checkpoint.Encoder) {
	e.Field("scenario.Survival.classes")
	e.I32(c.base)
	e.String(c.key)
	writeScenarioProfile(e, c.profile)
	c.regions.WriteCheckpoint(e)
}

// Profile: BadSlope, BadWaterSlope, FootPrintX, FootPrintZ, MaxSlope,
// MaxWaterDepth, MaxWaterSlope, MinWaterDepth; preserve narrow source widths.
func writeScenarioProfile(e *checkpoint.Encoder, p movement.Profile) {
	e.U8(p.BadSlope)
	e.U8(p.BadWaterSlope)
	e.I16(p.FootPrintX)
	e.I16(p.FootPrintZ)
	e.U8(p.MaxSlope)
	e.I32(p.MaxWaterDepth)
	e.U8(p.MaxWaterSlope)
	e.I32(p.MinWaterDepth)
}

// Wave: Budget, Groups, Number. Group: Angle, Domain, Picks.
func writeScenarioWave(e *checkpoint.Encoder, w *survival.Wave) {
	e.Field("scenario.Survival.plan")
	e.I64(w.Budget)
	e.Count(len(w.Groups))
	for _, g := range w.Groups {
		e.U16(g.Angle)
		e.U8(uint8(g.Domain))
		e.Count(len(g.Picks))
		for _, v := range g.Picks {
			e.I64(int64(v))
		}
	}
	e.I64(int64(w.Number))
}

// Pool: MaxTier, RatioE, RatioM, Tier1Median, Units.
// Unit: Cost, Def, Domain, Key, Tier.
func writeScenarioPool(e *checkpoint.Encoder, p *survival.Pool, keys *content.CheckpointKeys) {
	e.Field("scenario.Survival.pool")
	e.I64(int64(p.MaxTier))
	e.I64(p.RatioE)
	e.I64(p.RatioM)
	e.I64(p.Tier1Median)
	e.Count(len(p.Units))
	for i, v := range p.Units {
		path := fmt.Sprintf("scenario.Survival.pool.Units[%d]", i)
		e.Field(path)
		e.I64(v.Cost)
		e.Field(path + ".Def")
		e.Bool(v.Def != nil)
		if v.Def != nil {
			ref, err := keys.Unit(v.Def)
			if err != nil {
				e.Fail(fmt.Errorf("%w: %w", scenarioCheckpointError(path+".Def", "an admitted unit definition"), err))
				return
			}
			e.Definition(ref)
		}
		e.Field(path)
		e.U8(uint8(v.Domain))
		e.String(v.Key)
		e.I64(int64(v.Tier))
	}
}

// Tuning: AirFrom, BaseUnits, BuddyRing, CleanWaveBonus, DirectionEvery,
// Doubling, DowntimeBase, DowntimeMax, DowntimePerUnit, EdgeInset, FastClearBonus,
// FirstWaveDelay, MaxDirections, NewTierWeight, RetargetEvery, SpawnPerTick,
// Straggle, ThemeWeights, UnlockUnits, WarningTime, WaveReward. Store each
// effective value without pace defaults or a derived budget.
func writeScenarioTuning(e *checkpoint.Encoder, t *survival.Tuning) {
	e.Field("scenario.Survival.tuning")
	e.U32(t.AirFrom)
	e.I64(t.BaseUnits)
	e.I32(t.BuddyRing)
	e.I64(t.CleanWaveBonus)
	e.U32(t.DirectionEvery)
	e.U32(t.Doubling)
	e.U32(t.DowntimeBase)
	e.U32(t.DowntimeMax)
	e.U32(t.DowntimePerUnit)
	e.I32(t.EdgeInset)
	e.I64(t.FastClearBonus)
	e.U32(t.FirstWaveDelay)
	e.I64(int64(t.MaxDirections))
	e.I64(int64(t.NewTierWeight))
	e.U32(t.RetargetEvery)
	e.I64(int64(t.SpawnPerTick))
	e.U32(t.Straggle)
	for _, v := range t.ThemeWeights {
		e.I64(int64(v))
	}
	e.I64(t.UnlockUnits)
	e.U32(t.WarningTime)
	e.Field("scenario.Survival.tuning.WaveReward")
	e.F32(t.WaveReward)
}

// appendScenarioCheckpointSummary reads only the selected ring words (.38).
// In particular it does not validate bindings, aliases, regions or pool keys.
func (s *Session) appendScenarioCheckpointSummary(summary *checkpoint.Summary) error {
	if s == nil {
		return scenarioCheckpointError("scenario", "a present session")
	}
	if summary == nil {
		return scenarioCheckpointError("scenario.summary", "a summary accumulator")
	}
	next := *summary
	next.Word(scenarioCheckpointBool(s.Mission != nil))
	if s.Mission != nil {
		for _, rows := range [][]*triggers.Trigger{s.Mission.Defeat, s.Mission.Victory} {
			next.Word(uint64(len(rows)))
			for _, t := range rows {
				next.Word(scenarioCheckpointBool(t != nil))
				if t != nil {
					next.Word(scenarioCheckpointBool(t.Completed))
					next.Word(scenarioCheckpointBool(t.Celebrated))
				}
			}
		}
	}
	next.Word(scenarioCheckpointBool(s.Survival != nil))
	if st := s.Survival; st != nil {
		next.Word(uint64(st.phase))
		next.Word(uint64(st.phaseEnd))
		next.Word(uint64(int64(st.wave)))
		next.Word(uint64(int64(st.nextG)))
		next.Word(uint64(int64(st.nextP)))
		next.Word(uint64(st.nextRetarget))
		next.Word(uint64(int64(st.survived)))
		next.Word(uint64(st.wavePoints))
		for _, v := range st.stats {
			next.Word(uint64(v.Damage))
			next.Word(uint64(v.Destroyed))
			next.Word(uint64(v.Lost))
		}
		for i, a := range st.accts {
			for _, v := range [...]struct {
				name  string
				value float32
			}{{"Capacity", a.Capacity}, {"Earned", a.Earned}, {"Stock", a.Stock}} {
				bits := math.Float32bits(v.value)
				if bits&0x7f800000 == 0x7f800000 && bits&0x007fffff != 0 {
					return scenarioCheckpointError(fmt.Sprintf("scenario.Survival.accts[%d].%s", i, v.name), "non-NaN binary32")
				}
				next.Word(uint64(bits))
			}
		}
	}
	*summary = next
	return nil
}

func scenarioCheckpointBool(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func scenarioCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint capture failed: logical path %s, providers searched [scenario], expected %s", path, expected)
}
