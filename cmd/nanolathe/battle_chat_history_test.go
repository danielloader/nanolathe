package main

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/mission"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Command history is the host input policy in DESIGN_INTERFACE_HUD_INPUT §3.9.
// These tests use the production TALK path, with no advancing simulation tick.
type talkHistoryDriver struct {
	t *testing.T
	b *battleSession
}

func newTalkHistoryDriver(t *testing.T) talkHistoryDriver {
	t.Helper()
	b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	if b.cl == nil {
		t.Fatal("fixture client unavailable")
	}
	t.Cleanup(b.cl.Close)
	b.sess.Econ = &economy.Service{}
	b.sess.Econ.Players[0] = economy.Player{Exists: true, Name: "Player", Stock: [2]float32{123, 456}}
	b.sess.SeedSessionRNG(7, 11)
	b.hud = &retailBattleHUD{talkWin: testTalkWindow()}
	b.millisSource = &fakeMillisSource{}
	b.controller = NewBattleController(b, b.millisSource)
	b.cl.SetFocused(true)
	b.cl.Input().Mouse.SetPosition(320, 200)
	return talkHistoryDriver{t: t, b: b}
}

func historyKey(key input.Key) input.Token {
	return input.Token{Kind: input.TokenEdit, Key: key}
}

func historyText(r rune) input.Token {
	return input.Token{Kind: input.TokenText, Rune: r}
}

func historyPaste(text string) input.Token {
	return input.Token{Kind: input.TokenEdit, Key: input.KeyV, Ctrl: true, Clipboard: input.ClipboardText{Text: text, Available: true}}
}

func (d talkHistoryDriver) step(tokens ...input.Token) {
	d.t.Helper()
	for _, token := range tokens {
		if !d.b.cl.Input().EnqueueToken(token) {
			d.t.Fatal("fixture exceeded the input token ring")
		}
	}
	d.b.viewerStep(0, d.b.cl)
}

func (d talkHistoryDriver) open() {
	d.t.Helper()
	d.step(historyKey(input.KeyEnter))
	if !d.b.chat.active {
		d.t.Fatal("Enter did not open TALK")
	}
}

func (d talkHistoryDriver) submit(text string) {
	d.t.Helper()
	d.open()
	d.step(historyPaste(text), historyKey(input.KeyEnter))
	if d.b.chat.active {
		d.t.Fatal("Enter did not submit TALK")
	}
}

func (d talkHistoryDriver) recalled(want string) {
	d.t.Helper()
	p := d.b.hud.talkPanel
	if got := p.TextOf("TALK"); got != want || p.EditorCaret() != len(want) || !p.EditorCaptured() {
		d.t.Fatalf("recalled text/caret/capture = %q/%d/%v, want %q/%d/true", got, p.EditorCaret(), p.EditorCaptured(), want, len(want))
	}
}

func TestTalkHistoryOrderedRecallEditsAndDraft(t *testing.T) {
	d := newTalkHistoryDriver(t)
	d.submit("+one")
	d.submit("+two")
	d.open()
	// The draft is captured after earlier tokens in the same producer frame.
	d.step(historyText('d'), historyKey(input.KeyUp), historyKey(input.KeyDown), historyText('!'))
	d.recalled("d!")
	d.step(historyKey(input.KeyEnter), historyKey(input.KeyUp))
	d.recalled("+two")
	d.step(historyKey(input.KeyLeft), historyKey(input.KeyBackspace), historyText('X'))
	if got := d.b.hud.talkPanel.TextOf("TALK"); got != "+tXo" {
		t.Fatalf("ordinary edits of recalled line = %q", got)
	}
	d.step(historyKey(input.KeyUp), historyKey(input.KeyUp))
	d.recalled("+one") // oldest saturates
	d.step(historyKey(input.KeyDown))
	d.recalled("+two") // editing did not replace the stored line
	d.step(historyKey(input.KeyDown), historyKey(input.KeyDown))
	d.recalled("d!")
	// Enter before another token remains inert, as in the ordinary editor
	// [07 R-WGT-01 §6]. An Escape stops before the later recall token.
	d.step(historyKey(input.KeyUp), historyKey(input.KeyEnter), historyText('!'))
	d.recalled("+two!")
	if d.b.chat.lastCommand != "two" || len(d.b.chat.commandHistory) != 2 {
		t.Fatal("recall or editing dispatched or recorded a command")
	}
	d.step(historyKey(input.KeyEscape), historyKey(input.KeyUp))
	if d.b.chat.active || d.b.cl.Input().PendingTokens() != 0 || len(d.b.chat.commandHistory) != 2 {
		t.Fatal("Escape failed to cancel and consume the history frame")
	}
	d.open()
	d.step(historyKey(input.KeyUp), historyText('!'), historyKey(input.KeyEnter))
	if d.b.chat.active || d.b.chat.lastCommand != "two!" || d.b.chat.commandHistory[2] != "+two!" {
		t.Fatal("edited recall did not submit through the original command path")
	}
}

