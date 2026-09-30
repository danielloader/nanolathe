package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Movement extraction (-moves): every unit's recorded builds joined with the
// movement its owner broadcast in unit-sync records [fmt tad §7], as
// per-unit time series. The output joins builds, completion, death and
// movement per unit for offline analysis. Sparse telemetry does not supply
// an exact replay, original player intent or proved catalog identity.
//
// Width and catalog identity. The definition-index width W is the bit
// length of the retained definition count including row zero [fmt tad §7];
// the count itself is recording-specific and Unknown. The reader takes the
// format's labeled candidate — enabled keys plus the reserved row — and
// keeps the movement only when it passes the legacy consistency threshold: entries
// must name, in at least minAgree of cases, the same definition index as the
// unit's own 0x09 start (the same name-sorted index space [fmt tad §6.6]),
// and at least 99% of counted statuses must fit a candidate body length.
// Otherwise the recording falls back to builds only, with the reason recorded.
// The strict path-evidence admission is separate from this legacy threshold.
//
// Grammar. An entry's definition selects the ground or air grammar by the
// definition's canfly [fmt tad §7]. canfly comes from the engine stock
// catalog summary (derived/unit_table.json, whose "air" role is set exactly
// when a definition authors canfly), joined by unit name through the
// recording's matching candidate table. A definition outside that summary (a
// catalog unit the stock install lacks) has no known grammar: the rest of
// that body is skipped and counted.

// minAgree is the share of movement entries whose definition index must
// equal the recorded start type for the legacy consistency threshold.
const (
	minAgreePermille = 990
	minAgreeEntries  = 50
)

// unitTrack is one recorded unit: its start, completion and death, and its
// movement. Ticks are the owner's unit-sync ticks (30 per second of game
// time at normal speed [fmt tad §9]); positions are whole world units
// (map pixels).
type unitTrack struct {
	NetID int    `json:"net_id"`
	Type  int    `json:"type"`
	Unit  string `json:"unit,omitempty"`
	Air   *bool  `json:"air,omitempty"`
	// Legacy markers: last sync before the start and first after (-1 when
	// none), without segment validation. Use path-evidence for §9 brackets.
	Tick     int64  `json:"tick"`
	TickNext int64  `json:"tick_next"`
	X        int    `json:"x"`
	Z        int    `json:"z"`
	Builder  int    `json:"builder,omitempty"`
	FinTick  *int64 `json:"finished_tick,omitempty"`
	DiedTick *int64 `json:"died_tick,omitempty"`
	// KillerPlayer is the recorder number credited with the death, zero
	// when absent.
	KillerPlayer int `json:"killer_player,omitempty"`
	// Path holds ground entries [tick, blocked, n, x1, z1, ..., xn, zn]:
	// the blocked flag and the first n (0..3) retained path points
	// [fmt tad §7 "Ground movement entry"]. An entry identical to the
	// previous one is dropped.
	Path [][]int32 `json:"path,omitempty"`
	// AirGoal holds [tick, x, z] from selector-1 goal positions (flag bit
	// 5), AirFollow [tick, followed unit ID] (flag bit 0), AirPos [tick, x,
	// z] from selector-2 positions [fmt tad §7 "Air movement entry"].
	// Repeats of the previous value are dropped.
	AirGoal   [][3]int32 `json:"air_goal,omitempty"`
	AirFollow [][2]int32 `json:"air_follow,omitempty"`
	AirPos    [][3]int32 `json:"air_pos,omitempty"`
	// Pos holds scheduled statuses [tick, x, z, health, remaining, flags]:
	// the unit's position, health word, construction-remaining byte and
	// activation/status flags (bit 3: building), sent
	// when the sender's tick modulo maxUnits reaches its slot
	// [fmt tad §7 "Scheduled status"]. Attached statuses carry no position
	// and are not listed.
	Pos [][6]int32 `json:"pos,omitempty"`
	// Pose holds, for each Pos row in the same order, [tick, raw X, raw Z,
	// heading, speed]: the exact signed 16.16 position words, the heading
	// angle16 and the signed 16.16 mover scalar speed, or -1 when the status
	// carried no speed word [fmt tad §7 "Scheduled status"].
	Pose [][5]int32 `json:"pose,omitempty"`
	// Hurt holds [tick bucket, damage] and Fire [tick bucket, shots]: damage
	// records naming this unit as victim (0x0b, the low amount word summed)
	// and projectile creations naming it as shooter (0x0d), each bucketed to
	// combatBucket sender ticks [fmt tad §6.1] [fmt tad §6.3]. They exist to
	// tell a unit that stands still because it fights from one that is held.
	Hurt [][2]int32 `json:"hurt,omitempty"`
	Fire [][2]int32 `json:"fire,omitempty"`

	ev *buildEvent
}

