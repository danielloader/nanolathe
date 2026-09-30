package main

// This opt-in projection is intentionally separate from the legacy extracts:
// only these allowlisted types may reach a path evidence artifact. Setup
// names, transport identities, chat and arbitrary decoder errors never do.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
)

const pathEvidenceSchema = 1
const pathEvidenceVersion = "1"

type evidenceParser struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Modified bool   `json:"modified"`
}

type evidenceSource struct {
	Source         string `json:"source"`
	ID             string `json:"id"`
	SHA256         string `json:"sha256"`
	Bytes          int    `json:"bytes"`
	MetadataSHA256 string `json:"metadata_sha256,omitempty"`
	ExpectedBytes  *int64 `json:"expected_bytes,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
}

type evidenceTable struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256,omitempty"`
	Rows     int    `json:"rows"`
	Selected bool   `json:"selected"`
	Error    string `json:"error,omitempty"` // categorical, never a raw OS/JSON error
}

type evidenceRejection struct {
	Code  string `json:"code"`
	Count int64  `json:"count"`
}

type evidenceBracket struct {
	WallMs             int64  `json:"wall_ms"`
	TickLower          *int64 `json:"tick_lower"`
	TickUpper          *int64 `json:"tick_upper"`
	ClockSegment       uint32 `json:"clock_segment"`
	SpeedSegment       uint32 `json:"speed_segment"`
	SenderOwnerOrdinal int    `json:"sender_owner_ordinal"`
}

type evidenceBuild struct {
	evidenceBracket
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

type evidencePosition struct {
	Tick          int64 `json:"tick"`
	X             int32 `json:"x"`
	Z             int32 `json:"z"`
	RawX          int32 `json:"x_raw_fixed"`
	RawZ          int32 `json:"z_raw_fixed"`
	Health        int32 `json:"health"`
	Remaining     int   `json:"remaining"`
	Flags         int   `json:"flags"`
	OccupancyMode int   `json:"occupancy_mode"`
}

type evidencePrefix struct {
	Tick    int64      `json:"tick"`
	Blocked bool       `json:"blocked"`
	Points  [][2]int32 `json:"points"`
}

type evidenceInstance struct {
	ID           string             `json:"id"`
	OwnerOrdinal int                `json:"owner_ordinal"`
	Ordinal      int                `json:"ordinal"`
	NetID        int                `json:"net_id"` // unit ID, never a participant transport ID
	TypeIndex    int                `json:"type_index"`
	UnitName     string             `json:"unit_name_candidate,omitempty"`
	Air          *bool              `json:"air_candidate"`
	Build        evidenceBuild      `json:"build"`
	Finished     *evidenceBracket   `json:"finished,omitempty"`
	Died         *evidenceBracket   `json:"died,omitempty"`
	Positions    []evidencePosition `json:"positions"`
	Prefixes     []evidencePrefix   `json:"prefixes"`
}

type evidenceReclaim struct {
	OwnerOrdinal int   `json:"owner_ordinal"`
	Tick         int64 `json:"tick"` // preceding sender sync, not exact removal time
	X            int   `json:"x"`    // attribute-tile anchor, not world coordinates
	Z            int   `json:"z"`
}

type evidenceJoins struct {
	UnknownSender         int64 `json:"unknown_sender_bodies"`
	StatusNoStart         int64 `json:"status_no_start"`
	StatusGrammarUnknown  int64 `json:"status_grammar_unknown"`
	StatusAfterDeath      int64 `json:"status_after_death"`
	PrefixAfterDeath      int64 `json:"prefix_after_death"`
	PositionNonincreasing int64 `json:"status_positions_nonincreasing"`
	PrefixNonincreasing   int64 `json:"prefix_ticks_nonincreasing"`
	SlotOutOfRange        int64 `json:"slot_out_of_range"`
	IdleWidthUnknown      int64 `json:"idle_width_unknown"`
	OwnershipTransfer     int64 `json:"ownership_transfer_unknown"`
	Positions             int64 `json:"positions_kept"`
	Prefixes              int64 `json:"prefixes_kept"`
	Attached              int64 `json:"attached_statuses_skipped"`
	AirPositions          int64 `json:"air_statuses_skipped"`
	Reclaims              int64 `json:"reclaims_kept"`
	ReclaimsUnbracketed   int64 `json:"reclaims_without_lower_bound"`
}

type evidenceCounters struct {
	Framing           *decodeStats  `json:"framing"`
	Sync              syncCounts    `json:"sync"`
	Joins             evidenceJoins `json:"joins"`
	Lifecycle         extractStats  `json:"lifecycle"`
	UnitData          unitData      `json:"unit_data"`
	RosterValid       bool          `json:"complete_unique_roster"`
	CommandersChecked int           `json:"commanders_checked"`
}

type pathEvidence struct {
	Schema            int                 `json:"schema"`
	Parser            evidenceParser      `json:"parser"`
	Source            evidenceSource      `json:"source"`
	Map               string              `json:"map"`
	MaxUnits          int                 `json:"max_units"`
	CandidateWidth    int                 `json:"candidate_width"`
	Tables            []evidenceTable     `json:"tables"`
	GrammarTable      evidenceTable       `json:"grammar_table"`
	CatalogConfidence string              `json:"catalog_confidence"`
	Admitted          bool                `json:"admitted"`
	Rejections        []evidenceRejection `json:"rejections"`
	Counters          evidenceCounters    `json:"counters"`
	Instances         []*evidenceInstance `json:"instances"`
	Reclaims          []evidenceReclaim   `json:"reclaims"`
	Caveats           []string            `json:"caveats"`
}

func evidenceSHA(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func evidenceParserID() evidenceParser {
	p := evidenceParser{Name: "nanolathe/tad-extract", Version: pathEvidenceVersion, Revision: "unknown"}
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				p.Revision = s.Value
			case "vcs.modified":
				p.Modified = s.Value == "true"
			}
		}
	}
	return p
}

