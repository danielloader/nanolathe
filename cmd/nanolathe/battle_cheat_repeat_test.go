package main

import (
	"fmt"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// Replay requires developer access separately from the skirmish cheat gate.
// Repeated character events drain one per host pass without another TALK post
// [07 R-CAM-01 §2][07 R-CAM-01 §9][08 R-OOS-01 §2].
func TestBackslashReplayThroughBattleInput(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
			if b.cl == nil {
				t.Fatal("fixture client unavailable")
			}
			t.Cleanup(b.cl.Close)
			b.sess.Gameplay = mode
			b.sess.Econ = &economy.Service{}
			b.sess.Econ.Players[0] = economy.Player{Exists: true, Name: "Player", Stock: [2]float32{123, 456}}
			b.sess.SeedSessionRNG(7, 11)
			stock, simRNG, crtRNG := b.sess.Econ.Players[0].Stock, *b.sess.SimRNG(), *b.sess.CrtRNG()
			b.hud = &retailBattleHUD{talkWin: testTalkWindow()}
			b.millisSource = &fakeMillisSource{}
			b.controller = NewBattleController(b, b.millisSource)
			b.cl.SetFocused(true)
			in := b.cl.Input()
			in.ShortcutTokenMode = true
			commit := func(text string) {
				t.Helper()
				in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
				b.viewerStep(0, b.cl)
				if !b.chat.active {
					t.Fatal("Enter did not open TALK")
				}
				in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyV, Ctrl: true, Clipboard: input.ClipboardText{Text: text, Available: true}})
				in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
				b.viewerStep(0, b.cl)
				if b.chat.active {
					t.Fatal("Enter did not commit TALK")
				}
			}
			pendingATM := func(want int) {
				t.Helper()
				commands := b.sess.PendingHumanCommands()
				if len(commands) != want {
					t.Fatalf("pending commands = %d, want %d", len(commands), want)
				}
				for _, command := range commands {
					if command.Kind != session.HumanATM {
						t.Fatalf("pending command = %+v, want ATM", command)
					}
				}
			}

			commit("+aTm")
			pendingATM(1)
			in.EnqueueToken(input.Token{Kind: input.TokenText, Rune: '\\'})
			b.viewerStep(0, b.cl)
			pendingATM(1)
			if b.developer.authorized {
				t.Fatal("ordinary cheat enabled developer access")
			}

			// No startup developer option is needed: the ordinary focused editor
			// accepts the historical phrase in every gameplay mode.
			commit("+nOw Film Chris Include Reload Assert")
			if !b.developer.authorized || b.developer.film {
				t.Fatal("historical phrase did not enable access independently of film")
			}
			commit("+aTm")
			pendingATM(2)
			lines := len(b.cl.MessageRing().Visible())
			// Printable-key hold repeats arrive as successive translated text
			// events. The shifted character has no replay case.
			for _, r := range "\\\\|" {
				in.EnqueueToken(input.Token{Kind: input.TokenText, Rune: r})
			}
			for step := 1; step <= 4; step++ {
				b.viewerStep(0, b.cl)
				pendingATM(2 + min(step, 2))
			}
			if b.chat.lastCommand != "aTm" || len(b.cl.MessageRing().Visible()) != lines {
				t.Fatal("replay replaced the retained command or posted another TALK line")
			}
			commit("\\") // TALK owns the character while its editor is open.
			pendingATM(4)
			if b.chat.lastCommand != "aTm" {
				t.Fatal("ordinary chat replaced the retained command")
			}
			if b.sess.Econ.Players[0].Stock != stock || *b.sess.SimRNG() != simRNG || *b.sess.CrtRNG() != crtRNG {
				t.Fatal("shortcut mutated resources or RNG before the command boundary")
			}
		})
	}
}
