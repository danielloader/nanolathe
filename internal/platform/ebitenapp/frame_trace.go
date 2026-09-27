package ebitenapp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/metrics"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
)

// FrameTraceOptions configure the live window's frame trace: a host-only
// pacing diagnostic that times the ordinary window loop — Ebitengine's own
// scheduling, the present cap, the record/submit pipeline and the asynchronous
// simulation — rather than a benchmark's own pacer (docs/BATTLE_BENCHMARK.md
// "Live window trace"). Nothing it records reaches the client or the session.
type FrameTraceOptions struct {
	// Directory receives frames.csv, census.jsonl and summary.json. It must not
	// exist yet.
	Directory string
	// Seconds ends the run once this much host time has passed since the first
	// battle Draw; zero runs until the window closes.
	Seconds float64
	// ProfileFrom starts cpu.pprof this many seconds after the first battle
	// Draw, and the Go execution trace (exec.trace, ExecTraceSeconds long) at
	// the same instant. Negative disables both.
	ProfileFrom      float64
	ExecTraceSeconds float64
	// Flight keeps the last few seconds of Go execution trace in memory (a
	// runtime/trace flight recorder) and writes flight-<frame>.trace whenever a
	// frame spikes: a host step, record or Execute far over budget, or a
	// presented interval over 25 ms. It is how a hitch that comes once every
	// few minutes is caught with every goroutine's state around it. It cannot
	// be combined with ExecTraceSeconds.
	Flight bool
	// FlightLimit caps the snapshots a run writes; zero keeps eight. A trace of
	// a whole play session needs more than a timed run, and each snapshot is
	// the recorder's whole window, tens of megabytes.
	FlightLimit int
	// Census is called about once a second on the game goroutine, after a host
	// step has joined the simulation, and its value is appended to census.jsonl.
	Census func() any
	// Battle reports whether a battle is live. The trace starts at the first
	// Draw it reports one, and marks every row with it, so a trace of menu play
	// leaves the menus before the first battle out and flags those between
	// battles. Nil means a battle from the first Draw, which a --map run is.
	Battle func() bool
	// Metadata is written into summary.json unchanged.
	Metadata map[string]any
}

// frameTrace writes one CSV row per Ebitengine frame: one Update call and the
// Draw that follows it. Times are microseconds since the first battle Draw;
// spans are microseconds. It costs one nil check per mark when disabled.
type frameTrace struct {
	opts    FrameTraceOptions
	w       *bufio.Writer
	f       *os.File
	census  *os.File
	origin  time.Time
	started bool
	rows    int
	samples []metrics.Sample
	row     frameRow
	// nextCensus is the host time of the next census sample.
	nextCensus time.Duration
	profile    *os.File
	execTrace  *os.File
	profiling  bool
	tracing    bool
	traceUntil time.Duration
	finished   bool
	// flight is the flight recorder (FrameTraceOptions.Flight); flights counts
	// the snapshots written and lastFlight is when the last one was.
	flight     *trace.FlightRecorder
	flights    int
	lastFlight time.Duration
	lastDraw   int64
}

type frameRow struct {
	updStart, updEnd                  int64
	steps, updBodies                  int
	drawStart, drawEnd                int64
	due, hit, armed, sync             bool
	join1, join2                      int64
	record, execute, blit             int64
	body, launch                      int64
	simWait, simBatch                 int64
	simJoins, released                int
	passes, vertices, subs            int
	xPrepare, xModel, xPlace, xReplay int64
	preNanos                          int64
	// What the presented frame showed and when, for motion evenness: the
	// refresh period presentDue measured and the cap it applied, whether a
	// battle was live and the window focused, the host steps run before the
	// Draw (the camera's sample count), the committed pair the world blended
	// and the two fractions the presented list was recorded at, and the
	// blended camera origin in hundredths of a world pixel. Zero where the
	// Draw presented no recorded battle frame.
	battle, focused  bool
	refresh, capUS   int64
	bodies           int64
	tickPrev, tick   uint32
	tick16, cam16    int32
	camX100, camZ100 int64
}

func newFrameTrace(opts *FrameTraceOptions) (*frameTrace, error) {
	if opts == nil || opts.Directory == "" {
		return nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.Directory), 0o755); err != nil {
		return nil, err
	}
	if err := os.Mkdir(opts.Directory, 0o755); err != nil {
		return nil, fmt.Errorf("nanolathe: create fresh frame trace directory: %w", err)
	}
	f, err := os.Create(filepath.Join(opts.Directory, "frames.csv"))
	if err != nil {
		return nil, err
	}
	census, err := os.Create(filepath.Join(opts.Directory, "census.jsonl"))
	if err != nil {
		f.Close()
		return nil, err
	}
	t := &frameTrace{opts: *opts, f: f, census: census, w: bufio.NewWriterSize(f, 1<<16),
		samples: []metrics.Sample{
			{Name: "/gc/cycles/total:gc-cycles"},
			{Name: "/gc/heap/allocs:bytes"},
			{Name: "/cpu/classes/gc/total:cpu-seconds"},
			{Name: "/sched/pauses/total/gc:seconds"},
			{Name: "/gc/heap/live:bytes"},
		}}
	fmt.Fprintln(t.w, "frame,upd_start,upd_end,steps,upd_bodies,draw_start,draw_end,due,hit,armed,sync,join1,join2,record,execute,blit,body,sim_wait,sim_batch,sim_joins,launch,passes,vertices,subjects,gc_cycles,alloc_bytes,gc_cpu_us,gc_pause_us,heap_live,x_prepare,x_model,x_place,x_replay,pre_us,released,battle,focused,refresh_us,cap_us,bodies,tick_prev,tick,tick16,cam16,cam_x100,cam_z100")
	return t, nil
}

