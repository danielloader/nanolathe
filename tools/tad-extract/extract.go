package main

import (
	"encoding/binary"
	"sort"
)

// buildEvent is one unit whose construction started, from a 0x09 record
// [fmt tad §6], joined with its later 0x12 completion and 0x0c death.
//
// Coordinates. The format lists u32 x, y, z at offsets 7, 11 and 15 in
// map-pixel units. X, Y and Z here are the low 16 bits of those words; Hi
// keeps the high 16 bits, uninterpreted, whenever any is nonzero. See
// README.md "Coordinates" for the corpus evidence behind the split.
type buildEvent struct {
	TMs  int64 `json:"t_ms"`
	Tick int64 `json:"tick"`
	// TickNext is the first unit-sync tick reconstructed from the same
	// sender after the start, or -1 when none followed. These legacy markers
	// do not validate clock/speed continuity; path-evidence owns §9 brackets.
	TickNext int64      `json:"tick_next"`
	Type     int        `json:"type"`
	Unit     string     `json:"unit,omitempty"`
	X        int        `json:"x"`
	Z        int        `json:"z"`
	Y        int        `json:"y"`
	Hi       *[3]uint16 `json:"hi,omitempty"`
	NetID    int        `json:"net_id"`
	Dups     int        `json:"dup,omitempty"`
	FinMs    *int64     `json:"finished_ms,omitempty"`
	FinTick  *int64     `json:"finished_tick,omitempty"`
	Builder  int        `json:"builder,omitempty"`
	DiedMs   *int64     `json:"died_ms,omitempty"`
	DiedTick *int64     `json:"died_tick,omitempty"`
	// KillerPlayer is the recorder number of the attacker's transport
	// identity, zero when absent or unmatched; KillerUnit is the attacker
	// unit ID, zero when absent [fmt tad §6.2].
	KillerPlayer int `json:"killer_player,omitempty"`
	KillerUnit   int `json:"killer_unit,omitempty"`

	sender int
}

// extractStats counts lifecycle joins and checks made while extracting.
type extractStats struct {
	Starts         int `json:"starts"`
	Duplicates     int `json:"duplicate_starts"`
	DupMoved       int `json:"duplicate_starts_moved"`
	ReuseNoDeath   int `json:"net_reuse_without_death"`
	Finishes       int `json:"finishes"`
	FinishNoStart  int `json:"finish_without_start"`
	FinishRepeat   int `json:"finish_repeat"`
	Deaths         int `json:"deaths"`
	DeathNoStart   int `json:"death_without_start"`
	DeathRepeat    int `json:"death_repeat"`
	BlockOK        int `json:"block_ok"`
	BlockMismatch  int `json:"block_mismatch"`
	BlockUnknown   int `json:"block_unknown"`
	UnknownSenders int `json:"unknown_senders"`
	HighWords      int `json:"starts_with_high_words"`
	// Bytes of 0x09 fields the format marks Unknown (offsets 5..6 and
	// 19..22) that were skipped without interpretation.
	UnknownFieldBytes int64 `json:"unknown_field_bytes_skipped"`
	ChatSkipped       int   `json:"chat_subpackets_skipped"`
}

type extractor struct {
	su     *setup
	events []*buildEvent
	byNet  map[int]*buildEvent
	st     extractStats

	playerByNum map[int]*player
	numByDPID   map[uint32]int
	blockByNum  map[int]int // recorder number -> unit-ID block rank

	// pending holds, per sender, starts still waiting for their next sync.
	pending map[int][]*buildEvent

	// Distinct values seen in the 0x09 Unknown fields, kept only as a count
	// for the corpus report; no meaning is assigned.
	unk5  map[uint16]bool
	unk19 map[uint32]bool
}

func newExtractor(su *setup) *extractor {
	x := &extractor{
		su: su, byNet: map[int]*buildEvent{},
		playerByNum: map[int]*player{}, numByDPID: map[uint32]int{},
		blockByNum: map[int]int{}, pending: map[int][]*buildEvent{},
		unk5: map[uint16]bool{}, unk19: map[uint32]bool{},
	}
	for i := range su.Players {
		x.playerByNum[su.Players[i].Number] = &su.Players[i]
	}
	// Unit-ID blocks: participants, watchers included, sorted by unsigned
	// transport identity; rank r owns IDs r*maxUnits+1 .. (r+1)*maxUnits
	// [fmt tad §4 "Recording sender, transport identity and unit block"].
	// The join is kept only when every participant has a decoded identity.
	type ident struct {
		num  int
		dpid uint32
	}
	var ids []ident
	for _, s := range su.Status {
		if s.Error == "" {
			ids = append(ids, ident{s.Number, s.DPID})
			x.numByDPID[s.DPID] = s.Number
		}
	}
	if len(ids) == len(su.Players) {
		sort.Slice(ids, func(i, j int) bool { return ids[i].dpid < ids[j].dpid })
		for r, id := range ids {
			x.blockByNum[id.num] = r
		}
	}
	return x
}