func TestTalkHistoryBoundDuplicateAndCancel(t *testing.T) {
	d := newTalkHistoryDriver(t)
	d.submit("+unknown")
	d.submit("plain chat")
	d.submit("\\+escaped")
	d.submit("+unknown")
	if len(d.b.chat.commandHistory) != 1 {
		t.Fatal("plain chat entered history or consecutive command duplicate survived")
	}
	d.open()
	d.step(historyPaste("+cancelled"), historyKey(input.KeyEscape))
	d.open()
	d.step(historyKey(input.KeyUp), historyText('!'))
	d.b.cl.Input().Mouse.SetButton(input.MouseButtonRight, true)
	d.step(historyKey(input.KeyEnter))
	if d.b.chat.active || len(d.b.chat.commandHistory) != 1 || d.b.chat.lastCommand != "unknown" {
		t.Fatal("right cancellation submitted the recalled line")
	}
	d.b.cl.Input().Mouse.ResetEdges()
	d.b.cl.Input().Mouse.SetButton(input.MouseButtonRight, false)
	d.b.cl.Input().Mouse.ResetEdges()
	for i := 0; i < talkCommandHistoryLimit+1; i++ {
		d.b.commitLocalChat(fmt.Sprintf("+history%d", i))
	}
	history := append([]string(nil), d.b.chat.commandHistory...)
	if len(history) != talkCommandHistoryLimit || history[0] != "+history1" || history[len(history)-1] != "+history64" {
		t.Fatalf("bounded history retained %d entries with ends %q / %q", len(history), history[0], history[len(history)-1])
	}
	d.b.commitLocalChat("+history64")
	if !reflect.DeepEqual(history, d.b.chat.commandHistory) {
		t.Fatal("a repeated newest command evicted another entry")
	}
	d.open()
	d.step(historyKey(input.KeyUp), historyKey(input.KeyDown))
	d.recalled("") // opening a new line forgot the cancelled draft
	other := newTalkHistoryDriver(t)
	other.open()
	other.step(historyKey(input.KeyUp), historyKey(input.KeyDown))
	other.recalled("") // history belongs to the battle
}

func TestTalkHistoryUsesCurrentEditorAdmission(t *testing.T) {
	for _, tc := range []struct {
		name     string
		maxChars int16
		width    int32
		want     string
	}{{"byte limit", 3, 350, "+un"}, {"width minus four", 63, 7, "+un"}} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTalkHistoryDriver(t)
			d.submit("+unknown")
			d.open()
			// Paste has its own admission and may retain a draft wider than
			// typing allows; returning to it must restore the exact draft.
			d.step(historyPaste("draft"))
			g := &d.b.hud.talkWin.Gadgets[d.b.hud.talkPanel.Index("TALK")]
			g.MaxChars, g.Rect.W = tc.maxChars, tc.width
			d.step(historyKey(input.KeyUp))
			d.recalled(tc.want)
			d.step(historyKey(input.KeyDown))
			d.recalled("draft")
		})
	}
}

