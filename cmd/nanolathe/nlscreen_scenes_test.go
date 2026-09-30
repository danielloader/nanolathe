package main

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// These lock the Nanolathe screen's preview compositions, not retail
// behavioral claims (DESIGN_INTERFACE_HUD_INPUT §3.17): what each scene is
// staged to show has to happen in front of the camera.

// stageNLTestScene stages a preset the way the preview does, without a
// client, under Modern rules.
func stageNLTestScene(t *testing.T, name string) (*nlStage, func()) {
	return stageNLTestSceneUnder(t, name, gameplay.Modern)
}

// stageNLTestSceneUnder stages a preset under the given rules and returns a
// stepper that runs its script and exactly one simulation tick, as the
// preview's advance does.
func stageNLTestSceneUnder(t *testing.T, name string, rules gameplay.Mode) (*nlStage, func()) {
	t.Helper()
	opts, cs := openNLTestContent(t)
	preset := nlPresets[name]
	st, _, err := stageNLSession(opts, cs, preset, rules, "")
	if err != nil {
		t.Fatal(err)
	}
	s := st.s
	step := 0
	return st, func() {
		step++
		nlScriptTick(preset, st.events, s, step)
		// One scaled unit past the anchor releases one sub-tick at the
		// nominal speed; a pump that only dispatches state releases none.
		for end := s.Clock.GlobalTick + 1; s.State != session.StatePostBattle && s.Clock.GlobalTick < end; {
			s.Step(s.Clock.ScaledAnchor + 1)
		}
	}
}

