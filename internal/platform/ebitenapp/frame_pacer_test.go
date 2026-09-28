package ebitenapp

import (
	"testing"
	"time"
)

// pacedLoop runs a pacer between a display link and a game loop in virtual
// time, the way the window does. The link reports a refresh every period
// unless its thread is held: a slot it passed on is not returned until a
// frame takes its drawable, and a refresh that falls due meanwhile is lost.
// A frame begins when the pacer lets it, works, waits for a slot and
// presents. A frame presented for a refresh reaches the display latency
// refreshes after it.
type pacedLoop struct {
	p       framePacer
	period  time.Duration
	latency time.Duration
	at      time.Duration
	next    int
	// lost names refreshes the link never reports.
	lost map[int]bool
	// held is whether a slot is with the loop's frame, and heldSlot its
	// display time.
	held     bool
	heldSlot time.Duration

	begins, works, callbacks, slots []time.Duration
}

func newPacedLoop(hz int, cap time.Duration) *pacedLoop {
	l := &pacedLoop{period: time.Second / time.Duration(hz), latency: 2, lost: map[int]bool{}}
	l.p.setInterval(cap)
	return l
}

func (l *pacedLoop) now() time.Time { return time.Unix(1000, 0).Add(l.at) }

// advance moves time on to t, reporting the refreshes that fall due.
func (l *pacedLoop) advance(t time.Duration) {
	for {
		due := time.Duration(l.next) * l.period
		if due > t {
			break
		}
		l.at = max(l.at, due)
		if !l.held && !l.lost[l.next] {
			if l.p.refresh(l.now(), due+l.latency*l.period) {
				l.held, l.heldSlot = true, due+l.latency*l.period
				l.callbacks = append(l.callbacks, due)
			}
		}
		l.next++
	}
	l.at = max(l.at, t)
}

// sleep is the pacer's wait: time passes to the next refresh, or for as long
// as the pacer asked when the link's thread is held.
func (l *pacedLoop) sleep(p *framePacer, limit time.Duration) {
	p.mu.Unlock()
	defer p.mu.Lock()
	until := l.at + limit
	if due := time.Duration(l.next) * l.period; !l.held && due < until {
		until = due
	}
	l.advance(until)
}

// frame runs one frame whose Update and Draw take work, and whose flush
// takes a quarter of a refresh more.
func (l *pacedLoop) frame(work time.Duration) {
	l.p.awaitFrame(l.now, l.sleep)
	l.begins = append(l.begins, l.at)
	l.works = append(l.works, work)
	l.advance(l.at + work)
	l.p.observeWork(work)
	l.advance(l.at + l.period/4)
	for !l.held {
		l.advance(time.Duration(l.next) * l.period)
	}
	l.slots = append(l.slots, l.heldSlot)
	l.held = false
	l.p.returned()
	// The present is registered and the loop comes round.
	l.advance(l.at + 300*time.Microsecond)
}

// lead is how long before the refresh frame i presented at the frame began.
func (l *pacedLoop) lead(i int) time.Duration {
	return l.slots[i] - l.latency*l.period - l.begins[i]
}

// A 60 cap on a 120 Hz display presents on every other refresh, and each
// frame begins at the refresh before the one it presents at.
func TestFramePacerHalvesA120HzDisplay(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	for range 60 {
		l.frame(3 * time.Millisecond)
	}
	for i := 2; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		if got := l.lead(i); got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
	if got := l.p.refreshPeriod(l.now()); got != l.period {
		t.Fatalf("paced refresh period %v, want the display's %v", got, l.period)
	}
}

// A composited window's frames reach the display five refreshes after the
// refresh they are presented for; the cadence is the same.
func TestFramePacerHalvesA120HzDisplayThroughACompositor(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	l.latency = 5
	for range 60 {
		l.frame(3 * time.Millisecond)
	}
	for i := pacerSpacings; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		if got := l.lead(i); got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
}

// A 30 cap on a 120 Hz display presents on one refresh in four.
func TestFramePacerQuartersA120HzDisplay(t *testing.T) {
	l := newPacedLoop(120, time.Second/30)
	for range 40 {
		l.frame(3 * time.Millisecond)
	}
	for i := 2; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 4*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want four refreshes", i, got)
		}
		if got := l.lead(i); got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
}

// With no cap, a cap at the display's rate or a display no faster than the
// cap, every refresh is passed on and the loop is never held.
func TestFramePacerLeavesAnUncappedDisplayAlone(t *testing.T) {
	for _, c := range []struct {
		name string
		cap  time.Duration
		hz   int
	}{
		{"uncapped", 0, 120},
		{"cap at the refresh", time.Second / 120, 120},
		{"60 cap at 60 Hz", time.Second / 60, 60},
		{"120 cap at 60 Hz", time.Second / 120, 60},
	} {
		l := newPacedLoop(c.hz, c.cap)
		for range 30 {
			l.frame(time.Millisecond)
		}
		for i := 2; i < len(l.slots); i++ {
			if got := l.slots[i] - l.slots[i-1]; got != l.period {
				t.Fatalf("%s: frame %d reached the display %v after the one before, want every refresh", c.name, i, got)
			}
			// The frame begins as soon as the one before has presented.
			if wait := l.begins[i] - l.callbacks[i-1]; wait > time.Millisecond {
				t.Fatalf("%s: frame %d was held for %v", c.name, i, wait)
			}
		}
		if got := l.p.refreshPeriod(l.now()); got != 0 {
			t.Fatalf("%s: reported a paced refresh period %v", c.name, got)
		}
	}
}

