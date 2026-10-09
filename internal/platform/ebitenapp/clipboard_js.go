//go:build js && wasm && !ebitenginevmguest

package ebitenapp

import (
	"sync"
	"syscall/js"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/input"
)

// A page receives clipboard text only in a paste event. The child page
// (web/clipboard.js) lets the browser paste on its own paste shortcut, hands
// the event's text to nanolatheBrowserPaste, and makes sure the engine sees a
// paste key — Ctrl+V or Insert — in the same browser task, before its next
// update samples it (DESIGN_BROWSER_HOST §4 contract 9). The paste token that
// key makes takes the text once.
var browserPaste struct {
	mu   sync.Mutex
	text input.ClipboardText
}

func init() {
	js.Global().Set("nanolatheBrowserPaste", js.FuncOf(func(_ js.Value, args []js.Value) any {
		// A null argument is a paste with no text format, which preserves the
		// editor as an unavailable native clipboard does [07 §2].
		var text input.ClipboardText
		if len(args) == 1 && args[0].Type() == js.TypeString {
			text = input.ClipboardText{Text: args[0].String(), Available: true}
		}
		browserPaste.mu.Lock()
		browserPaste.text = text
		browserPaste.mu.Unlock()
		return nil
	}))
}

// readHostClipboard runs only for a new paste key token. A paste key with no
// paste event before it, which the browser fires only for its own shortcuts
// and menus, finds the clipboard unavailable and leaves the editor alone.
func readHostClipboard() input.ClipboardText {
	browserPaste.mu.Lock()
	defer browserPaste.mu.Unlock()
	text := browserPaste.text
	browserPaste.text = input.ClipboardText{}
	return text
}

// browserClipboardWriteWait bounds how long a Copy waits for the browser to
// settle its write: a host bound, not a simulation value.
const browserClipboardWriteWait = 2 * time.Second

func browserClipboard() (js.Value, bool) {
	clipboard := js.Global().Get("navigator").Get("clipboard")
	return clipboard, clipboard.Truthy() && clipboard.Get("writeText").Type() == js.TypeFunction
}

// HostClipboardWritable reports whether the page has the asynchronous
// clipboard, which browsers offer only to secure pages, loopback included.
func HostClipboardWritable() bool {
	_, ok := browserClipboard()
	return ok
}

// WriteHostClipboard writes text with navigator.clipboard.writeText and
// reports whether the browser accepted it. The caller is a frame's update, a
// goroutine the browser's event loop keeps running while it waits, so the
// promise settles meanwhile. A browser that refuses — the page unfocused, or
// the click no longer counting as the player's — reports failure.
func WriteHostClipboard(text string) bool {
	clipboard, ok := browserClipboard()
	if !ok {
		return false
	}
	settled := make(chan bool, 1)
	var resolve, reject js.Func
	settle := func(accepted bool) js.Func {
		return js.FuncOf(func(js.Value, []js.Value) any {
			settled <- accepted
			resolve.Release()
			reject.Release()
			return nil
		})
	}
	resolve, reject = settle(true), settle(false)
	clipboard.Call("writeText", text).Call("then", resolve, reject)
	select {
	case accepted := <-settled:
		return accepted
	case <-time.After(browserClipboardWriteWait):
		// The callbacks release themselves if the promise settles later.
		return false
	}
}
