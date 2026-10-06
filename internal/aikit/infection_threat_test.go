package aikit

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestInfectionThreatObservationUsesLivePolicyAndSight(t *testing.T) {
	d := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "infector"}, UnitName: "infector", MaxDamage: 100, BMCode: 1, CanMove: true, Category: "NANOLATHE_INFECTOR", NanolatheInfector: true}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"infector": d}}
	w := fixtureWorld(cat)
	hd, err := w.Create(d, 1, 100<<16, 0, 100<<16)
	if err != nil {
		t.Fatal(err)
	}
	u := w.Unit(hd)
	u.Remaining = 0
	binding := &orders.QueueBinding{Rules: &orders.ModernRules{}}
	orders.BindQueueBinding(u, binding)
	visible := true
	m := &ai.Manager{Player: 0, Catalog: cat, UnitVisible: func(uint8, *units.Unit) bool { return visible }}
	h := NewHost(m, &countBrain{}, PersonaHard)
	defer h.Close()
	h.kit.Table = BuildTable(cat, nil)
	econ := computerEconomy()
	econ.Players[1].Exists = true
	for _, tc := range []string{"modern", "strict", "community", "unbound", "stunned", "unfinished", "ordinary host", "unseen", "omniscient unseen"} {
		t.Run(tc, func(t *testing.T) {
			binding.Rules = &orders.ModernRules{}
			d.Category, d.NanolatheInfector = "NANOLATHE_INFECTOR", true
			u.Stunned, u.Remaining, visible = false, 0, true
			h.persona.Omniscient = false
			switch tc {
			case "strict":
				binding.Rules = orders.StrictRules{}
			case "community":
				binding.Rules = orders.CommunityRules{}
			case "unbound":
				binding.Rules = nil
			case "stunned":
				u.Stunned = true
			case "unfinished":
				u.Remaining = .5
			case "ordinary host":
				d.Category, d.NanolatheInfector = "MOBILE", false
			case "unseen":
				visible = false
			case "omniscient unseen":
				visible = false
				h.persona.Omniscient = true
			}
			h.buildObs(1, w, econ)
			if tc == "unseen" {
				if len(h.obs.Enemy) != 0 {
					t.Fatal("hidden unit entered observation")
				}
				return
			}
			if len(h.obs.Enemy) != 1 || h.obs.Enemy[0].InfectionThreat != (tc == "modern") {
				t.Fatalf("unexpected contact %+v", h.obs.Enemy)
			}
		})
	}
}
