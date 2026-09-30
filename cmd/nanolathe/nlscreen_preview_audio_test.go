package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

type previewOutputSpy struct {
	config       audio.OutputConfig
	calls, plays int
}

func (s *previewOutputSpy) ConfigureOutput(c audio.OutputConfig)             { s.config = c; s.calls++ }
func (s *previewOutputSpy) PlaySample(*audio.Sample, float64, float64) error { s.plays++; return nil }

// A settings preview may stage on a worker while the battle remains active.
// Neither installation, visible stepping nor retirement may touch its output.
func TestNLPreviewIsSilentAndPreservesBattlePreferences(t *testing.T) {
	_, cs, cl := retailAssetShell(t)
	t.Cleanup(cl.Close)
	path := filepath.Join(t.TempDir(), "settings.json")
	t.Setenv(settings.EnvPath, path)
	stored := settings.Defaults()
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	old := audio.GlobalOutput()
	bars := client.DamageBars()
	t.Cleanup(func() {
		audio.SetGlobalOutput(old)
		client.SetDamageBars(bars)
		audio.ConfigureOutput(audio.OutputConfig{MasterEnabled: true, EffectsVolume: 1, SoundMode: audio.SoundModeMono, MixingBuffers: settings.DefaultMixingBuffers})
	})
	want := audio.OutputConfig{MasterEnabled: false, EffectsVolume: 0.37, SoundMode: audio.SoundMode3D, MixingBuffers: 3}
	audio.ConfigureOutput(want)
	spy := &previewOutputSpy{}
	audio.SetGlobalOutput(spy)
	initialCalls := spy.calls
	client.SetDamageBars(true)
	inst, err := buildNLPreviewScene(Options{}, cs, nlSceneKey{preset: "lighting", gameplay: gameplay.Modern, w: 640, h: 480}, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 90; i++ {
		inst.update(1.0/30, 0)
	}
	inst.close()
	if spy.calls != initialCalls || spy.config != want || spy.plays != 0 || !client.DamageBars() {
		t.Fatalf("preview changed shared presentation: configs=%d plays=%d bars=%v", spy.calls-initialCalls, spy.plays, client.DamageBars())
	}
}

func TestNLArrivalPreviewBindsPublishedCommander(t *testing.T) {
	opts, cs := openNLTestContent(t)
	inst, err := buildNLPreviewScene(opts, cs, nlSceneKey{preset: "arrival", gameplay: gameplay.Modern, w: 640, h: 480}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer inst.close()
	if inst.arrivalUnit == nil {
		t.Fatal("arrival preview has no published commander")
	}
	before := inst.sess.Clock.GlobalTick
	for i := 0; i < 10; i++ {
		inst.update(1.0/30, 0)
	}
	if inst.sess.Clock.GlobalTick <= before {
		t.Fatal("arrival preview held its simulation")
	}
}
