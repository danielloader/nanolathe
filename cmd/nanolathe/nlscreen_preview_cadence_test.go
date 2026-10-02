package main

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
)

// These lock the settings screen's background budget, independent of retail
// assets and gameplay (DESIGN_INTERFACE_HUD_INPUT §3.17). Image pointers below
// are retained-picture markers; no test composes or touches a graphics device.

func TestNLPreviewLoadingPausesCurrentAndTwin(t *testing.T) {
	primary := nlRender{fps: 30, effects: drawlist.AllEffects()}
	key := nlSceneKey{preset: "armor", paired: true}
	inst := &nlPreviewInstance{key: key, preset: nlPreset{loop: 60}, ticks: 17, acc: 0.25}
	inst.twin = &nlPreviewInstance{preset: nlPreset{loop: 60}, ticks: 17, acc: 0.25}
	inst.advance = func() { t.Fatal("advanced the old scene during loading") }
	inst.twin.advance = func() { t.Fatal("advanced the old twin during loading") }
	p := newNLPreview(Options{}, nil)
	p.cur, p.want, p.loading = inst, key, true
	// The outstanding load can be stale after the player revisits a cached
	// card. Its worker still competes for CPU, so that also pauses the preview.
	p.loadKey = nlSceneKey{preset: "water"}
	p.frame, p.alt = new(ebiten.Image), new(ebiten.Image)
	p.fadeLeft = 0.75
	p.lastDrawn = time.Now().Add(time.Hour)
	p.lastRender = nlRenderInputs(primary, &primary, true)
	lastDrawn, frame, alt := p.lastDrawn, p.frame, p.alt
	for range 180 {
		p.Frame(1.0/60, primary, &primary)
	}
	if inst.ticks != 17 || inst.twin.ticks != 17 || inst.acc != 0.25 || inst.twin.acc != 0.25 ||
		p.frame != frame || p.alt != alt || p.lastDrawn != lastDrawn || p.fadeLeft != 0.75 {
		t.Fatal("loading changed the paused scene clock or retained pictures")
	}

	// A completed stale load is still drained. The resumed pair receives only
	// this display frame's time, rather than the three seconds spent loading.
	stale := &nlPreviewInstance{key: p.loadKey, preset: nlPreset{loop: 60}}
	p.results <- nlPreviewResult{inst: stale}
	p.Frame(1.0/120, primary, &primary)
	if p.loading || len(p.cache) != 1 || p.cache[0].inst != stale {
		t.Fatal("paused preview did not retire the outstanding load handshake")
	}
	if inst.ticks != 17 || inst.twin.ticks != 17 || inst.acc != 0.5 || inst.twin.acc != 0.5 {
		t.Fatal("resuming caught up the loading interval")
	}
}

func TestNLPreviewRenderCadenceIncludesCompare(t *testing.T) {
	primary := nlRender{fps: 30, effects: drawlist.AllEffects()}
	now := time.Unix(1000, 0)
	for _, paired := range []bool{false, true} {
		sig := nlRenderInputs(primary, &primary, paired)
		p := &nlPreview{frame: new(ebiten.Image), alt: new(ebiten.Image), lastDrawn: now, lastRender: sig}
		if p.renderDue(now.Add(time.Second/60), sig, true) {
			t.Fatalf("compare refreshed between 30 FPS pictures, paired=%v", paired)
		}
		if !p.renderDue(now.Add(time.Second/30), sig, true) {
			t.Fatalf("compare missed its next picture, paired=%v", paired)
		}
		uncapped := primary
		uncapped.fps = 0
		sig = nlRenderInputs(uncapped, &uncapped, paired)
		p.lastRender = sig
		if !p.renderDue(now.Add(time.Microsecond), sig, false) {
			t.Fatalf("explicit Display cadence was throttled, paired=%v", paired)
		}
	}
}

