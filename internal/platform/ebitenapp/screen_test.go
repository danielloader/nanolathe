package ebitenapp

import (
	"testing"
	"time"
)

// The screen's layout asks the monitor for its scale only when the window's
// size changes or half a second has passed, never every frame.
func TestScreenScaleQueriesTheMonitorRarely(t *testing.T) {
	reads := 0
	s := screenScale{monitorScaleF: func() float64 { reads++; return 2 }}
	at := time.Unix(1000, 0)
	for i := range 60 {
		if got := s.get(800, 600, at.Add(time.Duration(i)*8*time.Millisecond)); got != 2 {
			t.Fatalf("scale %v", got)
		}
	}
	if reads != 1 {
		t.Fatalf("60 frames at one size read the monitor %d times", reads)
	}
	s.get(1024, 600, at.Add(480*time.Millisecond))
	s.get(1024, 600, at.Add(990*time.Millisecond))
	if reads != 3 {
		t.Fatalf("a resize and a half-second refresh made %d reads, want 3", reads)
	}
	if (&screenScale{monitorScaleF: func() float64 { return 0 }}).get(1, 1, at) != 1 {
		t.Fatal("an unknown scale did not fall back to 1")
	}
}
