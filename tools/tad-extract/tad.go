package main

// Reader for TA Demo Recorder recordings, implemented from the format
// description in research/formats/tad.md [fmt tad]. It covers the record
// envelope, the setup records, the encrypted player-info status messages and
// the stored match packets with their Smartpak unit-sync forms. It never
// panics on corrupt input: every length is checked against the bytes that are
// actually present, and every allocation is bounded by a record length.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Record framing: every record starts with a little-endian u16 total length
// that includes the two length bytes themselves [fmt tad §1].
const maxRecord = 0xffff

var (
	errShortRecord = errors.New("record length below 2")
	errTruncated   = errors.New("recording ends inside a record")
)

type recordReader struct {
	r      *bufio.Reader
	buf    [maxRecord]byte
	offset int64 // file offset of the next record
	count  int
}

func newRecordReader(r io.Reader) *recordReader {
	return &recordReader{r: bufio.NewReaderSize(r, 1<<16)}
}

// next returns the body of the next record, without its length bytes. The
// slice aliases an internal buffer and is valid until the next call. A clean
// end of input returns io.EOF.
func (rr *recordReader) next() ([]byte, error) {
	var lb [2]byte
	if n, err := io.ReadFull(rr.r, lb[:]); err != nil {
		if n == 0 && err == io.EOF {
			return nil, io.EOF
		}
		return nil, errTruncated
	}
	total := int(binary.LittleEndian.Uint16(lb[:]))
	if total < 2 {
		return nil, errShortRecord
	}
	body := rr.buf[:total-2]
	if _, err := io.ReadFull(rr.r, body); err != nil {
		return nil, errTruncated
	}
	rr.offset += int64(total)
	rr.count++
	return body, nil
}

// header is the first record [fmt tad §2].
type header struct {
	Version    uint16 `json:"format_version"`
	NumPlayers int    `json:"num_players"`
	MaxUnits   int    `json:"max_units"`
	Map        string `json:"map"`
}

const demoMagic = "TA Demo\x00"

func parseHeader(b []byte) (header, error) {
	var h header
	if len(b) < len(demoMagic)+3 || string(b[:len(demoMagic)]) != demoMagic {
		return h, errors.New("bad magic")
	}
	p := b[len(demoMagic):]
	h.Version = binary.LittleEndian.Uint16(p)
	h.NumPlayers = int(p[2])
	p = p[3:]
	// Only version 5 is described with maxUnits and extra sectors; versions 3
	// and 4 omit maxUnits and have no validated sample [fmt tad §2].
	if h.Version != 5 {
		return h, fmt.Errorf("unsupported format version %d", h.Version)
	}
	if len(p) < 2 {
		return h, errors.New("header too short for maxUnits")
	}
	h.MaxUnits = int(binary.LittleEndian.Uint16(p))
	name := p[2:]
	// Header names can end at the record boundary [fmt tad §2]. A final
	// NUL is accepted; interpreting data after an embedded NUL is unknown.
	if i := bytes.IndexByte(name, 0); i >= 0 {
		if i != len(name)-1 {
			return h, errors.New("header map has data after NUL")
		}
		name = name[:i]
	}
	if len(name) == 0 {
		return h, errors.New("header map is empty")
	}
	h.Map = string(name)
	return h, nil
}

// cString returns bytes up to the first NUL or the end of the slice. Observed
// map names sometimes fill the record without a terminator.
func cString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// Extra sector types [fmt tad §3]. Only the recorder version (3) and the
// recording date (4) are retained. Comments (1), the chat log (2), the
// recorded-from string (5) and the masked lobby addresses (6) are never
// decoded; chat and addresses are privacy-sensitive.
const (
	sectorRecorder = 3
	sectorDate     = 4
)

// player is a setup player record [fmt tad §4].
type player struct {
	Number int    `json:"number"`
	Color  int    `json:"color"`
	Side   int    `json:"side"`
	Name   string `json:"name"`
}

