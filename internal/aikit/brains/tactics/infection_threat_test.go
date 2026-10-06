package tactics

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
)

func TestInfectorLocalFocusPreference(t *testing.T) {
	for _, tc := range []string{"active", "disabled", "unseen", "distant", "defended", "air", "cooldown", "retained infector", "tie"} {
		t.Run(tc, func(t *testing.T) {
			a, m := testArmy(40, 40)
			a.static = aikit.NewGrid(m)
			info := &aikit.UnitInfo{Role: aikit.RoleMobile | aikit.RoleCombat, DPS: 50, HP: 1000, Value: 200}
			enemy := []aikit.Contact{
				{H: 9, Gen: 1, Info: info, X: 1100, Z: 1000, HPPct: 100, Visible: true, Built: true},
				{H: 10, Gen: 1, Info: info, X: 1300, Z: 1000, HPPct: 100, Visible: true, Built: true, InfectionThreat: true},
			}
			s := &squad{rng: 300, cx: 1000, cz: 1000, focus: 9, focusG: 1, focusTick: 1, present: force{dps: 100}, ratio: 3000}
			b := &core.Board{K: &aikit.Kit{Persona: aikit.PersonaHard}, O: &aikit.Obs{Enemy: enemy}, Tick: 100}
			want := pool.Handle(10)
			switch tc {
			case "disabled":
				enemy[1].InfectionThreat = false
				want = 9
			case "unseen":
				enemy[1].Visible = false
				want = 9
			case "distant":
				enemy[1].X = 1451
				want = 9
			case "defended":
				a.static.V[(1000/aikit.SectorWorld)*a.static.W+1300/aikit.SectorWorld] = 100
				want = 9
			case "air":
				air := *info
				air.Role |= aikit.RoleAir
				enemy[1].Info = &air
				want = 9
			case "cooldown":
				s.focusTick = 99
				want = 9
			case "retained infector":
				s.focus = 10
				enemy[0].InfectionThreat = true
			case "tie":
				s.focus = 0
				enemy[0].InfectionThreat = true
				want = 9
			}
			a.focusFire(b, s)
			if s.focus != want {
				t.Fatalf("focus=%v want %v", s.focus, want)
			}
		})
	}
}
