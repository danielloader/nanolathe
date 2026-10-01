package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/audio"
	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The first own-side reclaim pulse can request a warning before stamping its
// provenance, but must cross publication for the viewport gate. The next pulse
// cannot request it [06 R-WPN-04 §2][07 R-HUD-03 §14.1].
func TestOwnReclaimNoticePublishesBeforeProvenanceInEveryMode(t *testing.T) {
	for _, mode := range []gameplay.Mode{gameplay.Strict31, gameplay.Modern} {
		t.Run(string(mode), func(t *testing.T) {
			s, _ := statusCueFixture(0, true, false)
			s.Units = units.NewSliced(2, nil)
			def := *strictMinimalCatalog().Units["armcom"] // authored fixture COB
			def.UnitName, def.MaxDamage, def.CanCapture = "fixture", 100, false
			victim, err := s.Units.Create(&def, 0, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			builder, err := s.Units.Create(&def, 0, 0, 0, 0)
			if err != nil {
				t.Fatal(err)
			}
			u := s.Units.Unit(victim)
			u.Flags |= 0x10 // selection is no longer an admission predicate
			s.Combat = &combat.Service{}
			s.SetGameplay(mode)
			s.SeedSessionRNG(17, 19)
			s.InitAudio(nil)
			s.bindDamageReaction()
			simBefore, crtBefore := *s.SimRNG(), *s.CrtRNG()
			for _, tick := range []uint32{41, 43} {
				s.Clock.GlobalTick = tick
				s.Combat.AcceptDamage(s.Units, tick, combat.DamageInput{Victim: victim, Attacker: builder, Nominal: 1, Kind: uint8(combat.CauseReclaim)})
			}
			got := statusEvents(s, uint8(audio.SlotUnderAttack))
			if len(got) != 1 || got[0].Source != victim || got[0].Tick != 41 || got[0].StatusText != "Under Attack" {
				t.Fatalf("reclaim warning events = %+v, want first pulse only", got)
			}
			if u.Health != 98 || u.LastDamageSide != u.Owner || u.LastDamageCause != uint8(combat.CauseReclaim) {
				t.Fatalf("health/provenance = %d/%d/%d", u.Health, u.LastDamageSide, u.LastDamageCause)
			}
			if s.Audio.Queue.Count != 0 || *s.SimRNG() != simBefore || *s.CrtRNG() != crtBefore {
				t.Fatal("notice bypassed publication or drew an authoritative RNG")
			}
		})
	}
}
