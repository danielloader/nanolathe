package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Validation compares extracts with the archive's own decoded views and
// checks that coordinates are plausible. It reads <root>/<source>/<id>.site.json
// where present (build_order and players views) and writes
// <out>/validation.json.

type siteCompare struct {
	Recordings      int `json:"recordings"`
	JoinedByDPID    int `json:"players_joined_by_dpid"`
	JoinedSideColor int `json:"players_joined_by_side_color"`
	JoinedOrder     int `json:"players_joined_by_order"`
	Unjoined        int `json:"players_unjoined"`
	SideMismatch    int `json:"side_mismatch"`
	CountEqual      int `json:"players_count_equal"`
	CountDiffer     int `json:"players_count_differ"`
	SiteEntries     int `json:"site_entries"`
	ExtractFinished int `json:"extract_finished_starts"`
	// TypeAgree sums, per player and unit type, the smaller of the two
	// counts: agreement of unit numbering independent of either clock.
	TypeAgree        int `json:"type_count_agree"`
	StartExact       int `json:"start_exact"`
	StartWithin1s    int `json:"start_within_1s"`
	SiteUnmatched    int `json:"site_unmatched"`
	ExtractUnmatched int `json:"extract_unmatched"`
	CompleteCompared int `json:"complete_compared"`
	CompleteExact    int `json:"complete_exact"`
	CompleteWithin1s int `json:"complete_within_1s"`
}

type plausibility struct {
	Recordings      int `json:"recordings"`
	Players         int `json:"combat_players"`
	EarlyStarts     int `json:"early_starts"`
	NearestOwnStart int `json:"early_nearest_own_start"`
	MaxX            int `json:"max_x"`
	MaxZ            int `json:"max_z"`
	MaxY            int `json:"max_y"`
	Over16384       int `json:"coords_over_16384"`
	Structures      int `json:"named_structures"`
	OnLattice       int `json:"structures_on_footprint_lattice"`
	HighStructures  int `json:"structures_with_high_words"`
	HighMobiles     int `json:"mobiles_with_high_words"`
	Mobiles         int `json:"named_mobiles"`
}

type recordingCompare struct {
	Source        string `json:"source"`
	ID            string `json:"id"`
	SiteEntries   int    `json:"site_entries"`
	Exact         int    `json:"exact"`
	Within1s      int    `json:"within_1s"`
	SiteUnmatched int    `json:"site_unmatched"`
	Unmatched     int    `json:"extract_unmatched"`
}

type validation struct {
	Note        string                   `json:"note"`
	Site        map[string]*siteCompare  `json:"site_build_order"`
	Plausible   map[string]*plausibility `json:"coordinates"`
	Worst       []recordingCompare       `json:"worst_recordings"`
	ClockRule   string                   `json:"site_clock_rule"`
	NamingNotes string                   `json:"naming"`
}

type siteViews struct {
	Players []struct {
		Side  int     `json:"side"`
		Color int     `json:"color"`
		DPID  *uint32 `json:"dpid"`
	} `json:"players"`
	BuildOrder [][]struct {
		Type      int   `json:"unit_category_id"`
		Started   int64 `json:"started_ms"`
		Completed int64 `json:"completed_ms"`
	} `json:"build_order"`
}

type footprint struct {
	fx, fz int
	mobile bool
}

func loadFootprints(root string) map[string]footprint {
	var rows []struct {
		Unitname string  `json:"unitname"`
		FootX    int     `json:"foot_x"`
		FootZ    int     `json:"foot_z"`
		Speed    float64 `json:"speed"`
	}
	fp := map[string]footprint{}
	if !readJSON(filepath.Join(root, stockTablePath), &rows) {
		return fp
	}
	for _, r := range rows {
		fp[strings.ToUpper(r.Unitname)] = footprint{r.FootX, r.FootZ, r.Speed > 0}
	}
	return fp
}

// siteSecond is the archive's start clock: the sync tick at the nominal 30
// ticks per second, floored to a whole second, in milliseconds.
func siteSecond(tick int64) int64 {
	if tick < 0 {
		return -1
	}
	return tick / 30 * 1000
}

