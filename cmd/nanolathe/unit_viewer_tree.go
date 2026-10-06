package main

import (
	"slices"

	"github.com/nanolathe-gg/nanolathe/internal/construction"
	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// unitViewerLink is one build-tree entry. Modern marks a product that only
// the complete authored membership lists; the retail list omits it
// (DESIGN_ECONOMY_CONSTRUCTION "Modern authored build membership").
type unitViewerLink struct {
	Def    *content.UnitDef
	Modern bool
}

// unitViewerTree is built once per catalog load by the catalog worker. Its
// maps are lookups only; every list is already in library order. Hidden
// records are those the name lookup does not select.
type unitViewerTree struct {
	builds, builtBy map[*content.UnitDef][]unitViewerLink
	hidden          map[*content.UnitDef]bool
}

// unitViewerResolves reports whether the game's name lookup selects this very
// record. A record hidden by a duplicate name never builds or is built in a
// battle, because every product and builder name resolves to the other one
// [02 R-CAT-01 §§4–5].
func unitViewerResolves(cat *content.Catalog, def *content.UnitDef) bool {
	if cat == nil || def == nil {
		return false
	}
	found, ok := cat.Unit(def.UnitName)
	return ok && found == def
}

// unitViewerBuildTree derives both directions from the immutable build menus.
// Buttons is the retail list (CANBUILD plus downloads under the retail
// cutoff); AuthoredButtons is the complete membership the Modern rule offers
// [02 R-CAT-01 §8]. Builders are visited in the sorted entry order, never by
// ranging over the menu map, and products are resolved by the same lookup a
// build button uses. Only builder definitions can construct [04 R-ORD-02 §1].
func unitViewerBuildTree(cat *content.Catalog, entries []unitViewerEntry) unitViewerTree {
	tree := unitViewerTree{builds: map[*content.UnitDef][]unitViewerLink{}, builtBy: map[*content.UnitDef][]unitViewerLink{}, hidden: map[*content.UnitDef]bool{}}
	if cat == nil {
		return tree
	}
	rank := make(map[*content.UnitDef]int, len(entries))
	for i, e := range entries {
		if _, ok := rank[e.Def]; !ok {
			rank[e.Def] = i
		}
	}
	resolve := func(names []string) []*content.UnitDef {
		out := make([]*content.UnitDef, 0, len(names))
		for _, name := range names {
			if def, ok := cat.Unit(name); ok && def != nil {
				out = append(out, def)
			}
		}
		return out
	}
	for _, e := range entries {
		builder := e.Def
		if !unitViewerResolves(cat, builder) {
			tree.hidden[builder] = true
			continue
		}
		if !builder.Builder {
			continue
		}
		menu := cat.BuildMenus[content.CanonicalKey(builder.UnitName)]
		if menu == nil {
			continue
		}
		retail := resolve(menu.Buttons)
		authored := menu.AuthoredButtons
		if authored == nil {
			authored = menu.Buttons
		}
		var links []unitViewerLink
		for _, def := range append(slices.Clone(retail), resolve(authored)...) {
			if !slices.ContainsFunc(links, func(l unitViewerLink) bool { return l.Def == def }) {
				links = append(links, unitViewerLink{Def: def, Modern: !slices.Contains(retail, def)})
			}
		}
		slices.SortStableFunc(links, func(a, b unitViewerLink) int { return rank[a.Def] - rank[b.Def] })
		if len(links) == 0 {
			continue
		}
		tree.builds[builder] = links
		for _, l := range links {
			tree.builtBy[l.Def] = append(tree.builtBy[l.Def], unitViewerLink{Def: builder, Modern: l.Modern})
		}
	}
	return tree
}

// unitViewerWorkLimit bounds the build-time estimate to one day of game time
// at normal speed. It is a viewer bound, not a game rule.
const unitViewerWorkLimit = 24 * 60 * 60 * unitViewerTickRate

type unitViewerWorkKey struct{ quantum, buildTime int32 }

// workTicks memoizes construction.WorkTicks; the result depends only on the
// worker quantum and the product's buildtime.
func (s *toolsScreen) workTicks(builder, product *content.UnitDef) (int, bool) {
	key := unitViewerWorkKey{construction.WorkerQuantum(builder.WorkerTime), product.BuildTime}
	if r, ok := s.workCache[key]; ok {
		return r.ticks, r.ok
	}
	ticks, ok := construction.WorkTicks(builder.WorkerTime, product.BuildTime, unitViewerWorkLimit)
	if s.workCache == nil {
		s.workCache = map[unitViewerWorkKey]unitViewerWork{}
	}
	s.workCache[key] = unitViewerWork{ticks, ok}
	return ticks, ok
}

type unitViewerWork struct {
	ticks int
	ok    bool
}

// unitViewerHistoryLimit keeps a long browsing session bounded.
const unitViewerHistoryLimit = 64

// visit is a user selection: the unit it replaces becomes Back's target.
func (s *toolsScreen) visit(def *content.UnitDef) {
	if def == s.selected {
		return
	}
	if s.selected != nil && def != nil {
		s.histBack = append(s.histBack, s.selected)
		if len(s.histBack) > unitViewerHistoryLimit {
			s.histBack = slices.Delete(s.histBack, 0, len(s.histBack)-unitViewerHistoryLimit)
		}
		s.histForward = s.histForward[:0]
	}
	s.selectUnit(def)
}

// navigate follows a build-tree link and keeps the target visible in the
// library, clearing a search that excludes it.
func (s *toolsScreen) navigate(def *content.UnitDef) {
	if def == nil {
		return
	}
	s.visit(def)
	s.showInLibrary()
}

func (s *toolsScreen) goBack()    { s.step(&s.histBack, &s.histForward) }
func (s *toolsScreen) goForward() { s.step(&s.histForward, &s.histBack) }

func (s *toolsScreen) step(from, to *[]*content.UnitDef) {
	if len(*from) == 0 {
		return
	}
	def := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	if s.selected != nil {
		*to = append(*to, s.selected)
	}
	s.selectUnit(def)
	s.showInLibrary()
	s.refreshControls()
}

// showInLibrary reveals the selection, clearing the search when the query
// excludes it so the library always shows what the stage shows.
func (s *toolsScreen) showInLibrary() {
	if s.selected == nil {
		return
	}
	if s.selectionIndex() < 0 && s.query != "" {
		s.query = ""
		if s.panel != nil && s.viewer {
			s.panel.SetText("SEARCH", "")
		}
		s.filtered = filterUnitViewerEntries(s.entries, "")
		s.refreshLists()
	}
	s.revealSelection()
}
