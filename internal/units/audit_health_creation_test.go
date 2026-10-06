package units

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

// Current health is a signed word while maximum health retains its complete
// definition value. Create callbacks already observe that narrowing [04 §4.4].
func TestCreationHealthWordBeforeScriptBind(t *testing.T) {
	for _, tc := range []struct {
		name            string
		maximum, health int32
	}{
		{"ordinary", 100, 100},
		{"signed boundary", 32768, -32768},
		{"word wrap", 65536, 0},
		{"high-bit maximum", -2147483548, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, kind := range []string{"complete", "forced", "nanoframe"} {
				t.Run(kind, func(t *testing.T) {
					w := newFixtureWorld(4, nil)
					def := &content.UnitDef{UnitName: "health-word", MaxDamage: tc.maximum, Limit: -1}
					wantHealth, wantRemaining := tc.health, float32(0)
					if kind == "nanoframe" {
						wantHealth, wantRemaining = 0, 1
					}
					bound := false
					w.SetCOBBinder(func(u *Unit) error {
						bound = true
						if u.Health != wantHealth || u.MaxHealth != tc.maximum || u.Remaining != wantRemaining {
							t.Fatalf("script bind health=%d max=%d remaining=%v; want %d/%d/%v", u.Health, u.MaxHealth, u.Remaining, wantHealth, tc.maximum, wantRemaining)
						}
						return nil
					})
					var h pool.Handle
					var err error
					switch kind {
					case "complete":
						h, err = w.Create(def, 0, 0, 0, 0)
					case "forced":
						h, err = w.CreateWithForcedSlot(def, 0, 0, 0, 0, 2)
					case "nanoframe":
						h, err = w.CreateNanoframe(def, 0, 0, 0, 0)
					}
					if err != nil {
						t.Fatal(err)
					}
					u := w.Unit(h)
					if !bound || u == nil || u.Health != wantHealth || !u.Alive || u.Dying {
						t.Fatalf("creation did not retain the initialized allocated record: bound=%v unit=%+v", bound, u)
					}
				})
			}
		})
	}
}
