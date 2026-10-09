package frame

// NewEventBufferWithIndependentEffects creates a presentation buffer and a
// separate simulation effect window (DESIGN_MULTIPLAYER §16.4.1). The latter
// uses normalized MaxEffectEvents as its total capacity, independently of
// MaxEvents. Each window assigns its own IDs and sequences. Presentation
// verdicts and publication ordering remain those of NewEventBuffer.
func NewEventBufferWithIndependentEffects(limits Limits) *EventBuffer {
	c := NewEventBuffer(limits)
	c.independentEffects = NewEventBuffer(Limits{
		MaxEvents: c.limits.MaxEffectEvents, MaxEffectEvents: c.limits.MaxEffectEvents,
	})
	return c
}

// EffectEvents borrows the current simulation effect window until Reset. The
// caller must not modify or retain it. Ordinary buffers return StagingEvents
// exactly, preserving the single-player effect consumer's input (§16.4.1).
func (c *EventBuffer) EffectEvents() []Event {
	if c != nil && c.independentEffects != nil {
		return c.independentEffects.StagingEvents()
	}
	return c.StagingEvents()
}

// SmokeEnd is an effect operation even though it allocates no record: the
// consumer removes matching smoke records. All local audio, status, music,
// announcement and shake cues remain presentation-only (§16.4.1).
func simulationEffectEvent(kind Kind) bool {
	switch kind {
	case KindCOBSFX, KindNanolathe, KindMuzzleFlash, KindSmokeStart, KindSmokeEnd,
		KindProjectileTrail, KindImpact, KindWaterImpact, KindExplosion,
		KindLHTFlash, KindCorpse:
		return true
	default:
		return false
	}
}
