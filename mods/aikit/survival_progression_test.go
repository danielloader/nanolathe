package aikit

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/aikit"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/brains/utility"
	"github.com/nanolathe-gg/nanolathe/internal/aikit/core"
)

// Survival changes the existing utility weights only. Configured choices,
// including delaying or disabling tech, are authoritative key by key.
func TestSurvivalProgressionDefaultsAndOverrides(t *testing.T) {
	if err := ValidateParams(survivalDefaults); err != nil {
		t.Fatal(err)
	}
	before := maps.Clone(survivalDefaults)
	configured := map[string]string{"tech_time": "20", "tech": "0", "w_cons": "80", "w_assist": "30", "v_ref": "100", "open_army": "0"}
	kv := survivalParams(configured)
	p, err := utility.ParseParams(UtilityParams(kv))
	if err != nil {
		t.Fatal(err)
	}
	if p.TechTime != 20 || p.Tech != 0 || p.WCons != 80 || p.WAssist != 30 || p.VRef != 100 {
		t.Fatalf("configured progression was overridden: %+v", p)
	}
	v, err := utility.VarietyFrom(kv)
	if err != nil || v.OpenArmy != 0 {
		t.Fatalf("configured opening was overridden: %+v %v", v, err)
	}
	if !maps.Equal(survivalDefaults, before) || !maps.Equal(configured, map[string]string{"tech_time": "20", "tech": "0", "w_cons": "80", "w_assist": "30", "v_ref": "100", "open_army": "0"}) {
		t.Fatal("constructing a survival brain modified its input parameters")
	}
}

// Authored menus, richer extraction and ordinary resource gates determine
// tech readiness. The earlier Survival timeline cannot turn a basic menu
// into advanced tech or make a low-income economy ready.
func TestSurvivalTechIsEarlierAndResourceGated(t *testing.T) {
	early := progressionSnapshot(t, nil, 28, true, false)
	later := progressionSnapshot(t, map[string]string{"tech_time": "12"}, 28, true, false)
	poor := progressionSnapshot(t, nil, 5, true, false)
	if !(early.want > later.want && early.want > poor.want && poor.want == 0) {
		t.Fatalf("tech want: survival %d, configured later %d, poor %d", early.want, later.want, poor.want)
	}
	if !early.advanced || progressionSnapshot(t, nil, 28, false, false).advanced {
		t.Fatal("advanced factory classification did not follow its authored constructor menu")
	}
	if got := progressionSnapshot(t, map[string]string{"tech": "0"}, 28, true, false); got.want != -1 {
		t.Fatal("configured tech=0 did not disable the timed transition")
	}
	ready := progressionSnapshot(t, nil, 28, true, true)
	if !ready.constructor {
		t.Fatal("an authored advanced factory did not prefer its first constructor with a supported economy")
	}
	stalled := progressionSnapshot(t, nil, 28, true, true, true)
	if stalled.constructor {
		t.Fatal("energy-stalled production bypassed the constructor resource gate")
	}
}

type progressionResult struct {
	want        int
	advanced    bool
	constructor bool
}

