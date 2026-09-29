package ebitenapp

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

// presentPattern feeds Draw arrivals, in milliseconds apart, through a 60 FPS
// cap and returns which ones presented.
func presentPattern(t *testing.T, gaps []float64) []bool {
	t.Helper()
	a := &app{presentInterval: time.Second / 60}
	at := time.Unix(1000, 0)
	out := make([]bool, 0, len(gaps)+1)
	out = append(out, a.presentDue(at))
	for _, g := range gaps {
		at = at.Add(time.Duration(g * float64(time.Millisecond)))
		out = append(out, a.presentDue(at))
	}
	return out
}

func repeatGaps(n int, gaps ...float64) []float64 {
	out := make([]float64, 0, n*len(gaps))
	for range n {
		out = append(out, gaps...)
	}
	return out
}

const refresh120 = 1000.0 / 120

// At 120 Hz the 60 cap presents exactly every other Draw.
func TestPresentCapAlternatesAt120Hz(t *testing.T) {
	got := presentPattern(t, repeatGaps(40, refresh120))
	for i := 12; i < len(got); i++ {
		if want := i%2 == 0; got[i] != want {
			t.Fatalf("draw %d presented=%v, want %v: %v", i, got[i], want, got)
		}
	}
}

// A heavy presented frame delays the odd Draw at 120 Hz, which then arrives
// long after the present and shortly before the next refresh. It is still the
// odd refresh and must not present: a wider allowance that presented it showed
// the heavy frame for one refresh and the next for three in the window trace.
func TestPresentCapSkipsDelayedOddDrawAt120Hz(t *testing.T) {
	got := presentPattern(t, repeatGaps(20, 12.6, 2*refresh120-12.6))
	for i := 12; i < len(got); i++ {
		if want := i%2 == 0; got[i] != want {
			t.Fatalf("draw %d presented=%v, want %v: %v", i, got[i], want, got[12:])
		}
	}
}

// At 60 Hz every Draw presents, including the early Draw that follows a late
// one: the eighth-interval test alone dropped it, so a 5 ms hitch cost two
// refreshes.
func TestPresentCapKeepsLateThenEarlyDrawAt60Hz(t *testing.T) {
	gaps := append(repeatGaps(12, 16.67), 21.9, 11.9, 16.4, 16.9, 19.6, 13.7, 17.4)
	got := presentPattern(t, gaps)
	for i, p := range got {
		if !p {
			t.Fatalf("draw %d skipped at 60 Hz: %v", i, got)
		}
	}
}

// A switch from 120 Hz to 60 Hz presents every Draw at once, before the
// refresh measurement has caught up.
func TestPresentCapSwitchTo60Hz(t *testing.T) {
	gaps := append(repeatGaps(20, refresh120), repeatGaps(12, 16.67)...)
	got := presentPattern(t, gaps)
	for i := 21; i < len(got); i++ {
		if !got[i] {
			t.Fatalf("draw %d skipped after the switch to 60 Hz: %v", i, got[20:])
		}
	}
}

// Back-to-back Draws do not present twice, and a long stall presents the late
// Draw and then resumes alternating instead of bursting to catch up.
func TestPresentCapBurstAndStall(t *testing.T) {
	gaps := append(repeatGaps(12, refresh120), 0.8, 5.8, refresh120)
	got := presentPattern(t, gaps)
	if !got[12] || got[13] || got[14] || !got[15] {
		t.Fatalf("burst presented %v, want only the Draw a cap interval later", got[12:])
	}
	gaps = append(repeatGaps(12, refresh120), 120)
	gaps = append(gaps, repeatGaps(6, refresh120)...)
	got = presentPattern(t, gaps)
	after := got[13:]
	if !after[0] || after[1] || !after[2] || after[3] || !after[4] {
		t.Fatalf("stall recovery presented %v, want alternating from the late Draw", after)
	}
}

// At 60 Hz a Draw arriving within half a refresh of the last present, one of a
// burst, is still skipped.
func TestPresentCapSkipsBurstAt60Hz(t *testing.T) {
	got := presentPattern(t, append(repeatGaps(12, 16.67), 0.8, 15.9, 16.67))
	if !got[12] || got[13] || !got[14] || !got[15] {
		t.Fatalf("60 Hz burst presented %v", got[12:])
	}
}

// A present whose Draw arrived late — here 5 ms into its refresh — still
// reached the screen at the refresh after its own, and the Draw after it
// arrives back on the display's refresh. The next present stays two refreshes
// after the late one's refresh instead of waiting a third, which is what the
// window trace of a heavy save under host load showed for half its late
// frames.
func TestPresentCapBooksLatePresentAtItsRefresh(t *testing.T) {
	gaps := append(repeatGaps(13, refresh120), refresh120+5, refresh120-5)
	gaps = append(gaps, repeatGaps(4, refresh120)...)
	got := presentPattern(t, gaps)
	want := map[int]bool{14: true, 15: false, 16: true, 17: false, 18: true}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("draw %d presented=%v, want %v: %v", i, got[i], w, got[12:])
		}
	}
}

