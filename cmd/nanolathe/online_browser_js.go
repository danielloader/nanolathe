//go:build js && wasm

package main

// The browser page's background step for online play (DESIGN_BROWSER_HOST §4
// contract 10). Ebitengine runs one frame — an Update and a Draw — per
// animation-frame callback, and a browser makes no animation frames while the
// page is hidden. The child page's frame gate (web/frames.js) records whether
// a frame is running and lets this goroutine keep the next one from starting.
// While the page presents no frames this goroutine runs the shell's online
// step (onlineBackgroundStep) between them, holding the gate, so the battle
// state is only ever touched by one of the two.
//
// Go's js/wasm runtime runs goroutines only when a JavaScript event hands it
// control. A hidden page's timers are throttled to about once a second, but
// its WebSocket messages still arrive, and each grant the relay sends wakes
// the runtime; a short sleep therefore lets this goroutine run at every
// grant's arrival, at the relay's pace.

import (
	"sync"
	"syscall/js"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
)

// Host polling intervals, not simulation values. The first is the sleep
// between background steps, shorter than a tick so the next wake runs one; the
// second is how often a presenting page checks that it still is; the third is
// how long an online game may go without a shell step watching it, while the
// page presents frames, before the goroutine ends.
const (
	onlineBackgroundStepPoll  = 4 * time.Millisecond
	onlineBackgroundIdlePoll  = 100 * time.Millisecond
	onlineBackgroundUnwatched = time.Second
	// onlineBackgroundStallMillis is how long a visible page may present no
	// frame before the background step stands in for its frames.
	onlineBackgroundStallMillis = 200
)

var onlineBackground struct {
	mu      sync.Mutex
	shell   *gameShell
	cl      *client.Client
	watched time.Time
	running bool
}

var jsVisibleState = js.ValueOf("visible")

// browserFrames is the child page's frame gate.
type browserFrames struct{ gate, document, performance js.Value }

// Idle reports that no frame is running and the page is hidden, or its frames
// have stopped for onlineBackgroundStallMillis.
func (f browserFrames) Idle() bool {
	if f.gate.Get("running").Bool() || f.gate.Get("held").Bool() {
		return false
	}
	if !f.document.Get("visibilityState").Equal(jsVisibleState) {
		return true
	}
	return f.performance.Call("now").Float()-f.gate.Get("idleSince").Float() >= onlineBackgroundStallMillis
}

func (f browserFrames) Hold(held bool) { f.gate.Set("held", held) }

// watchOnlineBackground names the shell whose online game a hidden page keeps
// in step, and starts the background goroutine if it is not running. Shell
// steps call it while a room or an online battle is open. A page without the
// frame gate has no background step.
func watchOnlineBackground(g *gameShell, cl *client.Client) {
	if g == nil {
		return
	}
	gate := js.Global().Get("nanolatheBrowserFrames")
	if gate.Type() != js.TypeObject {
		return
	}
	bg := &onlineBackground
	bg.mu.Lock()
	defer bg.mu.Unlock()
	bg.shell, bg.cl, bg.watched = g, cl, time.Now()
	if bg.running {
		return
	}
	bg.running = true
	frames := browserFrames{gate: gate, document: js.Global().Get("document"), performance: js.Global().Get("performance")}
	go runOnlineBackground(frames)
}

// runOnlineBackground steps the watched shell's online game whenever the
// page's frames are idle. It ends once no online game remains: when a step
// reports none, or when the page presents frames and no shell step has
// watched one for onlineBackgroundUnwatched.
func runOnlineBackground(frames browserFrames) {
	bg := &onlineBackground
	for {
		bg.mu.Lock()
		g, cl := bg.shell, bg.cl
		bg.mu.Unlock()
		active := false
		stepped := serviceOnlineBackground(frames, func() { active = g.onlineBackgroundStep(cl) })
		bg.mu.Lock()
		if stepped && !active || !stepped && time.Since(bg.watched) > onlineBackgroundUnwatched {
			if bg.shell == g {
				bg.running, bg.shell, bg.cl = false, nil, nil
				bg.mu.Unlock()
				return
			}
		}
		bg.mu.Unlock()
		if stepped {
			time.Sleep(onlineBackgroundStepPoll)
		} else {
			time.Sleep(onlineBackgroundIdlePoll)
		}
	}
}
