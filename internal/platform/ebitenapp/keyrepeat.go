package ebitenapp

import "github.com/nanolathe-gg/nanolathe/internal/input"

// Keyboard auto-repeat at the platform edge.
//
// Retail had no repeat policy of its own. Its window procedure ignores the
// repeat count, so every key-down or character message the operating system
// repeated for a held key entered the token ring through the same producer
// path, and the battle frame drains one token per host frame [07 §2]
// [07 R-CAM-01 §1]. The delay and the rate were the user's Windows keyboard
// settings. Ebiten delivers the repeats of printable characters in its text
// batch but reports the non-printing keys only as held state, so a held
// Backspace deleted one character and stopped. The repeat of those keys is
// reconstructed here from the scaled host timestamp the samples already carry.
// The values are recorded HOST POLICY — the Windows defaults, not a retail
// measurement; see docs/DESIGN_INTERFACE_HUD_INPUT.md §5.
const (
	// keyRepeatDelay is the default keyboard delay (setting 1, about 500 ms)
	// in the scaled 30 Hz host-clock unit: 500 × 30 / 1000 = 15. A key must
	// stay held this long after its press before the first repeat.
	keyRepeatDelay uint32 = 15
	// keyRepeatInterval is the default repeat speed (setting 31, about 30 per
	// second): one scaled unit, so at most one repeat per 30 Hz host service —
	// the same bound retail's one-token-per-frame drain put on the OS rate.
	keyRepeatInterval uint32 = 1
)

// keyRepeats reports whether a held key is re-issued. The set is the editing
// and cursor keys a text field and a list consume: Backspace, Delete, the four
// arrows, Home, End, Page Up and Page Down. Retail repeated every held key;
// the toggles (Enter, Escape, Tab, Insert's paste, Pause, the function keys,
// Ctrl and Alt compositions) keep a single token per press here, so holding
// one cannot flicker a menu, re-paste, or re-issue an order.
// TODO(T25): a native event source would carry the OS repeat for every key.
func keyRepeats(key input.Key) bool {
	switch key {
	case input.KeyBackspace, input.KeyDelete,
		input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown,
		input.KeyHome, input.KeyEnd, input.KeyPrior, input.KeyNext:
		return true
	}
	return false
}

// keyRepeater holds, per key, when the next repeat falls due. It belongs to
// one keyboard state: a new state (a new window, or a test) starts clean.
type keyRepeater struct {
	kbd   *input.KeyboardState
	armed [input.KeyCount]bool
	next  [input.KeyCount]uint32
}

// sync forgets every key when the published keyboard state changes identity.
func (r *keyRepeater) sync(kbd *input.KeyboardState) {
	if r.kbd != kbd {
		*r = keyRepeater{kbd: kbd}
	}
}

// press arms a repeatable key at its initial transition.
func (r *keyRepeater) press(key input.Key, at uint32) {
	if keyRepeats(key) {
		r.armed[key], r.next[key] = true, at+keyRepeatDelay
	}
}

// release disarms a key that is no longer held.
func (r *keyRepeater) release(key input.Key) {
	r.armed[key] = false
}

// due reports whether a held key repeats at this service and schedules the
// next repeat. Services that share one timestamp (a catch-up drain) repeat at
// most once, like an OS rate that never bursts.
func (r *keyRepeater) due(key input.Key, at uint32) bool {
	if !r.armed[key] || int32(at-r.next[key]) < 0 {
		return false
	}
	r.next[key] = at + keyRepeatInterval
	return true
}

// nativeKeyRepeat is the production repeater, one per process like the
// double-click recognizer.
var nativeKeyRepeat keyRepeater