// A frame that runs past its refresh still presents at it, and the frames
// after it keep their places.
func TestFramePacerKeepsItsPlacesAfterALateFrame(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	for i := range 40 {
		work := 3 * time.Millisecond
		if i == 20 {
			work = 9 * time.Millisecond
		}
		l.frame(work)
	}
	for i := 2; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		if got := l.lead(i); i != 20 && got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
}

// A frame that runs past the refresh after its own holds the link's thread
// through it. The next frame begins at once, and the cadence resumes on the
// display's grid.
func TestFramePacerRecoversFromAFrameThatHeldTheLink(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	for i := range 40 {
		work := 3 * time.Millisecond
		if i == 20 {
			work = 17 * time.Millisecond
		}
		l.frame(work)
	}
	ended := l.begins[20] + 17*time.Millisecond + l.period/4
	if wait := l.begins[21] - ended; wait > l.period {
		t.Fatalf("the frame after the long one waited %v to begin", wait)
	}
	for i := 23; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		if got := l.lead(i); got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
	for i := 1; i < len(l.slots); i++ {
		if (l.slots[i]-l.slots[0])%l.period != 0 || l.slots[i] <= l.slots[i-1] {
			t.Fatalf("frame %d left the display's grid: %v after %v", i, l.slots[i], l.slots[i-1])
		}
	}
}

// When the link does not report the refresh before a slot, the slot itself
// begins the frame.
func TestFramePacerBeginsAtTheSlotWhenTheRefreshBeforeItIsMissed(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	l.lost[41] = true
	for range 40 {
		l.frame(3 * time.Millisecond)
	}
	for i := 2; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		want := l.period
		if l.slots[i] == 44*l.period {
			want = 0
		}
		if got := l.lead(i); got != want {
			t.Fatalf("frame %d began %v before its refresh, want %v", i, got, want)
		}
	}
}

// When the link does not report a slot, the next refresh it reports is one,
// and the slots after it are spaced from that.
func TestFramePacerTakesTheNextRefreshWhenASlotIsMissed(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	l.lost[40] = true
	for range 40 {
		l.frame(3 * time.Millisecond)
	}
	late := 0
	for i := 2; i < len(l.slots); i++ {
		gap := l.slots[i] - l.slots[i-1]
		if gap == 3*l.period && l.lead(i) == 2*l.period {
			late++
			continue
		}
		if gap != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, gap)
		}
		if got := l.lead(i); got != l.period {
			t.Fatalf("frame %d began %v before its refresh, want one refresh", i, got)
		}
	}
	if late != 1 {
		t.Fatalf("%d frames waited out a missed slot, want 1", late)
	}
}

// Frames too long for one refresh begin a refresh earlier and present at
// their slots as they arrive; when the frames are short again they begin a
// refresh before their slots as before.
func TestFramePacerBeginsLongFramesEarlier(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	for range 30 {
		l.frame(3 * time.Millisecond)
	}
	for range 100 {
		l.frame(7 * time.Millisecond)
	}
	heavy := len(l.slots)
	for range 200 {
		l.frame(3 * time.Millisecond)
	}
	late := 0
	for i := 2; i < len(l.slots); i++ {
		if got := l.slots[i] - l.slots[i-1]; got != 2*l.period {
			t.Fatalf("frame %d reached the display %v after the one before, want two refreshes", i, got)
		}
		// A frame is late when it takes its slot after the slot arrived.
		if l.callbacks[i] < l.begins[i]+l.works[i]+l.period/4 {
			late++
			if i >= 30+2*pacerWorkLong && i < heavy {
				t.Fatalf("frame %d was late after the pacer had seen %d long frames", i, i-30)
			}
		}
	}
	if late == 0 || late > 2*pacerWorkLong {
		t.Fatalf("%d frames were late, want the few that showed the frames were long", late)
	}
	// Two refreshes before its slot the frame before it is presenting.
	if got := l.lead(heavy - 1); got > 2*l.period || got < 2*l.period-time.Millisecond {
		t.Fatalf("a long frame began %v before its refresh, want two refreshes", got)
	}
	if got := l.lead(len(l.slots) - 1); got != l.period {
		t.Fatalf("a short frame began %v before its refresh, want one refresh", got)
	}
}

