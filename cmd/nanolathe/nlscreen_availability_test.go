package main

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/platform/screenkit"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func configurationCard(t *testing.T, s *nlScreen, key string) nlCard {
	t.Helper()
	for _, page := range s.pages() {
		for _, c := range page.cards {
			if c.key == key {
				return c
			}
		}
	}
	t.Fatalf("missing configuration card %s", key)
	return nlCard{}
}

func TestNLUnavailableSettingsRejectEditsAndRetainPreferences(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Community39, gameplay.Modern} {
		for _, renderer := range []string{"classic", "modern"} {
			t.Run(string(mode)+"/"+renderer, func(t *testing.T) {
				file := settings.Defaults()
				file.Gameplay, file.Presentation.Renderer = mode, renderer
				_, s := settingsRegressionScreen(nil, file)
				for _, page := range s.pages() {
					for _, c := range page.cards {
						if s.cardUnavailable(c) == "" {
							continue
						}
						before := s.draft
						if c.kind == nlGroup {
							for i, p := range c.parts {
								s.setPart(c, i, (p.get(&before)+1)%len(p.steps))
							}
						} else {
							s.setCard(c, (c.get(&before)+1)%len(c.steps))
						}
						s.step(c, c.get(&s.draft), 1)
						if s.draft != before || s.touched[c.key] || s.compareAvailable(c) {
							t.Fatalf("unavailable %s still edits or compares", c.key)
						}
					}
				}
				if s.cardUnavailable(configurationCard(t, s, "selection")) != "" || s.cardUnavailable(configurationCard(t, s, "mut-damage")) != "" {
					t.Fatal("mode-independent input or mutators were disabled")
				}
			})
		}
	}
}

func TestNLEffectAmountsFollowTheirConsumers(t *testing.T) {
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	for _, tc := range []struct {
		card, part string
		off        func(*nlDraft)
	}{
		{"lighting", "groundLightStrength", func(d *nlDraft) { d.pres.GroundLight = 0 }},
		{"glow", "weaponGlowStrength", func(d *nlDraft) { d.glow = 0 }},
		{"glow", "explosionGlowStrength", func(d *nlDraft) { d.glowStrength = 0 }},
		{"heat", "blastRingStrength", func(d *nlDraft) { d.pres.BlastRings = 0 }},
		{"softshadows", "shadowSoftness", func(d *nlDraft) { d.pres.SoftShadows = 0 }},
	} {
		t.Run(tc.part, func(t *testing.T) {
			s.draft = s.draftOf(settings.Defaults())
			c := configurationCard(t, s, tc.card)
			for i, p := range c.parts {
				if p.key != tc.part {
					continue
				}
				if s.partUnavailable(c, p) != "" {
					t.Fatal("default effect amount unavailable")
				}
				tc.off(&s.draft)
				before := s.draft
				s.partSel[c.key] = i
				s.setPart(c, i, 0)
				s.step(c, c.get(&s.draft), 1)
				if s.partUnavailable(c, p) == "" || s.draft != before || s.compareAvailable(c) {
					t.Fatal("inactive amount still changes or compares")
				}
			}
		})
	}
	s.draft = s.draftOf(settings.Defaults())
	s.draft.glow = 0
	for _, p := range configurationCard(t, s, "glow").parts {
		if p.key == "nanoGlowStrength" && s.partUnavailable(configurationCard(t, s, "glow"), p) != "" {
			t.Fatal("nano amount lost its independent local-light consumer")
		}
	}
	file := settings.Defaults()
	file.Display.Shadows = 0
	_, s = settingsRegressionScreen(nil, file)
	if s.cardUnavailable(configurationCard(t, s, "softshadows")) == "" {
		t.Fatal("disabled unit shadows did not disable soft shadows")
	}
}

