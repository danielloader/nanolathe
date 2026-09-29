package ebitenapp

import (
	"sync"
	"sync/atomic"
	"time"
)

// framePacer chooses which of the display's refreshes the window presents on
// when the present cap is slower than the display, and starts each frame one
// refresh before its own (DESIGN_GPU_RENDERER §13.5 "Present slots").
//
// Ebitengine presents on every refresh it is given: a Draw that draws nothing
// still puts the retained screen on the display again. On a display link that
// hands out one drawable per refresh, under a cap of half the refresh rate,
// that is two presents for every frame of content, and the presentation queue
// never has a refresh to spare. Anything that then disturbs the window server
// — the pointer moving, another application drawing — puts a present on the
// display a refresh late, and the ones behind it follow. Presenting only the
// frames that carry content leaves every other refresh free, and the same
// disturbances no longer move a frame.
//
// The pacer therefore sits between the display link and Ebitengine. It is
// told of every refresh, passes on the ones a frame is to be presented at
// (slots) and keeps the rest to itself, so Ebitengine's loop runs once per
// presented frame. A frame's content is sampled when its frame begins, so the
// loop is also held at the top of each frame until the refresh before the
// frame's slot: the frame then has the refresh it always had to be recorded
// and drawn in, and reaches the display as soon as it did before. Frames
// that need longer begin a refresh earlier (observeWork).
//
// The zero value is a pacer that paces nothing; the platform hook feeds it.
type framePacer struct {
	// interval is the present cap in nanoseconds, zero for every refresh. The
	// game loop writes it and the display link's thread reads it.
	interval atomic.Int64

	mu   sync.Mutex
	wake *sync.Cond

	// period is the display's refresh period: the shortest of the recent
	// spacings between the refreshes its link reported, which last ended at
	// lastTarget.
	period     time.Duration
	spacings   [pacerSpacings]time.Duration
	spacing    int
	lastTarget time.Duration
	// display is the last period a full set of spacings gave. It outlives a
	// measurement that starts again, and is not moved by the first spacings
	// of the next, which can span refreshes the link missed: what is chosen
	// from it does not change with every refresh the link misses.
	display time.Duration
	// lastSlot is the display time of the last refresh passed on and open
	// that of the slot a frame may now begin for, both on the link's clock.
	lastSlot, open time.Duration
	// lead is how far the last slot fell after its deadline, and zero when
	// the schedule restarted from it: the next slot's deadline is a cap's
	// interval after the last slot, less lead. after is how long after the
	// last slot the next one is, chosen at the last slot under the cap
	// interval planFor and zero for no plan (presentPlan).
	lead, after, planFor time.Duration
	// handedAt is when the last slot was passed on, until Ebitengine returns
	// it: a frame has taken its drawable and presented.
	handedAt time.Time
	// seenAt is when the link last reported a refresh, and paced whether
	// that refresh was one of a cadence with refreshes kept back.
	seenAt time.Time
	paced  bool
	// early is how many refreshes more than one a frame begins before its
	// slot, and work the recent frames' own times it is chosen from.
	early     int
	work      [pacerWorkWindow]time.Duration
	workCount int
	workNext  int
}

const (
	// A link that reports a period outside these bounds is not describing a
	// display this pacer understands, and every refresh is passed on.
	pacerPeriodFloor   = time.Second / 500
	pacerPeriodCeiling = time.Second / 20
	// pacerIntervalCeiling bounds the cap the pacer will keep refreshes back
	// for: Ebitengine gives up on a drawable that takes a tenth of a second
	// to arrive.
	pacerIntervalCeiling = time.Second / 15
	// pacerQuiet is how long after the link's last report the loop still
	// waits for the next one. A hidden or occluded window's link stops
	// reporting, and the loop must not wait on it.
	pacerQuiet = 50 * time.Millisecond
	// pacerHandOff is how long a slot can have been with Ebitengine and still
	// be the one the frame that just ended presented at. One that has waited
	// longer is waiting for a frame.
	pacerHandOff = time.Millisecond
	// pacerPoll bounds one wait for the link, so a slot that is waiting for
	// a frame is noticed while the link's thread is held up handing it over.
	pacerPoll = 2 * time.Millisecond
	// pacerSettle is how long a frame waits for the slot the frame before it
	// presented at to be returned: the link's thread says so a moment after
	// the present, and the loop can be here first.
	pacerSettle = 250 * time.Microsecond
	// pacerSpacings is how many of the link's reports the refresh period is
	// taken from. The link misses a refresh while its thread is held, and the
	// spacing across one it missed is a multiple of the period.
	pacerSpacings = 8
	// pacerWorkWindow is how many frames' times decide when a frame begins,
	// about a second of them, and pacerWorkLong how many of those must have
	// overrun before frames begin a refresh earlier.
	pacerWorkWindow = 64
	pacerWorkLong   = 4
	// pacerFastDisplay is the longest refresh period of a display whose
	// fullscreen window is composited (nativeScanout). At 120 refreshes a
	// second the direct route showed frames late and the composited one did
	// not; at 60 the direct route showed none late, and the composited one
	// only cost a refresh more. Nothing between the two was measured.
	pacerFastDisplay = time.Second / 100
)

