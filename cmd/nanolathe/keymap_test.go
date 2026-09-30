package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/economy"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func testChords(t *testing.T, written ...string) []input.Chord {
	t.Helper()
	out := make([]input.Chord, len(written))
	for i, s := range written {
		c, err := input.ParseChord(s)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = c
	}
	return out
}

// TestKeyMapBypassedWhileTalkOpen: TALK's line receives the physical keys
// even with Attack moved from A to Q, while outside it the battle hook drops
// the unbound retail key and plays the rebound one.
func TestKeyMapBypassedWhileTalkOpen(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(100, 100))
	b.sess.Econ = &economy.Service{}
	b.sess.Econ.Players[0].Exists = true
	b.sess.Econ.Players[0].Name = "Player"
	b.hud = &retailBattleHUD{talkWin: testTalkWindow()}
	b.millisSource = &chatMillis{}
	b.controller = NewBattleController(b, b.millisSource)
	b.shell = &gameShell{keyMap: input.NewKeyMap(input.ProfileRetail, map[string][]input.Chord{
		"attack":        testChords(t, "q"),
		"clearMessages": testChords(t, "home"),
	})}
	cl := b.cl
	cl.SetFocused(true)
	in := cl.Input()

	in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
	b.viewerStep(0, cl)
	if !b.chat.active {
		t.Fatal("Enter, still the chat key, did not open TALK")
	}
	enqueueText(in, "aq")
	in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyEnter})
	b.viewerStep(0, cl)
	lines := cl.MessageRing().Visible()
	if len(lines) != 1 || lines[0].Text != "<Player> aq" {
		t.Fatalf("TALK line = %+v, want the typed <Player> aq", lines)
	}
	// Outside TALK: the retail F12 no longer clears, the rebound Home does.
	in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyF12})
	b.viewerStep(0, cl)
	if got := len(cl.MessageRing().Visible()); got != 1 || in.PendingTokens() != 0 {
		t.Fatalf("unbound F12: %d lines, %d tokens pending; want 1 and 0", got, in.PendingTokens())
	}
	in.EnqueueToken(input.Token{Kind: input.TokenEdit, Key: input.KeyHome})
	b.viewerStep(0, cl)
	if got := len(cl.MessageRing().Visible()); got != 0 {
		t.Fatalf("rebound Home left %d lines, want the ring cleared", got)
	}
}

// TestKeyBindingsSettingsRoundTrip: the file keeps the profile and only the
// rebound actions; unknown actions and unreadable chords are dropped on load,
// and an action saved with no keys stays unbound.
func TestKeyBindingsSettingsRoundTrip(t *testing.T) {
	m := keyMapFromSettings(settings.KeyBindings{Profile: " Zero ", Bindings: map[string][]string{
		"attack":      {"Q", "hyper+q", "q"},
		"trackNext":   {},
		"noSuchThing": {"x"},
		"patrol":      {"alt+p"},
		"recallGroup": {"z"},
	}})
	if m.Profile() != input.ProfileZero {
		t.Fatalf("profile = %q, want zero", m.Profile())
	}
	if got := m.Keys("attack"); !slices.Equal(got, testChords(t, "q")) {
		t.Errorf("attack = %v, want q", got)
	}
	if got := m.Keys("trackNext"); len(got) != 0 {
		t.Errorf("trackNext = %v, want unbound", got)
	}
	if got := m.Keys("patrol"); !slices.Equal(got, testChords(t, "p")) {
		t.Errorf("patrol = %v, want its default after an unreadable list", got)
	}
	saved := keyBindingsSetting(m)
	want := settings.KeyBindings{Profile: "zero", Bindings: map[string][]string{"attack": {"q"}, "trackNext": {}}}
	if saved.Profile != want.Profile || len(saved.Bindings) != 2 || !slices.Equal(saved.Bindings["attack"], want.Bindings["attack"]) ||
		saved.Bindings["trackNext"] == nil || len(saved.Bindings["trackNext"]) != 0 {
		t.Fatalf("saved = %+v, want %+v", saved, want)
	}

	if !(settings.KeyBindings{Profile: " Retail "}).IsZero() || (settings.KeyBindings{Bindings: map[string][]string{"attack": {}}}).IsZero() {
		t.Fatal("IsZero: a retail block with nothing rebound is zero; an unbound action is not")
	}
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	file := settings.Defaults()
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(os.Getenv(settings.EnvPath)); err != nil || strings.Contains(string(data), "keyBindings") {
		t.Fatalf("a retail block with nothing rebound was written: %v", err)
	}
	file.KeyBindings = saved
	if err := file.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &gameShell{keyMap: keyMapFromSettings(loaded.KeyBindings)}
	for _, a := range input.Actions() {
		if !slices.Equal(g.keyMap.Keys(a.ID), m.Keys(a.ID)) {
			t.Errorf("%s reloaded as %v, want %v", a.ID, g.keyMap.Keys(a.ID), m.Keys(a.ID))
		}
	}
}

