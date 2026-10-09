package effects

import (
	"fmt"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// CheckpointContext binds the one shared fixed-pool owner. These writers add no
// object tables and do not resolve art (DESIGN_MULTIPLAYER §16.3.17).
type CheckpointContext struct {
	Keys    *content.CheckpointKeys
	Pool    *FixedEffectPool
	binding checkpointEffectBindings
}

func NewCheckpointContext(keys *content.CheckpointKeys, p *FixedEffectPool) *CheckpointContext {
	return &CheckpointContext{Keys: keys, Pool: p}
}

func (s *EffectService) CollectCheckpointReferences(c *CheckpointContext) (int, error) {
	return 0, s.checkpointBoundary(c)
}

// WriteCheckpoint retains art, lastSequence, max, nextID, owner in lexical
// order. Art is presence after admitted-pointer validation; owner is presence
// after exact concrete-type/identity validation against the context pool. Pending fallback
// state is unsupported; dropped/refusedAtCapacity are diagnostic-only
// (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.17).
func (s *EffectService) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("effects.EffectService")
	if err := s.checkpointBoundary(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("effects.EffectService.art")
	e.Bool(s.art != nil)
	e.Field("effects.EffectService.lastSequence")
	e.U64(s.lastSequence)
	e.Field("effects.EffectService.max")
	e.I64(int64(s.max))
	e.Field("effects.EffectService.nextID")
	e.U32(s.nextID)
	e.Field("effects.EffectService.owner")
	e.Bool(s.owner != nil)
	return e.Err()
}

func (s *EffectService) checkpointBoundary(c *CheckpointContext) error {
	if s == nil {
		return effectsCheckpointError("effects.EffectService", "a present service")
	}
	if c == nil {
		return effectsCheckpointError("effects.context", "a checkpoint context")
	}
	if c.binding.service != nil && c.binding.service != s {
		return effectsCheckpointError("effects.bindings", "the registered effect service")
	}
	if err := c.validateCheckpointBindings(); err != nil {
		return err
	}
	if s.art != nil && c.binding.service == nil {
		return effectsCheckpointError("effects.EffectService.art", "an absent or U6-attested simulation-art binding")
	}
	if s.owner != nil {
		p, ok := s.owner.(*FixedEffectPool)
		if !ok || p == nil || p != c.Pool {
			return effectsCheckpointError("effects.EffectService.owner", "the exact context FixedEffectPool owner")
		}
	}
	if len(s.pending) != 0 {
		return effectsCheckpointError("effects.EffectService.pending", "no pending fallback records")
	}
	return nil
}

// AppendCheckpointSummary appends nextID, lastSequence without consulting
// unselected bindings or pending views (DESIGN_MULTIPLAYER §16.3.17).
func (s *EffectService) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if s == nil {
		return effectsCheckpointError("effects.EffectService", "a present service")
	}
	if summary == nil {
		return effectsCheckpointError("effects.summary", "a summary accumulator")
	}
	summary.Word(uint64(s.nextID))
	summary.Word(s.lastSequence)
	return nil
}

// WriteCheckpoint preserves Active, Countdown, Durations, Frames, Idx, Loop
// exactly, including inactive residuals and producer-overridden durations. It
// never steps or resolves a cursor (DESIGN_MULTIPLAYER §16.3.17; [03 §4.4]).
func (a *EffectAnimPlayer) WriteCheckpoint(e *checkpoint.Encoder) error {
	e.Field("effects.EffectAnimPlayer")
	if a == nil {
		e.Fail(effectsCheckpointError("effects.EffectAnimPlayer", "a present animation player"))
		return e.Err()
	}
	writeCheckpointAnimation(e, a, "effects.EffectAnimPlayer")
	return e.Err()
}