// A frame begins as many refreshes before its slot as it needs, and no
// earlier than the frame before it presented.
func TestFramePacerBeginsNoEarlierThanTheCapAllows(t *testing.T) {
	for _, c := range []struct {
		cap   time.Duration
		work  time.Duration
		early int
	}{
		{time.Second / 30, 16 * time.Millisecond, 2},
		{time.Second / 30, 28 * time.Millisecond, 3},
		{time.Second / 60, 12 * time.Millisecond, 1},
	} {
		l := newPacedLoop(120, c.cap)
		for range 200 {
			l.frame(c.work)
		}
		if l.p.early != c.early {
			t.Fatalf("cap %v, frames of %v: they begin %d refreshes early, want %d", c.cap, c.work, l.p.early, c.early)
		}
		for i := 100; i < len(l.slots); i++ {
			if got := l.slots[i] - l.slots[i-1]; got != c.cap/l.period*l.period {
				t.Fatalf("cap %v, frames of %v: frame %d reached the display %v after the one before", c.cap, c.work, i, got)
			}
			if l.callbacks[i] < l.begins[i]+l.works[i]+l.period/4 {
				t.Fatalf("cap %v, frames of %v: frame %d was late for its slot", c.cap, c.work, i)
			}
		}
	}
}

// A link that stops reporting releases the loop, which then runs unheld.
func TestFramePacerReleasesTheLoopWhenTheLinkFallsQuiet(t *testing.T) {
	l := newPacedLoop(120, time.Second/60)
	for range 10 {
		l.frame(3 * time.Millisecond)
	}
	for i := l.next; i < l.next+1000; i++ {
		l.lost[i] = true
	}
	started := l.at
	l.p.awaitFrame(l.now, l.sleep)
	if waited := l.at - started; waited > pacerQuiet+l.period {
		t.Fatalf("a quiet link held the loop for %v", waited)
	}
	started = l.at
	l.p.awaitFrame(l.now, l.sleep)
	if waited := l.at - started; waited != 0 {
		t.Fatalf("the loop waited %v on a link that had fallen quiet", waited)
	}
}

// A link whose reports are spaced as no display's are is not paced.
func TestFramePacerIgnoresAnImplausiblePeriod(t *testing.T) {
	for _, spacing := range []time.Duration{time.Millisecond / 4, time.Second / 8} {
		var p framePacer
		p.setInterval(time.Second / 30)
		at := time.Unix(1000, 0)
		for i := range 20 {
			base := time.Duration(i+1) * spacing
			if !p.refresh(at.Add(base), base) {
				t.Fatalf("spaced %v: refresh %d kept back", spacing, i)
			}
			p.returned()
		}
		if got := p.refreshPeriod(at.Add(20 * spacing)); got != 0 {
			t.Fatalf("spaced %v: reported a paced refresh period %v", spacing, got)
		}
	}
}

// A window with no pacer keeps its own cap.
// A display is fast from 100 refreshes a second, whatever the cap, once it
// has been measured.
func TestFramePacerFindsAFastDisplay(t *testing.T) {
	for _, c := range []struct {
		name string
		cap  time.Duration
		hz   int
		want bool
	}{
		{"uncapped at 120 Hz", 0, 120, true},
		{"120 cap at 120 Hz", time.Second / 120, 120, true},
		{"60 cap at 120 Hz", time.Second / 60, 120, true},
		{"30 cap at 120 Hz", time.Second / 30, 120, true},
		{"60 cap at 144 Hz", time.Second / 60, 144, true},
		{"uncapped at 60 Hz", 0, 60, false},
		{"60 cap at 60 Hz", time.Second / 60, 60, false},
		{"30 cap at 60 Hz", time.Second / 30, 60, false},
	} {
		l := newPacedLoop(c.hz, c.cap)
		if l.p.fastDisplay() {
			t.Fatalf("%s: fast before the display was measured", c.name)
		}
		for range 30 {
			l.frame(3 * time.Millisecond)
		}
		if got := l.p.fastDisplay(); got != c.want {
			t.Fatalf("%s: fast %v, want %v", c.name, got, c.want)
		}
	}
}

// The link misses refreshes while its thread is held, and a measurement that
// starts again must not change the window's route and back.
func TestFramePacerKeepsTheDisplayWhileItIsMeasuredAgain(t *testing.T) {
	var p framePacer
	p.setInterval(time.Second / 120)
	at, period := time.Unix(1000, 0), time.Second/120
	target := time.Duration(0)
	report := func(spacing time.Duration) {
		target += spacing
		p.refresh(at.Add(target), target)
		p.returned()
	}
	for range 20 {
		report(period)
	}
	if !p.fastDisplay() {
		t.Fatalf("a 120 Hz display is not fast")
	}
	report(time.Second / 4)
	if got := p.refreshPeriod(at.Add(target)); got != 0 || p.period != 0 {
		t.Fatalf("the measurement did not start again: period %v", p.period)
	}
	// The first spacings of a measurement can span refreshes the link
	// missed; they do not make the display a slow one.
	report(2 * period)
	if !p.fastDisplay() {
		t.Fatalf("a measurement that started again made the display slow")
	}
}

func TestFramePacerIsOptional(t *testing.T) {
	var p *framePacer
	p.setInterval(time.Second / 60)
	if p.fastDisplay() {
		t.Fatalf("no pacer reported a fast display")
	}
	p.awaitFrame(time.Now, waitForRefresh)
	if got := p.refreshPeriod(time.Now()); got != 0 {
		t.Fatalf("no pacer reported a refresh period %v", got)
	}
}
