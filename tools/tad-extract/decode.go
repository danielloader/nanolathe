package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// setup is everything before the match packets: the header, the version-5
// extra sectors, the player and status records and the stored unit data
// [fmt tad "Format at a glance"].
type setup struct {
	Header   header         `json:"header"`
	Recorder string         `json:"recorder,omitempty"`
	Date     string         `json:"date,omitempty"`
	Sectors  map[string]int `json:"sector_types"`
	Players  []player       `json:"-"`
	Status   []playerInfo   `json:"status"`
	UnitData unitData       `json:"unit_data"`
}

// unitData summarizes the stored 0x1a unit-data records [fmt tad §7
// "Catalog provenance and recording-specific width"]. Enabled counts distinct
// compatibility keys whose subtype-3 record has both negotiation bytes
// nonzero; the all-ones key is the recorder's list checksum, not a unit.
type unitData struct {
	Records   int `json:"records"`
	Malformed int `json:"malformed_bytes,omitempty"`
	Keys      int `json:"distinct_keys"`
	Enabled   int `json:"enabled_keys"`
	Checksums int `json:"checksum_keys"`
}

func parseUnitData(b []byte) unitData {
	var u unitData
	all := map[uint32]bool{}
	enabled := map[uint32]bool{}
	sums := map[uint32]bool{}
	le := binary.LittleEndian
	for len(b) >= 14 && b[0] == 0x1a {
		rec := b[:14]
		b = b[14:]
		u.Records++
		key := le.Uint32(rec[6:])
		if key == 0xffffffff {
			continue
		}
		all[key] = true
		switch rec[1] {
		case 2:
			sums[key] = true
		case 3:
			if rec[10] != 0 && rec[11] != 0 {
				enabled[key] = true
			}
		}
	}
	u.Malformed = len(b)
	u.Keys, u.Enabled, u.Checksums = len(all), len(enabled), len(sums)
	return u
}

// decodeStats counts what the packet walk saw. Unknown and skipped material
// is counted, never interpreted.
type decodeStats struct {
	Records          int              `json:"records"`
	Packets          int              `json:"packets"`
	Plain            int              `json:"plain"`
	Compressed       int              `json:"compressed"`
	ChecksumMarkers  int              `json:"checksum_markers"`
	RepeatedPackets  int              `json:"repeated_packets"`
	PlayerInfoLong   int              `json:"player_info_200"`
	PlayerInfoRetail int              `json:"player_info_186"`
	UnknownFlags     map[string]int   `json:"unknown_flags,omitempty"`
	ShortPackets     int              `json:"short_packets,omitempty"`
	LZUnterminated   int              `json:"lz_unterminated,omitempty"`
	LZErrors         int              `json:"lz_errors,omitempty"`
	LZLateMatches    int              `json:"lz_late_matches,omitempty"`
	LZMaxOut         int              `json:"lz_max_out"`
	Subpackets       map[string]int64 `json:"subpackets"`
	UnknownIDs       map[string]int   `json:"unknown_ids,omitempty"`
	UnsplitBytes     int64            `json:"unsplit_bytes,omitempty"`
	TruncatedSubs    int              `json:"truncated_subpackets,omitempty"`
	BadLengthSubs    int              `json:"bad_length_subpackets,omitempty"`
	SyncWithoutBase  int              `json:"sync_without_tick_base,omitempty"`
	TickChecks       int              `json:"tick_resync_checks"`
	TickEqual        int              `json:"tick_resync_equal"`
	TickRewind       int              `json:"tick_resync_rewind"`
	TickGap          int              `json:"tick_resync_gap"`
	WallMs           int64            `json:"wall_ms"`
	End              string           `json:"end"`
	EndOffset        int64            `json:"end_offset"`
	subCounts        [256]int64
	unknownIDCounts  [256]int
	unknownFlagCount [256]int
}

func (s *decodeStats) finish() {
	s.Subpackets = map[string]int64{}
	for id, n := range s.subCounts {
		if n > 0 {
			s.Subpackets[fmt.Sprintf("0x%02x", id)] = n
		}
	}
	for id, n := range s.unknownIDCounts {
		if n > 0 {
			if s.UnknownIDs == nil {
				s.UnknownIDs = map[string]int{}
			}
			s.UnknownIDs[fmt.Sprintf("0x%02x", id)] = n
		}
	}
	for f, n := range s.unknownFlagCount {
		if n > 0 {
			if s.UnknownFlags == nil {
				s.UnknownFlags = map[string]int{}
			}
			s.UnknownFlags[fmt.Sprintf("0x%02x", f)] = n
		}
	}
}