func TestNLGroupedEditsAndPreviewsKeepExactAmounts(t *testing.T) {
	file := settings.Defaults()
	file.Display.GlowStrength = 137
	file.Presentation.GroundLightStrength, file.Presentation.BlastRingStrength = 137, 0
	file.Presentation.TrailStrength = 39
	g, s := settingsRegressionScreen(nil, file)
	for _, page := range s.pages() {
		for _, card := range page.cards {
			// Grouped selectors pack multiple values into an integer; their
			// carousel caption still has only a single placeholder step.
			_ = nlCardValueText(card, &s.draft)
		}
	}
	for _, key := range []string{"lighting", "heat", "marks"} {
		c := configurationCard(t, s, key)
		s.setPart(c, 0, 1-c.parts[0].get(&s.draft))
	}
	s.apply()
	p := g.captureSettings()
	if p.Presentation.GroundLightStrength != 137 || p.Presentation.BlastRingStrength != 0 || p.Presentation.TrailStrength != 39 || p.Display.GlowStrength != 137 {
		t.Fatal("editing a sibling quantized an unrelated stored amount")
	}
	for _, key := range []string{"renderer", "lighting", "glow", "heat", "marks"} {
		c := configurationCard(t, s, key)
		_, r, _ := s.plan(c, c.get(&s.draft))
		if r.groundLightStrength != 137 || r.blastRingStrength != 0 || r.trailStrength != 39 || r.glowStrength != 137 {
			t.Fatalf("%s preview replaced exact strengths: %+v", key, r)
		}
	}
	file.Presentation.FPS, file.UnitLimit = 75, 320
	g, s = settingsRegressionScreen(nil, file)
	fps := configurationCard(t, s, "fps")
	if nlCardValueText(fps, &s.draft) != "75" {
		t.Fatal("custom frame cap is mislabeled")
	}
	_, r, _ := s.plan(fps, fps.get(&s.draft))
	if r.fps != 75 {
		t.Fatal("preview replaced custom frame cap")
	}
	s.setCard(fps, 1) // An explicit 60 must replace 75 even though both select the same notch.
	s.applyPresetToDraft(nlPresetEntry{name: "Custom limit", patch: json.RawMessage(`{"unitLimit":321}`)}, []bool{true, false, false})
	s.apply()
	if g.presentation.FPS != 60 || g.savedUnitLimit != 321 {
		t.Fatalf("explicit/preset values lost: fps %d, limit %d", g.presentation.FPS, g.savedUnitLimit)
	}
}

func TestNLDisabledZoomLockRejectsPointerAndDrag(t *testing.T) {
	oldW, oldH := nlScreenW, nlScreenH
	nlScreenW, nlScreenH = 1560, 900
	t.Cleanup(func() { nlScreenW, nlScreenH = oldW, oldH })
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	s.draft.gameplay = gameplay.Strict31
	s.fonts, s.art = screenkit.LoadFonts(), &nlArt{}
	c := configurationCard(t, s, "zoomlock")
	r := screenkit.Rect{X: 72, Y: 300, W: 700, H: 58}
	s.hits.Begin()
	s.drawZoomLockRow(ebiten.NewImage(1560, 900), c, r)
	s.hits.End()
	_, _, plus, _, track := nlZoomLockRects(r, 1)
	for _, hit := range []screenkit.Rect{plus, track} {
		x, y := hit.X+hit.W/2, hit.Y+hit.H/2
		s.hits.Update(screenkit.Input{X: x, Y: y, Pressed: true}, 0)
		s.hits.Update(screenkit.Input{X: hit.X + hit.W, Y: y, Down: true}, 0)
		s.hits.Update(screenkit.Input{X: x, Y: y, Released: true}, 0)
	}
	if s.draft.pres.ZoomLockPercent != 100 || s.touched["zoomlock"] {
		t.Fatal("disabled slider accepted a pointer edit")
	}
}

func TestNLSmallPositiveAmountComparesWithOff(t *testing.T) {
	file := settings.Defaults()
	file.Presentation.GroundLightStrength = 10
	_, s := settingsRegressionScreen(nil, file)
	c := configurationCard(t, s, "lighting")
	for i, p := range c.parts {
		if p.key == "groundLightStrength" {
			s.partSel[c.key] = i
		}
	}
	s.compare = true
	_, primary, alt := s.plan(c, c.get(&s.draft))
	if primary.groundLightStrength != 10 || alt == nil || alt.groundLightStrength != 0 {
		t.Fatalf("positive amount did not compare exactly against Off: %+v, %+v", primary, alt)
	}
}

func TestNLProfileShowsPendingMouseValuesBeforeEditing(t *testing.T) {
	g, s := settingsRegressionScreen(nil, settings.Defaults())
	s.chooseProfile(2)
	c := configurationCard(t, s, "dblclick")
	if c.get(&s.draft) != 1 || g.presentation.DoubleClickSelection != 0 {
		t.Fatal("pending profile did not preview its mouse setting independently of live state")
	}
	s.setCard(c, 0)
	s.apply()
	if g.presentation.DoubleClickSelection != 0 {
		t.Fatal("explicit Off was lost behind the pending profile")
	}
}

