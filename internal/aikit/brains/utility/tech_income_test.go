package utility

import (
	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
	"testing"
)

// Survival can invest wave rewards at a lower recurring income. The default
// income ramp is retained, and changing the preference does not change stocks.
func TestTechIncomeScalesPreferenceAndPreservesSafety(t *testing.T) {
	kit := &aikit.Kit{Map: &aikit.MapInfo{}, Persona: aikit.PersonaHard}
	b := &core.Board{K: kit, O: &aikit.Obs{Metal: aikit.Res{Stock: 250, Cap: 1000}, Energy: aikit.Res{Stock: 600, Cap: 1000}}}
	p := DefaultParams()
	p.TechTime = 8
	p.Growth = 0
	want := func(pct int32, income, enemy int64) int64 {
		p.TechIncome = pct
		s := &shared{p: p, k: kit, tick: 8 * 1800, mInc: income, armyRatio: enemy}
		s.observeTech(b)
		return s.tech.want
	}
	if want(100, 10000, 500) != 0 || want(60, 10000, 500) <= 0 || want(60, 5000, 500) != 0 {
		t.Fatal("scaled income ramp lost its thresholds")
	}
	if want(60, 20000, 1600) != 0 {
		t.Fatal("tech preference bypassed losing-army safety")
	}
	if want(0, 20000, 500) != want(100, 20000, 500) {
		t.Fatal("legacy parameter fixture changed default ramp")
	}
	if b.O.Metal.Stock != 250 || b.O.Energy.Stock != 600 {
		t.Fatal("investment preference changed resources")
	}
}
