package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

type unitViewerEntry struct {
	Key string
	Def *content.UnitDef
}

// unitViewerEntries reads the immutable content catalog, without build-menu or
// battle admission filters (DESIGN_DEVELOPER_TOOLS §7). UnitRecords preserves
// definitions hidden by duplicate or empty lookup names [02 R-CAT-01 §§4–5].
func unitViewerEntries(c *content.Catalog) []unitViewerEntry {
	type orderedEntry struct {
		entry                  unitViewerEntry
		name, identity         string
		foldName, foldIdentity string
		ordinal                int
	}
	records := c.UnitRecords()
	ordered := make([]orderedEntry, 0, len(records))
	for ordinal, def := range records {
		if def == nil {
			continue
		}
		key := def.CanonicalKey
		if key == "" {
			key = content.CanonicalKey(def.UnitName)
		}
		identity := def.UnitName
		if identity == "" {
			identity = key
		}
		ordered = append(ordered, orderedEntry{
			entry: unitViewerEntry{Key: key, Def: def},
			name:  def.Name, identity: identity,
			foldName: strings.ToLower(def.Name), foldIdentity: strings.ToLower(identity),
			ordinal: ordinal,
		})
	}
	// Name then internal ID is the viewer's presentation order. The catalog
	// ordinal makes even identical names and IDs a total order without writing
	// a sort index into the immutable definitions [I1][I6].
	slices.SortFunc(ordered, func(a, b orderedEntry) int {
		return cmp.Or(
			cmp.Compare(a.foldName, b.foldName),
			cmp.Compare(a.foldIdentity, b.foldIdentity),
			cmp.Compare(a.entry.Def.UnitDefID, b.entry.Def.UnitDefID),
			cmp.Compare(a.name, b.name),
			cmp.Compare(a.identity, b.identity),
			cmp.Compare(a.entry.Key, b.entry.Key),
			cmp.Compare(a.ordinal, b.ordinal),
		)
	})
	entries := make([]unitViewerEntry, len(ordered))
	for i, item := range ordered {
		entries[i] = item.entry
	}
	return entries
}

// Each query token may match any identity field. Copying the entry slice keeps
// filtering and later list selection independent of the complete catalog list.
func filterUnitViewerEntries(entries []unitViewerEntry, query string) []unitViewerEntry {
	tokens := strings.Fields(strings.ToLower(query))
	filtered := make([]unitViewerEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Def == nil {
			continue
		}
		name := strings.ToLower(entry.Def.Name)
		identity := strings.ToLower(entry.Def.UnitName)
		key := strings.ToLower(entry.Key)
		matches := true
		for _, token := range tokens {
			if !strings.Contains(name, token) && !strings.Contains(identity, token) && !strings.Contains(key, token) {
				matches = false
				break
			}
		}
		if matches {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

type unitViewerStat struct {
	Label, Value string
}

// unitViewerStats presents compiled definitions only. The costs and movement
// strings reuse the unit-information conversions [07 R-HUD-03 §8]; buildtime
// is authored work, not seconds [05 "Construction arithmetic"]. Range fields
// retain authored world units [02 "Unit record"][02 "Weapon record"].
func unitViewerStats(def *content.UnitDef) []unitViewerStat {
	if def == nil {
		return nil
	}
	if def.DiscoveryOnly {
		// The secondary parse never populated the gameplay fields; their
		// allocation zeros are not authored statistics [02 R-CAT-01 §5].
		return []unitViewerStat{{Label: "Stats", Value: "Unavailable"}}
	}
	info := unitInfoValues(def)
	stats := []unitViewerStat{
		{Label: "Health", Value: fmt.Sprint(def.MaxDamage)},
		{Label: "Metal cost", Value: info[2]},
		{Label: "Energy cost", Value: info[1]},
		{Label: "Build work", Value: info[3]},
	}
	if def.BMCode != 0 {
		stats = append(stats,
			unitViewerStat{Label: "Speed", Value: info[5]},
			unitViewerStat{Label: "Acceleration", Value: info[6]},
			unitViewerStat{Label: "Turn rate", Value: info[7]},
		)
	}
	stats = append(stats,
		unitViewerStat{Label: "Sight range", Value: fmt.Sprintf("%d world units", def.SightDistance)},
		unitViewerStat{Label: "Radar range", Value: fmt.Sprintf("%d world units", def.RadarDistance)},
	)
	for i, weapon := range [3]*content.WeaponDef{def.Weapon1Def, def.Weapon2Def, def.Weapon3Def} {
		if content.IsWeaponInactive(weapon) {
			continue
		}
		name := weapon.Name
		if name == "" {
			name = weapon.CanonicalKey
		}
		// ReloadTime is already truncated to ticks at compile time. This is
		// the base definition interval, not a prediction of firing cadence
		// or script timing [02 "Weapon record"][06 §4.2].
		stats = append(stats,
			unitViewerStat{Label: fmt.Sprintf("Weapon %d", i+1), Value: name},
			unitViewerStat{Label: fmt.Sprintf("W%d range", i+1), Value: fmt.Sprintf("%d world units", weapon.Range)},
			unitViewerStat{Label: fmt.Sprintf("W%d base reload", i+1), Value: fmt.Sprintf("%.2f s", float64(weapon.ReloadTime)/unitInfoTicksPerSecond)},
		)
	}
	return stats
}
