package main

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These bytes are authored from [fmt tad §§4, 6, 7, 9]. A one-slot owner
// makes every sync a scheduled observation; the two-row candidate needs W=2.
func pathTestBody(position bool, extra bool) []byte {
	var w bitWriter
	if position {
		w.put(0, 16)
		w.put(1, 2)
		w.put(1, 1)
		w.put(2, 2)
		w.put(100, 16)
		w.put(200, 16)
		w.put(300, 16)
		w.put(400, 16)
	}
	w.put(0xffff, 16)
	w.put(1, 1)
	if !position {
		w.put(0, 2)
		return w.b
	}
	w.put(1, 2)
	w.put(95, 16)
	w.put(0, 8)
	w.put(9, 8)
	w.put(1, 2)
	w.put(0, 1)
	w.put(0xffff8000, 32) // -0.5 world units: floor is -1, not truncation to 0
	w.put(0, 32)
	w.put(128<<16|32768, 32)
	w.put(0, 16)
	w.put(0, 16)
	w.put(0, 16)
	if extra {
		return append(w.b, 0)
	} // fits neither status length
	return w.b
}

func pathTestSync(position, extra bool) []byte {
	b := pathTestBody(position, extra)
	return append(append([]byte{0xfd}, u16(len(b)+7)...), b...)
}

func pathTestFixture() []byte {
	var data []byte
	data = append(data, record([]byte(demoMagic), u16(5), []byte{1}, u16(1), []byte("Authored Map\x00"))...)
	data = append(data, record(u32(3))...)
	data = append(data, record(u32(2), []byte("private chat sentinel"))...)
	data = append(data, record(u32(3), []byte("private recorder sentinel"))...)
	data = append(data, record(u32(6), []byte("private address sentinel"))...)
	data = append(data, record([]byte{0, 0, 9}, []byte("private player sentinel\x00"))...)
	pi := fixturePlayerInfo(3737844653)
	binary.LittleEndian.PutUint16(pi[166:], 1)
	data = append(data, record([]byte{9}, sealEnvelope(append(u32(0xffffffff), pi...), false))...)
	data = append(data, record(unitRecord(3, 7, 1, 1), unitRecord(3, 8, 1, 1))...)
	data = append(data, packet(1, 9, false, append([]byte{0xfe}, u32(0)...), pathTestSync(false, false), buildStart(1, 1, 20, 3, 40))...)
	for tick := 1; tick <= 60; tick++ {
		data = append(data, packet(1, 9, false, pathTestSync(true, false))...)
	}
	return data
}

func pathTestExtract(t *testing.T, data []byte) *pathEvidence {
	t.Helper()
	table, err := parseTable([]byte(`[{"unitname":"ARMCOM"},{"unitname":"CORCOM"}]`), "authored/table.json")
	if err != nil {
		t.Fatal(err)
	}
	info := evidenceTable{Path: table.Name, SHA256: table.SHA256, Rows: table.Real}
	return extractPathEvidence(data, evidenceSource{Source: "external", ID: "authored"}, []*unitTable{table}, []evidenceTable{info}, map[string]bool{"ARMCOM": false, "CORCOM": false}, evidenceTable{Path: "authored/grammar.json", SHA256: evidenceSHA([]byte("authored grammar")), Rows: 2})
}

func hasEvidenceRejection(e *pathEvidence, code string) bool {
	for _, r := range e.Rejections {
		if r.Code == code {
			return true
		}
	}
	return false
}