// While the pacer places frames, the Draws Ebitengine runs are the ones the
// pacer passed a refresh on for, a cap's interval apart, and each presents;
// the refresh the window records is the display's.
func TestPresentCapDefersToThePacer(t *testing.T) {
	a := &app{presentInterval: time.Second / 60, pacer: &framePacer{}}
	a.pacer.setInterval(a.presentInterval)
	at := time.Unix(1000, 0)
	const period = time.Second / 120
	presented := 0
	for i := range 80 {
		at = at.Add(period)
		if !a.pacer.refresh(at, time.Duration(i+1)*period) {
			continue
		}
		a.pacer.returned()
		if i < 2*pacerSpacings {
			a.presentDue(at)
			continue
		}
		// A late Draw is still the frame the refresh was passed on for.
		if !a.presentDue(at.Add(time.Duration(i%3) * time.Millisecond)) {
			t.Fatalf("refresh %d was passed on and its Draw did not present", i)
		}
		presented++
		if a.refreshPeriod != period {
			t.Fatalf("refresh %d: recorded a refresh period of %v, want %v", i, a.refreshPeriod, period)
		}
	}
	if want := (80 - 2*pacerSpacings) / 2; presented != want {
		t.Fatalf("%d Draws presented, want %d", presented, want)
	}
}

// presentTrain feeds presentDue one Draw for each of n refreshes of a display
// refreshing hz times a second, under a cap of fps. Each Draw arrives late for
// its refresh by up to jitter, drawn from a fixed sequence, and the refreshes
// in drop have no Draw. It returns the refreshes that presented and, for each,
// the spacing the schedule planned to the next.
func presentTrain(hz float64, fps int, jitter time.Duration, n int, drop map[int]bool) (presents []int, planned []time.Duration) {
	a := &app{presentInterval: time.Second / time.Duration(fps)}
	rng := rand.New(rand.NewPCG(uint64(hz*1000), uint64(fps)))
	start := time.Unix(1000, 0)
	for i := range n {
		late := time.Duration(rng.Int64N(int64(jitter) + 1))
		if drop[i] {
			continue
		}
		if a.presentDue(start.Add(time.Duration(float64(i)*float64(time.Second)/hz) + late)) {
			presents = append(presents, i)
			planned = append(planned, a.presentPlan.spacing)
		}
	}
	return presents, planned
}

// scheduleRatio is how many refreshes a cap's interval is, and 1 where every
// refresh presents: a display no more than an eighth of the cap's interval
// faster than the cap.
func scheduleRatio(hz float64, fps int) float64 {
	if interval := 1 / float64(fps); 1/hz >= interval-interval/8 {
		return 1
	}
	return hz / float64(fps)
}

// Where the display's refresh does not divide the cap, the schedule still
// presents at the cap's rate, never more than one frame ahead of or behind the
// cap's own clock, with the refreshes between two presents always one of the
// two whole numbers either side of the ratio: never a burst and never an extra
// refresh held. Before the schedule a 120 cap at 144 Hz presented 72 times a
// second, a 60 cap 48 and a 30 cap 28.8; at 165 Hz 82.5, 55 and 33; at 75 Hz
// a 60 cap 37.5; at 100 Hz a 60 cap 50 and a 30 cap 33.3; and at 240 Hz a 30
// cap 32. The Draws arrive up to an eighth of a refresh late.
func TestPresentScheduleMatchesTheCapOnAnyRefresh(t *testing.T) {
	const seconds = 30
	for _, hz := range []float64{75, 100, 120, 144, 165, 240} {
		for _, fps := range []int{30, 60, 120} {
			period := time.Duration(float64(time.Second) / hz)
			presents, _ := presentTrain(hz, fps, period/8, seconds*int(hz), nil)
			ratio := scheduleRatio(hz, fps)
			// The first second settles the refresh measurement.
			first := 0
			for presents[first] < int(hz) {
				first++
			}
			lo, hi := int(math.Floor(ratio)), int(math.Ceil(ratio))
			for i := first + 1; i < len(presents); i++ {
				if gap := presents[i] - presents[i-1]; gap < lo || gap > hi {
					t.Fatalf("%v Hz, %d cap: refreshes %d and %d presented %d apart, want %d or %d", hz, fps, presents[i-1], presents[i], gap, lo, hi)
				}
				// The cap's own clock, counted from the first present.
				want := float64(presents[i]-presents[first]) / ratio
				if got := float64(i - first); math.Abs(got-want) > 1 {
					t.Fatalf("%v Hz, %d cap: %v presents by refresh %d, want %.2f", hz, fps, got, presents[i], want)
				}
			}
			rate := float64(len(presents)-first-1) * hz / float64(presents[len(presents)-1]-presents[first])
			if want := hz / ratio; math.Abs(rate-want) > want/200 {
				t.Fatalf("%v Hz, %d cap: %.2f presents a second, want %.2f", hz, fps, rate, want)
			}
		}
	}
}

