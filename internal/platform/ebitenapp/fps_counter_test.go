package ebitenapp

import (
	"testing"
	"time"
)

func TestFPSLiveMedianSmoothsShortSpikes(t *testing.T) {
	var counter fpsCounter
	start := time.Unix(1, 0)
	counter.observe(start, 0, 0, 0, 0, 0, 0)
	for frame := 1; frame <= 20; frame++ {
		interval := 16 * time.Millisecond
		if frame == 11 {
			interval = 40 * time.Millisecond
		}
		counter.observe(counter.presented.Add(interval), 0, 4*time.Millisecond, time.Millisecond, time.Millisecond, 2*time.Millisecond, time.Millisecond)
	}
	live := counter.live(counter.presented)
	if live.frames != 20 || live.interval != 16*time.Millisecond || live.draw != 4*time.Millisecond {
		t.Fatalf("500 ms medians = %+v", live)
	}
	var columns [10]fpsGraphColumn
	if peak := counter.graph(counter.presented, columns[:]).peakInterval; peak != 40*time.Millisecond {
		t.Fatalf("smoothing hid graph peak: %s", peak)
	}
}

func TestFPSLiveMedianIgnoresSkippedPhaseSamples(t *testing.T) {
	var counter fpsCounter
	start := time.Unix(1, 0)
	counter.observe(start, 0, 0, 0, 0, 0, 0)
	counter.observe(start.Add(16*time.Millisecond), 0, 4*time.Millisecond, 3*time.Millisecond, 0, 2*time.Millisecond, time.Millisecond)
	counter.observe(start.Add(32*time.Millisecond), 0, 6*time.Millisecond, 0, time.Millisecond, 3*time.Millisecond, 2*time.Millisecond)
	counter.observe(start.Add(48*time.Millisecond), 0, 8*time.Millisecond, 5*time.Millisecond, 0, 4*time.Millisecond, 3*time.Millisecond)
	counter.observe(start.Add(64*time.Millisecond), 0, 10*time.Millisecond, 0, 3*time.Millisecond, 5*time.Millisecond, 4*time.Millisecond)
	live := counter.live(start.Add(64 * time.Millisecond))
	if live.sim != 4*time.Millisecond || live.blend != 2*time.Millisecond || live.draw != 7*time.Millisecond || live.record != 3500*time.Microsecond {
		t.Fatalf("phase medians = %+v", live)
	}
	if expired := counter.live(start.Add(time.Second)); expired.frames != 0 || expired.interval != 0 || expired.sim != 0 {
		t.Fatalf("old samples remained live: %+v", expired)
	}
}

func TestFPSGraphRetainsRecentStallsAndBoundedHistory(t *testing.T) {
	var counter fpsCounter
	start := time.Unix(1, 0)
	const late = 16 * time.Millisecond
	counter.observe(start, late, 0, 0, 0, 0, 0)
	for frame := 1; frame <= fpsHistoryCapacity+1; frame++ {
		counter.observe(start.Add(time.Duration(frame)*time.Millisecond), late, 4*time.Millisecond, time.Millisecond, time.Millisecond, 2*time.Millisecond, time.Millisecond)
	}
	if counter.count != fpsHistoryCapacity || len(counter.history) != fpsHistoryCapacity {
		t.Fatalf("history has %d of %d samples", counter.count, len(counter.history))
	}
	var columns [32]fpsGraphColumn
	summary := counter.graph(start.Add(time.Duration(fpsHistoryCapacity+1)*time.Millisecond), columns[:])
	if summary.frames != fpsHistoryCapacity || summary.peakInterval != time.Millisecond || summary.latest.draw != 4*time.Millisecond {
		t.Fatalf("bounded graph summary = %+v", summary)
	}
	stall := start.Add(40 * time.Second)
	counter.observe(stall, late, 12*time.Millisecond, 7*time.Millisecond, 3*time.Millisecond, 9*time.Millisecond, 5*time.Millisecond)
	columns = [32]fpsGraphColumn{}
	summary = counter.graph(stall, columns[:])
	if summary.frames != 1 || summary.peakInterval != stall.Sub(start.Add(time.Duration(fpsHistoryCapacity+1)*time.Millisecond)) || summary.late != 1 {
		t.Fatalf("stale samples leaked into graph: %+v", summary)
	}
	if columns[len(columns)-1].interval != summary.peakInterval || columns[len(columns)-1].draw != 12*time.Millisecond || columns[len(columns)-1].sim != 7*time.Millisecond || columns[len(columns)-1].blend != 3*time.Millisecond || columns[len(columns)-1].record != 9*time.Millisecond || columns[len(columns)-1].submit != 5*time.Millisecond {
		t.Fatal("newest stall is not visible in the graph's right edge")
	}
	if summary.peakDraw != 12*time.Millisecond || summary.peakSim != 7*time.Millisecond || summary.peakBlend != 3*time.Millisecond || summary.peakRecord != 9*time.Millisecond || summary.peakSubmit != 5*time.Millisecond {
		t.Fatalf("phase peaks lost: %+v", summary)
	}
}