func TestPathEvidencePrivacyAndLifetimeIdentity(t *testing.T) {
	data := pathTestFixture()
	death := append([]byte{0x0c}, u16(1)...)
	death = append(death, u32(3737844653)...)
	death = append(death, 0, 0, 0, 0)
	data = append(data, packet(1, 9, false, death, pathTestSync(false, false), buildStart(1, 1, 20, 3, 40), pathTestSync(true, false))...)
	data = append(data, packet(1, 9, false, []byte{0x0f, 0xff, 3, 0, 4, 0})...)
	e := pathTestExtract(t, data)
	if !e.Admitted {
		t.Fatalf("authored evidence rejected: %+v", e.Rejections)
	}
	if len(e.Instances) != 2 || e.Instances[0].ID == e.Instances[1].ID || e.Instances[0].NetID != e.Instances[1].NetID {
		t.Fatal("recorded death must create a fresh lifetime for reused ID")
	}
	old, fresh := e.Instances[0], e.Instances[1]
	if len(old.Prefixes) != 60 || len(fresh.Prefixes) != 1 || len(old.Positions) != 60 || len(fresh.Positions) != 1 {
		t.Fatal("unchanged prefixes must remain observations in their own lifetime")
	}
	if old.OwnerOrdinal != 0 || fresh.Ordinal != 2 || old.Build.TickLower == nil || *old.Build.TickLower != 0 || *old.Build.TickUpper != 1 {
		t.Fatal("anonymous owner/generation or build bracket differs")
	}
	if old.Died == nil || *old.Died.TickLower != 60 || *old.Died.TickUpper != 61 || *fresh.Build.TickLower != 61 || *fresh.Build.TickUpper != 62 {
		t.Fatal("reuse brackets do not follow capture order")
	}
	p := old.Positions[0]
	if p.X != -1 || p.RawX != -32768 || p.Z != 128 || p.RawZ != 128<<16|32768 || p.OccupancyMode != 1 {
		t.Fatalf("fixed point or occupancy lost: %+v", p)
	}
	if len(e.Reclaims) != 1 || e.Reclaims[0] != (evidenceReclaim{0, 62, 3, 4}) {
		t.Fatal("reclaim did not keep lower tick and tile anchors")
	}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private player sentinel", "private chat sentinel", "private recorder sentinel", "private address sentinel", "3737844653", `"dpid"`, `"sender"`} {
		if bytes.Contains(b, []byte(private)) {
			t.Fatalf("private field escaped: %q", private)
		}
	}
	if e.Source.SHA256 != evidenceSHA(data) || e.Source.Bytes != len(data) {
		t.Fatal("source digest does not bind the parsed bytes")
	}
	again, err := json.Marshal(pathTestExtract(t, data))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, again) {
		t.Fatal("same immutable source/table inputs must produce the same evidence")
	}
}

func TestPathEvidenceRejectsBodyDamageAndClockGaps(t *testing.T) {
	base := pathTestFixture()
	for _, tc := range []struct {
		name   string
		tail   []byte
		reason string
	}{
		{"body length", packet(1, 9, false, pathTestSync(true, true)), "status_length_mismatch"},
		{"clock gap", packet(1, 9, false, append([]byte{0xfe}, u32(63)...), pathTestSync(true, false)), "tick_resync_gap"},
		{"clock rewind", packet(1, 9, false, append([]byte{0xfe}, u32(59)...), pathTestSync(true, false)), "tick_resync_rewind"},
		{"truncated record", []byte{5, 0, 1}, "framing_not_eof"},
		{"transfer unknown", packet(1, 9, false, append([]byte{0x14}, make([]byte, 23)...)), "ownership_transfer_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pathTestExtract(t, append(bytes.Clone(base), tc.tail...))
			if e.Admitted || !hasEvidenceRejection(e, tc.reason) {
				t.Fatalf("missing rejection %s: %+v", tc.reason, e.Rejections)
			}
			if len(e.Instances) == 0 || len(e.Instances[0].Positions) < 60 {
				t.Fatal("earlier valid partial evidence was lost")
			}
			if tc.name == "body length" && len(e.Instances[0].Prefixes) != 60 {
				t.Fatal("a prefix escaped its malformed enclosing body")
			}
		})
	}
	// A raw sync also has to agree with the expected next sender tick.
	b := pathTestBody(true, false)
	raw := append(append(append([]byte{0x2c}, u16(len(b)+7)...), u32(59)...), b...)
	e := pathTestExtract(t, append(bytes.Clone(base), packet(1, 9, false, raw)...))
	if !hasEvidenceRejection(e, "tick_resync_rewind") || !hasEvidenceRejection(e, "status_positions_nonincreasing") {
		t.Fatal("raw clock/pose rewind escaped admission")
	}
}