func openNLTestContent(t *testing.T) (Options, *contentSet) {
	t.Helper()
	root := testsupport.RetailRoot(t)
	opts := Options{Root: root}
	cs, err := openContent(opts)
	if err != nil {
		t.Skipf("retail assets unavailable: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return opts, cs
}

// Every scene's camera holds one frame for its whole loop: a slow pan moves
// pixel art in uneven one-pixel steps, which reads as wobble.
func TestNLPreviewCamerasHoldStill(t *testing.T) {
	for _, name := range slices.Sorted(maps.Keys(nlPresets)) {
		p := nlPresets[name]
		x0, z0, f0 := p.camera(0)
		for _, sec := range []float64{0.5, 1.7, p.loop / 2, p.loop} {
			if x, z, f := p.camera(sec); x != x0 || z != z0 || f != f0 {
				t.Errorf("%s: camera at %.1fs is %v,%v ×%v, at 0s %v,%v ×%v", name, sec, x, z, f, x0, z0, f0)
			}
		}
	}
}

// The presented view is the same on the first visible tick and three
// seconds later: for a still scene, for a tracking scene, which frames the
// fight at the end of its lead-in and then holds, and for both halves of a
// mutator compare, which must also frame the same ground.
func TestNLPreviewFrameHoldsOnScreen(t *testing.T) {
	opts, cs := openNLTestContent(t)
	for _, key := range []nlSceneKey{
		{preset: "blast", gameplay: gameplay.Modern, w: 960, h: 540},
		{preset: "armor", gameplay: gameplay.Modern, w: 960, h: 540},
		{preset: "sight", gameplay: gameplay.Modern, w: 960, h: 540, mutators: "sight=2", paired: true},
	} {
		inst, err := buildNLPreview(opts, cs, key)
		if err != nil {
			t.Fatal(err)
		}
		// One display frame as nlPreview.Frame steps it: the twin first, on
		// the pair's focus.
		frame := func() {
			if twin := inst.twin; twin != nil {
				twin.focusX, twin.focusZ, twin.focusSeen = inst.focusX, inst.focusZ, inst.focusSeen
				twin.update(1.0/30, 1)
			}
			inst.update(1.0/30, 1)
		}
		frame()
		first := inst.b.cam.PresentationView()
		for i := 0; i < 90; i++ {
			frame()
		}
		if v := inst.b.cam.PresentationView(); v != first {
			t.Errorf("%s: the view moved from %+v to %+v", key.preset, first, v)
		}
		if twin := inst.twin; twin != nil {
			if v := twin.b.cam.PresentationView(); v != first {
				t.Errorf("%s: the compare's halves frame %+v and %+v", key.preset, first, v)
			}
		}
		if inst.preset.track && !inst.focusSeen {
			t.Errorf("%s: a tracking scene framed no fight", key.preset)
		}
		inst.close()
	}
}

// The build mutators compare build rates, so every constructor must be
// raising a structure by the end of the lead-in, from about where it was
// placed: one that walks off, or stands idle on a site another claimed,
// shows the compare nothing. A builder still steps to the edge of its site
// before it starts [04 R-ORD-01 §5], which the lead-in hides; the bound is
// one small footprint.
func TestNLSceneWorksiteBuildsWithoutWalking(t *testing.T) {
	st, tick := stageNLTestScene(t, "construct")
	s := st.s
	type start struct {
		u    *units.Unit
		x, z int32
	}
	var builders []start
	for _, u := range s.Units.Iter() {
		if u != nil && u.Alive && u.Owner == s.LocalOwner && u.Def != nil && u.Def.Builder && u.Def.BMCode != 0 && !u.Def.Commander {
			builders = append(builders, start{u, int32(u.X.Int()), int32(u.Z.Int())})
		}
	}
	if len(builders) != len(nlWorksite) {
		t.Fatalf("staged %d constructors, want %d", len(builders), len(nlWorksite))
	}
	for i := 0; i < nlPresets["construct"].scene.PreTicks; i++ {
		tick()
	}
	frames := 0
	for _, u := range s.Units.Iter() {
		if u != nil && u.Alive && u.Owner == s.LocalOwner && u.Remaining > 0 && u.Remaining < 1 {
			frames++
		}
	}
	if frames < len(builders) {
		t.Fatalf("%d structures rising after the lead-in, want one per constructor (%d)", frames, len(builders))
	}
	for _, b := range builders {
		dx, dz := int32(b.u.X.Int())-b.x, int32(b.u.Z.Int())-b.z
		if dx*dx+dz*dz > 32*32 {
			t.Errorf("constructor at %d,%d walked %d,%d", b.x, b.z, dx, dz)
		}
	}
}

// The build and salvage mutators' constructors, sites and wrecks stand in
// the open: no tree or other destructible feature lies over one, nor in the
// cells south of it whose sprites the oblique view draws up over it. The
// salvage crews are unhurt, so none carries a health bar.
func TestNLSceneWorksiteStandsInTheOpen(t *testing.T) {
	for _, name := range []string{"construct", "salvage"} {
		st, tick := stageNLTestScene(t, name)
		s := st.s
		for i := 0; i < nlPresets[name].scene.PreTicks; i++ {
			tick()
		}
		type spot struct {
			what string
			x, z int32
		}
		var spots []spot
		for _, u := range s.Units.Iter() {
			if u == nil || !u.Alive || u.Def == nil || u.Def.Commander {
				continue
			}
			x, z := int32(u.X.Int()), int32(u.Z.Int())
			if wsAbs(x-st.cx) > 200 || wsAbs(z-st.cz) > 260 {
				continue // storage and sentinels, out of frame
			}
			spots = append(spots, spot{u.Def.UnitName, x, z})
			if u.Owner == s.LocalOwner && u.Def.Builder && u.Def.BMCode != 0 && u.Health < u.MaxHealth {
				t.Errorf("%s: a constructor at %d,%d is hurt, so it carries a health bar", name, x, z)
			}
		}
		for _, f := range s.Features.Instances() {
			if f == nil || f.Def == nil || !f.Def.Reclaimable || f.Def.Metal <= 0 || f.Def.Indestructible {
				continue
			}
			if x, z := int32(f.CX)*16+16, int32(f.CZ)*16+16; wsAbs(x-st.cx) <= 200 && wsAbs(z-st.cz) <= 260 {
				spots = append(spots, spot{"wreck", x, z})
			}
		}
		if len(spots) < 6 {
			t.Fatalf("%s: %d units and wrecks in frame", name, len(spots))
		}
		for _, f := range s.Features.Instances() {
			if f == nil || f.Def == nil || f.Def.Indestructible || f.Def.Reclaimable && f.Def.Metal > 0 {
				continue
			}
			fx, fz := int32(f.CX)*16+8, int32(f.CZ)*16+8
			for _, p := range spots {
				if fx > p.x-48 && fx < p.x+48 && fz > p.z-48 && fz < p.z+112 {
					t.Errorf("%s: %s at %d,%d covers the %s at %d,%d", name, f.Def.Description, fx, fz, p.what, p.x, p.z)
				}
			}
		}
	}
}

// The blast-size compare and the Heat card's blast rings need every shell to
// read: the first lands under a second and a half after the scene appears
// and then one every reload, each on the middle of the block — the middle
// tank takes the most damage — and splashing others; no tank dies of it, and
// the block is back at full health before the next. Under every rule set,
// since the draft's rules stage the scene.
func TestNLSceneBlastShellsTheMiddleTank(t *testing.T) {
	for _, rules := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		st, tick := stageNLTestSceneUnder(t, "blast", rules)
		s := st.s
		var block []*units.Unit
		var mid *units.Unit
		for _, u := range s.Units.Iter() {
			if u != nil && u.Alive && u.Owner == s.EnemyOwner && u.Def != nil && strings.EqualFold(u.Def.UnitName, nlBlastTank) {
				block = append(block, u)
				if int32(u.X.Int()) == st.cx && int32(u.Z.Int()) == st.cz {
					mid = u
				}
			}
		}
		if len(block) != 15 || mid == nil {
			t.Fatalf("%s: staged %d tanks (middle found %v), want 15 about the anchor", rules, len(block), mid != nil)
		}
		pre := nlPresets["blast"].scene.PreTicks
		var shown []int
		pending := false // a shell's damage still shows
		before := make([]int32, len(block))
		for i := 1; i <= pre+30*10; i++ {
			for k, u := range block {
				before[k] = u.Health
			}
			tick()
			hurt, worst, worstDrop := 0, -1, int32(0)
			for k, u := range block {
				if !u.Alive {
					t.Fatalf("%s: a tank died at tick %d", rules, i)
				}
				if drop := before[k] - u.Health; drop > 0 {
					hurt++
					if drop > worstDrop {
						worst, worstDrop = k, drop
					}
				}
			}
			if hurt == 0 {
				if pending {
					pending = slices.ContainsFunc(block, func(u *units.Unit) bool { return u.Health < u.MaxHealth })
				}
				continue
			}
			if pending {
				t.Errorf("%s: a shell landed at tick %d before the block was back at full health", rules, i)
			}
			pending = true
			if i > pre {
				shown = append(shown, i-pre)
			}
			if block[worst] != mid {
				t.Errorf("%s: the shell at tick %d hurt the tank at %d,%d most, not the middle one", rules, i,
					int32(block[worst].X.Int())-st.cx, int32(block[worst].Z.Int())-st.cz)
			}
			if hurt < 3 {
				t.Errorf("%s: the shell at tick %d hurt %d tanks, want its splash to reach several", rules, i, hurt)
			}
		}
		if len(shown) < 3 {
			t.Fatalf("%s: %d shells landed in the first ten seconds on screen, want one every reload", rules, len(shown))
		}
		if shown[0] > 45 {
			t.Errorf("%s: the first shell landed %d ticks after the scene appeared", rules, shown[0])
		}
		for k := 2; k < len(shown); k++ {
			if a, b := shown[k-1]-shown[k-2], shown[k]-shown[k-1]; a-b > 3 || b-a > 3 {
				t.Errorf("%s: shells landed %v ticks into the scene, not on a steady rhythm", rules, shown)
				break
			}
		}
	}
}

// The fire scenes' copses stand far enough apart that a burn's spread
// cannot jump from one to another, so each burns when its scene lights it:
// every shape lies within nlCopseReach of its middle, and no two middles are
// closer than nlCopseSpacing.
func TestNLSceneCopsesStandApart(t *testing.T) {
	for _, shape := range nlCopseTrees {
		for _, c := range shape {
			if wsAbs(c[0]) > nlCopseReach || wsAbs(c[1]) > nlCopseReach {
				t.Errorf("a copse tree at %v lies past the reach of %d cells", c, nlCopseReach)
			}
		}
	}
	for _, scene := range []struct {
		name   string
		copses [][2]int32
	}{{"fireshimmer", nlFireCopses}, {"lighting", nlLightingCopses}} {
		name, copses := scene.name, scene.copses
		for i := range copses {
			for j := i + 1; j < len(copses); j++ {
				a, b := copses[i], copses[j]
				if d := max(wsAbs(a[0]-b[0]), wsAbs(a[1]-b[1])); d < nlCopseSpacing {
					t.Errorf("%s: copses at %v and %v stand %d apart, want %d", name, a, b, d, nlCopseSpacing)
				}
			}
		}
	}
}

// Fire shimmer needs trees burning in frame for the whole loop, and the
// lighting scene's copses have to burn beside its fight the same way; the
// fire-shimmer frame holds no unit. The loop is checked two seconds past its
// end, which covers the restaged copy's staging and fade.
func TestNLSceneFiresBurnThroughTheLoop(t *testing.T) {
	for _, name := range []string{"fireshimmer", "lighting"} {
		st, tick := stageNLTestScene(t, name)
		s := st.s
		p := nlPresets[name]
		dx, dz, zoom := p.camera(0)
		// The part of the frame right of the hero text and above the cards
		// at a 1920x1080 preview; nlPreviewInstance.applyCamera puts the aim
		// point 66% across and 42% down.
		ax, az := float64(st.cx)+dx, float64(st.cz)+dz
		x0, x1 := int32(ax-0.2*1920/zoom), int32(ax+0.34*1920/zoom)
		z0, z1 := int32(az-0.32*1080/zoom), int32(az+0.3*1080/zoom)
		for i := 1; i <= p.scene.PreTicks+int(p.loop*30)+60; i++ {
			tick()
			if i < p.scene.PreTicks || (i-p.scene.PreTicks)%30 != 0 {
				continue
			}
			burning := 0
			for _, f := range s.Features.Instances() {
				x, z := int32(f.CX)*16, int32(f.CZ)*16
				if f.IsBurning && x >= x0 && x < x1 && z >= z0 && z < z1 {
					burning++
				}
			}
			if burning < 3 {
				t.Errorf("%s: %d trees burning in frame %d seconds into the scene", name, burning, (i-p.scene.PreTicks)/30)
			}
			if name != "fireshimmer" {
				continue
			}
			for _, u := range s.Units.Iter() {
				if u == nil || !u.Alive {
					continue
				}
				if x, z := int32(u.X.Int()), int32(u.Z.Int()); x >= x0-100 && x < x1+100 && z >= z0-100 && z < z1+100 {
					t.Errorf("%s: %s in frame at %d,%d", name, u.Def.UnitName, x, z)
				}
			}
		}
	}
}