// TestControlsPresetSelectsKeyboardProfile: a controls preset selects its
// keyboard profile and keeps the player's rebound actions; leaving it
// restores the retail keys.
func TestControlsPresetSelectsKeyboardProfile(t *testing.T) {
	g := presetTestShell(t)
	g.liveKeyMap().Rebind("attack", testChords(t, "q"))
	g.applyControlsPreset(controlsPresetZero)
	if g.keyMap.Profile() != input.ProfileZero || !slices.Equal(g.keyMap.Keys("attack"), testChords(t, "q")) {
		t.Fatalf("zero preset: profile %q, attack %v", g.keyMap.Profile(), g.keyMap.Keys("attack"))
	}
	if got, _ := g.keyMap.Translate(input.Token{Kind: input.TokenText, Rune: 'z'}); got.Rune != ',' {
		t.Fatalf("zero preset: z = %+v, want the previous-page ,", got)
	}
	g.applyControlsPreset(controlsPresetRetail)
	if g.keyMap.Profile() != input.ProfileRetail || !slices.Equal(g.keyMap.Keys("attack"), testChords(t, "q")) {
		t.Fatalf("retail preset: profile %q, attack %v", g.keyMap.Profile(), g.keyMap.Keys("attack"))
	}
}

// TestKeyCaptureChord: a captured key is the adapter's own key, with the
// modifiers the battle can tell apart.
func TestKeyCaptureChord(t *testing.T) {
	for _, tc := range []struct {
		key         ebiten.Key
		ctrl, shift bool
		want        string
	}{
		{ebiten.KeyQ, false, false, "q"},
		{ebiten.KeyT, false, true, "shift+t"},
		{ebiten.KeyA, true, true, "ctrl+a"},
		{ebiten.KeyF5, false, true, "f5"},
		{ebiten.KeyNumpadEnter, false, false, "enter"},
		{ebiten.KeyNumpadAdd, false, false, "shift+="},
		{ebiten.KeyShiftRight, false, true, ""},
		{ebiten.KeyCapsLock, false, false, ""},
	} {
		c, ok := keyCaptureChord(tc.key, tc.ctrl, tc.shift)
		if got := c.String(); ok != (tc.want != "") || got != tc.want {
			t.Errorf("%v ctrl=%v shift=%v = %q (%v), want %q", tc.key, tc.ctrl, tc.shift, got, ok, tc.want)
		}
	}
}

// orderGadgets names the command-palette gadget, without its side prefix,
// that each order action's retail key activates [07 R-WGT-01 §3].
var orderGadgets = map[string]string{
	"move": "MOVE", "attack": "ATTACK", "patrol": "PATROL", "guard": "DEFEND", "stop": "STOP",
	"repair": "REPAIR", "reclaim": "RECLAIM", "capture": "CAPTURE", "load": "LOAD", "unload": "UNLOAD",
	"dgun": "BLAST", "fireOrders": "FIREORD", "moveOrders": "MOVEORD", "onOff": "ONOFF", "cloak": "CLOAK",
	"ordersPage": "ORDERS", "buildPage": "BUILD",
}

// TestRetailOrderKeysMatchAuthoredPalette locks the Orders actions to the
// quick keys ARMGEN.GUI and CORGEN.GUI author: every quick key of both
// windows is an order action's default, case aside.
func TestRetailOrderKeysMatchAuthoredPalette(t *testing.T) {
	root := testsupport.RetailRoot(t)
	fs := vfs.New()
	if err := fs.MountGameDirectory(root); err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	for _, side := range []string{"ARM", "COR"} {
		window, err := gui.LoadWithTranslation(fs, "guis/"+side+"GEN.GUI", nil)
		if err != nil {
			t.Fatal(err)
		}
		authored := map[string]byte{}
		for _, g := range window.Gadgets {
			if g.QuickKey != 0 {
				authored[strings.TrimPrefix(g.Name, side)] = g.QuickKey
			}
		}
		if len(authored) != len(orderGadgets) {
			t.Errorf("%sGEN.GUI authors %d quick keys, the catalogue %d orders", side, len(authored), len(orderGadgets))
		}
		for id, gadget := range orderGadgets {
			action, ok := input.LookupAction(id)
			if !ok || !action.AnyShift || len(action.Default) != 1 {
				t.Fatalf("%s is not a one-key order action", id)
			}
			key := action.Default[0].String()
			if got := strings.ToLower(string(rune(authored[gadget]))); got != key {
				t.Errorf("%s%s quick key %q, catalogue %s has %q", side, gadget, got, id, key)
			}
		}
	}
}