// The fixture names are deliberately unrelated to any retail faction:
// the advanced constructor is identified by the extractor in its menu.
func progressionSnapshot(t *testing.T, configured map[string]string, income int32, richer, haveFactory bool, energyStall ...bool) progressionResult {
	t.Helper()
	kv := survivalParams(configured)
	for key, value := range map[string]string{"air": "0", "naval": "0", "layout": "0", "jitter": "0", "reach_mix": "0"} {
		kv[key] = value
	}
	ut, err := NewUtilTac(kv)
	if err != nil {
		t.Fatal(err)
	}
	tab := &aikit.Table{}
	unit := func(key string, role aikit.Role, metal, energy int32) *aikit.UnitInfo {
		u := &aikit.UnitInfo{Index: int32(len(tab.Units)), Key: key, Side: "custom", Role: role, Metal: metal, Energy: energy,
			Value: metal + energy/aikit.EnergyPerMetal, HP: 1000, FootX: 2, FootZ: 2, BuildTime: 3000, Speed: 30, BuildPower: 100}
		tab.Units = append(tab.Units, u)
		return u
	}
	com := unit("leader", aikit.RoleCommander|aikit.RoleBuilder|aikit.RoleMobile, 1000, 1000)
	basicMex := unit("shallow-drill", aikit.RoleExtractor, 50, 100)
	basicMex.MetalMake = 2
	advMex := unit("deep-drill", aikit.RoleExtractor, 300, 1000)
	advMex.MetalMake = 2
	if richer {
		advMex.MetalMake = 4
	}
	energy := unit("generator", aikit.RoleEnergy, 100, 0)
	energy.EnergyMake = 20
	con := unit("worker", aikit.RoleBuilder|aikit.RoleMobile, 100, 1000)
	advCon := unit("specialist", aikit.RoleBuilder|aikit.RoleMobile, 300, 2000)
	advCon.Builds = []*aikit.UnitInfo{advMex, energy}
	combat := unit("defender", aikit.RoleCombat|aikit.RoleMobile, 100, 1000)
	combat.DPS, combat.Value = 100, 10000
	factory := unit("workshop", aikit.RoleFactory, 600, 1000)
	factory.Builds = []*aikit.UnitInfo{con, combat}
	advanced := unit("deep-workshop", aikit.RoleFactory, 1800, 8000)
	advanced.Depth = 3
	advanced.Builds = []*aikit.UnitInfo{advCon, combat}
	con.Builds = []*aikit.UnitInfo{basicMex, energy, factory, advanced}
	com.Builds = []*aikit.UnitInfo{basicMex, energy, factory}
	m := &aikit.MapInfo{CellW: 256, CellH: 256, WorldW: 4096, WorldH: 4096, SectorW: 32, SectorH: 32, HomeX: 512, HomeZ: 512,
		Starts: [][2]int32{{512, 512}, {3584, 3584}}}
	rnd := aikit.PlayerRand(1, 0)
	k := &aikit.Kit{Side: "custom", Table: tab, Map: m, Persona: aikit.PersonaHard, Rand: &rnd, Budget: -1}
	b := &core.Board{K: k}
	ut.Strategy.Init(b)
	ut.Brain.Economy.Init(b)
	ut.Brain.Prod.Init(b)
	o := &aikit.Obs{Tick: 8 * 1800, UnitLimit: 250,
		Metal:  aikit.Res{Stock: 400, Cap: 1000, Income: income, Expense: 10},
		Energy: aikit.Res{Stock: 2000, Cap: 4000, Income: 400, Expense: 50},
		Own: []aikit.OwnUnit{{H: 1, Gen: 1, Info: com, X: 512, Z: 512, Built: true}, {H: 2, Gen: 1, Info: con, X: 600, Z: 512, Built: true},
			{H: 3, Gen: 1, Info: combat, X: 700, Z: 512, Built: true}}}
	if haveFactory {
		o.Own = append(o.Own, aikit.OwnUnit{H: 4, Gen: 1, Info: advanced, X: 650, Z: 650, Built: true})
	}
	if len(energyStall) > 0 && energyStall[0] {
		o.Energy = aikit.Res{Stock: 0, Cap: 4000, Income: 1, Expense: 400}
	}
	b.Update(k, o)
	ut.Strategy.Plan(b)
	ut.Brain.Prod.Plan(b)
	x := &aikit.Explain{}
	ut.Strategy.Explain(b, x)
	ut.Brain.Prod.(core.Explaining).Explain(b, x)
	out := progressionResult{want: -1}
	for _, note := range x.Notes {
		if strings.HasPrefix(note, "tech: want ") {
			if _, err := fmt.Sscanf(note, "tech: want %d", &out.want); err != nil {
				t.Fatal(err)
			}
		}
		if strings.Contains(note, "factory option deep-workshop:") && strings.Contains(note, "t2 true") {
			out.advanced = true
		}
	}
	for _, g := range x.Goals {
		if g.Chosen && strings.Contains(g.Label, "constructor specialist") {
			out.constructor = true
		}
	}
	return out
}
