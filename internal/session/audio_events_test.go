package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

type sessionAudioOutputSpy struct {
	plays int
}

func (s *sessionAudioOutputSpy) PlaySample(*audio.Sample, float64, float64) error {
	s.plays++
	return nil
}

func TestCommittedAudioSurvivesAudienceChangeBeforeDrain(t *testing.T) {
	s := &Session{
		Audio:       audio.NewService(nil),
		Clock:       &clock.State{GlobalTick: 7},
		Vis:         visibility.New(&world.Terrain{CellW: 2, CellH: 2}, visibility.ModeHistoryEnabled|visibility.ModeCurrentEnabled),
		publication: newPublicationState(frame.NewEventBuffer(frame.Limits{}), 0, nil),
	}
	cache := s.Audio.Cache
	if _, err := cache.Put("shot", []byte{128, 129}); err != nil {
		t.Fatal(err)
	}
	s.Audio.Registry.SetCache(cache)
	s.Audio.BindCatalog(&content.Catalog{
		AliasOrder:       []*content.SoundAlias{{Alias: "shot", Sound: "shot"}},
		WeaponSoundPaths: []string{"shot"},
	}, nil)
	s.Vis.ByteGrid(0)[0] = 1
	if _, _, ok := s.EmitWeaponHit("shot", [3]numeric.Fixed{}, false); !ok {
		t.Fatal("audible event was not admitted")
	}
	s.Vis.ByteGrid(0)[0] = 0
	old := audio.GlobalOutput()
	spy := &sessionAudioOutputSpy{}
	audio.SetGlobalOutput(spy)
	t.Cleanup(func() { audio.SetGlobalOutput(old) })
	s.Audio.DrainEvents(7, s.publication.events.SnapshotEvents())
	if spy.plays != 1 {
		t.Fatalf("admitted event was suppressed after audience changed: plays=%d", spy.plays)
	}
}

// Source identity crosses publication as authored text, not an audio-service
// index. Only the weapon helpers request anonymous lookup [03 §8.3].
func TestWeaponAudioPublicationKeepsAnonymousSource(t *testing.T) {
	s := &Session{
		Audio:       audio.NewService(nil),
		Vis:         visibility.New(&world.Terrain{CellW: 2, CellH: 2}, visibility.ModeHistoryEnabled|visibility.ModeCurrentEnabled),
		publication: newPublicationState(frame.NewEventBuffer(frame.Limits{}), 0, nil),
	}
	s.Vis.ByteGrid(0)[0] = 1
	pos := [3]numeric.Fixed{}
	s.EmitPositional("same", pos)
	s.EmitWeaponStart("same", pos)
	s.EmitWeaponHit("same", pos, true)
	events := s.publication.events.SnapshotEvents()
	if len(events) != 3 || events[0].AudioAnonymous || !events[1].AudioAnonymous || !events[2].AudioAnonymous || !events[2].AudioWater {
		t.Fatalf("published sources = %+v", events)
	}
	for _, event := range events {
		if event.Sound != "same" || !event.AudioAudible {
			t.Fatalf("authored text or audience changed = %+v", event)
		}
	}
	if s.Audio.Registry.Count() != 0 {
		t.Fatal("authoritative publication registered an audio identity")
	}
}
