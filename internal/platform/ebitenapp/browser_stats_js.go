//go:build js && wasm

package ebitenapp

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"runtime/pprof"
	"syscall/js"
	"time"
)

var browserStats struct {
	last       time.Time
	tick       uint32
	frames     int
	draw       time.Duration
	sim        time.Duration
	steps      int
	totalAlloc uint64
	pause      uint64
}

// A diagnostic runs on its own goroutine so the JavaScript event callback can
// return. It pauses for GC, reads the heap, and never changes simulation state.
var browserHeapProfile = js.FuncOf(func(this js.Value, args []js.Value) any {
	go func() {
		runtime.GC()
		var profile bytes.Buffer
		err := pprof.Lookup("heap").WriteTo(&profile, 1)
		message := ""
		if err != nil {
			message = err.Error()
		}
		data := js.Global().Get("Uint8Array").New(profile.Len())
		js.CopyBytesToJS(data, profile.Bytes())
		if callback := js.Global().Get("nanolatheBrowserHeapReady"); callback.Type() == js.TypeFunction {
			callback.Invoke(data, message)
		}
	}()
	return nil
})

func init() { js.Global().Set("browserHeapProfile", browserHeapProfile) }

type browserSampleTime struct{ began time.Time }

// Timing acquisition belongs to this target-specific host hook so a native
// build's inert browser hooks do not read the clock (DESIGN_BROWSER_HOST §5).
func browserBeginSample() browserSampleTime { return browserSampleTime{began: time.Now()} }

// Browser host telemetry; reads only published frames and never feeds the sim.
func browserEndDrawSample(a *app, sample browserSampleTime) {
	now := time.Now()
	browserStats.frames++
	browserStats.draw += now.Sub(sample.began)
	if browserStats.last.IsZero() {
		browserStats.last = now
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		browserStats.totalAlloc, browserStats.pause = memory.TotalAlloc, memory.PauseTotalNs
		return
	}
	span := now.Sub(browserStats.last).Seconds()
	if span < 1 {
		return
	}
	frame := a.c.Buffer().Current()
	tick, units := uint32(0), 0
	if frame != nil {
		tick, units = frame.Tick, len(frame.Units)
	}
	fps := float64(browserStats.frames) / span
	draw := float64(browserStats.draw) / float64(time.Millisecond) / float64(browserStats.frames)
	sim := 0.0
	if browserStats.steps > 0 {
		sim = ms(browserStats.sim) / float64(browserStats.steps)
	}
	w, h := a.c.Size()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	tps := 0.0
	if tick >= browserStats.tick {
		tps = float64(tick-browserStats.tick) / span
	}
	const mib = 1024 * 1024
	data, _ := json.Marshal(map[string]any{
		"renderer": rendererName(a.mode), "width": w, "height": h, "fps": fps, "tps": tps, "tick": tick, "units": units,
		"draw_ms": draw, "sim_ms": sim, "heap_mib": float64(memory.HeapAlloc) / mib,
		"heap_inuse_mib": float64(memory.HeapInuse) / mib, "heap_sys_mib": float64(memory.HeapSys) / mib,
		"next_gc_mib": float64(memory.NextGC) / mib, "alloc_mib_s": float64(memory.TotalAlloc-browserStats.totalAlloc) / mib / span,
		"gc_count": memory.NumGC, "gc_pause_ms_s": float64(memory.PauseTotalNs-browserStats.pause) / float64(time.Millisecond) / span,
	})
	if callback := js.Global().Get("nanolatheBrowserSample"); callback.Type() == js.TypeFunction {
		callback.Invoke(string(data))
	}
	browserStats.last, browserStats.tick = now, tick
	browserStats.totalAlloc, browserStats.pause = memory.TotalAlloc, memory.PauseTotalNs
	browserStats.frames, browserStats.draw = 0, 0
	browserStats.steps, browserStats.sim = 0, 0
}

func browserEndSimulationSample(sample browserSampleTime) {
	browserStats.sim += time.Since(sample.began)
	browserStats.steps++
}

// The browser host favors smaller texture uploads over the desktop's eager
// packing. Reuse the renderer's existing pixel-equivalent sparse atlas path.
func browserPrepareRenderer(a *app) {
	if os.Getenv("NANOLATHE_BROWSER_MEMORY") == "lower" {
		a.gpu.SetSparseTerrain(true)
	}
}

// The browser currently uses full paused compositions. WebGL save dialogs
// showed blank background frames while editing when reusing the paused world.
// Keep the ordinary renderer path until browser reuse has visual acceptance.
// TODO(question): isolate the WebGL paused snapshot discrepancy by comparing
// cached and full modal captures (docs/DESIGN_BROWSER_HOST.md §5).
func browserPausedReuse() bool { return false }

// The browser records each Modern frame in its own Draw. Go's js/wasm
// scheduler returns to the browser only once every goroutine has blocked, so a
// next-frame pre-record ran in the same task as the Draw before it and
// overlapped nothing; a predicted miss recorded the frame twice
// (DESIGN_BROWSER_HOST §5).
func browserPreRecord() bool { return false }