// The present schedule — which refreshes a capped window presents at — is
// shared by presentDue and the pacer (DESIGN_GPU_RENDERER §13.5 "Present
// schedule"). It applies while the display is faster than the cap by more
// than an eighth of the cap's interval; nearer the cap every refresh
// presents.
//
// A cap that is a whole number of the display's refreshes (evenCadence)
// presents every that many refreshes, each present timed from the one
// before: a 60 cap presents every second refresh at 120 Hz. A cap the
// refresh does not divide cannot present evenly, and holding the whole
// number of refreshes above the cap's interval presented a 120 cap at 144 Hz
// on every second refresh, 72 times a second, and a 60 cap on every third, 48
// times. There each present is due a cap's interval after the last one's
// deadline rather than after the present, at the refresh nearest that
// deadline: the refreshes between presents take the two whole numbers either
// side of the ratio in turn — 1, 1, 1, 1, 2 for a 120 cap at 144 Hz — and
// average the cap. A present a refresh or more after the refresh its
// deadline chose, one that missed a refresh or followed a stall, starts the
// schedule again from itself, so the window never presents faster than the
// cap to catch up.

// cadenceTolerance is how near, as a fraction of the cap's interval, a whole
// number of refreshes must be for the cap to keep the even cadence. The
// display and the cap run on different clocks: a 60 cap is 1.998 refreshes of
// a 119.88 Hz panel, and a schedule held exactly to the cap would correct
// that drift with a frame one refresh short every eight seconds. The window's
// refresh estimate, taken from Draw arrivals, also moves by a percent or two
// when they jitter by a third of a refresh. The nearest ratio the schedule
// must still hold to the cap is a 30 cap at 144 Hz, 4% from five refreshes.
const cadenceTolerance = 32

// evenCadence reports whether a cap's interval is, within cadenceTolerance, a
// whole number of refresh periods.
func evenCadence(interval, period time.Duration) bool {
	if period <= 0 {
		return true
	}
	off := interval - max(1, (interval+period/2)/period)*period
	return off <= interval/cadenceTolerance && -off <= interval/cadenceTolerance
}

// refreshesUntil is how many refreshes after a present the refresh nearest a
// deadline that long after it is: the first no more than half a refresh
// before the deadline, and never the present's own.
func refreshesUntil(deadline, period time.Duration) time.Duration {
	return max(1, (deadline-period/2+period-1)/period)
}

// setInterval is nil-safe, as are refreshPeriod and awaitFrame: a host with
// no display link to pace holds no pacer.
func (p *framePacer) setInterval(interval time.Duration) {
	if p != nil {
		p.interval.Store(int64(interval))
	}
}