func (e *pathEvidence) reject(code string, count int64) {
	if count > 0 {
		for i := range e.Rejections {
			if e.Rejections[i].Code == code {
				e.Rejections[i].Count += count
				return
			}
		}
		e.Rejections = append(e.Rejections, evidenceRejection{code, count})
	}
}

// evidenceInputs reads every candidate and the grammar table once. Hashes bind
// the exact bytes parsed, including when the same stock file serves both roles.
func evidenceInputs(root, source string, ota bool) ([]*unitTable, []evidenceTable, map[string]bool, evidenceTable) {
	var paths []string
	if versionDir.MatchString(source) {
		paths = append(paths, source+"/mod_units.json")
	}
	if ota {
		if source != "v3.1" {
			paths = append(paths, "v3.1/mod_units.json")
		}
		paths = append(paths, stockTablePath)
	}
	cache := map[string][]byte{}
	read := func(rel string) ([]byte, error) {
		if b, ok := cache[rel]; ok {
			return b, nil
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err == nil {
			cache[rel] = b
		}
		return b, err
	}
	ts, infos := []*unitTable{}, []evidenceTable{}
	for _, rel := range paths {
		info := evidenceTable{Path: rel}
		b, err := read(rel)
		if err != nil {
			info.Error = "unavailable"
		} else {
			info.SHA256 = evidenceSHA(b)
			t, err := parseTable(b, rel)
			if err != nil {
				info.Error = "invalid_json"
			} else {
				info.Rows = t.Real
				if validEvidenceNames(t) {
					ts = append(ts, t)
				} else {
					info.Error = "empty_or_duplicate_unit_name"
				}
			}
		}
		infos = append(infos, info)
	}
	ginfo := evidenceTable{Path: stockTablePath}
	air := map[string]bool{}
	b, err := read(stockTablePath)
	if err != nil {
		ginfo.Error = "unavailable"
		return ts, infos, air, ginfo
	}
	ginfo.SHA256 = evidenceSHA(b)
	var rows []struct {
		Unitname string    `json:"unitname"`
		Roles    *[]string `json:"roles"`
	}
	if json.Unmarshal(b, &rows) != nil {
		ginfo.Error = "invalid_json"
		return ts, infos, air, ginfo
	}
	for _, row := range rows {
		// Absence of roles is unknown, not evidence that a definition is ground.
		if row.Roles == nil || row.Unitname == "" {
			continue
		}
		name := strings.ToUpper(row.Unitname)
		if _, dup := air[name]; dup {
			ginfo.Error = "duplicate_unit"
			continue
		}
		fly := false
		for _, r := range *row.Roles {
			if r == "air" {
				fly = true
			}
		}
		air[name] = fly
	}
	ginfo.Rows = len(air)
	ginfo.Selected = ginfo.Error == "" && len(air) > 0
	return ts, infos, air, ginfo
}

func validEvidenceNames(t *unitTable) bool {
	if len(t.Names) == 0 {
		return false
	}
	for i, name := range t.Names {
		if strings.TrimSpace(name) == "" || (i > 0 && name == t.Names[i-1]) {
			return false
		}
	}
	return true
}

// Unknown filenames are not copied into artifacts: they can contain names.
// Known archive IDs are numeric (version archive) or 40 hex digits (TADA).
func evidenceSourceID(path string, data []byte) (evidenceSource, bool) {
	abs, _ := filepath.Abs(path)
	source := filepath.Base(filepath.Dir(abs))
	id := strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	ota := false
	if versionDir.MatchString(source) && allDigits(id) {
		ota = source == "v3.1"
	} else if source == "files" && filepath.Base(filepath.Dir(filepath.Dir(abs))) == "tada" && isEvidenceArchiveID(id) {
		source, ota = "tada", strings.EqualFold(filepath.Ext(abs), ".tad")
	} else {
		source, id = "external", evidenceSHA(data)
		ota = strings.EqualFold(filepath.Ext(abs), ".tad")
	}
	return evidenceSource{Source: source, ID: id, SHA256: evidenceSHA(data), Bytes: len(data)}, ota
}

func isEvidenceArchiveID(id string) bool {
	if len(id) != 40 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func evidenceMetadata(path string, src *evidenceSource) string {
	b, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".json")
	if err != nil {
		return "metadata_unavailable"
	}
	src.MetadataSHA256 = evidenceSHA(b)
	var meta struct {
		Bytes  *int64 `json:"bytes"`
		SHA256 string `json:"sha256"`
		Detail struct {
			FileSize *int64 `json:"fileSize"`
		} `json:"detail"`
	}
	if json.Unmarshal(b, &meta) != nil {
		return "metadata_invalid"
	}
	if meta.SHA256 != "" {
		decoded, err := hex.DecodeString(meta.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return "metadata_source_sha256_invalid"
		}
		src.ExpectedSHA256 = strings.ToLower(meta.SHA256)
	}
	want := meta.Bytes
	if want == nil && src.Source == "tada" {
		want = meta.Detail.FileSize
	}
	if want == nil || *want <= 0 {
		return "metadata_size_unknown"
	}
	src.ExpectedBytes = want
	if *want != int64(src.Bytes) || (meta.Detail.FileSize != nil && *meta.Detail.FileSize != int64(src.Bytes)) {
		return "metadata_size_mismatch"
	}
	if meta.SHA256 != "" {
		if !strings.EqualFold(meta.SHA256, src.SHA256) {
			return "metadata_source_sha256_mismatch"
		}
	}
	return ""
}

func evidenceRoster(su *setup) bool {
	if su.Header.NumPlayers == 0 || su.Header.MaxUnits <= 0 || len(su.Status) != su.Header.NumPlayers || len(su.Players) != su.Header.NumPlayers {
		return false
	}
	players, statuses, ids := map[int]bool{}, map[int]bool{}, map[uint32]bool{}
	for _, p := range su.Players {
		if players[p.Number] {
			return false
		}
		players[p.Number] = true
	}
	for _, s := range su.Status {
		if s.Error != "" || !s.Checksum || s.DPID != s.DPID2 || statuses[s.Number] || ids[s.DPID] || !players[s.Number] || s.MaxUnits != su.Header.MaxUnits {
			return false
		}
		statuses[s.Number], ids[s.DPID] = true, true
	}
	return true
}

// Both passes read the caller's immutable byte slice. Neither can silently
// drift to a file that changed after its digest was taken.
func extractPathEvidence(data []byte, src evidenceSource, tables []*unitTable, infos []evidenceTable, air map[string]bool, grammar evidenceTable) *pathEvidence {
	src.Bytes, src.SHA256 = len(data), evidenceSHA(data)
	e := &pathEvidence{Schema: pathEvidenceSchema, Parser: evidenceParserID(), Source: src, Tables: infos, GrammarTable: grammar,
		CatalogConfidence: "candidate", Rejections: []evidenceRejection{}, Instances: []*evidenceInstance{}, Reclaims: []evidenceReclaim{},
		Caveats: []string{
			"Admission is complete framing and internal agreement under candidate catalog/grammar tables, not proof of the recording's original installed catalog, mod, map assets or complete recorder coverage.",
			"Positions are scheduled true nonair status observations, never interpolated: x/z are signed raw 16.16 arithmetic-right-shifted by 16 (floor for negatives); raw fixed words are retained. Occupancy mode 1 includes boats and does not mean moving.",
			"Prefixes are retained ground path lists, not actual poses, fresh collisions, issued orders, destinations or player intent. All valid observations are retained, including unchanged lists; no duration is inferred from deduplication.",
			"Lifecycle times are capture-order brackets from the event sender, not exact engine ticks. Both bounds are null without surrounding syncs in the same clock and speed segments. Creation x/y/z retain only low unsigned 16-bit words; high halves remain uninterpreted.",
			"Reclaim x/z are attribute-tile anchors; tick is only the previous same-sender sync lower bound, not exact removal time or proof of the resulting feature state.",
			"Mover scalar speed presence is selected by total body length and the word is skipped; dynamic movement-state lifetime is not proven. Air trajectories, attached poses and unknown grammars are not reconstructed.",
			"A recorded death followed by creation starts a new instance even if the unit ID is reused. Same live type/owner/creation X/Z repeats are counted as duplicate starts; indistinguishable reuse after an unrecorded death remains unknown.",
		}}
	var first *extractor
	su, firstStats, err := decode(bytes.NewReader(data), func(c subContext, b []byte) {
		if first != nil {
			first.onSub(c, b)
		}
	}, func(s *setup) { first = newExtractor(s) })
	e.Map, e.MaxUnits = su.Header.Map, su.Header.MaxUnits
	e.Counters.UnitData = su.UnitData
	e.Counters.RosterValid = evidenceRoster(su)
	if err != nil || first == nil {
		firstStats.End = "setup_failure"
		e.Counters.Framing = firstStats
		e.reject("setup_failure", 1)
		return e
	}
	players := make([]extractPlayer, 0, len(su.Players))
	for _, p := range su.Players {
		ep := extractPlayer{Number: p.Number, Side: p.Side}
		for _, ev := range first.events {
			if ev.sender == p.Number {
				ep.Builds = append(ep.Builds, ev)
			}
		}
		players = append(players, ep)
	}
	table, naming := chooseTable(su, players, tables)
	e.Counters.CommandersChecked = naming.Commanders
	if table == nil {
		e.reject("candidate_table_unmatched", 1)
	} else {
		e.CandidateWidth = bitLen(su.UnitData.Enabled + 1)
		for i := range e.Tables {
			e.Tables[i].Selected = e.Tables[i].Path == table.Name
		}
	}
	if grammar.Error != "" {
		e.reject("grammar_table_unavailable_or_invalid", 1)
	}
	m := &evidenceTracker{out: e, table: table, air: air, byEvent: map[*buildEvent]*evidenceInstance{}, ordinals: map[int]int{}, pending: map[int][]*evidenceBracket{}, lastSync: map[int]subContext{}, lastPose: map[*buildEvent]int64{}, lastPrefix: map[*buildEvent]int64{}}
	m.dec = syncDecoder{W: e.CandidateWidth, grammar: m.grammar}
	_, st, _ := decode(bytes.NewReader(data), m.onSub, func(s *setup) { m.x = newExtractor(s) })
	for _, pending := range m.pending {
		for _, b := range pending {
			b.TickLower = nil // no following sync: no complete capture bracket
		}
	}
	// Only a categorical end marker is exposed, never raw decoder errors.
	if st.End != "eof" {
		st.End = "truncated_or_invalid"
	}
	m.dec.st.AirSelOut = map[string]int64{}
	for selector, count := range m.dec.st.AirSel {
		if count > 0 {
			m.dec.st.AirSelOut[fmt.Sprint(selector)] = count
		}
	}
	e.Counters.Framing, e.Counters.Sync, e.Counters.Lifecycle = st, m.dec.st, m.x.st
	e.admit()
	return e
}

func (e *pathEvidence) admit() {
	st, s, j, l := e.Counters.Framing, e.Counters.Sync, e.Counters.Joins, e.Counters.Lifecycle
	if st.End != "eof" {
		e.reject("framing_not_eof", 1)
	}
	if !e.Counters.RosterValid {
		e.reject("complete_unique_roster_invalid", 1)
	}
	e.reject("unit_data_malformed", int64(e.Counters.UnitData.Malformed))
	// Every check is explicitly listed. No whole setup object or error text
	// can be serialized by adding a field to a different decoder type.
	checks := []evidenceRejection{
		{"checksum_markers", int64(st.ChecksumMarkers)}, {"short_packets", int64(st.ShortPackets)},
		{"lz_unterminated", int64(st.LZUnterminated)}, {"lz_errors", int64(st.LZErrors)}, {"lz_late_matches", int64(st.LZLateMatches)},
		{"unsplit_bytes", st.UnsplitBytes}, {"truncated_subpackets", int64(st.TruncatedSubs)}, {"bad_length_subpackets", int64(st.BadLengthSubs)},
		{"sync_without_tick_base", int64(st.SyncWithoutBase)}, {"tick_resync_gap", int64(st.TickGap)}, {"tick_resync_rewind", int64(st.TickRewind)},
		{"def_mismatch", s.DefMismatch}, {"def_no_start", s.DefNoStart}, {"def_out_of_table", s.DefOutOfTable}, {"grammar_unknown", s.GrammarUnknown},
		{"air_selector3_unknown", s.Selector3}, {"truncated_bodies", s.Truncated}, {"status_length_mismatch", s.LengthMismatch}, {"status_def_mismatch", s.StatusDefMis}, {"bodies_skipped", s.BodiesSkipped},
		{"unknown_sender_bodies", j.UnknownSender}, {"status_no_start", j.StatusNoStart}, {"status_grammar_unknown", j.StatusGrammarUnknown},
		{"status_after_death", j.StatusAfterDeath}, {"prefix_after_death", j.PrefixAfterDeath}, {"status_positions_nonincreasing", j.PositionNonincreasing}, {"prefix_ticks_nonincreasing", j.PrefixNonincreasing},
		{"slot_out_of_range", j.SlotOutOfRange}, {"idle_width_unknown", j.IdleWidthUnknown}, {"ownership_transfer_unknown", j.OwnershipTransfer},
		{"net_reuse_without_death", int64(l.ReuseNoDeath)}, {"duplicate_starts_moved", int64(l.DupMoved)},
		{"block_mismatch", int64(l.BlockMismatch)}, {"block_unknown", int64(l.BlockUnknown)}, {"unknown_start_senders", int64(l.UnknownSenders)},
		{"finish_without_start", int64(l.FinishNoStart)}, {"death_without_start", int64(l.DeathNoStart)},
	}
	for _, c := range checks {
		e.reject(c.Code, c.Count)
	}
	for _, n := range st.UnknownFlags {
		e.reject("unknown_flags", int64(n))
	}
	for _, n := range st.UnknownIDs {
		e.reject("unknown_ids", int64(n))
	}
	if s.DefAgree < 50 {
		e.reject("insufficient_definition_agreement", 1)
	}
	if j.Positions == 0 {
		e.reject("no_nonair_position_observations", 1)
	}
	sort.Slice(e.Rejections, func(i, j int) bool { return e.Rejections[i].Code < e.Rejections[j].Code })
	e.Admitted = len(e.Rejections) == 0
}

type evidenceTracker struct {
	out        *pathEvidence
	table      *unitTable
	air        map[string]bool
	x          *extractor
	dec        syncDecoder
	byEvent    map[*buildEvent]*evidenceInstance
	ordinals   map[int]int
	pending    map[int][]*evidenceBracket
	lastSync   map[int]subContext
	lastPose   map[*buildEvent]int64
	lastPrefix map[*buildEvent]int64
}

func (m *evidenceTracker) grammar(def int) (bool, bool) {
	if m.table == nil || def < 1 || def > m.table.Real {
		return false, false
	}
	a, ok := m.air[m.table.Names[def-1]]
	return a, ok
}

func (m *evidenceTracker) owner(sender int) int {
	if !m.out.Counters.RosterValid {
		return -1
	}
	if n, ok := m.x.blockByNum[sender]; ok {
		return n
	}
	return -1
}

func (m *evidenceTracker) bracket(c subContext) evidenceBracket {
	b := evidenceBracket{WallMs: c.WallMs, ClockSegment: c.ClockSegment, SpeedSegment: c.SpeedSegment, SenderOwnerOrdinal: m.owner(c.Sender)}
	if last, ok := m.lastSync[c.Sender]; ok && last.ClockSegment == c.ClockSegment && last.SpeedSegment == c.SpeedSegment {
		t := last.Tick
		b.TickLower = &t
	}
	return b
}

func (m *evidenceTracker) onSub(c subContext, sp []byte) {
	if m.x == nil {
		return
	}
	before := len(m.x.events)
	m.x.onSub(c, sp)
	switch sp[0] {
	case 0x09:
		if len(m.x.events) == before {
			return
		}
		ev := m.x.events[before]
		owner := m.owner(c.Sender)
		m.ordinals[owner]++
		i := &evidenceInstance{ID: fmt.Sprintf("%s/o%d/i%d", m.out.Source.SHA256, owner, m.ordinals[owner]), OwnerOrdinal: owner, Ordinal: m.ordinals[owner], NetID: ev.NetID, TypeIndex: ev.Type,
			Build: evidenceBuild{evidenceBracket: m.bracket(c), X: ev.X, Y: ev.Y, Z: ev.Z}, Positions: []evidencePosition{}, Prefixes: []evidencePrefix{}}
		if m.table != nil && ev.Type > 0 && ev.Type <= m.table.Real {
			i.UnitName = m.table.Names[ev.Type-1]
		}
		if a, ok := m.grammar(ev.Type); ok {
			i.Air = &a
		}
		m.byEvent[ev] = i
		m.out.Instances = append(m.out.Instances, i)
		m.pending[c.Sender] = append(m.pending[c.Sender], &i.Build.evidenceBracket)
		return
	case 0x12, 0x0c:
		ev := m.x.byNet[int(binary.LittleEndian.Uint16(sp[1:]))]
		i := m.byEvent[ev]
		if i == nil {
			return
		}
		var dest **evidenceBracket
		if sp[0] == 0x12 {
			dest = &i.Finished
		} else {
			dest = &i.Died
		}
		if *dest != nil {
			return
		}
		b := m.bracket(c)
		*dest = &b
		m.pending[c.Sender] = append(m.pending[c.Sender], &b)
		return
	case 0x14:
		// TODO(question): ownership-transfer layout is unresolved [fmt tad §6].
		// Reject the recording rather than reassigning an instance by guess.
		m.out.Counters.Joins.OwnershipTransfer++
		return
	case 0x0f:
		if sp[1] != 0xff {
			return
		}
		b := m.bracket(c)
		if b.TickLower == nil || m.owner(c.Sender) < 0 {
			m.out.Counters.Joins.ReclaimsUnbracketed++
			return
		}
		m.out.Reclaims = append(m.out.Reclaims, evidenceReclaim{m.owner(c.Sender), *b.TickLower, int(binary.LittleEndian.Uint16(sp[2:])), int(binary.LittleEndian.Uint16(sp[4:]))})
		m.out.Counters.Joins.Reclaims++
		return
	case 0xfd, 0x2c, 0xff:
		for _, b := range m.pending[c.Sender] {
			if b.TickLower != nil && b.ClockSegment == c.ClockSegment && b.SpeedSegment == c.SpeedSegment && *b.TickLower <= c.Tick {
				t := c.Tick
				b.TickUpper = &t
			} else {
				b.TickLower = nil
			}
		}
		m.pending[c.Sender] = nil
		m.lastSync[c.Sender] = c
	default:
		return
	}
	if m.table == nil || c.Tick < 0 {
		return
	}
	owner := m.owner(c.Sender)
	if owner < 0 {
		m.out.Counters.Joins.UnknownSender++
		return
	}
	if sp[0] == 0xff {
		m.dec.st.Idle++
		if m.dec.W != 9 {
			m.out.Counters.Joins.IdleWidthUnknown++
		}
		return
	}
	body := sp[3:]
	if sp[0] == 0x2c {
		body = sp[7:]
	}
	var entries []moveEntry
	ss, ok := m.dec.decodeBody(body, func(v moveEntry) { entries = append(entries, v) })
	if !ok {
		return
	} // no partial prefix escapes a malformed body
	mu := m.out.MaxUnits
	base := owner * mu
	for _, v := range entries {
		if v.Slot >= mu {
			m.out.Counters.Joins.SlotOutOfRange++
			continue
		}
		if v.Def < 1 || v.Def > m.table.Real {
			m.dec.st.DefOutOfTable++
			continue
		}
		ev := m.x.byNet[base+v.Slot+1]
		if ev == nil || ev.sender != c.Sender {
			m.dec.st.DefNoStart++
			continue
		}
		if ev.Type != v.Def {
			m.dec.st.DefMismatch++
			continue
		}
		m.dec.st.DefAgree++
		if ev.DiedMs != nil {
			m.out.Counters.Joins.PrefixAfterDeath++
			continue
		}
		if v.Air {
			continue
		}
		if t, seen := m.lastPrefix[ev]; seen && t >= c.Tick {
			m.out.Counters.Joins.PrefixNonincreasing++
			continue
		}
		m.lastPrefix[ev] = c.Tick
		p := evidencePrefix{Tick: c.Tick, Blocked: v.Ground.Blocked, Points: append([][2]int32{}, v.Ground.Pts[:v.Ground.N]...)}
		m.byEvent[ev].Prefixes = append(m.byEvent[ev].Prefixes, p)
		m.out.Counters.Joins.Prefixes++
	}
	if !ss.Present || ss.Def == 0 {
		return
	}
	a, known := m.grammar(ss.Def)
	if !known {
		m.out.Counters.Joins.StatusGrammarUnknown++
		return
	}
	ev := m.x.byNet[base+int(c.Tick%int64(mu))+1]
	if ev == nil || ev.sender != c.Sender {
		m.out.Counters.Joins.StatusNoStart++
		return
	}
	if ev.Type != ss.Def {
		m.dec.st.StatusDefMis++
		return
	}
	m.dec.st.StatusDefAgree++
	if ev.DiedMs != nil {
		m.out.Counters.Joins.StatusAfterDeath++
		return
	}
	if t, seen := m.lastPose[ev]; seen && t >= c.Tick {
		m.out.Counters.Joins.PositionNonincreasing++
		return
	}
	m.lastPose[ev] = c.Tick
	if ss.Attached {
		m.out.Counters.Joins.Attached++
		return
	}
	if a {
		m.out.Counters.Joins.AirPositions++
		return
	}
	m.byEvent[ev].Positions = append(m.byEvent[ev].Positions, evidencePosition{c.Tick, ss.X, ss.Z, ss.RawX, ss.RawZ, ss.Health, ss.Remaining, ss.Flags, ss.Mode})
	m.out.Counters.Joins.Positions++
}

// Manifest paths are relative to evidence-out, with no source filesystem
// paths or metadata payloads. Rejections do not prevent writing partial data.
type evidenceManifestEntry struct {
	Source     evidenceSource      `json:"source"`
	Path       string              `json:"path"`
	SHA256     string              `json:"sha256"`
	Admitted   bool                `json:"admitted"`
	Rejections []evidenceRejection `json:"rejections"`
}

type evidenceManifest struct {
	Schema int                     `json:"schema"`
	Parser evidenceParser          `json:"parser"`
	Files  []evidenceManifestEntry `json:"files"`
}

type evidenceSummary struct {
	Schema     int            `json:"schema"`
	Files      int            `json:"files"`
	Admitted   int            `json:"admitted"`
	Rejected   int            `json:"rejected"`
	Instances  int            `json:"instances"`
	Positions  int64          `json:"positions"`
	Prefixes   int64          `json:"prefixes"`
	Rejections map[string]int `json:"rejected_files_by_reason"`
}

func runPathEvidence(root, out string, paths []string) error {
	manifest := evidenceManifest{Schema: pathEvidenceSchema, Parser: evidenceParserID(), Files: []evidenceManifestEntry{}}
	summary := evidenceSummary{Schema: pathEvidenceSchema, Rejections: map[string]int{}}
	seen := map[string]string{}
	for index, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("evidence input %d: read failed", index+1)
		}
		src, ota := evidenceSourceID(path, data)
		metadataError := evidenceMetadata(path, &src)
		tables, infos, air, grammar := evidenceInputs(root, src.Source, ota)
		e := extractPathEvidence(data, src, tables, infos, air, grammar)
		if metadataError != "" {
			e.reject(metadataError, 1)
			e.Admitted = false
		}
		sort.Slice(e.Rejections, func(i, j int) bool { return e.Rejections[i].Code < e.Rejections[j].Code })
		rel := src.Source + "/" + src.ID + ".json.gz"
		if previous, ok := seen[rel]; ok {
			if previous != src.SHA256 {
				return fmt.Errorf("evidence input %d: output ID conflicts", index+1)
			}
			continue
		}
		seen[rel] = src.SHA256
		dest := filepath.Join(out, filepath.FromSlash(rel))
		if err := writeJSONGz(dest, e); err != nil {
			return fmt.Errorf("evidence input %d: write failed", index+1)
		}
		written, err := os.ReadFile(dest)
		if err != nil {
			return fmt.Errorf("evidence input %d: output digest failed", index+1)
		}
		manifest.Files = append(manifest.Files, evidenceManifestEntry{e.Source, rel, evidenceSHA(written), e.Admitted, e.Rejections})
		summary.Files++
		if e.Admitted {
			summary.Admitted++
		} else {
			summary.Rejected++
		}
		summary.Instances += len(e.Instances)
		summary.Positions += e.Counters.Joins.Positions
		summary.Prefixes += e.Counters.Joins.Prefixes
		for _, r := range e.Rejections {
			summary.Rejections[r.Code]++
		}
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	for _, f := range []struct {
		name  string
		value any
	}{{"manifest.json", manifest}, {"summary.json", summary}} {
		b, err := json.MarshalIndent(f.value, "", "  ")
		if err != nil {
			return fmt.Errorf("evidence %s: encode failed", f.name)
		}
		dest := filepath.Join(out, f.name)
		if err := os.WriteFile(dest+".tmp", append(b, '\n'), 0600); err != nil {
			return fmt.Errorf("evidence %s: write failed", f.name)
		}
		if err := os.Rename(dest+".tmp", dest); err != nil {
			return fmt.Errorf("evidence %s: publish failed", f.name)
		}
	}
	return nil
}