// subContext locates one subpacket in capture order.
type subContext struct {
	WallMs int64 // cumulative record deltas since the first match packet [fmt tad §9]
	Sender int   // recorder participant number [fmt tad §4]
	// Tick is the last unit-sync tick reconstructed from this sender up to
	// and including this subpacket, or -1. Non-sync records have no point
	// tick; this is only the lower end of a capture-order bracket [fmt tad §9].
	Tick int64
	// Segments change at a discontinuity or any speed record. They support
	// conservative capture brackets, not an inferred event tick [fmt tad §9].
	ClockSegment uint32
	SpeedSegment uint32
}

type subHandler func(ctx subContext, sp []byte)

type senderClock struct {
	lastBody []byte // sender's previous stored packet body, copied before reuse
	hasBody  bool
	counter  int64
	last     int64
	base     bool // a 0xfe or raw 0x2c has set the counter
	seen     bool // at least one sync tick has been reconstructed
	segment  uint32
}

var errSetup = errors.New("setup")

// decode reads a whole recording. It calls onSetup once the setup records
// are read and onSub for every split subpacket of every match packet. It
// returns the setup and statistics even when the input is truncated or
// corrupt; err is non-nil only when the setup itself could not be read.
func decode(r io.Reader, onSub subHandler, onSetup func(*setup)) (*setup, *decodeStats, error) {
	rr := newRecordReader(r)
	st := &decodeStats{}
	su := &setup{Sectors: map[string]int{}}
	fail := func(what string, err error) (*setup, *decodeStats, error) {
		st.Records, st.EndOffset = rr.count, rr.offset
		st.End = what + ": " + err.Error()
		st.finish()
		return su, st, fmt.Errorf("%w: %s: %v", errSetup, what, err)
	}

	b, err := rr.next()
	if err != nil {
		return fail("header", err)
	}
	if su.Header, err = parseHeader(b); err != nil {
		return fail("header", err)
	}

	// Version 5 extra sectors [fmt tad §3]. Only the recorder version and
	// date strings are read; comment, chat and address sectors are skipped.
	b, err = rr.next()
	if err != nil {
		return fail("extra header", err)
	}
	if len(b) < 4 {
		return fail("extra header", errors.New("record too short"))
	}
	nSectors := int32(binary.LittleEndian.Uint32(b))
	if nSectors < 0 || nSectors > 1024 {
		return fail("extra header", fmt.Errorf("implausible sector count %d", nSectors))
	}
	for i := 0; i < int(nSectors); i++ {
		b, err = rr.next()
		if err != nil {
			return fail("extra sector", err)
		}
		if len(b) < 4 {
			return fail("extra sector", errors.New("record too short"))
		}
		typ := int32(binary.LittleEndian.Uint32(b))
		su.Sectors[fmt.Sprint(typ)]++
		switch typ {
		case sectorRecorder:
			su.Recorder = cString(b[4:])
		case sectorDate:
			su.Date = cString(b[4:])
		}
	}

	for i := 0; i < su.Header.NumPlayers; i++ {
		b, err = rr.next()
		if err != nil {
			return fail("player", err)
		}
		p, perr := parsePlayer(b)
		if perr != nil {
			return fail("player", perr)
		}
		su.Players = append(su.Players, p)
	}
	var scratch []byte
	for i := 0; i < su.Header.NumPlayers; i++ {
		b, err = rr.next()
		if err != nil {
			return fail("status", err)
		}
		pi, perr := parseStatus(b, &scratch)
		if perr != nil {
			pi.Error = perr.Error()
		}
		su.Status = append(su.Status, pi)
	}
	b, err = rr.next()
	if err != nil {
		return fail("unit data", err)
	}
	su.UnitData = parseUnitData(b)
	if onSetup != nil {
		onSetup(su)
	}

	var clocks [256]senderClock
	var speedSegment uint32
	breakCapture := func() {
		for i := range clocks {
			clocks[i].segment++
		}
	}
	var wall int64
	for {
		b, err = rr.next()
		if err != nil {
			if err == io.EOF {
				st.End = "eof"
			} else {
				st.End = err.Error()
			}
			break
		}
		st.Packets++
		// Stored match packet: u16 delta ms, u8 sender, then the flag and
		// payload [fmt tad "Format at a glance"].
		if len(b) < 4 {
			st.ShortPackets++
			breakCapture()
			continue
		}
		dt := binary.LittleEndian.Uint16(b)
		wall += int64(dt)
		sender := int(b[2])
		body := b[3:]
		if isChecksumMarker(body) {
			st.ChecksumMarkers++
			breakCapture()
			continue
		}
		// Some recordings store bundles twice in succession. A body
		// byte-identical to the same sender's previous one at the same
		// capture time repeats its tick base and records, so it is counted
		// and skipped. Repeats that differ (for example by an added camera
		// record) are kept; the lifecycle joins deduplicate their records.
		clk := &clocks[sender]
		if clk.hasBody && bytes.Equal(clk.lastBody, body) && dt == 0 {
			st.RepeatedPackets++
			continue
		}
		clk.lastBody, clk.hasBody = append(clk.lastBody[:0], body...), true

		var stream []byte
		switch body[0] {
		case 0x03:
			st.Plain++
			stream = body[1:]
		case 0x04:
			st.Compressed++
			out, res, lerr := lz77(body[1:], &scratch)
			if len(out) > st.LZMaxOut {
				st.LZMaxOut = len(out)
			}
			if res.lateMatch {
				st.LZLateMatches++
				breakCapture()
			}
			if lerr != nil {
				st.LZErrors++
				breakCapture()
				continue
			}
			if !res.terminated {
				st.LZUnterminated++
				breakCapture()
			}
			stream = out
		default:
			st.unknownFlagCount[body[0]]++
			breakCapture()
			continue
		}
		beforeUnsplit := st.UnsplitBytes
		walkStream(stream, st, clk, func(sp []byte) {
			if sp[0] == 0x19 {
				speedSegment++
			}
			if onSub != nil {
				tick := int64(-1)
				if clk.seen {
					tick = clk.last
				}
				onSub(subContext{WallMs: wall, Sender: sender, Tick: tick, ClockSegment: clk.segment, SpeedSegment: speedSegment}, sp)
			}
		})
		if st.UnsplitBytes != beforeUnsplit {
			breakCapture()
		}
	}
	st.Records, st.EndOffset, st.WallMs = rr.count, rr.offset, wall
	st.finish()
	return su, st, nil
}