func TestPathEvidenceLifecycleBracketsRequireBothUninterruptedBounds(t *testing.T) {
	finish := []byte{0x12, 1, 0, 0, 0}
	for _, tc := range []struct {
		name    string
		tail    []byte
		bounded bool
	}{
		{"next sync", packet(1, 9, false, finish, pathTestSync(true, false)), true},
		{"end of capture", packet(1, 9, false, finish), false},
		{"speed change", packet(1, 9, false, finish, []byte{0x19, 1, 11}, pathTestSync(true, false)), false},
		{"clock gap", packet(1, 9, false, finish, append([]byte{0xfe}, u32(80)...), pathTestSync(true, false)), false},
		{"speed before event", packet(1, 9, false, []byte{0x19, 1, 11}, finish, pathTestSync(true, false)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := pathTestExtract(t, append(pathTestFixture(), tc.tail...))
			b := e.Instances[0].Finished
			if b == nil {
				t.Fatal("lifecycle evidence lost")
			}
			if tc.bounded {
				if b.TickLower == nil || b.TickUpper == nil || *b.TickLower != 60 || *b.TickUpper != 61 {
					t.Fatal("valid bounds lost")
				}
			} else if b.TickLower != nil || b.TickUpper != nil {
				t.Fatal("bounds crossed an unsupported segment")
			}
		})
	}
}

func TestPathEvidenceUnknownGrammarAndRosterAreNotGuessed(t *testing.T) {
	data := pathTestFixture()
	table, _ := parseTable([]byte(`[{"unitname":"ARMCOM"},{"unitname":"CORCOM"}]`), "table")
	e := extractPathEvidence(data, evidenceSource{}, []*unitTable{table}, nil, map[string]bool{}, evidenceTable{})
	if e.Admitted || !hasEvidenceRejection(e, "grammar_unknown") || len(e.Instances[0].Positions) != 0 {
		t.Fatal("missing canfly selected a grammar")
	}
	// Mutate the status checksum, while leaving all later evidence intact.
	var records [][]byte
	for p := 0; p < len(data); {
		n := int(binary.LittleEndian.Uint16(data[p:]))
		records = append(records, bytes.Clone(data[p:p+n]))
		p += n
	}
	records[6][4] ^= 1
	e = pathTestExtract(t, bytes.Join(records, nil))
	if e.Admitted || e.Counters.RosterValid || !hasEvidenceRejection(e, "complete_unique_roster_invalid") {
		t.Fatal("bad encrypted roster identity was accepted")
	}
}