// combatBucket is the width, in sender ticks, of the Hurt and Fire buckets.
const combatBucket = 30

type movesPlayer struct {
	Number int          `json:"number"`
	Side   int          `json:"side"`
	Block  *int         `json:"block,omitempty"`
	Units  []*unitTrack `json:"units"`
	// Reclaims holds the player's feature reclaim transitions [tick, tile
	// X, tile Z]: 0x0f records with code 0xff and the feature's anchor
	// tile [fmt tad §6.4], sent by this player. The tick is the sender's
	// last sync tick before the record, the lower end of its capture
	// bracket [fmt tad §9].
	Reclaims [][3]int32 `json:"reclaims,omitempty"`
	// Economy holds the player's participant snapshots [tick, metal
	// produced, energy produced, metal stock, energy stock, kills, losses]:
	// 0x28 records sent by this player, cumulative produced totals and
	// current stocks truncated to whole units [fmt tad §8]. The tick is the
	// sender's last sync tick before the record.
	Economy [][7]int32 `json:"economy,omitempty"`
}

// movesExtract is the -moves output for one recording.
type movesExtract struct {
	Source string `json:"source"`
	ID     string `json:"id"`
	// File is the recording's base name; its extension tells an original
	// TA recording (.tad) from a ProTA one (.pro).
	File     string `json:"file,omitempty"`
	Map      string `json:"map"`
	MaxUnits int    `json:"max_units"`
	Recorder string `json:"recorder,omitempty"`
	Table    string `json:"table,omitempty"`
	// Status: "moves" (legacy consistency threshold passed), "builds_only" (see
	// Reason) or "failed".
	Status   string        `json:"status"`
	Reason   string        `json:"reason,omitempty"`
	Width    int           `json:"width,omitempty"`
	LastTick int64         `json:"last_tick"`
	Sync     *syncCounts   `json:"sync,omitempty"`
	Extract  *extractStats `json:"extract,omitempty"`
	Tracks   trackCounts   `json:"tracks"`
	Players  []movesPlayer `json:"players"`
	Seconds  float64       `json:"seconds"`
}

// trackCounts counts the joins between movement and starts.
type trackCounts struct {
	UnknownSender    int64 `json:"unknown_sender_bodies"`
	PathKept         int64 `json:"path_entries_kept"`
	PathRepeat       int64 `json:"path_entries_repeated"`
	AirKept          int64 `json:"air_values_kept"`
	PosKept          int64 `json:"status_positions_kept"`
	PosOutOfOrder    int64 `json:"status_positions_out_of_order"`
	StatusNoStart    int64 `json:"status_no_start"`
	StatusAttachSkip int64 `json:"status_attached"`
	Reclaims         int64 `json:"reclaim_transitions"`
	Snapshots        int64 `json:"participant_snapshots"`
}

// grammarTable, when set by -grammar-table, replaces the mirror's stock
// catalog summary as the source of canfly by unit name, so recordings of a
// catalog with units the stock install lacks can choose a grammar for them.
var grammarTable string

// airNames loads canfly by unit name from the engine stock catalog summary.
func airNames(root string) (map[string]bool, error) {
	p := filepath.Join(root, stockTablePath)
	if grammarTable != "" {
		p = grammarTable
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Unitname string   `json:"unitname"`
		Roles    []string `json:"roles"`
	}
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rows {
		air := false
		for _, role := range r.Roles {
			if role == "air" {
				air = true
			}
		}
		out[strings.ToUpper(r.Unitname)] = air
	}
	return out, nil
}

// moveTracker joins decoded movement to the extractor's build events.
type moveTracker struct {
	x        *extractor
	reclaims map[int][][3]int32 // sender → reclaim transitions
	economy  map[int][][7]int32 // sender → participant snapshots
	dec      syncDecoder
	tracks   map[*buildEvent]*unitTrack
	tc       trackCounts
	last     int64
}

func (m *moveTracker) track(ev *buildEvent) *unitTrack {
	t := m.tracks[ev]
	if t == nil {
		t = &unitTrack{ev: ev}
		m.tracks[ev] = t
	}
	return t
}