func parsePlayer(b []byte) (player, error) {
	if len(b) < 3 {
		return player{}, errors.New("player record too short")
	}
	return player{Color: int(b[0]), Side: int(b[1]), Number: int(b[2]), Name: cString(b[3:])}, nil
}

// playerInfo holds the fields of the 192-byte 0x20 player-info record that the
// format description locates [fmt tad §4]. Everything else in the record is
// left undecoded.
type playerInfo struct {
	Number     int    `json:"number"`
	Checksum   bool   `json:"checksum_ok"`
	Compressed bool   `json:"compressed"`
	Sequence   int32  `json:"sequence"`
	RecordLen  int    `json:"record_len"`
	DPID       uint32 `json:"dpid"`
	DPID2      uint32 `json:"dpid_repeat"`
	// Word140 and Word142 are the pair once listed as map width and height.
	// Their meaning is Unknown; neither map extents nor screen resolution
	// is established [fmt tad §4]. They carry no assigned meaning here.
	Word140  int    `json:"u16_at_140"`
	Word142  int    `json:"u16_at_142"`
	MaxUnits int    `json:"max_units"`
	VerMajor int    `json:"ta_major"`
	VerMinor int    `json:"ta_minor"`
	Error    string `json:"error,omitempty"`
}

const playerInfoLen = 192

// parseStatus decodes a status record: a player number followed by a
// complete encrypted custom envelope holding a four-byte transport sequence
// and one 0x20 record [fmt tad §4] [fmt tad §5].
func parseStatus(b []byte, scratch *[]byte) (playerInfo, error) {
	var pi playerInfo
	if len(b) < 2 {
		return pi, errors.New("status record too short")
	}
	pi.Number = int(b[0])
	payload, ok, compressed, err := openEnvelope(b[1:], scratch)
	pi.Checksum, pi.Compressed = ok, compressed
	if err != nil {
		return pi, err
	}
	if len(payload) < 4+1 {
		return pi, errors.New("status payload too short")
	}
	pi.Sequence = int32(binary.LittleEndian.Uint32(payload))
	rec := payload[4:]
	if rec[0] != 0x20 {
		return pi, fmt.Errorf("status record type %#02x, want 0x20", rec[0])
	}
	pi.RecordLen = len(rec)
	if len(rec) < playerInfoLen {
		return pi, fmt.Errorf("player-info record %d bytes, want %d", len(rec), playerInfoLen)
	}
	le := binary.LittleEndian
	pi.Word140 = int(le.Uint16(rec[140:]))
	pi.Word142 = int(le.Uint16(rec[142:]))
	pi.DPID = le.Uint32(rec[145:])
	pi.MaxUnits = int(le.Uint16(rec[166:]))
	pi.VerMajor = int(rec[168])
	pi.VerMinor = int(rec[169])
	pi.DPID2 = le.Uint32(rec[187:])
	return pi, nil
}

// openEnvelope reverses the custom wire transformation [fmt tad §5]: bytes
// 3..W-4 of a W-byte message are XOR-masked with their index modulo 256, the
// u16 at 1 is the 16-bit sum of those masked bytes, and flag 0x04 means the
// unmasked payload after the three-byte header is LZ77-compressed. The final
// three bytes are neither masked nor summed but stay part of the payload.
func openEnvelope(msg []byte, scratch *[]byte) (payload []byte, checksumOK, compressed bool, err error) {
	w := len(msg)
	if w < 3 {
		return nil, false, false, errors.New("envelope shorter than its header")
	}
	flag := msg[0]
	want := binary.LittleEndian.Uint16(msg[1:])
	dec := make([]byte, w-3)
	copy(dec, msg[3:])
	var sum uint16
	for i := 3; i < w-3; i++ {
		sum += uint16(msg[i])
		dec[i-3] = msg[i] ^ byte(i)
	}
	checksumOK = sum == want
	switch flag {
	case 0x03:
		return dec, checksumOK, false, nil
	case 0x04:
		out, _, lerr := lz77(dec, scratch)
		if lerr != nil {
			return nil, checksumOK, true, lerr
		}
		return append([]byte(nil), out...), checksumOK, true, nil
	}
	return nil, checksumOK, false, fmt.Errorf("unknown envelope flag %#02x", flag)
}