func runValidate(root, out string, sources []string) error {
	demos, err := listDemos(root, sources)
	if err != nil {
		return err
	}
	fp := loadFootprints(root)
	v := validation{
		Note: "Starts are compared by unit type and whole second; an extract start matches a site entry " +
			"when its type equals unit_category_id and its site clock equals started_ms (exact) or differs by one second.",
		ClockRule: "started_ms = floor(tick/30)*1000, tick = last unit-sync tick of the sender before the start",
		Site:      map[string]*siteCompare{}, Plausible: map[string]*plausibility{},
	}
	var rows []recordingCompare
	for _, d := range demos {
		ex, err := readExtract(extractPath(out, d))
		if err != nil || ex.Setup == nil {
			continue
		}
		pl := v.Plausible[d.source]
		if pl == nil {
			pl = &plausibility{}
			v.Plausible[d.source] = pl
		}
		checkPlausible(ex, pl, fp)

		var sv siteViews
		if !strings.HasPrefix(d.source, "v") || !readJSON(filepath.Join(root, d.source, d.id+".site.json"), &sv) ||
			len(sv.BuildOrder) == 0 || len(sv.Players) == 0 {
			continue
		}
		sc := v.Site[d.source]
		if sc == nil {
			sc = &siteCompare{}
			v.Site[d.source] = sc
		}
		row := recordingCompare{Source: d.source, ID: d.id}
		compareSite(ex, &sv, sc, &row)
		if row.SiteEntries > 0 {
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a := rows[i].SiteUnmatched + rows[i].Unmatched
		b := rows[j].SiteUnmatched + rows[j].Unmatched
		if a != b {
			return a > b
		}
		return rows[i].ID < rows[j].ID
	})
	if len(rows) > 25 {
		rows = rows[:25]
	}
	v.Worst = rows
	v.NamingNotes = "Unit names are attached only when the recording's enabled unit-key count equals a candidate " +
		"table's unit count and every ARM/CORE player's first start is that table's commander; see naming in each extract."
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "validation.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	for _, k := range sortedKeys(v.Site) {
		s := v.Site[k]
		if s.SiteEntries == 0 {
			continue
		}
		fmt.Printf("%s: %d recordings, site entries %d: type counts agree %.2f%%; start exact %.2f%%, within 1s %.2f%%, unmatched %d; extract finished starts %d, unmatched %d\n",
			k, s.Recordings, s.SiteEntries, pct(s.TypeAgree, s.SiteEntries), pct(s.StartExact, s.SiteEntries),
			pct(s.StartExact+s.StartWithin1s, s.SiteEntries), s.SiteUnmatched, s.ExtractFinished, s.ExtractUnmatched)
	}
	for _, k := range sortedKeys(v.Plausible) {
		p := v.Plausible[k]
		fmt.Printf("%s: early starts nearest own start %.2f%% of %d; lattice %d/%d named structures; max x %d z %d y %d\n",
			k, pct(p.NearestOwnStart, p.EarlyStarts), p.EarlyStarts, p.OnLattice, p.Structures, p.MaxX, p.MaxZ, p.MaxY)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func compareSite(ex *extract, sv *siteViews, sc *siteCompare, row *recordingCompare) {
	sc.Recordings++
	for i, sp := range sv.Players {
		// Join by transport identity; older views lack it, so fall back to
		// side and colour, then to list position when the sides agree.
		var p *extractPlayer
		how := &sc.Unjoined
		for j := range ex.Players {
			q := &ex.Players[j]
			if sp.DPID != nil && q.DPID == *sp.DPID {
				p, how = q, &sc.JoinedByDPID
				break
			}
			if sp.DPID == nil && q.Side == sp.Side && q.Color == sp.Color {
				p, how = q, &sc.JoinedSideColor
				break
			}
		}
		if p == nil && sp.DPID == nil && i < len(ex.Players) && ex.Players[i].Side == sp.Side {
			p, how = &ex.Players[i], &sc.JoinedOrder
		}
		*how++
		if p == nil {
			continue
		}
		if p.Side != sp.Side {
			sc.SideMismatch++
		}
		if i >= len(sv.BuildOrder) {
			continue
		}
		site := sv.BuildOrder[i]
		// The archive lists starts that were completed; the commander,
		// which no 0x12 completes, is therefore absent.
		var mine []*buildEvent
		for _, b := range p.Builds {
			if b.FinMs != nil {
				mine = append(mine, b)
			}
		}
		if len(mine) == len(site) {
			sc.CountEqual++
		} else {
			sc.CountDiffer++
		}
		sc.SiteEntries += len(site)
		sc.ExtractFinished += len(mine)
		types := map[int]int{}
		for _, b := range mine {
			types[b.Type]++
		}
		for _, e := range site {
			if types[e.Type] > 0 {
				types[e.Type]--
				sc.TypeAgree++
			}
		}
		row.SiteEntries += len(site)
		used := make([]bool, len(mine))
		byType := map[int][]int{}
		for j, b := range mine {
			byType[b.Type] = append(byType[b.Type], j)
		}
		matched := make([]int, len(site))
		for k := range matched {
			matched[k] = -1
		}
		// Exact seconds first, then one-second neighbours.
		for pass := 0; pass < 2; pass++ {
			for k, e := range site {
				if matched[k] >= 0 {
					continue
				}
				for _, j := range byType[e.Type] {
					if used[j] {
						continue
					}
					d := siteSecond(mine[j].Tick) - e.Started
					if d == 0 || pass == 1 && (d == 1000 || d == -1000) {
						used[j], matched[k] = true, j
						if pass == 0 {
							sc.StartExact++
							row.Exact++
						} else {
							sc.StartWithin1s++
							row.Within1s++
						}
						break
					}
				}
			}
		}
		for k, e := range site {
			j := matched[k]
			if j < 0 {
				sc.SiteUnmatched++
				row.SiteUnmatched++
				continue
			}
			if ft := mine[j].FinTick; ft != nil {
				sc.CompleteCompared++
				switch d := siteSecond(*ft) - e.Completed; {
				case d == 0:
					sc.CompleteExact++
				case d == 1000 || d == -1000:
					sc.CompleteWithin1s++
				}
			}
		}
		for j := range mine {
			if !used[j] {
				sc.ExtractUnmatched++
				row.Unmatched++
			}
		}
	}
}

// checkPlausible measures whether coordinates look like map positions: each
// combat player's first start is taken as its start position, and starts in
// the first five minutes of sync ticks should lie nearest their own player's
// start. Structures named by an original-TA table are checked against the
// 16-pixel cell lattice with their footprint centre [03 §2.1], using the
// engine's stock footprints; other catalogs' footprints are not known here.
func checkPlausible(ex *extract, pl *plausibility, fp map[string]footprint) {
	ota := ex.Naming.Table == stockTablePath || ex.Naming.Table == filepath.Join("v3.1", "mod_units.json")
	pl.Recordings++
	type pt struct{ x, z int }
	var starts []pt
	var owner []int
	for i, p := range ex.Players {
		if (p.Side == 0 || p.Side == 1) && len(p.Builds) > 0 {
			starts = append(starts, pt{p.Builds[0].X, p.Builds[0].Z})
			owner = append(owner, i)
			pl.Players++
		}
	}
	for i, p := range ex.Players {
		for k, b := range p.Builds {
			pl.MaxX, pl.MaxZ, pl.MaxY = max(pl.MaxX, b.X), max(pl.MaxZ, b.Z), max(pl.MaxY, b.Y)
			if b.X > 16384 || b.Z > 16384 {
				pl.Over16384++
			}
			if f, ok := fp[b.Unit]; ok && ota {
				if f.mobile {
					pl.Mobiles++
					if b.Hi != nil {
						pl.HighMobiles++
					}
				} else {
					pl.Structures++
					if b.X%16 == f.fx%2*8 && b.Z%16 == f.fz%2*8 {
						pl.OnLattice++
					}
					if b.Hi != nil {
						pl.HighStructures++
					}
				}
			}
			if k == 0 || b.Tick < 0 || b.Tick > 9000 || len(starts) < 2 {
				continue
			}
			best, bestD := -1, 0
			for s, st := range starts {
				dx, dz := b.X-st.x, b.Z-st.z
				if d := dx*dx + dz*dz; best < 0 || d < bestD {
					best, bestD = s, d
				}
			}
			pl.EarlyStarts++
			if owner[best] == i {
				pl.NearestOwnStart++
			}
		}
	}
}