// At 144 Hz a 120 cap is 1.2 refreshes: five presents in every six
// refreshes, the long spacing once in five.
func TestPresentScheduleCadenceAt144Hz(t *testing.T) {
	presents, _ := presentTrain(144, 120, 0, 144*5, nil)
	long := -1
	for i := 20; i < len(presents); i++ {
		switch presents[i] - presents[i-1] {
		case 1:
		case 2:
			if long >= 0 && i-long != 5 {
				t.Fatalf("long spacings %d presents apart, want 5: %v", i-long, presents[long:i+1])
			}
			long = i
		default:
			t.Fatalf("refreshes %d and %d presented %d apart", presents[i-1], presents[i], presents[i]-presents[i-1])
		}
	}
	if long < 0 {
		t.Fatal("no long spacing: the cap was not held")
	}
}

// A cap within a thirty-second of a whole number of refreshes keeps the even
// cadence: the display and the cap run on different clocks, and holding the
// schedule to the cap would correct the drift with a frame one refresh short
// or long every several seconds.
func TestPresentScheduleHoldsAnEvenCadenceNearAMultiple(t *testing.T) {
	for _, c := range []struct {
		hz   float64
		fps  int
		gaps int
	}{
		{120000.0 / 1001, 60, 2},
		{120.5, 60, 2},
		{60000.0 / 1001, 30, 2},
		{240000.0 / 1001, 60, 4},
		{120000.0 / 1001, 30, 4},
	} {
		presents, planned := presentTrain(c.hz, c.fps, 0, 60*int(c.hz), nil)
		for i := 10; i < len(presents); i++ {
			if gap := presents[i] - presents[i-1]; gap != c.gaps {
				t.Fatalf("%.2f Hz, %d cap: refreshes %d and %d presented %d apart, want %d", c.hz, c.fps, presents[i-1], presents[i], gap, c.gaps)
			}
			if planned[i] != 0 {
				t.Fatalf("%.2f Hz, %d cap: an even cadence planned a spacing of %v", c.hz, c.fps, planned[i])
			}
		}
	}
}

// A stall, or a refresh with no Draw, restarts the schedule from the late
// present: the window does not present faster than the cap to catch up, and
// the cadence resumes from there.
func TestPresentScheduleRestartsAfterALatePresent(t *testing.T) {
	// The first refresh from 1000 on that the schedule presents at.
	undisturbed, _ := presentTrain(144, 120, 0, 144*10, nil)
	first := 0
	for undisturbed[first] < 1000 {
		first++
	}
	at := undisturbed[first]
	// That refresh with no Draw, and a stall of about a tenth of a second.
	stall := map[int]bool{}
	for i := at; i < at+14; i++ {
		stall[i] = true
	}
	for _, drop := range []map[int]bool{{at: true}, stall} {
		presents, _ := presentTrain(144, 120, 0, 144*10, drop)
		resumed := 0
		for i := 20; i < len(presents); i++ {
			if presents[i-1] < at && presents[i] > at {
				resumed = i
				continue
			}
			if gap := presents[i] - presents[i-1]; gap != 1 && gap != 2 {
				t.Fatalf("refreshes %d and %d presented %d apart", presents[i-1], presents[i], gap)
			}
		}
		// From the late present on, the cap's clock starts again.
		for i := resumed + 1; i < len(presents); i++ {
			want := float64(presents[i]-presents[resumed]) / 1.2
			if got := float64(i - resumed); got-want > 0.5 || want-got > 1 {
				t.Fatalf("%d refreshes dropped: %v presents %d refreshes after the late one, want %.2f", len(drop), got, presents[i]-presents[resumed], want)
			}
		}
	}
}

// While the cadence is uneven the schedule plans each present's spacing, and
// the pre-record extrapolates over that plan (observeDraw); the plan is the
// spacing that follows.
func TestPresentSchedulePlansTheNextPresent(t *testing.T) {
	for _, c := range []struct {
		hz  float64
		fps int
	}{{144, 120}, {144, 60}, {165, 30}, {75, 60}, {100, 60}} {
		presents, planned := presentTrain(c.hz, c.fps, 0, 10*int(c.hz), nil)
		period := float64(time.Second) / c.hz
		for i := 20; i+1 < len(presents); i++ {
			next := time.Duration(float64(presents[i+1]-presents[i]) * period)
			if d := planned[i] - next; d < -time.Microsecond || d > time.Microsecond {
				t.Fatalf("%v Hz, %d cap: planned %v after refresh %d, the next present came %v after it", c.hz, c.fps, planned[i], presents[i], next)
			}
			var p pipeline
			if got := p.observeDraw(time.Unix(1000, 0), time.Second/time.Duration(c.fps), planned[i]); got != planned[i] {
				t.Fatalf("%v Hz, %d cap: the pre-record extrapolates over %v, want the plan %v", c.hz, c.fps, got, planned[i])
			}
		}
	}
}
