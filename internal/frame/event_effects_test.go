package frame

import (
	"reflect"
	"testing"
)

func TestIndependentEffectsIgnoreLocalPresentationSaturation(t *testing.T) {
	quiet := NewEventBufferWithIndependentEffects(Limits{MaxEvents: 3, MaxEffectEvents: 4})
	noisy := NewEventBufferWithIndependentEffects(Limits{MaxEvents: 3, MaxEffectEvents: 4})
	for _, kind := range []Kind{KindAudio, KindStatus, KindMusicIntensity} {
		if !noisy.Admit(Event{Kind: kind}) {
			t.Fatal("local cue refused before presentation saturation")
		}
	}
	inputs := []Event{
		{Kind: KindSmokeStart, Source: 7, Target: 9, Producer: ProducerSmoke, DurationsA: []int32{2, 5}},
		{Kind: KindSmokeEnd, Source: 7, Target: 9},
	}
	for _, e := range inputs {
		if !quiet.Admit(e) || noisy.Admit(e) {
			t.Fatal("effect verdict did not describe presentation admission")
		}
	}
	want := []Event{
		{ID: 1, Sequence: 1, Kind: KindSmokeStart, Source: 7, Target: 9, Producer: ProducerSmoke, Strip: 9, DurationsA: []int32{2, 5}},
		{ID: 2, Sequence: 2, Kind: KindSmokeEnd, Source: 7, Target: 9, Strip: -1},
	}
	if !reflect.DeepEqual(quiet.EffectEvents(), want) || !reflect.DeepEqual(noisy.EffectEvents(), want) {
		t.Fatalf("local cues changed effect operations: quiet=%+v noisy=%+v", quiet.EffectEvents(), noisy.EffectEvents())
	}
	if noisy.Dropped() != 2 || !noisy.Overflow() {
		t.Fatal("presentation refusal diagnostics changed")
	}
	// Each channel owns a detached routed value, including authored timing.
	inputs[0].DurationsA[0] = 100
	quiet.StagingEvents()[0].DurationsA[1] = 100
	if !reflect.DeepEqual(quiet.EffectEvents(), want) || !reflect.DeepEqual(noisy.EffectEvents(), want) {
		t.Fatal("effect timing aliases producer or presentation storage")
	}
	quiet.Reset()
	noisy.Reset()
	if quiet.EffectEvents() != nil || noisy.EffectEvents() != nil || noisy.Dropped() != 0 || noisy.Overflow() {
		t.Fatal("reset did not clear both windows and presentation diagnostics")
	}
	if !quiet.EmitImpact(Event{}) || !noisy.EmitImpact(Event{}) {
		t.Fatal("presentation did not reopen after reset")
	}
	want = []Event{{ID: 3, Sequence: 3, Kind: KindImpact, Strip: -1}}
	if !reflect.DeepEqual(quiet.EffectEvents(), want) || !reflect.DeepEqual(noisy.EffectEvents(), want) {
		t.Fatal("reset changed independent effect identities")
	}
	if quiet.StagingEvents()[0].Sequence != 3 || noisy.StagingEvents()[0].Sequence != 4 {
		t.Fatal("presentation counters did not retain their own admissions")
	}
}

func TestIndependentEffectsClosedKindsAndValidation(t *testing.T) {
	c := NewEventBufferWithIndependentEffects(Limits{})
	// Invalid kinds/lifetimes consume neither channel's identities.
	for _, e := range []Event{{Kind: 0}, {Kind: Kind(255)}, {Kind: KindExplosion, Lifetime: -1}} {
		if c.Admit(e) {
			t.Fatal("invalid event admitted")
		}
	}
	for kind := KindCOBSFX; kind <= KindMusicIntensity; kind++ {
		if !c.Admit(Event{Kind: kind, ID: 99, Sequence: 99}) {
			t.Fatalf("valid kind %d refused", kind)
		}
	}
	want := []Kind{KindCOBSFX, KindNanolathe, KindMuzzleFlash, KindSmokeStart,
		KindSmokeEnd, KindProjectileTrail, KindImpact, KindWaterImpact,
		KindExplosion, KindLHTFlash, KindCorpse}
	got := c.EffectEvents()
	if len(got) != len(want) {
		t.Fatalf("effect kinds = %+v", got)
	}
	for i, kind := range want {
		if got[i].Kind != kind || got[i].ID != uint32(i+1) || got[i].Sequence != uint64(i+1) || got[i].Strip != -1 {
			t.Fatalf("effect[%d] = %+v, want kind %d at identity %d", i, got[i], kind, i+1)
		}
	}
	if c.Dropped() != 3 || c.Overflow() || c.StagingEvents()[0].ID != 1 {
		t.Fatal("invalid-input presentation diagnostics changed")
	}
}