func writeCheckpointAnimation(e *checkpoint.Encoder, a *EffectAnimPlayer, path string) {
	e.FieldChild(path, "Active")
	e.Bool(a.Active)
	e.FieldChild(path, "Countdown")
	e.I32(a.Countdown)
	e.FieldChild(path, "Durations")
	e.Count(len(a.Durations))
	for _, duration := range a.Durations {
		e.I32(duration)
	}
	e.FieldChild(path, "Frames")
	e.I64(int64(a.Frames))
	e.FieldChild(path, "Idx")
	e.I32(a.Idx)
	e.FieldChild(path, "Loop")
	e.Bool(a.Loop)
}

func (p *FixedEffectPool) CollectCheckpointReferences(c *CheckpointContext) (int, error) {
	return 0, p.checkpointBoundary(c)
}

// WriteCheckpoint retains capacity, fragmentContext, fragmentCursor,
// fragmentRoundRobin, fragments, gravity, heightAt, records, seaLevel, in that
// lexical order. Fragment context retains Gravity, Impact, Lava, SeaLevel,
// TerrainHeight, WaterEffectsWordZero. Ports carry validated presence tags.
// All physical fragment rows remain, including inactive residuals;
// fragmentStepping equals TerrainHeight presence and material is excluded artwork
// (DESIGN_MULTIPLAYER §16.3.5–§16.3.6, §16.3.17).
func (p *FixedEffectPool) WriteCheckpoint(e *checkpoint.Encoder, c *CheckpointContext) error {
	e.Field("effects.FixedEffectPool")
	if err := p.checkpointBoundary(c); err != nil {
		e.Fail(err)
		return e.Err()
	}
	e.Field("effects.FixedEffectPool.capacity")
	e.I64(int64(p.capacity))
	e.Field("effects.FixedEffectPool.fragmentContext.Gravity")
	e.I64(int64(p.fragmentContext.Gravity))
	e.Field("effects.FixedEffectPool.fragmentContext.Impact")
	e.Bool(p.fragmentContext.Impact != nil)
	e.Field("effects.FixedEffectPool.fragmentContext.Lava")
	e.Bool(p.fragmentContext.Lava)
	e.Field("effects.FixedEffectPool.fragmentContext.SeaLevel")
	e.I64(int64(p.fragmentContext.SeaLevel))
	e.Field("effects.FixedEffectPool.fragmentContext.TerrainHeight")
	e.Bool(p.fragmentContext.TerrainHeight != nil)
	e.Field("effects.FixedEffectPool.fragmentContext.WaterEffectsWordZero")
	e.Bool(p.fragmentContext.WaterEffectsWordZero)
	e.Field("effects.FixedEffectPool.fragmentCursor")
	e.I64(int64(p.fragmentCursor))
	e.Field("effects.FixedEffectPool.fragmentRoundRobin")
	e.Bool(p.fragmentRoundRobin)
	e.Field("effects.FixedEffectPool.fragments")
	e.Count(len(p.fragments))
	for i := range p.fragments {
		f := &p.fragments[i]
		// Fragment fields: angles, angularRates, baseVelocity, live, vertices.
		e.FieldIndex("effects.FixedEffectPool.fragments", i, ".angles")
		for _, v := range f.angles {
			e.U16(v)
		}
		e.FieldIndex("effects.FixedEffectPool.fragments", i, ".angularRates")
		for _, v := range f.angularRates {
			e.U16(v)
		}
		e.FieldIndex("effects.FixedEffectPool.fragments", i, ".baseVelocity")
		for _, v := range f.baseVelocity {
			e.I32(v)
		}
		e.FieldIndex("effects.FixedEffectPool.fragments", i, ".live")
		e.Bool(f.live)
		e.FieldIndex("effects.FixedEffectPool.fragments", i, ".vertices")
		for _, vertex := range f.vertices {
			for _, v := range vertex {
				e.I64(int64(v))
			}
		}
	}
	e.Field("effects.FixedEffectPool.gravity")
	e.I64(int64(p.gravity))
	e.Field("effects.FixedEffectPool.heightAt")
	e.U8(0)
	e.Field("effects.FixedEffectPool.records")
	e.Count(len(p.records))
	for i := range p.records {
		writeCheckpointEffect(e, &p.records[i], fmt.Sprintf("effects.FixedEffectPool.records[%d]", i))
	}
	e.Field("effects.FixedEffectPool.seaLevel")
	e.I64(int64(p.seaLevel))
	return e.Err()
}

