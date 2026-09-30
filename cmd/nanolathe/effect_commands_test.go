package main

import (
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/drawlist"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

// The five Enhanced effect family shortcuts are Nanolathe commands with no
// retail counterpart (DESIGN_GPU_RENDERER §30). A family is On while any of its
// switches is, and the command writes every one of them. A direct battle owns
// the live values and its write-all captures them; the windowed shell owns the
// preference the host polls, so it writes there and saves the whole block.
func TestEffectChatCommandsToggleAndPersist(t *testing.T) {
	t.Setenv(settings.EnvPath, filepath.Join(t.TempDir(), "settings.json"))
	cl, err := client.New(client.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cl.SetEffects(presentationEffects(settings.DefaultPresentation()))
	if cl.Effects() != drawlist.AllEffects() {
		t.Fatalf("defaults did not reach the client: %+v", cl.Effects())
	}
	// One part off leaves its family On, so the first press turns it off.
	part := drawlist.AllEffects()
	part.WaterFoam = false
	cl.SetEffects(part)

	direct := &battleSession{cl: cl}
	for _, command := range []string{"+Water", "+lights", "+FINISH", "+heat", "+marks"} {
		direct.dispatchLocalCommand(command + " ignored")
	}
	// Every family switch is off, and Marks' trail strength with them; the
	// glint, the soft shadows and the supersampling belong to no family and
	// keep their values.
	if want := (drawlist.Effects{HovercraftLandWash: true, Glint: true, SoftShadows: true, Supersample: true, WeaponGlowStrength: 100, ExplosionGlowStrength: 100, NanoGlowStrength: 100, ShadowSoftness: 100}); cl.Effects() != want || cl.TrailStrength() != 0 {
		t.Fatalf("direct toggles left %+v, trail strength %d", cl.Effects(), cl.TrailStrength())
	}
	stored, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := presentationEffects(stored.Presentation); got != cl.Effects() || stored.Presentation.TrailStrength != 0 {
		t.Fatalf("direct toggles stored %+v", stored.Presentation)
	}
	// Renderer and cap are not the commands' to write.
	if stored.Presentation.Renderer != settings.DefaultPresentation().Renderer {
		t.Fatalf("direct toggle rewrote the renderer: %+v", stored.Presentation)
	}
	// The write-all also captures the live strengths, which no shortcut moves.
	if stored.Presentation.GroundLightStrength != cl.GroundLightStrength() || stored.Presentation.BlastRingStrength != cl.BlastRingStrength() {
		t.Fatalf("direct write-all stored strengths %d/%d", stored.Presentation.GroundLightStrength, stored.Presentation.BlastRingStrength)
	}
	// Marks back on gives the zero trail strength its default.
	direct.dispatchLocalCommand("+marks")
	if !cl.Effects().Scorch || cl.TrailStrength() != settings.DefaultTrailStrength {
		t.Fatalf("marks on: %+v, trail strength %d", cl.Effects(), cl.TrailStrength())
	}

	shell := &gameShell{presentation: settings.DefaultPresentation(), settingsWritable: true}
	shell.presentation.FireShimmer = 0
	shellBattle := &battleSession{cl: cl, shell: shell}
	shellBattle.dispatchLocalCommand("+heat")
	p := shell.presentation
	if p.BlastRings != 0 || p.FireShimmer != 0 || p.WreckGlow != 0 || p.WreckShimmer != 0 || cl.Effects() != presentationEffects(p) {
		t.Fatalf("shell toggle off: live %+v shell %+v", cl.Effects(), p)
	}
	shellBattle.dispatchLocalCommand("+heat")
	p = shell.presentation
	if p.BlastRings != 1 || p.FireShimmer != 1 || p.WreckGlow != 1 || p.WreckShimmer != 1 || cl.Effects() != drawlist.AllEffects() {
		t.Fatalf("shell toggle on: live %+v shell %+v", cl.Effects(), p)
	}
	stored, err = settings.Load()
	if err != nil || stored.Presentation != shell.presentation {
		t.Fatalf("shell toggle stored %+v err=%v", stored.Presentation, err)
	}
}
