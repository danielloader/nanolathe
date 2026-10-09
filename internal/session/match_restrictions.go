package session

// Battle-configuration field 12, the online unit restrictions
// (docs/DESIGN_MULTIPLAYER.md §8.6 field 12, §16.6;
// docs/DESIGN_MODS_MUTATORS.md §15.3 "Field 12"). A content.Restrictions value
// names units; field 12 names records: one {DefinitionID, Unit, Limit} for
// every retained record an entry names, DefinitionID being that record's
// index in the unrestricted compiled catalog. An entry for a duplicated name
// therefore gives one record per copy, which is how the field keeps
// duplicate-name identities. The two directions are inverse on the sets a
// catalog accepts, so every seat of a match, whose configuration digest covers
// the records, applies one set to one catalog. These are Nanolathe protocol
// values, not retail findings.

import (
	"fmt"
	"math"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// matchRestrictionError is the one diagnostic shape of a field-12 refusal
// against a catalog.
func matchRestrictionError(path, expected string) error {
	return fmt.Errorf("nanolathe: match unit restrictions rejected: logical path %s, providers searched [match configuration v1, unrestricted unit catalog], expected %s", path, expected)
}

// matchRestrictionIssues refuses a set the catalog does not wholly accept
// (§15.3's three issues), keeping the content refusal attached so a host can
// name each entry.
func matchRestrictionIssues(r content.Restrictions, issues []content.RestrictionIssue) error {
	return fmt.Errorf("%w: %w", matchRestrictionError("unitRestrictions",
		"units the content defines that are not marked norestrict, removing no side's commander"),
		&content.RestrictionsError{Restrictions: r, Issues: issues})
}

// MatchUnitRestrictions maps a restriction set onto battle-configuration
// field 12 against cat, the unrestricted compiled catalog — the catalog
// before battle entry applies any restriction, whose record indices the
// definition IDs are. It gives one record for every retained record an entry
// names, in ascending definition-ID order: that record's index, the entry's
// key and its count. A zero set gives nil, so an unrestricted configuration
// encodes exactly as it always did.
//
// A set the catalog does not wholly accept is refused, the refusal wrapping
// *content.RestrictionsError: a name no record carries, a norestrict name, or
// the removal of a side's commander (§15.3), since a configuration carrying
// any of them is rejected anyway. So is a key longer than the configuration's
// key bound, or a record whose index field 12's 16 bits cannot hold.
func MatchUnitRestrictions(cat *content.Catalog, r content.Restrictions) ([]MatchUnitRestriction, error) {
	if r.IsZero() {
		return nil, nil
	}
	if cat == nil {
		return nil, matchRestrictionError("catalog", "the unrestricted compiled catalog the definition IDs index")
	}
	if _, issues := cat.CheckRestrictions(r); len(issues) != 0 {
		return nil, matchRestrictionIssues(r, issues)
	}
	for _, e := range r.Entries() {
		if err := validateMatchKey("unitRestrictions."+e.Unit, e.Unit); err != nil {
			return nil, err
		}
	}
	return matchRestrictionRecords(cat, r)
}

// matchRestrictionRecords walks the catalog's retained records in index order
// and writes a record for each one whose unit name has an entry in r, so the
// list is in ascending definition-ID order and covers every record of a
// duplicated name. The index is the record's 1-based position, the value
// UnitDefByIndex reads back.
func matchRestrictionRecords(cat *content.Catalog, r content.Restrictions) ([]MatchUnitRestriction, error) {
	var out []MatchUnitRestriction
	for i, u := range cat.UnitRecords() {
		if u == nil {
			continue
		}
		key := content.CanonicalKey(u.UnitName)
		count, restricted := r.Count(key)
		if !restricted {
			continue
		}
		id := i + 1
		if id > math.MaxUint16 {
			return nil, matchRestrictionError("unitRestrictions."+key, fmt.Sprintf("a definition ID of at most %d; unit %q is record %d", math.MaxUint16, key, id))
		}
		out = append(out, MatchUnitRestriction{DefinitionID: uint16(id), Unit: key, Limit: count})
	}
	return out, nil
}

// matchRestrictionSet reads the set field-12 records describe without a
// catalog: the schema checks resolution applies, then one limit per key and
// every key a stored restriction spelling. It is what admission compares with
// the set the frozen inputs record; whether the records describe a catalog is
// RestrictionsFromMatch's question.
func matchRestrictionSet(records []MatchUnitRestriction) (content.Restrictions, error) {
	if len(records) == 0 {
		return content.Restrictions{}, nil
	}
	if err := (&MatchConfigRequest{UnitRestrictions: records}).validateRestrictions(); err != nil {
		return content.Restrictions{}, err
	}
	// A map from key to limit keeps a remote list of up to 65535 records
	// linear; ParseRestrictions orders the keys, so nothing ranges it.
	limits := make(map[string]int, len(records))
	for i, rec := range records {
		if limit, seen := limits[rec.Unit]; seen && limit != int(rec.Limit) {
			return content.Restrictions{}, matchFieldError(fmt.Sprintf("unitRestrictions[%d].limit", i),
				fmt.Sprintf("%d, the limit every record of unit %q carries", limit, rec.Unit))
		}
		limits[rec.Unit] = int(rec.Limit)
	}
	set, err := content.ParseRestrictions(limits)
	if err != nil {
		return content.Restrictions{}, matchFieldError("unitRestrictions", "unit restriction keys and limits: "+err.Error())
	}
	return set, nil
}

// RestrictionsFromMatch is MatchUnitRestrictions' inverse: it gives back the
// restriction set field-12 records describe, verified against cat, the
// unrestricted compiled catalog their definition IDs index. Each record must
// name a retained record carrying its key, with no ID twice; every record
// carrying a named key must be present, with the one limit every record of
// that key carries; and the set must be one the catalog accepts — no
// norestrict name and no side's commander removed (§15.3, §8.6 field 12).
// Empty records give the zero set. A refusal names the record or entry; the
// catalog issues wrap *content.RestrictionsError.
func RestrictionsFromMatch(cat *content.Catalog, records []MatchUnitRestriction) (content.Restrictions, error) {
	set, err := matchRestrictionSet(records)
	if err != nil || set.IsZero() {
		return set, err
	}
	if cat == nil {
		return content.Restrictions{}, matchRestrictionError("catalog", "the unrestricted compiled catalog the definition IDs index")
	}
	for i, rec := range records {
		u, ok := cat.UnitDefByIndex(uint32(rec.DefinitionID))
		if !ok {
			return content.Restrictions{}, matchRestrictionError(fmt.Sprintf("unitRestrictions[%d].definitionID", i),
				fmt.Sprintf("a definition of the unrestricted catalog's %d records", len(cat.UnitRecords())))
		}
		if key := content.CanonicalKey(u.UnitName); key != rec.Unit {
			return content.Restrictions{}, matchRestrictionError(fmt.Sprintf("unitRestrictions[%d].unit", i),
				fmt.Sprintf("the unit key of definition %d, %q, got %q", rec.DefinitionID, key, rec.Unit))
		}
	}
	if _, issues := cat.CheckRestrictions(set); len(issues) != 0 {
		return content.Restrictions{}, matchRestrictionIssues(set, issues)
	}
	want, err := matchRestrictionRecords(cat, set)
	if err != nil {
		return content.Restrictions{}, err
	}
	// Every record names a definition carrying its key and the IDs are
	// unique, so the records are a subset of want with want's limits; a
	// shorter list leaves out a record of a named key, a partial name group.
	for i, w := range want {
		if i >= len(records) || records[i] != w {
			return content.Restrictions{}, matchRestrictionError("unitRestrictions",
				fmt.Sprintf("a record for every definition carrying unit key %q; definition %d has none", w.Unit, w.DefinitionID))
		}
	}
	return set, nil
}

// matchFrozenRestrictionRecords checks field-12 records against the frozen
// catalog battle entry already restricted with set, the set the records
// describe. Removal kept the survivors' order (§15.4 step 1), so walking the
// unrestricted index space — a removed record at each limit-0 ID, the next
// survivor at every other position — gives every survivor its unrestricted
// ID. Each capped record must then name a survivor carrying its key, and no
// survivor carrying a restricted key may be left out. It returns the refused
// path and expectation, or ok.
//
// Design reading: a removed record is absent from the frozen catalog, so its
// key cannot be read here. RestrictionsFromMatch verified it against the
// unrestricted catalog when the inputs were frozen (FreezeMatchInputs), and
// the content identity covers the restricted table every seat compares.
func matchFrozenRestrictionRecords(cat *content.Catalog, records []MatchUnitRestriction, set content.Restrictions) (path, expected string, ok bool) {
	survivors := cat.UnitRecords()
	next, k := 0, 0
	for id := 1; next < len(survivors) || k < len(records); id++ {
		if k < len(records) && int(records[k].DefinitionID) == id {
			rec := records[k]
			k++
			if rec.Limit == 0 {
				continue
			}
			if next >= len(survivors) {
				return fmt.Sprintf("unitRestrictions[%d].definitionID", k-1), "a definition of the index space the frozen catalog and the removed records span", false
			}
			u := survivors[next]
			next++
			if u == nil || content.CanonicalKey(u.UnitName) != rec.Unit {
				return fmt.Sprintf("unitRestrictions[%d].unit", k-1), fmt.Sprintf("the unit key of definition %d in the frozen catalog, got %q", id, rec.Unit), false
			}
			continue
		}
		if next >= len(survivors) {
			return fmt.Sprintf("unitRestrictions[%d].definitionID", k), "a definition of the index space the frozen catalog and the removed records span", false
		}
		u := survivors[next]
		next++
		if u == nil {
			continue
		}
		if _, restricted := set.Count(u.UnitName); restricted {
			return "unitRestrictions", fmt.Sprintf("a record for every definition carrying unit key %q; definition %d has none", content.CanonicalKey(u.UnitName), id), false
		}
	}
	return "", "", true
}
