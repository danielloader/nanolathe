package ebitenapp

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

// portableKeys is the adapter's key table read backwards, with the second
// physical key of each pair the forward table folds together.
var portableKeys = buildPortableKeys()

func buildPortableKeys() map[ebiten.Key]input.Key {
	out := make(map[ebiten.Key]input.Key, int(input.KeyCount)+4)
	for k := input.Key(1); k < input.KeyCount; k++ {
		keys, n := ebitenKeys(k)
		for _, ek := range keys[:n] {
			out[ek] = k
		}
	}
	// The forward table polls the left modifier only for its identity; the
	// live modifier state reads both sides.
	out[ebiten.KeyShiftRight] = input.KeyShift
	out[ebiten.KeyControlRight] = input.KeyCtrl
	out[ebiten.KeyAltRight] = input.KeyAlt
	return out
}

// PortableKey names an Ebitengine key in the platform-neutral vocabulary, for
// a settings screen that captures a key to bind
// (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.6 "Rebinding"). It is the reverse of
// the table the adapter polls, so a captured key is the key the battle's
// producer reports; a key the adapter never polls reports false.
func PortableKey(k ebiten.Key) (input.Key, bool) {
	key, ok := portableKeys[k]
	return key, ok
}