func (x *extractor) onSub(ctx subContext, sp []byte) {
	le := binary.LittleEndian
	switch sp[0] {
	case 0xfd, 0xff, 0x2c:
		if evs := x.pending[ctx.Sender]; len(evs) > 0 {
			for _, ev := range evs {
				ev.TickNext = ctx.Tick
			}
			x.pending[ctx.Sender] = evs[:0]
		}
	case 0x05, 0xf9:
		// Chat is privacy-sensitive and never decoded [fmt tad §6].
		x.st.ChatSkipped++
	case 0x09:
		x.buildStarted(ctx, sp)
	case 0x12:
		// Build finished: built unit ID at 1, builder unit ID at 3.
		built, builder := int(le.Uint16(sp[1:])), int(le.Uint16(sp[3:]))
		x.st.Finishes++
		ev := x.byNet[built]
		switch {
		case ev == nil:
			x.st.FinishNoStart++
		case ev.FinMs != nil:
			x.st.FinishRepeat++
		default:
			t, tick := ctx.WallMs, ctx.Tick
			ev.FinMs, ev.FinTick, ev.Builder = &t, &tick, builder
		}
	case 0x0c:
		// Death: victim ID at 1, attacker transport identity at 3, attacker
		// unit ID at 7 [fmt tad §6.2]. Severity and cause are not extracted.
		victim := int(le.Uint16(sp[1:]))
		attackerDP := le.Uint32(sp[3:])
		attacker := int(le.Uint16(sp[7:]))
		x.st.Deaths++
		ev := x.byNet[victim]
		switch {
		case ev == nil:
			x.st.DeathNoStart++
		case ev.DiedMs != nil:
			x.st.DeathRepeat++
		default:
			t, tick := ctx.WallMs, ctx.Tick
			ev.DiedMs, ev.DiedTick = &t, &tick
			ev.KillerUnit = attacker
			if attackerDP != 0xffffffff {
				ev.KillerPlayer = x.numByDPID[attackerDP]
			}
		}
	}
}

func (x *extractor) buildStarted(ctx subContext, sp []byte) {
	le := binary.LittleEndian
	typ := int(le.Uint16(sp[1:]))
	net := int(le.Uint16(sp[3:]))
	wx, wy, wz := le.Uint32(sp[7:]), le.Uint32(sp[11:]), le.Uint32(sp[15:])
	x.st.UnknownFieldBytes += 6
	x.unk5[le.Uint16(sp[5:])] = true
	x.unk19[le.Uint32(sp[19:])] = true
	x.st.Starts++

	if x.playerByNum[ctx.Sender] == nil {
		x.st.UnknownSenders++
	}
	// The same start can be sent twice for one net ID [fmt tad §6]. A live
	// event with the same type and sender is treated as that repeat; after
	// a recorded death the ID may be reused by a new unit.
	// TODO(question): indistinguishable reuse after an unrecorded death
	// needs recording-specific lifecycle evidence. Path admission rejects
	// moved duplicate starts, but cannot prove that all deaths were captured.
	if prev := x.byNet[net]; prev != nil && prev.DiedMs == nil {
		if prev.Type == typ && prev.sender == ctx.Sender {
			x.st.Duplicates++
			prev.Dups++
			if prev.X != int(wx&0xffff) || prev.Z != int(wz&0xffff) {
				x.st.DupMoved++
			}
			return
		}
		x.st.ReuseNoDeath++
	}

	if rank, ok := x.blockByNum[ctx.Sender]; ok && net != 0 {
		mu := x.su.Header.MaxUnits
		if mu > 0 && (net-1)/mu == rank {
			x.st.BlockOK++
		} else {
			x.st.BlockMismatch++
		}
	} else {
		x.st.BlockUnknown++
	}

	ev := &buildEvent{
		TMs: ctx.WallMs, Tick: ctx.Tick, TickNext: -1, Type: typ,
		X: int(wx & 0xffff), Y: int(wy & 0xffff), Z: int(wz & 0xffff),
		NetID: net, sender: ctx.Sender,
	}
	if hx, hy, hz := uint16(wx>>16), uint16(wy>>16), uint16(wz>>16); hx|hy|hz != 0 {
		ev.Hi = &[3]uint16{hx, hy, hz}
		x.st.HighWords++
	}
	x.byNet[net] = ev
	x.events = append(x.events, ev)
	x.pending[ctx.Sender] = append(x.pending[ctx.Sender], ev)
}