// refresh is the display link reporting one refresh: target is when a frame
// presented for it reaches the display, on the link's clock. It reports
// whether the refresh is a slot, to be passed on to Ebitengine, and runs on
// the link's thread. The caller reports a slot's return with returned.
//
// The refresh period is measured between reports. The link's own figures do
// not give it: a frame's display time is a refresh after its deadline when
// the display scans the window out directly and four when the window is
// composited.
//
// The pacer keeps refreshes back only while the display is faster than the
// cap by more than an eighth of the cap's interval, the allowance presentDue
// gives a Draw. Its slots then follow the present schedule presentDue keeps,
// on the link's display times: each slot is the refresh nearest its deadline.
// Slots are counted from the last slot's own display time and not from when
// its frame arrived, so a frame that was late does not move the ones after
// it.
func (p *framePacer) refresh(now time.Time, target time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wake == nil {
		p.wake = sync.NewCond(&p.mu)
	}
	defer p.wake.Broadcast()
	p.seenAt = now
	p.measure(target)
	interval := min(time.Duration(p.interval.Load()), pacerIntervalCeiling)
	threshold := interval - interval/8
	if p.period == 0 {
		threshold = 0
	}
	p.paced = threshold > p.period
	slot := !p.paced || p.lastSlot == 0 || target < p.lastSlot
	lead := time.Duration(0)
	if !slot {
		// The link's display times lie on the refresh grid, so a refresh is
		// within half of one of the refresh the plan chose, or a whole
		// refresh or more from it.
		since, after := target-p.lastSlot, p.plannedAfter(interval)
		slot = since >= after-p.period/2
		if slot && since < after+p.period/2 && !evenCadence(interval, p.period) {
			lead = since - (interval - p.lead)
		}
	}
	if slot {
		p.lastSlot, p.open, p.handedAt = target, target, now
		p.lead, p.after, p.planFor = lead, 0, interval
		if p.paced {
			p.after = refreshesUntil(interval-lead, p.period) * p.period
		}
	}
	if p.paced {
		// The next slot is the refresh nearest the next deadline, and its
		// frame begins when it is near enough.
		next := p.lastSlot + p.plannedAfter(interval)
		ahead := max(1, (next-target+p.period/2)/p.period)
		if int(ahead) <= 1+p.early {
			p.open = target + ahead*p.period
		}
	}
	return slot
}

// plannedAfter is how long after the last slot the next one is. It is chosen
// at the last slot, and afresh only when there is no plan for the cap in
// force. The caller holds mu, and the display is measured.
func (p *framePacer) plannedAfter(interval time.Duration) time.Duration {
	if p.after > 0 && p.planFor == interval {
		return p.after
	}
	return refreshesUntil(interval-p.lead, p.period) * p.period
}

// plannedSpacing is how long after the last slot the pacer means the next one
// to be while the present schedule's cadence is uneven, and zero otherwise:
// an even cadence's spacing is the cap's interval, which the window already
// has.
func (p *framePacer) plannedSpacing(now time.Time) time.Duration {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	interval := min(time.Duration(p.interval.Load()), pacerIntervalCeiling)
	if !p.pacing(now) || p.period <= 0 || evenCadence(interval, p.period) {
		return 0
	}
	return p.plannedAfter(interval)
}

// observeWork is the game loop reporting how long a frame took from its
// beginning to the end of its Draw, and chooses how many refreshes before
// their slots the frames after it begin.
//
// A frame that begins a refresh before its slot has that refresh to be
// recorded, drawn and flushed in, and the flush is Ebitengine's own: it
// follows the Draw and is not measured here, so three tenths of a refresh
// are allowed for it. A frame that overruns presents after its slot arrived,
// the display shows it late, and the frames around it are shown for uneven
// times. When frames overrun, those after them begin one refresh earlier,
// and reach the display one refresh later than they might have; when a
// second's frames would all have fitted in one refresh fewer, with two
// tenths of a refresh more to spare, they begin one later again.
func (p *framePacer) observeWork(work time.Duration) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.work[p.workNext] = work
	p.workNext = (p.workNext + 1) % len(p.work)
	p.workCount = min(p.workCount+1, len(p.work))
	if !p.paced || p.period <= 0 {
		p.early, p.workCount = 0, 0
		return
	}
	interval := min(time.Duration(p.interval.Load()), pacerIntervalCeiling)
	most := int((interval+p.period/2)/p.period) - 1
	has := time.Duration(1+p.early) * p.period
	long, longest := 0, time.Duration(0)
	for _, w := range p.work[:p.workCount] {
		if w > has-p.period*3/10 {
			long++
		}
		longest = max(longest, w)
	}
	switch {
	case p.early > most:
		p.early, p.workCount = max(most, 0), 0
	case long >= pacerWorkLong && p.early < most:
		p.early++
		p.workCount = 0
	case p.early > 0 && p.workCount == len(p.work) && longest < has-p.period-p.period*5/10:
		p.early--
		p.workCount = 0
	}
}