// A frame is late when it missed a refresh: at a 60 cap on a 120 Hz display
// the threshold is 16.7 ms plus half of 8.3 ms, so arrival jitter of a
// millisecond or two is on time and the 30 ms interval is the one late frame.
func TestFPSGraphCountsLateFramesWithoutLosingShortSpikes(t *testing.T) {
	var counter fpsCounter
	start := time.Unix(1, 0)
	lateAfter := time.Second/60 + time.Second/240
	counter.observe(start, lateAfter, 0, 0, 0, 0, 0)
	counter.observe(start.Add(17*time.Millisecond), lateAfter, 3*time.Millisecond, 0, 0, 0, 0)
	counter.observe(start.Add(35*time.Millisecond), lateAfter, 4*time.Millisecond, 0, 0, 0, 0)
	counter.observe(start.Add(65*time.Millisecond), lateAfter, 9*time.Millisecond, 0, 0, 0, 0)
	var columns [10]fpsGraphColumn
	summary := counter.graph(start.Add(65*time.Millisecond), columns[:])
	if summary.frames != 3 || summary.late != 1 || summary.peakInterval != 30*time.Millisecond {
		t.Fatalf("late count and spike peak = %+v", summary)
	}
	if columns[len(columns)-1].interval != 30*time.Millisecond || columns[len(columns)-1].draw != 9*time.Millisecond {
		t.Fatalf("same-column spike was lost: %+v", columns[len(columns)-1])
	}
}

// Where the display's refresh does not divide the cap, a frame is late against
// the spacing the present schedule planned for it. At a 120 cap on 144 Hz one
// frame in five is held for two refreshes as planned and is on time; against
// the cap alone, 8.3 ms plus half a 6.9 ms refresh, every one of them counted
// late. A frame that missed the one refresh it was planned for is late.
func TestFPSGraphCountsLateFramesAgainstTheSchedule(t *testing.T) {
	period := float64(time.Second) / 144
	start := time.Unix(1, 0)
	run := func(drop int) (fpsGraphSummary, int) {
		a := &app{presentInterval: time.Second / 120}
		var counter fpsCounter
		held := 0
		var at time.Time
		for i := range 144 * 5 {
			if i == drop {
				continue
			}
			at = start.Add(time.Duration(float64(i) * period))
			if !a.presentDue(at) || i < 144 {
				continue
			}
			if !counter.presented.IsZero() && at.Sub(counter.presented) > time.Second/120+time.Second/288 {
				held++
			}
			counter.observe(at, a.presentLateAfter(), 0, 0, 0, 0, 0)
		}
		var columns [30]fpsGraphColumn
		return counter.graph(at, columns[:]), held
	}
	summary, held := run(-1)
	if summary.late != 0 || held < summary.frames/6 {
		t.Fatalf("%d of %d frames late with %d held for two refreshes, want none late", summary.late, summary.frames, held)
	}
	// Take away the Draw of a present the schedule planned one refresh after
	// the present before it.
	presents, planned := presentTrain(144, 120, 0, 144*5, nil)
	drop := -1
	for i := 1; i < len(presents) && drop < 0; i++ {
		if presents[i] >= 400 && planned[i-1] < time.Second/120 {
			drop = presents[i]
		}
	}
	summary, _ = run(drop)
	if summary.late != 1 {
		t.Fatalf("%d of %d frames late after a missed refresh, want 1", summary.late, summary.frames)
	}
}
