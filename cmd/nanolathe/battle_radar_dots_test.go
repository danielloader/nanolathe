package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/render"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/visibility"
)

func radarDotBattle(t *testing.T, mode gameplay.Mode, style int) (*battleSession, pool.Handle) {
	t.Helper()
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	b.sess.SetGameplay(mode)
	target := placeUnit(b, "armcons", numeric.FixedFromInt(320), numeric.FixedFromInt(180))
	target.Owner = 1
	actor := placeUnit(b, "armcons", numeric.FixedFromInt(180), numeric.FixedFromInt(180))
	actor.Def.CanAttack = true
	replaceSelectionForTest(t, b, actor)
	cur, _ := b.currentSnapshot()
	written := b.sess.Snapshot.BeginWrite()
	*written = *cur
	written.Units = append([]frame.UnitView(nil), cur.Units...)
	written.Radar.Contacts = append([]frame.RadarContactView(nil), cur.Radar.Contacts...)
	written.Radar.MappingLOS = 3
	for i := range written.Units {
		v := &written.Units[i]
		if v.Slot == target.Handle {
			v.DirectVisibilityKnown, v.DirectlyVisible = true, false
		}
	}
	found := false
	for i := range written.Radar.Contacts {
		p := &written.Radar.Contacts[i]
		if p.Handle == target.Handle {
			p.Status, p.Seen, p.Visible, p.PaletteKnown, p.Palette = visibility.SeenBit, true, true, true, 0
			found = true
		}
	}
	if !found {
		t.Fatal("target has no committed contact")
	}
	if err := b.sess.Snapshot.Publish(cur.Tick + 1); err != nil {
		t.Fatal(err)
	}
	b.cl.SetEnhanced(true)
	b.cl.SetCamera(b.cam)
	b.cl.SetRadarDots(style)
	b.cl.SetStrategicBlipArt(&formats.GAFEntry{Frames: []formats.GAFFrameRef{{Frame: &formats.GAFFrame{Width: 1, Height: 1, Pixels: []byte{23}, Transparent: []bool{false}}}}})
	return b, target.Handle
}

func TestModernRadarDotAttackUsesOrdinaryCommandWithoutIdentifiedHover(t *testing.T) {
	for _, latch := range []input.Latch{input.LatchNormal, input.LatchAttack} {
		for _, queued := range []bool{false, true} {
			b, target := radarDotBattle(t, gameplay.Modern, 2)
			sim, crt := *b.sess.SimRNG(), *b.sess.CrtRNG()
			if h, _, _ := b.pickTarget(320, 180); h != 0 {
				t.Fatal("hidden dot became an identified pick")
			}
			if got := b.cursorShapeAt(latch, 320, 180); got != render.CursorAttack {
				t.Fatalf("cursor %d, want attack", got)
			}
			b.updateFooterHover(320, 180)
			if b.footerHoverUnit != 0 {
				t.Fatal("attack contact revealed footer identity")
			}
			if !b.orderSelected(hud.LatchToCode(latch), 320, 180, queued) {
				t.Fatal("contact attack refused")
			}
			pending := b.sess.PendingHumanCommands()
			if len(pending) != 1 || pending[0].Kind != session.HumanOrder || pending[0].Order.Target != target || pending[0].Order.Code != hud.LatchToCode(input.LatchAttack) || pending[0].Order.Queued != queued {
				t.Fatalf("anonymous attack command = %+v", pending)
			}
			if *b.sess.SimRNG() != sim || *b.sess.CrtRNG() != crt {
				t.Fatal("presentation/command construction drew RNG")
			}
		}
	}
}

func TestRadarDotAttacksRequireModernAttackableHostileAdmission(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for style := 0; style <= 2; style++ {
			b, target := radarDotBattle(t, mode, style)
			h, _ := b.pickRadarAttackTarget(320, 180, input.LatchAttack)
			want := pool.Handle(0)
			if mode == gameplay.Modern && style == 2 {
				want = target
			}
			if h != want {
				t.Fatalf("%s style %d hit %d, want %d", mode, style, h, want)
			}
			for _, latch := range []input.Latch{input.LatchRepair, input.LatchReclaim, input.LatchCapture, input.LatchFollow, input.LatchLoad} {
				if h, _ := b.pickRadarAttackTarget(320, 180, latch); h != 0 {
					t.Fatalf("%v admitted hidden action", latch)
				}
			}
		}
	}
	b, _ := radarDotBattle(t, gameplay.Modern, 2)
	f, _ := b.currentSnapshot()
	// Synthetic immutable-frame fixture: replacement publication owns copies.
	written := b.sess.Snapshot.BeginWrite()
	*written = *f
	written.Players[0].Allies[1] = true
	if err := b.sess.Snapshot.Publish(f.Tick + 1); err != nil {
		t.Fatal(err)
	}
	if h, _ := b.pickRadarAttackTarget(320, 180, input.LatchAttack); h != 0 {
		t.Fatal("allied anonymous contact became an attack target")
	}
}

func TestModernRadarDotLeftClickQueuesAttack(t *testing.T) {
	b, target := radarDotBattle(t, gameplay.Modern, 2)
	in := input.NewState()
	in.Mouse.X, in.Mouse.Y = 320, 180
	in.Mouse.SetButton(input.MouseButtonLeft, true)
	b.handleInput(in, nil)
	in.Mouse.ResetEdges()
	in.Mouse.SetButton(input.MouseButtonLeft, false)
	b.handleInput(in, nil)
	pending := b.sess.PendingHumanCommands()
	if len(pending) != 1 || pending[0].Kind != session.HumanOrder || pending[0].Order.Target != target || pending[0].Order.Code != hud.LatchToCode(input.LatchAttack) {
		t.Fatalf("dot left click queued %+v", pending)
	}
}
