package settings

import (
	"slices"
	"strings"
)

// KeyBindings is the settings key `keyBindings`: the player's keyboard
// profile and the actions they rebound from it
// (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.6 "Rebinding"). Bindings maps an
// action identifier to its chords in their written form ("ctrl+a", "f5",
// "q"); an action absent from it keeps the profile's keys, and an action
// with an empty list is unbound. The zero value is the retail profile with
// nothing rebound and is omitted from the file.
//
// This package stores the block only. The action catalogue and the chord
// grammar belong to internal/input, and the shell drops unknown actions and
// unreadable chords when it builds its key map from the block.
type KeyBindings struct {
	Profile  string              `json:"profile,omitempty"`
	Bindings map[string][]string `json:"bindings,omitempty"`
}

// IsZero reports whether the block is the retail profile with nothing
// rebound, which the settings file omits (`omitzero`). An unbound action is a
// binding, so a block holding one is not zero.
func (k KeyBindings) IsZero() bool {
	profile := strings.ToLower(strings.TrimSpace(k.Profile))
	return (profile == "" || profile == "retail") && len(k.Bindings) == 0
}

// Normalized returns a copy with the profile trimmed and lower-cased, blank
// action names dropped, and each chord list trimmed, lower-cased and cleared
// of blanks and repeats, keeping first-seen order. A list that was empty
// stays empty: it records an unbound action. A list whose every entry was
// blank is dropped, since it records nothing the player chose.
func (k KeyBindings) Normalized() KeyBindings {
	out := KeyBindings{Profile: strings.ToLower(strings.TrimSpace(k.Profile))}
	if out.Profile == "retail" {
		out.Profile = ""
	}
	// Sorted names make the first spelling of a repeated action win the
	// same way on every load.
	names := make([]string, 0, len(k.Bindings))
	for name := range k.Bindings {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		chords := k.Bindings[name]
		action := strings.TrimSpace(name)
		if _, seen := out.Bindings[action]; action == "" || seen {
			continue
		}
		list := make([]string, 0, len(chords))
		for _, chord := range chords {
			chord = strings.ToLower(strings.TrimSpace(chord))
			if chord == "" || slices.Contains(list, chord) {
				continue
			}
			list = append(list, chord)
		}
		if len(chords) != 0 && len(list) == 0 {
			continue
		}
		if out.Bindings == nil {
			out.Bindings = map[string][]string{}
		}
		out.Bindings[action] = list
	}
	return out
}