func TestPathEvidenceCLIOutputHashesAndMetadataGate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "derived"), 0700); err != nil {
		t.Fatal(err)
	}
	stock := []byte(`[{"unitname":"ARMCOM","roles":[]},{"unitname":"CORCOM","roles":[]}]`)
	if err := os.WriteFile(filepath.Join(root, stockTablePath), stock, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "private filename sentinel.tad")
	data := pathTestFixture()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(map[string]any{"bytes": len(data), "private": "metadata sentinel"})
	if err := os.WriteFile(strings.TrimSuffix(path, ".tad")+".json", metadata, 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "output")
	if err := runPathEvidence(root, out, []string{path}); err != nil {
		t.Fatal(err)
	}
	mb, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest evidenceManifest
	if err := json.Unmarshal(mb, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 1 || !manifest.Files[0].Admitted {
		t.Fatal("CLI rejected valid authored file")
	}
	row := manifest.Files[0]
	gz, err := os.ReadFile(filepath.Join(out, row.Path))
	if err != nil {
		t.Fatal(err)
	}
	if row.SHA256 != evidenceSHA(gz) || row.Source.MetadataSHA256 != evidenceSHA(metadata) {
		t.Fatal("manifest does not bind output and metadata")
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var e pathEvidence
	if err := json.NewDecoder(zr).Decode(&e); err != nil {
		t.Fatal(err)
	}
	if e.GrammarTable.SHA256 != evidenceSHA(stock) || e.Source.Source != "external" || e.Source.ID != evidenceSHA(data) {
		t.Fatal("input projection or exact grammar hash differs")
	}
	if bytes.Contains(mb, []byte("private")) {
		t.Fatal("private filename/metadata escaped manifest")
	}
	// External metadata must be supplied and agree; EOF alone is not enough.
	if err := os.Remove(strings.TrimSuffix(path, ".tad") + ".json"); err != nil {
		t.Fatal(err)
	}
	if err := runPathEvidence(root, out, []string{path}); err != nil {
		t.Fatal(err)
	}
	mb, err = os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mb, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Files[0].Admitted {
		t.Fatal("missing sidecar silently relaxed admission")
	}
}

func TestPathEvidenceDeathBracketUsesEventSender(t *testing.T) {
	e := &pathEvidence{Source: evidenceSource{SHA256: "authored"}, MaxUnits: 1, Counters: evidenceCounters{RosterValid: true}}
	su := &setup{Header: header{NumPlayers: 2, MaxUnits: 1}, Players: []player{{Number: 1}, {Number: 2}}, Status: []playerInfo{{Number: 1, DPID: 200}, {Number: 2, DPID: 100}}}
	m := &evidenceTracker{out: e, x: newExtractor(su), byEvent: map[*buildEvent]*evidenceInstance{}, ordinals: map[int]int{}, pending: map[int][]*evidenceBracket{}, lastSync: map[int]subContext{}}
	// No table: this tests only established lifecycle capture context.
	m.onSub(subContext{Sender: 1, Tick: 10}, []byte{0xff})
	m.onSub(subContext{Sender: 1, Tick: 10}, buildStart(1, 2, 0, 0, 0))
	m.onSub(subContext{Sender: 1, Tick: 11}, []byte{0xff})
	m.onSub(subContext{Sender: 2, Tick: 500}, []byte{0xff})
	death := append([]byte{0x0c}, u16(2)...)
	death = append(death, make([]byte, 8)...)
	m.onSub(subContext{Sender: 2, Tick: 500}, death)
	m.onSub(subContext{Sender: 1, Tick: 12}, []byte{0xff})
	if e.Instances[0].Died.TickUpper != nil {
		t.Fatal("victim owner's clock incorrectly closed attacker's event bracket")
	}
	m.onSub(subContext{Sender: 2, Tick: 501}, []byte{0xff})
	b := e.Instances[0].Died
	if b.SenderOwnerOrdinal != 0 || *b.TickLower != 500 || *b.TickUpper != 501 || e.Instances[0].OwnerOrdinal != 1 {
		t.Fatal("death evidence used the victim's clock")
	}
}

func TestPathEvidenceSidecarDigestAndBothLayouts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "authored.tad")
	src := evidenceSource{Source: "tada", Bytes: 7, SHA256: evidenceSHA([]byte("fixture"))}
	for _, tc := range []struct {
		body   string
		reason string
	}{
		{`{"bytes":7,"sha256":"` + src.SHA256 + `"}`, ""},
		{`{"detail":{"fileSize":7}}`, ""},
		{`{"bytes":7,"detail":{"fileSize":8}}`, "metadata_size_mismatch"},
		{`{"bytes":7,"sha256":"` + strings.Repeat("0", 64) + `"}`, "metadata_source_sha256_mismatch"},
		{`{"bytes":8,"sha256":"private malformed digest sentinel"}`, "metadata_source_sha256_invalid"},
	} {
		if err := os.WriteFile(strings.TrimSuffix(path, ".tad")+".json", []byte(tc.body), 0600); err != nil {
			t.Fatal(err)
		}
		got := src
		if reason := evidenceMetadata(path, &got); reason != tc.reason {
			t.Fatalf("sidecar reason %q, want %q", reason, tc.reason)
		}
		if strings.Contains(got.ExpectedSHA256, "private") {
			t.Fatal("invalid metadata text escaped")
		}
	}
}

func TestHeaderMapMayEndAtRecordBoundary(t *testing.T) {
	prefix := append(append(append([]byte(demoMagic), u16(5)...), 1), u16(500)...)
	for _, name := range []string{"Authored Map", "Authored Map\x00"} {
		h, err := parseHeader(append(bytes.Clone(prefix), []byte(name)...))
		if err != nil || h.Map != "Authored Map" {
			t.Fatal("valid map record boundary rejected")
		}
	}
	if _, err := parseHeader(append(bytes.Clone(prefix), []byte("Authored\x00Unknown")...)); err == nil {
		t.Fatal("unestablished map tail was silently ignored")
	}
}

func TestPathEvidenceCandidateNamesMustBeUnambiguous(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "v3.1"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`[{"unitname":"ARMCOM"},{"unitname":"armcom"}]`,
		`[{"unitname":"ARMCOM"},{"unitname":""}]`,
	} {
		if err := os.WriteFile(filepath.Join(root, "v3.1/mod_units.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		ts, infos, _, _ := evidenceInputs(root, "v3.1", false)
		if len(ts) != 0 || len(infos) != 1 || infos[0].Error != "empty_or_duplicate_unit_name" || infos[0].SHA256 != evidenceSHA([]byte(body)) {
			t.Fatal("ambiguous numbering escaped the candidate gate or lost byte provenance")
		}
	}
}