// markDraw records the presentation context of a battle Draw, presented or
// skipped by the cap.
func (t *frameTrace) markDraw(refresh, capInterval time.Duration, focused bool) {
	if t == nil || !t.started {
		return
	}
	t.row.battle = t.opts.Battle == nil || t.opts.Battle()
	t.row.focused = focused
	t.row.refresh = int64(refresh / time.Microsecond)
	t.row.capUS = int64(capInterval / time.Microsecond)
}

// markShown records what the presented frame shows: the committed pair the
// world blends, the two fractions its list was recorded at, the host steps
// run so far (the camera blends between the last two of them), and the camera
// origin those inputs put on screen.
func (t *frameTrace) markShown(c *client.Client, shown client.PresentationInputs, bodies int64) {
	if t == nil || !t.started {
		return
	}
	prev, cur, _ := c.PresentedTicks()
	view := c.CameraViewFor(shown)
	t.row.tickPrev, t.row.tick = prev, cur
	t.row.tick16, t.row.cam16 = shown.TickFraction16, shown.CameraFraction16
	t.row.bodies = bodies
	t.row.camX100, t.row.camZ100 = int64(math.Round(view.X*100)), int64(math.Round(view.Z*100))
}

func (t *frameTrace) now() int64 {
	if !t.started {
		return 0
	}
	return int64(time.Since(t.origin) / time.Microsecond)
}

func (t *frameTrace) since(start time.Time) int64 {
	return int64(time.Since(start) / time.Microsecond)
}

// begin starts the clock at the first battle Draw, so menus and loading are
// outside every measurement.
func (t *frameTrace) begin() {
	if t == nil || t.started || t.opts.Battle != nil && !t.opts.Battle() {
		return
	}
	t.started = true
	t.origin = time.Now()
	t.row = frameRow{}
	if t.opts.Flight && t.opts.ExecTraceSeconds <= 0 {
		t.flight = trace.NewFlightRecorder(trace.FlightRecorderConfig{MinAge: 3 * time.Second, MaxBytes: 96 << 20})
		if t.flight.Start() != nil {
			t.flight = nil
		}
	}
}

// beginUpdate closes the previous frame's row and opens the next one.
func (t *frameTrace) beginUpdate() {
	if t == nil || !t.started || t.finished {
		return
	}
	if t.row.updStart != 0 || t.row.drawStart != 0 {
		t.flushRow()
	}
	t.row = frameRow{updStart: t.now()}
	elapsed := time.Since(t.origin)
	if t.opts.ProfileFrom >= 0 && elapsed >= seconds(t.opts.ProfileFrom) {
		if t.profile == nil && !t.profiling {
			if f, err := os.Create(filepath.Join(t.opts.Directory, "cpu.pprof")); err == nil {
				t.profile = f
				t.profiling = pprof.StartCPUProfile(f) == nil
			}
			t.writeAllocs("alloc-base.pprof")
		}
		if t.opts.ExecTraceSeconds > 0 && t.execTrace == nil && !t.tracing {
			if f, err := os.Create(filepath.Join(t.opts.Directory, "exec.trace")); err == nil {
				t.execTrace = f
				t.tracing = trace.Start(f) == nil
				t.traceUntil = elapsed + seconds(t.opts.ExecTraceSeconds)
			}
		}
	}
	if t.tracing && elapsed >= t.traceUntil {
		trace.Stop()
		t.execTrace.Close()
		t.tracing = false
	}
	if t.tracing || t.flight != nil {
		// Frame numbers in the execution trace align it with frames.csv.
		trace.Log(context.Background(), "frame", strconv.Itoa(t.rows))
	}
}

// flightSpike reports whether the frame just closed is worth a flight
// snapshot: a segment far over a 120 Hz budget, a host step held up by the
// simulation, or a presented frame at least 12 ms later than the interval the
// window presents at — two refreshes at 120 Hz, whatever the cap.
func (t *frameTrace) flightSpike(r frameRow) bool {
	if r.body-r.simWait > 6000 || r.simWait > 6000 || r.record > 6000 || r.execute > 10000 || r.join1+r.join2 > 6000 {
		return true
	}
	nominal := max(r.capUS, r.refresh)
	if nominal == 0 {
		nominal = 8333
	}
	return r.due && t.lastDraw != 0 && r.drawStart-t.lastDraw-nominal >= 12000
}

