package ebitenapp

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

func heldKeySample(key input.Key, at uint32) sampledInput {
	sample := sampledInput{timestamp: at}
	sample.keys[key] = true
	return sample
}

// A held editing key repeats after the host delay, then at most once per
// service; a release re-arms the delay (DESIGN_INTERFACE_HUD_INPUT §5 host
// policy, Windows defaults — not a retail measurement).
func TestHeldEditingKeyRepeatsAfterDelay(t *testing.T) {
	in := input.NewState()
	count := func(at uint32, held bool) int {
		sample := sampledInput{timestamp: at}
		sample.keys[input.KeyBackspace] = held
		applyInput(in, sample)
		return len(in.DrainTokens())
	}
	start := uint32(1000)
	if got := count(start, true); got != 1 {
		t.Fatalf("press tokens = %d, want 1", got)
	}
	for at := start + 1; at < start+keyRepeatDelay; at++ {
		if got := count(at, true); got != 0 {
			t.Fatalf("repeat at %d before the delay", at-start)
		}
	}
	for at := start + keyRepeatDelay; at < start+keyRepeatDelay+3; at++ {
		if got := count(at, true); got != 1 {
			t.Fatalf("held key at +%d produced %d tokens, want 1", at-start, got)
		}
	}
	// A catch-up drain reuses the service timestamp and must not burst.
	if got := count(start+keyRepeatDelay+2, true); got != 0 {
		t.Fatalf("same-timestamp service repeated %d times", got)
	}
	if got := count(start+keyRepeatDelay+3, false); got != 0 {
		t.Fatal("release produced a token")
	}
	again := start + keyRepeatDelay + 4
	if got := count(again, true); got != 1 {
		t.Fatalf("second press tokens = %d, want 1", got)
	}
	if got := count(again+keyRepeatDelay-1, true); got != 0 {
		t.Fatal("release did not restart the delay")
	}
}

// Toggle keys keep one token per press however long they are held; only the
// editing and cursor keys repeat.
func TestHeldToggleKeysDoNotRepeat(t *testing.T) {
	for _, key := range []input.Key{input.KeyEnter, input.KeyEscape, input.KeyTab, input.KeyInsert, input.KeyPause, input.KeyF10} {
		in := input.NewState()
		applyInput(in, heldKeySample(key, 0))
		in.DrainTokens()
		for at := uint32(1); at < 3*keyRepeatDelay; at++ {
			applyInput(in, heldKeySample(key, at))
		}
		if got := in.DrainTokens(); len(got) != 0 {
			t.Fatalf("held %v repeated: %v", key, got)
		}
	}
	for _, key := range []input.Key{input.KeyDelete, input.KeyLeft, input.KeyRight, input.KeyUp, input.KeyDown, input.KeyHome, input.KeyEnd, input.KeyPrior, input.KeyNext} {
		in := input.NewState()
		applyInput(in, heldKeySample(key, 0))
		applyInput(in, heldKeySample(key, keyRepeatDelay))
		got := in.DrainTokens()
		if len(got) != 2 || got[1] != (input.Token{Kind: input.TokenEdit, Key: key}) {
			t.Fatalf("held %v tokens = %v, want press and one repeat", key, got)
		}
	}
}

// Repeat state belongs to one keyboard: a key left held on another state
// cannot fire into a fresh one.
func TestKeyRepeatDoesNotLeakAcrossStates(t *testing.T) {
	first := input.NewState()
	applyInput(first, heldKeySample(input.KeyBackspace, 0))
	second := input.NewState()
	second.Kbd.SetKey(input.KeyBackspace, true)
	applyInput(second, heldKeySample(input.KeyBackspace, 10*keyRepeatDelay))
	if got := second.DrainTokens(); len(got) != 0 {
		t.Fatalf("stale repeat reached a new keyboard: %v", got)
	}
}

// The chat report: holding Backspace in a captured editor keeps deleting.
func TestHeldBackspaceKeepsDeletingInEditor(t *testing.T) {
	p := ui.NewPanel(&gui.Window{Gadgets: []gui.Gadget{{Kind: gui.KindTextBox, Name: "TALK", Active: 1, MaxChars: 127, Rect: gui.Rect{W: 200}}}})
	p.SetTextAt(0, "hello")
	p.FocusEditor(0)
	in := input.NewState()
	for at := uint32(0); at <= keyRepeatDelay+2; at++ {
		applyInput(in, heldKeySample(input.KeyBackspace, at))
		p.ApplyEditorTokens(in.DrainTokens(), nil)
	}
	if got := p.TextAt(0); got != "h" {
		t.Fatalf("text after holding Backspace = %q, want %q", got, "h")
	}
}