func TestTalkHistoryKeepsWorldInputOwnership(t *testing.T) {
	d := newTalkHistoryDriver(t)
	d.submit("+unknown")
	u := placeUnit(d.b, "armcons", numeric.Fixed(200*65536), numeric.Fixed(120*65536))
	replaceSelectionForTest(t, d.b, u)
	d.b.battleState().Input.Latch = input.LatchAttack
	d.open()
	in := d.b.cl.Input()
	in.Mouse.SetButton(input.MouseButtonLeft, true)
	in.Kbd.SetKey(input.KeyDown, true)
	d.b.cam.Z = 200
	d.step(historyKey(input.KeyUp), historyKey(input.KeyDown))
	if len(d.b.sess.PendingHumanCommands()) != 0 || !hostSelected(d.b, u) || d.b.battleState().Input.Latch != input.LatchAttack || d.b.cam.Z != 200 {
		t.Fatal("history admitted world input or held-arrow scrolling")
	}
	in.Mouse.ResetEdges()
	in.Mouse.SetButton(input.MouseButtonLeft, false)
	in.Mouse.ResetEdges()
	d.step(historyKey(input.KeyEscape))
	// Once TALK closes, the existing held-arrow camera path remains live.
	d.b.millisSource = &chatMillis{}
	d.b.controller = NewBattleController(d.b, d.b.millisSource)
	d.step(historyKey(input.KeyDown))
	before := d.b.cam.Z
	d.step(historyKey(input.KeyDown))
	if d.b.cam.Z <= before || d.b.chat.active {
		t.Fatal("closed TALK intercepted the battle's Down key")
	}
}

func TestTalkHistoryAccessAndCommandBoundaryAllModes(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, campaign := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/campaign=%v", mode, campaign), func(t *testing.T) {
				d := newTalkHistoryDriver(t)
				b := d.b
				b.sess.Gameplay = mode
				if campaign {
					b.sess.Mission = &mission.Mission{Type: mission.TypeCampaign}
				}
				stock, simRNG, crtRNG := b.sess.Econ.Players[0].Stock, *b.sess.SimRNG(), *b.sess.CrtRNG()
				d.submit("+ATM")
				want := 1
				if campaign {
					want = 0
				}
				pendingATM := func(count int) {
					t.Helper()
					commands := b.sess.PendingHumanCommands()
					if len(commands) != count {
						t.Fatalf("pending commands = %d, want %d", len(commands), count)
					}
					for _, command := range commands {
						if command.Kind != session.HumanATM {
							t.Fatalf("pending command = %+v, want ATM", command)
						}
					}
				}
				d.open()
				d.step(historyKey(input.KeyUp))
				d.recalled("+ATM")
				pendingATM(want)
				if b.developer.authorized {
					t.Fatal("history granted developer access")
				}
				d.step(historyKey(input.KeyEnter))
				pendingATM(want * 2)
				// The access phrase itself can be recalled but is executed only
				// by Enter. Its original retail case checks remain in force.
				d.submit("+Now Film Chris Include Reload Assert")
				if !b.developer.authorized {
					t.Fatal("ordinary phrase submission failed")
				}
				b.developer.authorized = false
				d.open()
				d.step(historyKey(input.KeyUp))
				if b.developer.authorized || b.chat.lastCommand != "Now Film Chris Include Reload Assert" {
					t.Fatal("recall dispatched the access phrase")
				}
				d.step(historyKey(input.KeyEnter))
				if !b.developer.authorized {
					t.Fatal("recalled phrase bypassed the original dispatch path")
				}
				d.submit("+ATM")
				d.open()
				d.step(historyKey(input.KeyUp), historyKey(input.KeyUp))
				if b.chat.lastCommand != "ATM" {
					t.Fatal("recall replaced the retail backslash command")
				}
				d.step(historyKey(input.KeyEscape))
				b.cl.Input().ShortcutTokenMode = true
				d.step(historyText('\\'))
				pendingATM(want * 4)
				if b.sess.Econ.Players[0].Stock != stock || *b.sess.SimRNG() != simRNG || *b.sess.CrtRNG() != crtRNG {
					t.Fatal("history changed resources or RNG before the command boundary")
				}
			})
		}
	}
}
