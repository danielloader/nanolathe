package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func TestCheckpointAIOriginalManager(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Session)
	}{
		{"copy", func(s *Session) { m := *s.AI[0]; s.AI[0] = &m }},
		{"player", func(s *Session) { s.AI[0].Player = 1 }},
		{"ordinary queue", func(s *Session) { s.AI[0].SetQueueBuildTyped(s.AI[0].QueueBuildTypedHook()) }},
		{"foreign survival", func(s *Session) { s.AI[0].Survival = &ai.SurvivalInfo{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, inputs := checkpointSessionScriptsFixture(t, true)
			ota, err := formats.LoadOTA([]byte("[GlobalHeader]{SurfaceMetal=0;}"))
			if err != nil {
				t.Fatal(err)
			}
			s.Mission.OTA = ota
			if err := initializeBattleAI(s, 0, &ai.Profile{}, sessionKindSkirmish); err != nil {
				t.Fatal(err)
			}
			uc := checkpointSessionScriptContext(t, s, inputs)
			wc := world.NewCheckpointContext(uc.Keys)
			wc.Terrain = s.World
			c := ai.NewCheckpointContext(uc, wc)
			if err := s.prepareCheckpointAI(c, 0); err != nil {
				t.Fatal(err)
			}
			tc.change(s)
			if err := s.prepareCheckpointAI(c, 0); err == nil {
				t.Fatal("changed manager retained admission")
			}
		})
	}
}