func (m *moveTracker) onSub(ctx subContext, sp []byte) {
	m.x.onSub(ctx, sp)
	var body []byte
	switch sp[0] {
	case 0x0f:
		// Feature record: code u8 at 1, anchor tile X u16 at 2, tile Z u16
		// at 4; code 0xff requests the reclaim transition [fmt tad §6.4].
		if sp[1] == 0xff && ctx.Tick >= 0 {
			tx, tz := int32(binary.LittleEndian.Uint16(sp[2:])), int32(binary.LittleEndian.Uint16(sp[4:]))
			m.reclaims[ctx.Sender] = append(m.reclaims[ctx.Sender], [3]int32{int32(ctx.Tick), tx, tz})
			m.tc.Reclaims++
		}
		return
	case 0x0b:
		// Damage: victim ID at 1, amount low word at 5 [fmt tad §6.1].
		if ctx.Tick >= 0 {
			le := binary.LittleEndian
			if ev := m.x.byNet[int(le.Uint16(sp[1:]))]; ev != nil {
				t := m.track(ev)
				t.Hurt = bump(t.Hurt, int32(ctx.Tick), int32(int16(le.Uint16(sp[5:]))))
			}
		}
		return
	case 0x0d:
		// Projectile creation: shooter unit ID at 33 [fmt tad §6.3].
		if ctx.Tick >= 0 && len(sp) >= 35 {
			if ev := m.x.byNet[int(binary.LittleEndian.Uint16(sp[33:]))]; ev != nil {
				t := m.track(ev)
				t.Fire = bump(t.Fire, int32(ctx.Tick), 1)
			}
		}
		return
	case 0x28:
		// Participant snapshot: kills i32 at 2, losses at 6, metal and
		// energy stock f32 at 18 and 22, cumulative energy produced f32
		// at 34, cumulative metal produced at 46 [fmt tad §8].
		if ctx.Tick >= 0 {
			le := binary.LittleEndian
			f := func(o int) int32 { return int32(math.Float32frombits(le.Uint32(sp[o:]))) }
			m.economy[ctx.Sender] = append(m.economy[ctx.Sender], [7]int32{int32(ctx.Tick),
				f(46), f(34), f(18), f(22), int32(le.Uint32(sp[2:])), int32(le.Uint32(sp[6:]))})
			m.tc.Snapshots++
		}
		return
	case 0xfd:
		body = sp[3:]
	case 0x2c:
		body = sp[7:]
	case 0xff:
		m.dec.st.Idle++
		return
	default:
		return
	}
	if ctx.Tick < 0 {
		return
	}
	if ctx.Tick > m.last {
		m.last = ctx.Tick
	}
	rank, ok := m.x.blockByNum[ctx.Sender]
	mu := m.x.su.Header.MaxUnits
	if !ok || mu <= 0 {
		m.tc.UnknownSender++
		return
	}
	// Local slots are relative to the sender's own unit-ID block; unit ID
	// = block base + slot + 1 [fmt tad §7 "identity and repetition"].
	base := rank * mu
	tick := int32(ctx.Tick)
	status, ok := m.dec.decodeBody(body, func(e moveEntry) {
		ev := m.x.byNet[base+e.Slot+1]
		if ev == nil || ev.sender != ctx.Sender {
			m.dec.st.DefNoStart++
			return
		}
		if ev.Type != e.Def {
			m.dec.st.DefMismatch++
			return
		}
		m.dec.st.DefAgree++
		t := m.track(ev)
		if !e.Air {
			g := e.Ground
			row := make([]int32, 0, 3+2*g.N)
			bl := int32(0)
			if g.Blocked {
				bl = 1
			}
			row = append(row, tick, bl, int32(g.N))
			for i := 0; i < g.N; i++ {
				row = append(row, g.Pts[i][0], g.Pts[i][1])
			}
			if n := len(t.Path); n > 0 && samePath(t.Path[n-1], row) {
				m.tc.PathRepeat++
				return
			}
			t.Path = append(t.Path, row)
			m.tc.PathKept++
			return
		}
		a := e.AirMv
		if a.HasGoal {
			if n := len(t.AirGoal); n == 0 || t.AirGoal[n-1][1] != a.GoalX || t.AirGoal[n-1][2] != a.GoalZ {
				t.AirGoal = append(t.AirGoal, [3]int32{tick, a.GoalX, a.GoalZ})
				m.tc.AirKept++
			}
		}
		if a.Target != 0 {
			if n := len(t.AirFollow); n == 0 || t.AirFollow[n-1][1] != int32(a.Target) {
				t.AirFollow = append(t.AirFollow, [2]int32{tick, int32(a.Target)})
				m.tc.AirKept++
			}
		}
		if a.HasPos {
			if n := len(t.AirPos); n == 0 || t.AirPos[n-1][1] != a.PosX || t.AirPos[n-1][2] != a.PosZ {
				t.AirPos = append(t.AirPos, [3]int32{tick, a.PosX, a.PosZ})
				m.tc.AirKept++
			}
		}
	})
	if !ok || !status.Present || status.Def == 0 {
		return
	}
	// The scheduled slot is the sender's tick modulo maxUnits [fmt tad §7].
	ev := m.x.byNet[base+int(ctx.Tick%int64(mu))+1]
	if ev == nil || ev.sender != ctx.Sender {
		m.tc.StatusNoStart++
		return
	}
	if ev.Type != status.Def {
		m.dec.st.StatusDefMis++
		return
	}
	m.dec.st.StatusDefAgree++
	if status.Attached {
		m.tc.StatusAttachSkip++
		return
	}
	t := m.track(ev)
	if n := len(t.Pos); n > 0 && t.Pos[n-1][0] >= tick {
		m.tc.PosOutOfOrder++
		return
	}
	t.Pos = append(t.Pos, [6]int32{tick, status.X, status.Z, status.Health, int32(status.Remaining), int32(status.Flags)})
	speed := int32(-1)
	if status.HasSpeed {
		speed = status.Speed
	}
	t.Pose = append(t.Pose, [5]int32{tick, status.RawX, status.RawZ, int32(status.Heading), speed})
	m.tc.PosKept++
}

