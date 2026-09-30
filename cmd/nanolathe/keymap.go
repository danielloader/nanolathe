package main

// Keyboard rebinding: the shell's saved key map and the battle hook that
// applies it (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.6 "Rebinding"). The map is
// a host preference. It rewrites the queued keyboard token the battle is
// about to service and never reaches the simulation, a digest or a save [I6].

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/platform/ebitenapp"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

// keyProfiles are the Keyboard controls-preset row's stored values: the
// profiles in their Retail/Community/Zero display order.
var keyProfiles = input.Profiles()

// keyMapFromSettings builds the shell's key map from the saved block. Unknown
// actions and unreadable chords are dropped; a list with chords that all fail
// to read keeps the profile's keys, while a list saved empty unbinds the
// action.
func keyMapFromSettings(kb settings.KeyBindings) *input.KeyMap {
	kb = kb.Normalized()
	overrides := map[string][]input.Chord{}
	for _, action := range input.Actions() {
		written, ok := kb.Bindings[action.ID]
		if !ok || action.Fixed {
			continue
		}
		chords := make([]input.Chord, 0, len(written))
		for _, s := range written {
			if c, err := input.ParseChord(s); err == nil {
				chords = append(chords, c)
			}
		}
		if len(written) != 0 && len(chords) == 0 {
			continue
		}
		overrides[action.ID] = chords
	}
	return input.NewKeyMap(kb.Profile, overrides)
}

// keyBindingsSetting is the block captureSettings writes: the profile, and
// only the actions that differ from it.
func keyBindingsSetting(m *input.KeyMap) settings.KeyBindings {
	var kb settings.KeyBindings
	if m == nil {
		return kb
	}
	if p := m.Profile(); p != input.ProfileRetail {
		kb.Profile = p
	}
	overrides := m.Overrides()
	for _, action := range input.Actions() {
		chords, ok := overrides[action.ID]
		if !ok {
			continue
		}
		written := make([]string, 0, len(chords))
		for _, c := range chords {
			written = append(written, c.String())
		}
		if kb.Bindings == nil {
			kb.Bindings = map[string][]string{}
		}
		kb.Bindings[action.ID] = written
	}
	return kb
}

// liveKeyMap is the shell's key map, which a settings screen edits in place;
// a shell that never loaded settings starts the retail one.
func (g *gameShell) liveKeyMap() *input.KeyMap {
	if g.keyMap == nil {
		g.keyMap = input.NewKeyMap(input.ProfileRetail, nil)
	}
	return g.keyMap
}

// setKeyProfile switches the keyboard profile and keeps every action the
// player rebound.
func (g *gameShell) setKeyProfile(profile string) {
	g.keyMap = input.NewKeyMap(profile, g.liveKeyMap().Overrides())
}

// keyProfileIndex is the Keyboard row's value for the live profile.
func (g *gameShell) keyProfileIndex() int {
	profile := input.ProfileRetail
	if g.keyMap != nil {
		profile = g.keyMap.Profile()
	}
	for i, p := range keyProfiles {
		if p == profile {
			return i
		}
	}
	return 0
}

// keyCaptureChord is the chord a settings screen binds when the player
// presses k with the given live modifiers. Alt is not a parameter: the battle
// folds Alt+key into the plain key. A modifier alone, or a key the battle
// cannot tell apart, is not a chord.
func keyCaptureChord(k ebiten.Key, ctrl, shift bool) (input.Chord, bool) {
	key, ok := ebitenapp.PortableKey(k)
	if !ok {
		return input.Chord{}, false
	}
	return input.Chord{Key: key, Ctrl: ctrl, Shift: shift}.Normalize()
}

// keyMap is the key map the battle applies, nil for a battle without a shell
// (captures, benchmarks, headless runs), which play the retail keys.
func (b *battleSession) keyMap() *input.KeyMap {
	if b == nil || b.shell == nil {
		return nil
	}
	return b.shell.keyMap
}

// textEntryActive reports whether a battle text editor has the keyboard: the
// TALK chat line or a focused text field. Its keys are text, never shortcuts.
func (b *battleSession) textEntryActive() bool {
	return b.isTalkGUIActive() || b.hud != nil && b.hud.editorFocused()
}

// serviceKeyMap rewrites the queued token the battle services this pass,
// after UNITINFO's authored keys and Zero's drag-filter key have had their
// look at the physical key and before the command palette, TALK's opener and
// the residual hotkey table read it. A dropped token is claimed, as a consumed
// one would be, so no later consumer services the next queued token in the
// same pass [07 §2][07 R-WGT-01 §1].
func (b *battleSession) serviceKeyMap(in *input.State) (claimed bool) {
	m := b.keyMap()
	if m == nil || in == nil || b.textEntryActive() {
		return false
	}
	return m.TranslatePending(in)
}

// serviceKeyMapModal lets a rebound options key close the options window it
// opened. Inside the window only the two options actions are rewritten, and
// only while the window's own toggle would act on them; every other key
// reaches the window's authored controls as the physical key.
func (b *battleSession) serviceKeyMapModal(in *input.State) {
	m := b.keyMap()
	if m == nil || in == nil || b.textEntryActive() || b.battleState().Modal() != ui.BattleModalOptions {
		return
	}
	if g := b.shell; g.frontend == nil || b.battlePrefsActive() || g.saveLoadPanelActive() || g.frontend.Panels.Modal() != nil {
		return
	}
	tokens := in.PeekTokens()
	if len(tokens) == 0 {
		return
	}
	c, ok := input.ChordOf(tokens[0])
	if !ok {
		return
	}
	if id, ok := m.Owner(c); ok && (id == "options" || id == "optionsTab") {
		m.TranslatePending(in)
	}
}