func TestNLPreviewRenderEditsBypassCadence(t *testing.T) {
	primary := nlRender{fps: 30, effects: drawlist.AllEffects(), glow: true, glowStrength: 100, trailStrength: 100,
		groundLightStrength: 100, blastRingStrength: 100}
	baseline := nlRenderInputs(primary, &primary, false)
	now := time.Unix(1000, 0)
	p := &nlPreview{frame: new(ebiten.Image), alt: new(ebiten.Image), lastDrawn: now, lastRender: baseline}
	for _, edit := range []struct {
		name   string
		change func(primary, alt *nlRender)
	}{
		{"primary effect", func(r, a *nlRender) { r.effects.WaterMotion = false }},
		{"alternate effect", func(r, a *nlRender) { a.effects.Supersample = false }},
		{"source glow amount", func(r, a *nlRender) { a.effects.NanoGlowStrength = 50 }},
		{"shadow softness", func(r, a *nlRender) { r.effects.ShadowSoftness = 50 }},
		{"overall glow switch", func(r, a *nlRender) { r.glow = false }},
		{"overall glow amount", func(r, a *nlRender) { a.glowStrength = 50 }},
		{"trail amount", func(r, a *nlRender) { r.trailStrength = 0 }},
		{"ground light amount", func(r, a *nlRender) { a.groundLightStrength = 50 }},
		{"blast ring amount", func(r, a *nlRender) { r.blastRingStrength = 50 }},
		{"arrival", func(r, a *nlRender) { a.arrival = true }},
		{"placement ranges", func(r, a *nlRender) { r.placementRanges = true }},
		{"renderer", func(r, a *nlRender) { a.classic = true }},
		{"frame rate", func(r, a *nlRender) { r.fps = 60 }},
		{"zoom", func(r, a *nlRender) { a.zoom = 2 }},
	} {
		t.Run(edit.name, func(t *testing.T) {
			r, a := primary, primary
			edit.change(&r, &a)
			if !p.renderDue(now.Add(time.Millisecond), nlRenderInputs(r, &a, false), false) {
				t.Fatal("edited picture waited for the background cadence")
			}
		})
	}

	withoutCompare := nlRenderInputs(primary, nil, false)
	if !p.renderDue(now.Add(time.Millisecond), withoutCompare, false) {
		t.Fatal("turning Compare off waited for the background cadence")
	}
	p.lastRender = withoutCompare
	if !p.renderDue(now.Add(time.Millisecond), baseline, false) {
		t.Fatal("turning Compare on reused an old comparison picture")
	}
	p.lastRender, p.alt = baseline, nil
	if !p.renderDue(now.Add(time.Millisecond), baseline, false) {
		t.Fatal("missing comparison picture waited for the background cadence")
	}
	p.alt, p.presentNeeded = new(ebiten.Image), true
	if !p.renderDue(now.Add(time.Millisecond), baseline, false) {
		t.Fatal("activated scene waited for the background cadence")
	}
}

func TestNLScreenPreviewFrameRateCardUsesDraftBudget(t *testing.T) {
	s := newNLScreen(nil)
	s.canvasScale = 1
	s.draft.pres.FPS = 144
	var frameRate nlCard
	for _, card := range s.graphicsCards() {
		if card.key == "fps" {
			frameRate = card
			continue
		}
		_, render, _ := s.plan(card, card.get(&s.draft))
		if render.fps != s.draft.pres.FPS {
			t.Fatalf("ordinary %s card has cadence %d, want the draft's %d", card.key, render.fps, s.draft.pres.FPS)
		}
	}
	if frameRate.key == "" {
		t.Fatal("missing Frame rate card")
	}
	for i, fps := range nlFPS {
		frameRate.set(&s.draft, i)
		_, render, _ := s.plan(frameRate, i)
		if render.fps != fps {
			t.Fatalf("Frame rate value %s has cadence %d, want %d", frameRate.steps[i], render.fps, fps)
		}
	}
}