// bump adds v to the bucket holding tick, appending a bucket when tick has
// moved past the last one. Records arrive in capture order per sender, so a
// tick behind the last bucket is folded into it.
func bump(rows [][2]int32, tick, v int32) [][2]int32 {
	b := tick - tick%combatBucket
	if n := len(rows); n > 0 && rows[n-1][0] >= b {
		rows[n-1][1] += v
		return rows
	}
	return append(rows, [2]int32{b, v})
}

func samePath(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 1; i < len(a); i++ { // [0] is the tick
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// extractMoves reads one recording twice: first as the build extract does,
// to verify its unit table, then with the unit-sync decoder.
func extractMoves(path string, d demo, air map[string]bool) (*movesExtract, error) {
	t0 := time.Now()
	ex, err := extractFile(path, d.tables)
	if err != nil {
		return nil, err
	}
	out := &movesExtract{Source: d.source, ID: d.id, File: filepath.Base(path), Players: []movesPlayer{}}
	if ex.Setup != nil {
		out.Map, out.MaxUnits, out.Recorder = ex.Setup.Header.Map, ex.Setup.Header.MaxUnits, ex.Setup.Recorder
	}
	out.Table = ex.Naming.Table
	if ex.Status == "failed" || ex.Setup == nil {
		out.Status, out.Reason = "failed", ex.Error
		return out, nil
	}
	var table *unitTable
	for _, t := range d.tables {
		if t.Name == ex.Naming.Table {
			table = t
		}
	}
	width := 0
	if table != nil {
		// The format's labeled candidate: enabled keys plus the reserved row.
		width = bitLen(ex.Setup.UnitData.Enabled + 1)
	}
	grammar := func(def int) (bool, bool) {
		if table == nil || def < 1 || def > table.Real {
			return false, false
		}
		a, ok := air[table.Names[def-1]]
		return a, ok
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var mt *moveTracker
	decodeMoves := table != nil
	su, _, derr := decode(f, func(ctx subContext, sp []byte) {
		if mt != nil {
			mt.onSub(ctx, sp)
		}
	}, func(su *setup) {
		mt = &moveTracker{x: newExtractor(su), tracks: map[*buildEvent]*unitTrack{}, reclaims: map[int][][3]int32{}, economy: map[int][][7]int32{}}
		mt.dec = syncDecoder{W: width, grammar: grammar}
		if !decodeMoves {
			mt.dec.W = 0
		}
	})
	if derr != nil || mt == nil {
		out.Status, out.Reason = "failed", fmt.Sprint(derr)
		return out, nil
	}
	if !decodeMoves {
		// Without a matching candidate table neither the width candidate nor the
		// grammar can be tied to the recording; keep builds only. The
		// decoder still ran with width 0 so the build joins are the same.
		out.Status, out.Reason = "builds_only", "no verified unit table: "+ex.Naming.Reason
	}
	st := mt.dec.st
	st.AirSelOut = map[string]int64{}
	for i, n := range st.AirSel {
		if n > 0 {
			st.AirSelOut[fmt.Sprint(i)] = n
		}
	}
	out.Sync, out.Extract, out.Tracks, out.LastTick = &st, &mt.x.st, mt.tc, mt.last
	if decodeMoves {
		out.Width = width
		agree, all := st.DefAgree, st.DefAgree+st.DefMismatch
		switch {
		case all < minAgreeEntries:
			out.Status, out.Reason = "builds_only", fmt.Sprintf("width %d unconfirmed: only %d movement entries joined a start", width, all)
		case agree*1000 < all*minAgreePermille:
			out.Status, out.Reason = "builds_only", fmt.Sprintf("width %d unconfirmed: %d of %d movement definitions match their starts", width, agree, all)
		case st.LengthMismatch*1000 > (st.StatusAbsent+st.StatusEmpty+st.StatusAttached+st.StatusPos)*(1000-minAgreePermille):
			out.Status, out.Reason = "builds_only", fmt.Sprintf("width %d unconfirmed: %d status bodies fit no length", width, st.LengthMismatch)
		default:
			out.Status = "moves"
		}
	}
	keepMoves := out.Status == "moves"
	byNum := map[int]int{}
	for _, p := range su.Players {
		mp := movesPlayer{Number: p.Number, Side: p.Side, Units: []*unitTrack{}, Reclaims: mt.reclaims[p.Number], Economy: mt.economy[p.Number]}
		if r, ok := mt.x.blockByNum[p.Number]; ok {
			mp.Block = &r
		}
		byNum[p.Number] = len(out.Players)
		out.Players = append(out.Players, mp)
	}
	for _, ev := range mt.x.events {
		i, ok := byNum[ev.sender]
		if !ok {
			continue
		}
		t := mt.tracks[ev]
		if t == nil || !keepMoves {
			t = &unitTrack{ev: ev}
		}
		t.NetID, t.Type, t.Tick, t.TickNext, t.X, t.Z = ev.NetID, ev.Type, ev.Tick, ev.TickNext, ev.X, ev.Z
		t.Builder, t.FinTick, t.DiedTick, t.KillerPlayer = ev.Builder, ev.FinTick, ev.DiedTick, ev.KillerPlayer
		if table != nil && ev.Type >= 1 && ev.Type <= table.Real {
			t.Unit = table.Names[ev.Type-1]
			if a, ok := air[t.Unit]; ok {
				t.Air = &a
			}
		}
		out.Players[i].Units = append(out.Players[i].Units, t)
	}
	out.Seconds = time.Since(t0).Seconds()
	return out, nil
}

// runMoves writes movement extracts for the listed recordings (paths, or
// source names as for the build extract) under out.
func runMoves(root, out string, args []string) error {
	air, err := airNames(root)
	if err != nil {
		return fmt.Errorf("engine stock unit summary: %w", err)
	}
	var demos []demo
	var sources []string
	for _, a := range args {
		if strings.HasSuffix(a, ".tad") || strings.HasSuffix(a, ".pro") {
			demos = append(demos, demoForPath(root, a))
		} else {
			sources = append(sources, a)
		}
	}
	if len(sources) > 0 || len(args) == 0 {
		ds, err := listDemos(root, sources)
		if err != nil {
			return err
		}
		demos = append(demos, ds...)
	}
	for _, d := range demos {
		me, err := extractMoves(d.path, d, air)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s/%s: %v\n", d.source, d.id, err)
			continue
		}
		p := filepath.Join(out, d.source, d.id+".json.gz")
		if err := writeJSONGz(p, me); err != nil {
			return err
		}
		agree := int64(0)
		if me.Sync != nil {
			agree = me.Sync.DefAgree
		}
		fmt.Fprintf(os.Stderr, "%s/%s: %s %s (width %d, %d entries joined, %.1fs)\n", d.source, d.id, me.Status, me.Reason, me.Width, agree, me.Seconds)
	}
	return nil
}