func TestNLProfileKeepsUnrelatedDraftChoices(t *testing.T) {
	file := settings.Defaults()
	file.Fullscreen = true
	g, s := settingsRegressionScreen(nil, file)
	mutators, err := content.ParseMutators(map[string]string{"damage": "2"})
	if err != nil {
		t.Fatal(err)
	}
	g.opts.Mutators, s.draft.mutators = mutators, mutators
	s.chooseProfile(2)
	if !s.draft.fullscreen || s.draft.mutators != mutators || s.touched["fullscreen"] || s.touched["mut-damage"] {
		t.Fatal("choosing a controls profile changed unrelated draft choices")
	}
	s.applyPresetToDraft(nlPresetEntry{name: "Water", patch: json.RawMessage(`{"presentation":{"waterSurface":0}}`)}, []bool{false, true, false})
	if s.touched["fullscreen"] || s.touched["mut-damage"] {
		t.Fatal("applying a preset marked unrelated draft choices as edited")
	}
}

func TestNLPendingContentHonorsItsSettingAndKeyLocks(t *testing.T) {
	for _, keys := range []bool{false, true} {
		_, s := settingsRegressionScreen(nil, settings.Defaults())
		mod := settingsRegressionMod()
		s.mods = []modlibrary.Mod{*mod}
		s.setCard(configurationCard(t, s, "content"), 1)
		before := s.draft
		if keys {
			s.clearKey("attack", 0)
		} else {
			c := configurationCard(t, s, "metal")
			s.setPart(c, 1, 1-s.draft.pres.Glint)
		}
		if s.dialog != "override" || s.draft.pres != before.pres || len(s.draft.keys.Keys("attack")) == 0 {
			t.Fatal("target content lock was bypassed before reload")
		}
		s.confirmOverride()
		if !s.draft.override {
			t.Fatal("target lock approval was lost")
		}
		if keys && len(s.draft.keys.Keys("attack")) != 0 {
			t.Fatal("approved keyboard edit was lost")
		}
	}
}

func TestNLPendingContentChecksEarlierEditsBeforeApply(t *testing.T) {
	_, s := settingsRegressionScreen(nil, settings.Defaults())
	s.clearKey("attack", 0)
	s.mods = []modlibrary.Mod{*settingsRegressionMod()}
	s.setCard(configurationCard(t, s, "content"), 1)
	s.apply()
	if s.dialog != "override" || s.reload != nil {
		t.Fatal("Apply bypassed the selected content's locks for an earlier edit")
	}
	s.confirmOverride()
	if s.reload == nil || !s.reload.draft.override || len(s.reload.draft.keys.Keys("attack")) != 0 {
		t.Fatal("approved earlier edit did not survive into the content reload")
	}
}

func TestNLLockGuardAcceptsAlreadyBoundPreferences(t *testing.T) {
	for _, tc := range []struct {
		name, patch string
		locks       []string
		minimum     string
		opts        Options
	}{
		{"canonical keys", `{"keyBindings":{"profile":"retail","bindings":{"attack":["Q"]}}}`, []string{"keyBindings"}, "", Options{}},
		{"minimum rules", `{}`, []string{"gameplay"}, string(gameplay.Community39), Options{}},
		{"command-line renderer", `{"presentation":{"renderer":"modern"}}`, []string{"presentation.renderer"}, "", Options{Renderer: "classic", RendererSet: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := settings.Defaults()
			file.Gameplay = gameplay.Strict31
			mod := &modlibrary.Mod{Metadata: modlibrary.Metadata{ID: "bound-settings", Name: "Bound settings", MinimumGameplay: tc.minimum,
				Config: &modlibrary.Config{Settings: json.RawMessage(tc.patch), Locks: tc.locks}}}
			g, s := settingsRegressionScreen(mod, file)
			g.opts = tc.opts
			g.applySettings(file)
			g.enforceModGameplayMinimum()
			s.draft = s.freshDraft(g)
			s.bindSource(g)
			if s.guardApplyLocks() || s.dialog != "" {
				t.Fatal("unchanged bound preferences asked for lock approval")
			}
		})
	}
}