// EffectRecord retained fields are AnimA, AnimB, ExpiryTick,
// FragmentExplodeOnHit, FragmentSlot, Gravity, HasModel, Kind, Source, Target,
// VX, VY, VZ, X, Y, Z. FragmentSlot is a u16 geometry index; Source and Target
// use the explicit raw-handle u32 override. All other fields are excluded
// presentation/graphic/flash metadata under the reviewed inventory.
func writeCheckpointEffect(e *checkpoint.Encoder, r *EffectRecord, path string) {
	writeCheckpointAnimation(e, &r.AnimA, path+".AnimA")
	writeCheckpointAnimation(e, &r.AnimB, path+".AnimB")
	e.FieldChild(path, "ExpiryTick")
	e.U32(r.ExpiryTick)
	e.FieldChild(path, "FragmentExplodeOnHit")
	e.Bool(r.FragmentExplodeOnHit)
	e.FieldChild(path, "FragmentSlot")
	e.U16(r.FragmentSlot)
	e.FieldChild(path, "Gravity")
	e.I64(int64(r.Gravity))
	e.FieldChild(path, "HasModel")
	e.Bool(r.HasModel)
	e.FieldChild(path, "Kind")
	e.String(r.Kind)
	e.FieldChild(path, "Source")
	e.U32(uint32(r.Source))
	e.FieldChild(path, "Target")
	e.U32(uint32(r.Target))
	for _, value := range []struct {
		field string
		value int64
	}{
		{"VX", int64(r.VX)}, {"VY", int64(r.VY)}, {"VZ", int64(r.VZ)},
		{"X", int64(r.X)}, {"Y", int64(r.Y)}, {"Z", int64(r.Z)},
	} {
		e.FieldChild(path, value.field)
		e.I64(value.value)
	}
}

func (p *FixedEffectPool) checkpointBoundary(c *CheckpointContext) error {
	if p == nil {
		return effectsCheckpointError("effects.FixedEffectPool", "a present fixed pool")
	}
	if c == nil || c.Pool != p {
		return effectsCheckpointError("effects.context.Pool", "the exact fixed pool identity")
	}
	if c.binding.service != nil {
		return c.validateCheckpointBindings()
	}
	return p.validateCheckpointFragmentBindings(c)
}

// AppendCheckpointSummary reads record count; each X,Y,Z,VX,VY,VZ,ExpiryTick,
// AnimA.Idx/Countdown, AnimB.Idx/Countdown; then the live fragment count. It
// does not inspect ports or advance players (DESIGN_MULTIPLAYER §16.3.17).
func (p *FixedEffectPool) AppendCheckpointSummary(summary *checkpoint.Summary) error {
	if p == nil {
		return effectsCheckpointError("effects.FixedEffectPool", "a present fixed pool")
	}
	if summary == nil {
		return effectsCheckpointError("effects.summary", "a summary accumulator")
	}
	summary.Word(uint64(len(p.records)))
	for i := range p.records {
		r := &p.records[i]
		for _, v := range [...]int64{int64(r.X), int64(r.Y), int64(r.Z), int64(r.VX), int64(r.VY), int64(r.VZ)} {
			summary.Word(uint64(v))
		}
		summary.Word(uint64(r.ExpiryTick))
		summary.Word(uint64(int64(r.AnimA.Idx)))
		summary.Word(uint64(int64(r.AnimA.Countdown)))
		summary.Word(uint64(int64(r.AnimB.Idx)))
		summary.Word(uint64(int64(r.AnimB.Countdown)))
	}
	var live uint64
	for i := range p.fragments {
		if p.fragments[i].live {
			live++
		}
	}
	summary.Word(live)
	return nil
}

func effectsCheckpointError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint capture failed: logical path %s, providers searched [effects], expected %s", path, expected)
}