// lzResult describes how an LZ77 stream ended.
type lzResult struct {
	terminated bool
	// lateMatch reports a match emitted after 4095 output bytes existed, the
	// region the twelve-bit absolute index cannot reach beyond.
	lateMatch bool
}

var (
	errLZOffset    = errors.New("lz77 index beyond output")
	errLZTruncated = errors.New("lz77 match word truncated")
)

// lz77 decompresses the recorder's control-byte LZ77 [fmt tad §5]: control
// bits are consumed least-significant first; a clear bit copies one literal,
// a set bit reads u16 w whose high twelve bits are a one-based absolute index
// into the output produced so far (zero ends the stream) and whose low four
// bits plus two give the copy length. Copies may overlap their own output.
//
// The output is bounded by nine times the input length: a match costs at
// least two input bytes plus an eighth of a control byte and yields at most
// seventeen bytes. The returned slice aliases *scratch.
func lz77(in []byte, scratch *[]byte) ([]byte, lzResult, error) {
	var res lzResult
	limit := 9*len(in) + 16
	if cap(*scratch) < limit {
		*scratch = make([]byte, 0, limit)
	}
	out := (*scratch)[:0]
	i := 0
	for i < len(in) {
		ctrl := in[i]
		i++
		for bit := 0; bit < 8; bit++ {
			if ctrl&(1<<bit) == 0 {
				if i >= len(in) {
					return out, res, nil
				}
				out = append(out, in[i])
				i++
				continue
			}
			if i+2 > len(in) {
				return out, res, errLZTruncated
			}
			w := int(in[i]) | int(in[i+1])<<8
			i += 2
			idx := w >> 4
			if idx == 0 {
				res.terminated = true
				return out, res, nil
			}
			n := w&0x0f + 2
			src := idx - 1
			if src >= len(out) {
				return out, res, errLZOffset
			}
			if len(out) >= 4095 {
				res.lateMatch = true
			}
			if len(out)+n > limit {
				return out, res, errLZOffset
			}
			for k := 0; k < n; k++ {
				out = append(out, out[src+k])
			}
		}
	}
	return out, res, nil
}

// Subpacket lengths by id for the recording strata, including the one-byte id
// [fmt tad §6]. Zero means the id is unknown or has a computed length.
var fixedLen = [256]int{
	0x02: 13, 0x03: 7, 0x06: 1, 0x07: 1, 0x08: 1, 0x09: 23, 0x0a: 7,
	0x0b: 9, 0x0c: 11, 0x0d: 36, 0x0e: 14, 0x0f: 6, 0x10: 22, 0x11: 4,
	0x12: 5, 0x13: 19, 0x14: 24, 0x15: 1, 0x16: 17, 0x17: 2, 0x18: 2,
	0x19: 3, 0x1a: 14, 0x1b: 6, 0x1e: 2, 0x1f: 5, 0x20: 192, 0x21: 10,
	0x22: 6, 0x23: 14, 0x24: 6, 0x26: 41, 0x28: 58, 0x29: 3, 0x2a: 2,
	0x2e: 9, 0xf6: 1, 0xf9: 73, 0xfa: 1, 0xfc: 5, 0xfe: 5, 0xff: 1,
}

const chatLen = 65

// splitStatus tells why splitting a subpacket stream stopped early.
type splitStatus int

const (
	splitDone splitStatus = iota
	splitUnknownID
	splitTruncated
	splitBadLength
)

