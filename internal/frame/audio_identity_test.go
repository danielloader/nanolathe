package frame

import "testing"

func TestAudioSnapshotPreservesAnonymousSource(t *testing.T) {
	buffer := NewEventBuffer(Limits{})
	buffer.EmitAudio(Event{Sound: "same", AudioAnonymous: true, AudioAudible: true})
	buffer.EmitAudio(Event{Sound: "same", AudioAudible: true})
	got := buffer.SnapshotEvents()
	if len(got) != 2 || !got[0].AudioAnonymous || got[1].AudioAnonymous || got[0].Sound != "same" || got[1].Sound != "same" {
		t.Fatalf("audio identity snapshot = %+v", got)
	}
}
