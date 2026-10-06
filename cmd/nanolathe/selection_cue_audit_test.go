package main

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Observe requests before any audio drain: playback throttles must not hide a
// wrong producer decision [07 §9 "selection acknowledgement"].
func selectionCueAudio(t *testing.T, b *battleSession) *compositionAudioSpy {
	t.Helper()
	spy := &compositionAudioSpy{}
	old := audio.GlobalOutput()
	audio.SetGlobalOutput(spy)
	t.Cleanup(func() { audio.SetGlobalOutput(old) })
	b.sess.Audio = audio.NewService(authoredVoiceFS(t, "multiple"))
	b.sess.Audio.Registry.RegisterPath(cueSelectMultiple, "sounds/multiple.wav")
	return spy
}

func selectionCueGesture(b *battleSession, x1, y1, x2, y2 int32) {
	in := input.NewState()
	in.Kbd.SetKey(input.KeyShift, true)
	in.Mouse.SetPosition(float32(x1), float32(y1))
	in.Mouse.SetButton(input.MouseButtonLeft, true)
	b.handleInput(in, nil)
	in.Mouse.ResetEdges()
	in.Mouse.SetPosition(float32(x2), float32(y2))
	b.handleInput(in, nil)
	in.Mouse.ResetEdges()
	in.Mouse.SetButton(input.MouseButtonLeft, false)
	b.handleInput(in, nil)
}

func TestSelectionPointCueFollowsToggleResult(t *testing.T) {
	for _, minimap := range []bool{false, true} {
		for _, selected := range []bool{false, true} {
			name := "viewport"
			if minimap {
				name = "minimap"
			}
			t.Run(name+map[bool]string{false: "/add", true: "/remove"}[selected], func(t *testing.T) {
				b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
				b.millisSource = &fakeMillisSource{ms: 1000}
				wx, wz := int32(200), int32(120)
				sx, sy := int32(70), int32(50)
				if minimap {
					dst := withMinimap(b)
					w, h, _ := b.sess.PlayArea()
					layout, _, _ := b.minimapLayout()
					var ok bool
					wx, wz, ok = client.MinimapPointerWorld(layout, dst, w, h, sx, sy)
					if !ok {
						t.Fatal("invalid authored minimap point")
					}
				}
				u := placeUnit(b, "armcons", numeric.Fixed(wx<<16), numeric.Fixed(wz<<16))
				var initial []*units.Unit
				if selected {
					initial = append(initial, u)
				}
				replaceSelectionForTest(t, b, initial...)
				if !minimap {
					sx, sy = screenPos(b.cam, u)
				}
				spy := selectionCueAudio(t, b)
				selectionCueGesture(b, sx, sy, sx, sy)
				if hostSelected(b, u) == selected {
					t.Fatal("gesture did not toggle the pointed unit")
				}
				want := 1
				if selected {
					want = 0
				}
				q := b.sess.Audio.Queue
				if q.Count != want || len(spy.aliases) != 0 {
					t.Fatalf("voice requests=%d UI cues=%v, want %d voices", q.Count, spy.aliases, want)
				}
				if want == 1 && (q.Entries[0].Slot != audio.SlotSelect || q.Entries[0].Unit != u.Handle) {
					t.Fatalf("wrong point voice request: %+v", q.Entries[0])
				}
				if len(b.sess.PendingHumanCommands()) != 0 {
					t.Fatal("local selection entered the world command queue")
				}
			})
		}
	}
}

func TestSelectionRectangleCueUsesRemainingEligibleSelection(t *testing.T) {
	for _, scenario := range []string{"one-off", "both-off", "empty", "ineligible-outside"} {
		t.Run(scenario, func(t *testing.T) {
			b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
			b.millisSource = &fakeMillisSource{ms: 1000}
			a := placeUnit(b, "armcons", 200<<16, 120<<16)
			c := placeUnit(b, "armcons", 280<<16, 120<<16)
			replaceSelectionForTest(t, b, a, c)
			if scenario == "ineligible-outside" {
				c.Remaining = 0.5
				applyPendingBattleCommands(b)
			}
			sx, sy := screenPos(b.cam, a)
			ex, ey := sx+20, sy+20
			sx, sy = sx-20, sy-20
			want := []pool.Handle{c.Handle}
			voice := c.Handle
			multiple := false
			switch scenario {
			case "both-off":
				ex, _ = screenPos(b.cam, c)
				ex += 20
				want, voice = nil, 0
			case "empty":
				sx, ex = sx+160, ex+160
				want, voice, multiple = []pool.Handle{a.Handle, c.Handle}, 0, true
			case "ineligible-outside":
				voice = 0
			}
			spy := selectionCueAudio(t, b)
			selectionCueGesture(b, sx, sy, ex, ey)
			if got := b.selectedHandlesInSlotOrder(); !slices.Equal(got, want) {
				t.Fatalf("selection=%v, want %v", got, want)
			}
			q := b.sess.Audio.Queue
			if voice != 0 {
				if q.Count != 1 || q.Entries[0].Unit != voice || q.Entries[0].Slot != audio.SlotSelect {
					t.Fatalf("remaining unit voice=%+v count=%d", q.Entries[0], q.Count)
				}
			} else if q.Count != 0 {
				t.Fatalf("unexpected voice requests: %d", q.Count)
			}
			if (len(spy.aliases) == 1) != multiple || len(spy.aliases) > 1 {
				t.Fatalf("multiple cue requests=%v, want multiple=%t", spy.aliases, multiple)
			}
			if len(b.sess.PendingHumanCommands()) != 0 {
				t.Fatal("local rectangle selection entered the world command queue")
			}
		})
	}
}