func TestIndependentEffectsCapacityUsesNormalizedEffectLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits Limits
		bound  int
	}{
		{"larger-than-presentation", Limits{MaxEvents: 1, MaxEffectEvents: 3}, 3},
		{"smaller-than-presentation", Limits{MaxEvents: 5, MaxEffectEvents: 2}, 2},
		{"default-from-presentation", Limits{MaxEvents: 2}, 2},
		{"default", Limits{}, 4096},
		{"negative-defaults", Limits{MaxEvents: -1, MaxEffectEvents: -1}, 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewEventBufferWithIndependentEffects(tc.limits)
			for i := 0; i < c.limits.MaxEvents; i++ {
				c.EmitStatus(Event{})
			}
			for i := 0; i <= tc.bound; i++ {
				if c.EmitSmokeEnd(Event{}) {
					t.Fatal("full presentation window admitted an effect operation")
				}
			}
			if len(c.EffectEvents()) != tc.bound {
				t.Fatalf("independent capacity = %d, want %d", len(c.EffectEvents()), tc.bound)
			}
			c.Reset()
			c.EmitSmokeStart(Event{})
			got := c.EffectEvents()
			if len(got) != 1 || got[0].ID != uint32(tc.bound+1) || got[0].Sequence != uint64(tc.bound+1) {
				t.Fatalf("capacity refusal consumed identity or reset did not reopen channel: %+v", got)
			}
		})
	}
}

func TestIndependentEffectsPresentationCounterExhaustion(t *testing.T) {
	for _, field := range []string{"ID", "sequence", "exhausted"} {
		t.Run(field, func(t *testing.T) {
			c := NewEventBufferWithIndependentEffects(Limits{})
			switch field {
			case "ID":
				c.nextID = ^uint32(0)
			case "sequence":
				c.nextSequence = ^uint64(0)
			case "exhausted":
				c.exhausted = true
			}
			if c.EmitAudio(Event{}) != (field != "exhausted") {
				t.Fatal("last presentation identity was not admitted exactly once")
			}
			for i := 0; i < 2; i++ {
				if c.EmitExplosion(Event{}) {
					t.Fatal("exhausted presentation admitted an event")
				}
				if got := c.EffectEvents(); len(got) != 1 || got[0].ID != uint32(i+1) || got[0].Sequence != uint64(i+1) {
					t.Fatalf("presentation exhaustion reached effect channel: %+v", got)
				}
				c.Reset()
			}
		})
	}
}

func TestIndependentEffectsOwnExhaustionDoesNotChangePresentation(t *testing.T) {
	c := NewEventBufferWithIndependentEffects(Limits{})
	c.independentEffects.nextID = ^uint32(0)
	c.independentEffects.nextSequence = ^uint64(0)
	if !c.EmitImpact(Event{}) || !c.EmitSmokeEnd(Event{}) {
		t.Fatal("effect exhaustion reached presentation verdict")
	}
	got := c.EffectEvents()
	if len(got) != 1 || got[0].ID != ^uint32(0) || got[0].Sequence != ^uint64(0) {
		t.Fatalf("last effect identity = %+v", got)
	}
	c.Reset()
	if !c.EmitExplosion(Event{}) || c.EffectEvents() != nil {
		t.Fatal("reset revived exhausted effect identities or stopped presentation")
	}
	if got := c.StagingEvents(); len(got) != 1 || got[0].ID != 3 || got[0].Sequence != 3 {
		t.Fatalf("presentation identities = %+v", got)
	}
}

func TestIndependentEffectsPreserveOrdinaryPresentation(t *testing.T) {
	var absent *EventBuffer
	if absent.EffectEvents() != nil {
		t.Fatal("nil buffer returned effect events")
	}
	ordinary := NewEventBuffer(Limits{MaxEvents: 4, MaxEffectEvents: 2})
	online := NewEventBufferWithIndependentEffects(Limits{MaxEvents: 4, MaxEffectEvents: 2})
	inputs := []Event{{Kind: KindAudio}, {Kind: KindImpact}, {Kind: KindSmokeEnd},
		{Kind: KindStatus}, {Kind: KindMusicIntensity}, {Kind: KindCorpse},
		{Kind: 0}, {Kind: KindExplosion, Lifetime: -1}}
	for window := 0; window < 2; window++ {
		for _, e := range inputs {
			if ordinary.Admit(e) != online.Admit(e) {
				t.Fatalf("presentation verdict changed for %+v", e)
			}
			if !reflect.DeepEqual(ordinary.Snapshot(), online.Snapshot()) {
				t.Fatal("presentation values, order or diagnostics changed")
			}
		}
		if effects, staging := ordinary.EffectEvents(), ordinary.StagingEvents(); len(effects) != len(staging) || &effects[0] != &staging[0] {
			t.Fatal("ordinary effect input does not alias the exact staging window")
		}
		ordinary.Reset()
		online.Reset()
	}
}