// measure takes the refresh period from the spacing of the link's reports.
// A spacing no display has starts the measurement again, and until there is
// one every refresh is passed on. The caller holds mu.
func (p *framePacer) measure(target time.Duration) {
	last := p.lastTarget
	p.lastTarget = target
	if last == 0 {
		return
	}
	spacing := target - last
	if spacing < pacerPeriodFloor || spacing > pacerIntervalCeiling {
		p.period, p.spacings = 0, [pacerSpacings]time.Duration{}
		return
	}
	p.spacings[p.spacing] = spacing
	p.spacing = (p.spacing + 1) % len(p.spacings)
	p.period = 0
	settled := true
	for _, s := range p.spacings {
		if s == 0 {
			settled = false
		} else if p.period == 0 || s < p.period {
			p.period = s
		}
	}
	if p.period > pacerPeriodCeiling {
		p.period = 0
	}
	if settled && p.period != 0 {
		p.display = p.period
	}
}

// fastDisplay reports whether the display the link reports for refreshes
// fast enough for a fullscreen window on it to be composited (nativeScanout).
// It is false until the display has been measured, and where there is no
// link to measure it by.
func (p *framePacer) fastDisplay() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.display != 0 && p.display <= pacerFastDisplay
}

// returned is Ebitengine returning the slot it was last passed: the frame
// that was waiting for a drawable has one and has presented.
func (p *framePacer) returned() {
	p.mu.Lock()
	p.handedAt = time.Time{}
	p.wake.Broadcast()
	p.mu.Unlock()
}

// refreshPeriod is the display's refresh period while the pacer keeps
// refreshes back, and zero while it does not: the window's own estimate,
// taken from Draw arrivals, then measures the cap instead of the display.
func (p *framePacer) refreshPeriod(now time.Time) time.Duration {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.pacing(now) {
		return 0
	}
	return p.period
}

// pacing reports whether the link is reporting refreshes and the pacer is
// keeping some of them back. The caller holds mu.
func (p *framePacer) pacing(now time.Time) bool {
	return p.paced && !p.seenAt.IsZero() && now.Sub(p.seenAt) < pacerQuiet
}

// awaitFrame holds the game loop at the top of a frame until the refresh
// before the frame's slot. The frame that just ended presented at the last
// slot passed on, so this one begins when the link opens a later one: at the
// refresh before it, or at the slot itself when the link did not report that
// refresh. It returns at once when the pacer keeps no refreshes back, and
// when the opening has already passed because the frame before ran long.
//
// A slot passed on while the loop waits here cannot return until a frame
// takes its drawable, and the link reports nothing more until it does; nor
// can one the frame before did not present at. Either is this frame's, and
// it begins. A link that stops reporting releases the loop as well. sleep
// waits for a report for at most the given time; tests supply their own.
//
// It returns how many refreshes before its slot the frame was to begin, zero
// for a frame the pacer did not place.
func (p *framePacer) awaitFrame(now func() time.Time, sleep func(*framePacer, time.Duration)) int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.wake == nil {
		p.wake = sync.NewCond(&p.mu)
	}
	if !p.pacing(now()) {
		return 0
	}
	lead := 1 + p.early
	presented := p.lastSlot
	if !p.handedAt.IsZero() && p.open <= presented {
		sleep(p, pacerSettle)
	}
	for started := now(); p.pacing(now()) && p.open <= presented; {
		at := now()
		left := pacerQuiet - at.Sub(started)
		if left <= 0 {
			break
		}
		if !p.handedAt.IsZero() {
			held := at.Sub(p.handedAt)
			if held >= pacerHandOff {
				break
			}
			left = min(left, pacerHandOff-held)
		}
		sleep(p, min(left, pacerPoll))
	}
	return lead
}

// waitForRefresh is awaitFrame's production sleep: it releases mu until the
// link reports or the time runs out. The caller holds mu.
func waitForRefresh(p *framePacer, limit time.Duration) {
	timer := time.AfterFunc(limit, func() {
		p.mu.Lock()
		p.wake.Broadcast()
		p.mu.Unlock()
	})
	p.wake.Wait()
	timer.Stop()
}
