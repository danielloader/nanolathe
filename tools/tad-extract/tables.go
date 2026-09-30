package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// unitTable is a candidate numbering for 0x09 unit types: names sorted
// case-insensitively, type N naming entry N-1, because the recording's
// indices are positions in the retained, name-sorted definition table with
// row zero reserved [fmt tad §7 "Catalog provenance and recording-specific
// width"]. A recording's own catalog varies with the installed units, so a
// table is used only after the recording is checked against it.
type unitTable struct {
	Name   string   // provenance, for the extract
	Sig    string   // path, size and mtime; a change re-extracts
	SHA256 string   // exact bytes parsed, for the path evidence manifest
	Names  []string // sorted, upper case
	// Real counts rows other than the "ZZZ" row the archive's tables end
	// with. Most recordings' enabled-key counts equal the table size without
	// that row; a count one higher leaves the extra unit's sort position
	// unknown, so only the exact count is accepted.
	Real int
	arm  int // 1-based type of ARMCOM, 0 when absent
	core int // 1-based type of CORCOM, 0 when absent
}

func loadTable(path, name string) (*unitTable, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	t, err := parseTable(b, name)
	if err != nil {
		return nil, err
	}
	t.Sig = fmt.Sprintf("%s:%d:%d", name, fi.Size(), fi.ModTime().Unix())
	return t, nil
}

func parseTable(b []byte, name string) (*unitTable, error) {
	var rows []struct {
		Unitname string `json:"unitname"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	t := &unitTable{Name: name, SHA256: evidenceSHA(b)}
	for _, r := range rows {
		t.Names = append(t.Names, strings.ToUpper(r.Unitname))
	}
	sort.Strings(t.Names)
	t.Real = len(t.Names)
	if t.Real > 0 && t.Names[t.Real-1] == "ZZZ" {
		t.Real--
	}
	for i, n := range t.Names {
		switch n {
		case "ARMCOM":
			t.arm = i + 1
		case "CORCOM":
			t.core = i + 1
		}
	}
	return t, nil
}

// naming records which table named a recording's unit types, or why none.
type naming struct {
	Table  string `json:"table,omitempty"`
	Reason string `json:"reason"`
	// Commanders counts players whose first start was checked against the
	// table's commander for the player's side.
	Commanders int `json:"commanders_checked"`
}

// chooseTable returns the first candidate that the recording verifies:
// its enabled unit-key count equals the table's real row count, and every
// ARM or CORE player's first start (the commander [fmt tad "Corpus
// validation"]) has that side's commander type in the table. Both checks
// are necessary; neither alone pins the numbering of every row, so an
// accepted table remains a verified candidate rather than the recording's
// proven catalog.
func chooseTable(su *setup, players []extractPlayer, cands []*unitTable) (*unitTable, naming) {
	if len(cands) == 0 {
		return nil, naming{Reason: "no unit table for this source"}
	}
	var why []string
	for _, t := range cands {
		if su.UnitData.Enabled != t.Real {
			why = append(why, fmt.Sprintf("%s: %d enabled keys, table has %d units", t.Name, su.UnitData.Enabled, t.Real))
			continue
		}
		checked, bad := 0, 0
		for _, p := range players {
			if len(p.Builds) == 0 {
				continue
			}
			want := 0
			switch p.Side {
			case 0:
				want = t.arm
			case 1:
				want = t.core
			default:
				continue
			}
			checked++
			if p.Builds[0].Type != want {
				bad++
			}
		}
		if checked == 0 || bad > 0 {
			why = append(why, fmt.Sprintf("%s: %d of %d commander checks failed", t.Name, bad, checked))
			continue
		}
		return t, naming{Table: t.Name, Reason: "enabled keys and commander types match", Commanders: checked}
	}
	return nil, naming{Reason: strings.Join(why, "; ")}
}