// walkStream splits one subpacket stream, advancing the sender's Smartpak
// clock [fmt tad §7]: 0xfe sets the counter, each reconstructed 0x2c (0xfd or
// 0xff) takes tick = counter++, and a raw 0x2c carries its own tick. A
// stream that cannot be split to its end is counted and abandoned at the
// failure; everything before it has been delivered.
func walkStream(stream []byte, st *decodeStats, clk *senderClock, emit func([]byte)) {
	for len(stream) > 0 {
		var n int
		status := splitDone
		if stream[0] == 0x20 {
			switch n = playerInfoLength(stream); n {
			case playerInfoLongLen:
				st.PlayerInfoLong++
			case playerInfoRetailLen:
				st.PlayerInfoRetail++
			}
			if n > len(stream) {
				status = splitTruncated
			}
		} else {
			n, status = subpacketLen(stream)
		}
		if status != splitDone {
			clk.segment++
			switch status {
			case splitUnknownID:
				st.unknownIDCounts[stream[0]]++
			case splitTruncated:
				st.TruncatedSubs++
			case splitBadLength:
				st.BadLengthSubs++
			}
			st.UnsplitBytes += int64(len(stream))
			return
		}
		sp := stream[:n]
		stream = stream[n:]
		st.subCounts[sp[0]]++
		switch sp[0] {
		case 0xfe:
			t := int64(binary.LittleEndian.Uint32(sp[1:]))
			if clk.base {
				// Predicting the base by counting reconstructed syncs is the
				// historical splitting check [fmt tad "Corpus validation"].
				// A base below the prediction replays ticks already seen (a
				// repeated bundle); above it, bundles are missing.
				st.TickChecks++
				switch {
				case clk.counter == t:
					st.TickEqual++
				case t < clk.counter:
					st.TickRewind++
					clk.segment++
				default:
					st.TickGap++
					clk.segment++
				}
			}
			clk.counter, clk.base = t, true
		case 0xfd, 0xff:
			if !clk.base {
				st.SyncWithoutBase++
				continue
			}
			clk.last, clk.seen = clk.counter, true
			clk.counter++
		case 0x2c:
			t := int64(binary.LittleEndian.Uint32(sp[3:]))
			if clk.base {
				st.TickChecks++
				switch {
				case t == clk.counter:
					st.TickEqual++
				case t < clk.counter:
					st.TickRewind++
					clk.segment++
				default:
					st.TickGap++
					clk.segment++
				}
			}
			clk.last, clk.counter, clk.base, clk.seen = t, t+1, true, true
		}
		emit(sp)
	}
}
