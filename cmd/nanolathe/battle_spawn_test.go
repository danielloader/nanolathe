package main

import (
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

func TestSpawnChatCapturesSubmissionPointer(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	b.sess.Econ = &economy.Service{}
	b.sess.Econ.Players[0].Exists = true
	b.hud = &retailBattleHUD{talkWin: testTalkWindow()}
	in := b.cl.Input()
	in.Mouse.SetPosition(200, 100)
	if !b.openTalk(in, b.cl) {
		t.Fatal("TALK did not open")
	}
	in.Mouse.SetPosition(500, 350)
	x, y, z := b.cursorWorld(500, 350)
	enqueueText(in, "+SpAwN ARmCK")
	in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
	b.serviceTalk(in)
	pending := b.sess.PendingHumanCommands()
	if len(pending) != 1 || pending[0].Kind != session.HumanSpawn || pending[0].Spawn.Unit != "ARmCK" || pending[0].Spawn.X != x || pending[0].Spawn.Y != y || pending[0].Spawn.Z != z {
		t.Fatalf("spawn request = %+v", pending)
	}
	in.Mouse.SetPosition(300, 200)
	b.cam.X += 50 << 16
	if b.sess.PendingHumanCommands()[0].Spawn != pending[0].Spawn {
		t.Fatal("pointer movement retargeted command")
	}
}

func TestSpawnChatRejectsUsageChromeAndStrict(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		x, y       float32
		mode       gameplay.Mode
	}{
		{"missing", "+spawn", 300, 200, gameplay.Modern},
		{"extra", "+spawn armck extra", 300, 200, gameplay.Modern},
		{"chrome", "+spawn armck", 10, 450, gameplay.Modern},
		{"strict", "+spawn armck", 300, 200, gameplay.Strict31},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
			b.sess.Gameplay = tc.mode
			b.cl.Input().Mouse.SetPosition(tc.x, tc.y)
			b.dispatchLocalCommand(tc.text)
			if len(b.sess.PendingHumanCommands()) != 0 {
				t.Fatal("invalid spawn was queued")
			}
			if len(b.messageRing().Visible()) == 0 {
				t.Fatal("missing chat feedback")
			}
		})
	}
}

// The Modern `+<unit>` shorthand queues the same pointer-captured request as
// `+spawn <unit>`; everything else stays plain chat without feedback.
func TestUnitNameChatShorthandParse(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	b.cl.Input().Mouse.SetPosition(300, 200)
	x, y, z := b.cursorWorld(300, 200)
	b.dispatchLocalCommand("  +ArmFav")
	pending := b.sess.PendingHumanCommands()
	if len(pending) != 1 || pending[0].Kind != session.HumanSpawn || pending[0].Spawn.Unit != "ArmFav" || pending[0].Spawn.X != x || pending[0].Spawn.Y != y || pending[0].Spawn.Z != z {
		t.Fatalf("shorthand request = %+v", pending)
	}
	for _, tc := range []struct {
		name, text string
		mode       gameplay.Mode
	}{
		{"unknown name", "+hello", gameplay.Modern},
		{"arguments", "+armfav extra", gameplay.Modern},
		{"no plus", "armfav", gameplay.Modern},
		{"strict", "+armfav", gameplay.Strict31},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
			b.sess.Gameplay = tc.mode
			b.cl.Input().Mouse.SetPosition(300, 200)
			b.dispatchLocalCommand(tc.text)
			if len(b.sess.PendingHumanCommands()) != 0 {
				t.Fatal("non-spawn text was queued")
			}
			if len(b.messageRing().Visible()) != 0 {
				t.Fatal("plain chat produced spawn feedback")
			}
		})
	}
	// Off the battlefield the shorthand answers like +spawn does.
	b = newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	b.cl.Input().Mouse.SetPosition(10, 450)
	b.dispatchLocalCommand("+armfav")
	if len(b.sess.PendingHumanCommands()) != 0 || len(b.messageRing().Visible()) == 0 {
		t.Fatal("off-world shorthand queued or stayed silent")
	}
}

// +LOS toggles current-sight enforcement (mode bit 1) and nothing else; it is
// not a one-way reveal. From a Permanent start the first press turns
// enforcement ON, from a True/Circular start it turns it OFF [07 R-CAM-01 §6]
// [03 R-VIS-01 §1].
func TestLOSChatCommandTogglesCurrentSightBit(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	b.dispatchLocalCommand("+los")
	pending := b.sess.PendingHumanCommands()
	if len(pending) != 1 || pending[0].Kind != session.HumanVisibility || pending[0].Visibility.ToggleMask != visibility.ModeCurrentEnabled || pending[0].Visibility.ClearMask != 0 {
		t.Fatalf("+LOS request = %+v", pending)
	}
}

func TestDeveloperSpawnChatAccessOwnerAndPointerCapture(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
		b.sess.Gameplay = mode
		b.cl.Input().Mouse.SetPosition(300, 200)
		x, y, z := b.cursorWorld(300, 200)
		b.dispatchLocalCommand("+arm*")
		if len(b.sess.PendingHumanCommands()) != 0 {
			t.Fatal("wildcard bypassed developer access")
		}
		b.dispatchLocalCommand("+now Film Chris Include Reload Assert")
		b.dispatchLocalCommand("+ArM* 258junk ignored # comment")
		b.dispatchLocalCommand("+armfav")
		pending := b.sess.PendingHumanCommands()
		if len(pending) != 2 {
			t.Fatalf("queued = %+v", pending)
		}
		want := session.HumanDeveloperSpawnCommand{Pattern: "arm*", Owner: 2, X: x, Y: y, Z: z}
		if pending[0].Kind != session.HumanDeveloperSpawn || pending[0].DeveloperSpawn != want || pending[1].Kind != session.HumanDeveloperSpawn || pending[1].DeveloperSpawn.Owner != 0 {
			t.Fatalf("developer requests = %+v", pending)
		}
		b.cl.Input().Mouse.SetPosition(500, 350)
		b.cam.X += 50 << 16
		b.dispatchLocalCommand("+now wrong")
		if b.sess.PendingHumanCommands()[0].DeveloperSpawn != want {
			t.Fatal("pointer or access changes retargeted an accepted submission")
		}
		b.dispatchLocalCommand("+arm*")
		if len(b.sess.PendingHumanCommands()) != 2 {
			t.Fatal("revoked developer access admitted a pattern")
		}
	}
}