// subpacketLen returns the length of the subpacket at the start of b, and a
// status other than splitDone when the stream cannot be split further. Chat
// ids are sized here only so they can be skipped; their bytes are never read
// beyond the overflow test the format prescribes [fmt tad §6].
func subpacketLen(b []byte) (int, splitStatus) {
	id := b[0]
	le := binary.LittleEndian
	switch id {
	case 0x00:
		n := 1
		for n < len(b) && b[n] == 0 {
			n++
		}
		return n, splitDone
	case 0x05:
		// A NUL-padded chat is 65 bytes; when its last byte is not NUL an
		// older recorder overflowed it and the chat runs to the end of the
		// stream, less a trailing five-byte 0xfc record if present.
		if len(b) >= chatLen && b[chatLen-1] == 0 {
			return chatLen, splitDone
		}
		if len(b) >= 5 && b[len(b)-5] == 0xfc {
			return len(b) - 5, splitDone
		}
		return len(b), splitDone
	case 0x2c: // raw unit sync: u16 total length at 1 [fmt tad §7]
		if len(b) < 3 {
			return 0, splitTruncated
		}
		n := int(le.Uint16(b[1:]))
		if n < 7 {
			return 0, splitBadLength
		}
		return checkLen(n, len(b))
	case 0xfd: // Smartpak sync: original 0x2c length less the four tick bytes
		if len(b) < 3 {
			return 0, splitTruncated
		}
		n := int(le.Uint16(b[1:])) - 4
		if n < 3 {
			return 0, splitBadLength
		}
		return checkLen(n, len(b))
	case 0xfb: // recorder data connect: u8 at 1 plus 3
		if len(b) < 2 {
			return 0, splitTruncated
		}
		return checkLen(int(b[1])+3, len(b))
	case 0x42: // later-patch extension: u16 at 1 plus 3
		if len(b) < 3 {
			return 0, splitTruncated
		}
		return checkLen(int(le.Uint16(b[1:]))+3, len(b))
	}
	n := fixedLen[id]
	if n == 0 {
		return 0, splitUnknownID
	}
	return checkLen(n, len(b))
}

// Match-stream 0x20 records occur in three lengths in the recording
// strata: the 192 bytes the format lists, a 200-byte form in some later
// recordings, and the 186 bytes the format gives as the retail length
// [fmt tad §6]. The first candidate that leaves a stream splittable to its
// end is taken, in that order; the extra or missing bytes are not
// interpreted.
const (
	playerInfoLongLen   = 200
	playerInfoRetailLen = 186
)

var playerInfoLens = [...]int{playerInfoLen, playerInfoLongLen, playerInfoRetailLen}

// playerInfoLength chooses the length of a 0x20 record at the start of b.
func playerInfoLength(b []byte) int {
	for _, n := range playerInfoLens {
		budget := splitBudget
		if len(b) >= n && splitsCleanly(b[n:], &budget) {
			return n
		}
	}
	return playerInfoLen
}

// splitBudget bounds the subpackets one length decision may visit.
const splitBudget = 1 << 14

// splitsCleanly reports whether b splits into known subpackets to its end,
// allowing each candidate 0x20 length at every player-info record.
func splitsCleanly(b []byte, budget *int) bool {
	for len(b) > 0 {
		if *budget--; *budget < 0 {
			return false
		}
		if b[0] == 0x20 {
			for _, n := range playerInfoLens {
				if len(b) >= n && splitsCleanly(b[n:], budget) {
					return true
				}
			}
			return false
		}
		n, st := subpacketLen(b)
		if st != splitDone {
			return false
		}
		b = b[n:]
	}
	return true
}

func checkLen(n, have int) (int, splitStatus) {
	if n > have {
		return 0, splitTruncated
	}
	return n, splitDone
}

// Checksum-failure marker text written by one recorder build in place of a
// packet body [fmt tad "Corpus validation"]. Records carrying either spelling,
// at the start of the body or after its flag byte, are excluded rather than
// split.
var (
	checksumMarkerFull = []byte("<error in checksum?!?!>")
	checksumMarkerTail = []byte("in checksum?!?!>")
)

func isChecksumMarker(body []byte) bool {
	for _, b := range [][]byte{body, body[min(1, len(body)):]} {
		if bytes.HasPrefix(b, checksumMarkerFull) || bytes.HasPrefix(b, checksumMarkerTail) {
			return true
		}
	}
	return false
}