// snapshotFlight writes the flight recorder's window, at most once every two
// seconds and FlightLimit times a run, so a burst of spikes costs one file.
func (t *frameTrace) snapshotFlight() {
	elapsed := time.Since(t.origin)
	limit := t.opts.FlightLimit
	if limit <= 0 {
		limit = 8
	}
	if t.flights >= limit || (t.flights > 0 && elapsed-t.lastFlight < 2*time.Second) {
		return
	}
	f, err := os.Create(filepath.Join(t.opts.Directory, fmt.Sprintf("flight-%d.trace", t.rows)))
	if err != nil {
		return
	}
	t.flight.WriteTo(f)
	f.Close()
	t.flights++
	t.lastFlight = elapsed
}

func seconds(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }

// expired reports that the requested run length has elapsed.
func (t *frameTrace) expired() bool {
	return t != nil && t.started && t.opts.Seconds > 0 && time.Since(t.origin) >= seconds(t.opts.Seconds)
}

// sampleCensus appends one census line when a second has passed.
func (t *frameTrace) sampleCensus(renderer any) {
	if t == nil || !t.started || t.opts.Census == nil {
		return
	}
	elapsed := time.Since(t.origin)
	if elapsed < t.nextCensus {
		return
	}
	t.nextCensus = elapsed + time.Second
	line, err := json.Marshal(map[string]any{"t_us": int64(elapsed / time.Microsecond), "frame": t.rows, "census": t.opts.Census(), "renderer": renderer})
	if err == nil {
		t.census.Write(append(line, '\n'))
	}
}

func (t *frameTrace) flushRow() {
	metrics.Read(t.samples)
	r := t.row
	if t.flight != nil && t.flightSpike(r) {
		t.snapshotFlight()
	}
	if r.due {
		t.lastDraw = r.drawStart
	}
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	var pauses float64
	if h := t.samples[3].Value; h.Kind() == metrics.KindFloat64Histogram {
		// The histogram carries counts per bucket; the bucket's lower bound is a
		// conservative per-pause figure, which is all a spike classifier needs.
		hist := h.Float64Histogram()
		for i, n := range hist.Counts {
			if n != 0 && i < len(hist.Buckets) {
				lo := hist.Buckets[i]
				if lo > 0 {
					pauses += float64(n) * lo
				}
			}
		}
	}
	fmt.Fprintf(t.w, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d\n", t.rows,
		r.updStart, r.updEnd, r.steps, r.updBodies, r.drawStart, r.drawEnd, b(r.due), b(r.hit), b(r.armed), b(r.sync),
		r.join1, r.join2, r.record, r.execute, r.blit, r.body, r.simWait, r.simBatch, r.simJoins, r.launch,
		r.passes, r.vertices, r.subs,
		t.samples[0].Value.Uint64(), t.samples[1].Value.Uint64(),
		int64(t.samples[2].Value.Float64()*1e6), int64(pauses*1e6), t.samples[4].Value.Uint64(),
		r.xPrepare, r.xModel, r.xPlace, r.xReplay, r.preNanos/1000, r.released,
		b(r.battle), b(r.focused), r.refresh, r.capUS, r.bodies, r.tickPrev, r.tick, r.tick16, r.cam16, r.camX100, r.camZ100)
	t.rows++
	if t.rows%256 == 0 {
		t.w.Flush()
	}
}

// writeAllocs writes the cumulative allocation profile; the pair around the
// profile window gives its allocations with pprof -base.
func (t *frameTrace) writeAllocs(name string) {
	if f, err := os.Create(filepath.Join(t.opts.Directory, name)); err == nil {
		pprof.Lookup("allocs").WriteTo(f, 0)
		f.Close()
	}
}

// close ends the trace and writes summary.json.
func (t *frameTrace) close() {
	if t == nil || t.finished {
		return
	}
	t.finished = true
	if t.row.updStart != 0 || t.row.drawStart != 0 {
		t.flushRow()
	}
	t.w.Flush()
	t.f.Close()
	t.census.Close()
	if t.profiling {
		pprof.StopCPUProfile()
		t.profile.Close()
		t.writeAllocs("alloc.pprof")
		// The live heap after a collection: what every GC cycle has to mark.
		runtime.GC()
		if f, err := os.Create(filepath.Join(t.opts.Directory, "heap.pprof")); err == nil {
			pprof.Lookup("heap").WriteTo(f, 0)
			f.Close()
		}
	}
	if t.tracing {
		trace.Stop()
		t.execTrace.Close()
	}
	if t.flight != nil {
		t.flight.Stop()
	}
	summary := map[string]any{
		"rows":           t.rows,
		"seconds":        time.Since(t.origin).Seconds(),
		"origin_unix_us": t.origin.UnixMicro(),
		"gomaxprocs":     runtime.GOMAXPROCS(0),
		"numcpu":         runtime.NumCPU(),
		"go":             runtime.Version(),
		"metadata":       t.opts.Metadata,
	}
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		os.WriteFile(filepath.Join(t.opts.Directory, "summary.json"), append(data, '\n'), 0o644)
	}
}
