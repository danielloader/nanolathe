package audio

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type streamTraceOutput struct {
	outputSpy
	trace []string
	fail  bool
}

func (s *streamTraceOutput) PlayStream(sample *Sample, _ float64) error {
	s.trace = append(s.trace, "play:"+sample.Alias)
	if s.fail {
		return errors.New("authored output failure")
	}
	return nil
}

func (s *streamTraceOutput) StopStream() { s.trace = append(s.trace, "stop") }

func installStreamTrace(t *testing.T) *streamTraceOutput {
	t.Helper()
	old := GlobalOutput()
	out := &streamTraceOutput{}
	SetGlobalOutput(out)
	t.Cleanup(func() { SetGlobalOutput(old) })
	return out
}

// The older callback removes the newest recorded timer. Its own live period
// reloads, so even an explicit stop cannot remove that lost ID [03 R-AUD-02 §1].
func TestStreamOlderTimerSurvivesCallbackAndStop(t *testing.T) {
	fs, _ := anonymousSoundFS(t, "first", "later")
	s := NewService(fs)
	out := installStreamTrace(t)
	s.StartStream("sounds/first.wav", 0, 60, 0)
	s.StartStream("sounds/later.wav", 0, 60, 10)
	s.TickStream(59)
	if len(out.trace) != 0 {
		t.Fatal("stream opened before due time")
	}
	s.TickStream(60)
	s.StopStream() // time 61: stop does not service the timer table.
	s.TickStream(119)
	s.TickStream(120)
	want := []string{"play:sounds/later.wav", "stop", "play:sounds/later.wav"}
	if !reflect.DeepEqual(out.trace, want) {
		t.Fatalf("overlap trace = %v, want %v", out.trace, want)
	}
	// A late service opens once, then reloads from that service time.
	s.TickStream(1000)
	s.TickStream(1001)
	s.TickStream(1059)
	if len(out.trace) != 5 {
		t.Fatalf("late service caught up or reloaded from old deadline: %v", out.trace)
	}
	s.TickStream(1060)
	if len(out.trace) != 7 {
		t.Fatalf("late service failed to reload its period: %v", out.trace)
	}
	s.Close()
	s.TickStream(2000)
	if len(out.trace) != 8 || out.trace[7] != "stop" {
		t.Fatalf("closed host service retained a callback: %v", out.trace)
	}
}

// Start replaces the shared path BEFORE registration services an already-due
// timer. That timer still owns the recorded ID and cancels itself; the new
// registration can reuse its slot [03 R-AUD-02 §1][01 R-PLAT-02 §4].
func TestStreamRegistrationServicesDueTimerBeforeSlotReuse(t *testing.T) {
	fs, _ := anonymousSoundFS(t, "first", "later")
	s := NewService(fs)
	out := installStreamTrace(t)
	s.StartStream("sounds/first.wav", 0, 60, 0)
	s.StartStream("sounds/later.wav", 0, 60, 60)
	if !reflect.DeepEqual(out.trace, []string{"play:sounds/later.wav"}) {
		t.Fatalf("registration pre-service = %v", out.trace)
	}
	if len(s.streamTimers) != 1 {
		t.Fatal("registration failed to reuse the canceled slot")
	}
	s.TickStream(120)
	s.TickStream(180)
	want := []string{"play:sounds/later.wav", "stop", "play:sounds/later.wav"}
	if !reflect.DeepEqual(out.trace, want) {
		t.Fatalf("replacement failed to self-cancel: %v", out.trace)
	}
}

// The live slot walk is not sorted by deadlines. The first due callback
// cancels the last recorded slot before that later slot can run.
func TestStreamDueWalkUsesLiveSlotOrder(t *testing.T) {
	s := NewService(testAudioFS(t, "voice"))
	out := installStreamTrace(t)
	s.StartStream("sounds/voice.wav", 0, 100, 0)
	s.StartStream("sounds/voice.wav", 0, 50, 0)
	s.StartStream("sounds/voice.wav", 0, 20, 0)
	s.TickStream(100)
	want := []string{"play:sounds/voice.wav", "stop", "play:sounds/voice.wav"}
	if !reflect.DeepEqual(out.trace, want) {
		t.Fatalf("live slot order = %v, want %v", out.trace, want)
	}
	s.TickStream(149)
	if len(out.trace) != 3 {
		t.Fatal("canceled slot fired or late service retained a deficit")
	}
	s.TickStream(150)
	if len(out.trace) != 5 {
		t.Fatal("second surviving slot did not reload its own period")
	}
}

func TestStreamReplacementLoadsBeforeStoppingCurrentSample(t *testing.T) {
	_, root := anonymousSoundFS(t, "first", "later")
	// The portable decoder rejects this bit depth; no device outcome is assumed.
	if err := os.WriteFile(filepath.Join(root, "sounds", "invalid.wav"), buildRIFF(1, 11025, 24, []byte{128, 129, 130}), 0644); err != nil {
		t.Fatal(err)
	}
	s := NewService(mountAnonymousSounds(t, root))
	out := installStreamTrace(t)
	s.StartStream("sounds/first.wav", 0, 60, 0)
	s.TickStream(60)
	for i, path := range []string{"sounds/missing.wav", "sounds/invalid.wav"} {
		now := uint32(100 + 100*i)
		s.StartStream(path, 0, 60, now)
		s.TickStream(now + 60)
		if len(out.trace) != 1 || !s.streamPlaying {
			t.Fatalf("unavailable replacement %q stopped prior stream: %v", path, out.trace)
		}
	}
	s.StartStream("sounds/later.wav", 0, 60, 300)
	s.TickStream(360)
	want := []string{"play:sounds/first.wav", "stop", "play:sounds/later.wav"}
	if !reflect.DeepEqual(out.trace, want) {
		t.Fatalf("valid replacement order = %v", out.trace)
	}
	out.fail = true
	s.StartStream("sounds/first.wav", 0, 60, 400)
	s.TickStream(460)
	if s.streamPlaying || len(out.trace) != 5 {
		t.Fatalf("portable output failure changed host state: playing=%v trace=%v", s.streamPlaying, out.trace)
	}
}

// Streaming retries are future callbacks, not the static registry's failed
// identity lifetime. A lost timer remains armed even when opening fails.
func TestStreamLostTimerCanOpenFileAfterEarlierFailure(t *testing.T) {
	fs, root := anonymousSoundFS(t)
	s := NewService(fs)
	out := installStreamTrace(t)
	s.StartStream("sounds/late.wav", 0, 60, 0)
	s.StartStream("sounds/late.wav", 0, 60, 10)
	s.TickStream(60)
	if len(out.trace) != 0 {
		t.Fatal("missing file played")
	}
	if err := os.WriteFile(filepath.Join(root, "sounds", "late.wav"), buildRIFF(1, 11025, 8, []byte{128}), 0644); err != nil {
		t.Fatal(err)
	}
	s.Init(mountAnonymousSounds(t, root))
	s.TickStream(120)
	if !reflect.DeepEqual(out.trace, []string{"play:sounds/late.wav"}) || s.Registry.Count() != 0 {
		t.Fatalf("lost stream timer failed to reopen independently: %v", out.trace)
	}
}
