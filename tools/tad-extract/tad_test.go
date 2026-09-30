package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/rand"
	"testing"
)

// The fixture below is authored here from the format description; it is not
// derived from any recording. It exercises the record envelope, the setup
// records, the encrypted status envelope (plain and LZ77-compressed), the
// Smartpak tick model and the 0x09/0x12/0x0c lifecycle joins.

// lz77Compress is a small greedy encoder for the decoder's contract [fmt tad
// §5]: a set control bit introduces u16 (index<<4 | length-2), index being a
// one-based absolute position in the output so far (at most 4095), and a
// zero index ends the stream.
func lz77Compress(in []byte) []byte {
	var out []byte
	i := 0
	for {
		ctrlAt := len(out)
		out = append(out, 0)
		for bit := 0; bit < 8; bit++ {
			if i >= len(in) {
				out[ctrlAt] |= 1 << bit
				out = append(out, 0, 0) // terminator: index zero
				return out
			}
			bestLen, bestSrc := 0, 0
			for src := 0; src < i && src < 4095; src++ {
				n := 0
				for n < 17 && i+n < len(in) && in[src+n] == in[i+n] {
					n++
				}
				if n > bestLen {
					bestLen, bestSrc = n, src
				}
			}
			if bestLen >= 3 {
				out[ctrlAt] |= 1 << bit
				w := uint16((bestSrc+1)<<4 | (bestLen - 2))
				out = binary.LittleEndian.AppendUint16(out, w)
				i += bestLen
			} else {
				out = append(out, in[i])
				i++
			}
		}
	}
}

// sealEnvelope builds a custom wire message: flag, checksum, payload, with
// bytes 3..W-4 XOR-masked by their index and summed [fmt tad §5].
func sealEnvelope(payload []byte, compress bool) []byte {
	flag := byte(0x03)
	if compress {
		flag = 0x04
		payload = lz77Compress(payload)
	}
	msg := append([]byte{flag, 0, 0}, payload...)
	var sum uint16
	for i := 3; i < len(msg)-3; i++ {
		msg[i] ^= byte(i)
		sum += uint16(msg[i])
	}
	binary.LittleEndian.PutUint16(msg[1:], sum)
	return msg
}

func record(body ...[]byte) []byte {
	b := bytes.Join(body, nil)
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(b)+2)), b...)
}

func u16(v int) []byte    { return binary.LittleEndian.AppendUint16(nil, uint16(v)) }
func u32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }

func fixturePlayerInfo(dpid uint32) []byte {
	r := make([]byte, playerInfoLen)
	r[0] = 0x20
	copy(r[1:], "Fixture Map")
	binary.LittleEndian.PutUint16(r[140:], 1024)
	binary.LittleEndian.PutUint16(r[142:], 768)
	binary.LittleEndian.PutUint32(r[145:], dpid)
	binary.LittleEndian.PutUint16(r[166:], 500)
	r[168], r[169] = 3, 1
	binary.LittleEndian.PutUint32(r[187:], dpid)
	return r
}

func unitRecord(sub byte, key uint32, a, b byte) []byte {
	r := make([]byte, 14)
	r[0], r[1] = 0x1a, sub
	binary.LittleEndian.PutUint32(r[6:], key)
	r[10], r[11] = a, b
	return r
}

func buildStart(typ, net int, x, y, z uint32) []byte {
	r := make([]byte, 23)
	r[0] = 0x09
	binary.LittleEndian.PutUint16(r[1:], uint16(typ))
	binary.LittleEndian.PutUint16(r[3:], uint16(net))
	binary.LittleEndian.PutUint32(r[7:], x)
	binary.LittleEndian.PutUint32(r[11:], y)
	binary.LittleEndian.PutUint32(r[15:], z)
	return r
}

func packet(dt int, sender byte, compress bool, subs ...[]byte) []byte {
	stream := bytes.Join(subs, nil)
	if compress {
		return record(u16(dt), []byte{sender, 0x04}, lz77Compress(stream))
	}
	return record(u16(dt), []byte{sender, 0x03}, stream)
}

// fixture is a two-player recording. Player 1 (transport identity 200) ranks
// second by identity and owns unit IDs 501..1000; player 2 (identity 100)
// owns 1..500 [fmt tad §4].
func fixture() []byte {
	chat := make([]byte, chatLen)
	chat[0] = 0x05
	copy(chat[1:], "<p1> never decoded")
	finish := []byte{0x12, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(finish[1:], 502)
	binary.LittleEndian.PutUint16(finish[3:], 501)
	death := make([]byte, 11)
	death[0] = 0x0c
	binary.LittleEndian.PutUint16(death[1:], 502)
	binary.LittleEndian.PutUint32(death[3:], 100)
	binary.LittleEndian.PutUint16(death[7:], 1)
	idleSync := []byte{0xff}
	sync := []byte{0xfd, 11, 0, 1, 2, 3, 4} // original 0x2c length 11
	var f []byte
	f = append(f, record([]byte(demoMagic), u16(5), []byte{2}, u16(500), []byte("Fixture Map\x00"))...)
	f = append(f, record(u32(2))...)
	f = append(f, record(u32(sectorRecorder), []byte("fixture-1\x00"))...)
	f = append(f, record(u32(2), []byte("private chat log"))...) // chat-log sector, skipped
	f = append(f, record([]byte{3, 0, 1}, []byte("Alpha\x00"))...)
	f = append(f, record([]byte{4, 1, 2}, []byte("Beta\x00"))...)
	seq := []byte{0xff, 0xff, 0xff, 0xff}
	f = append(f, record([]byte{1}, sealEnvelope(append(seq, fixturePlayerInfo(200)...), false))...)
	f = append(f, record([]byte{2}, sealEnvelope(append(seq, fixturePlayerInfo(100)...), true))...)
	f = append(f, record(unitRecord(2, 7, 0, 0), unitRecord(3, 7, 1, 1), unitRecord(3, 8, 1, 0), unitRecord(3, 9, 1, 1),
		unitRecord(9, 0xffffffff, 0, 0))...)
	f = append(f, packet(100, 1, false, []byte{0xfe, 10, 0, 0, 0}, idleSync, buildStart(36, 501, 3840, 85, 1872), sync, chat)...)
	f = append(f, packet(50, 2, true, []byte{0xfe, 10, 0, 0, 0}, sync, buildStart(169, 1, 336, 85, 3696), idleSync)...)
	f = append(f, packet(40, 1, false, []byte{0xfe, 12, 0, 0, 0}, idleSync,
		buildStart(80, 502, 3900|0x7fff0000, 90, 1900), sync)...)
	f = append(f, packet(30, 1, true, []byte{0xfe, 14, 0, 0, 0}, idleSync, finish, idleSync)...)
	f = append(f, packet(0, 1, true, []byte{0xfe, 14, 0, 0, 0}, idleSync, finish, idleSync)...) // stored twice
	f = append(f, packet(20, 2, false, []byte{0xfe, 12, 0, 0, 0}, idleSync, death)...)
	return f
}

func TestLZ77RoundTrip(t *testing.T) {
	inputs := [][]byte{
		{},
		[]byte("a"),
		bytes.Repeat([]byte{0x2c}, 300), // overlapping forward copies
		[]byte("unit sync unit sync unit sync, then something else entirely"),
	}
	rng := rand.New(rand.NewSource(7))
	noisy := make([]byte, 5000)
	for i := range noisy {
		noisy[i] = byte(rng.Intn(4))
	}
	inputs = append(inputs, noisy)
	var scratch []byte
	for _, in := range inputs {
		out, res, err := lz77(lz77Compress(in), &scratch)
		if err != nil || !res.terminated || !bytes.Equal(out, in) {
			t.Fatalf("round trip of %d bytes: err %v terminated %v equal %v", len(in), err, res.terminated, bytes.Equal(out, in))
		}
	}
	// A hand-written stream: literal 'x', then index 1 length 5 overlapping
	// its own output, then the terminator.
	out, res, err := lz77([]byte{0b110, 'x', 0x13, 0x00, 0x00, 0x00}, &scratch)
	if err != nil || !res.terminated || string(out) != "xxxxxx" {
		t.Fatalf("overlap copy: %q %v %v", out, res, err)
	}
	// An index beyond the output is an error, not a panic.
	if _, _, err := lz77([]byte{0b1, 0x50, 0x00}, &scratch); err != errLZOffset {
		t.Fatalf("bad index: %v", err)
	}
}

func TestEnvelopeChecksumAndMask(t *testing.T) {
	payload := append([]byte{0xff, 0xff, 0xff, 0xff}, fixturePlayerInfo(12345)...)
	var scratch []byte
	for _, compress := range []bool{false, true} {
		msg := sealEnvelope(payload, compress)
		got, ok, c, err := openEnvelope(msg, &scratch)
		if err != nil || !ok || c != compress || !bytes.Equal(got, payload) {
			t.Fatalf("compress=%v: err %v checksum %v equal %v", compress, err, ok, bytes.Equal(got, payload))
		}
		msg[10] ^= 0x40
		if _, ok, _, _ := openEnvelope(msg, &scratch); ok {
			t.Fatalf("compress=%v: a changed byte still passes the checksum", compress)
		}
	}
}

func TestDecodeFixture(t *testing.T) {
	var x *extractor
	var chatBytesSeen int
	su, st, err := decode(bytes.NewReader(fixture()), func(ctx subContext, sp []byte) {
		if sp[0] == 0x05 {
			chatBytesSeen++
		}
		x.onSub(ctx, sp)
	}, func(su *setup) { x = newExtractor(su) })
	if err != nil {
		t.Fatal(err)
	}
	if su.Header.MaxUnits != 500 || su.Header.Map != "Fixture Map" || su.Recorder != "fixture-1" || st.End != "eof" {
		t.Fatalf("setup %+v recorder %q end %q", su.Header, su.Recorder, st.End)
	}
	for _, s := range su.Status {
		if s.Error != "" || !s.Checksum || s.Sequence != -1 || s.DPID != s.DPID2 || s.RecordLen != playerInfoLen {
			t.Fatalf("status %+v", s)
		}
	}
	if !su.Status[1].Compressed || su.Status[0].Compressed {
		t.Fatalf("compression flags %+v", su.Status)
	}
	// Key 7 is enabled, 8 has one flag clear, 9 is enabled, all-ones is the list checksum.
	if su.UnitData.Enabled != 2 || su.UnitData.Records != 5 || su.UnitData.Keys != 3 {
		t.Fatalf("unit data %+v", su.UnitData)
	}
	// The repeated bundle is skipped before it is counted as compressed;
	// every later 0xfe base equals the count of reconstructed syncs.
	if st.RepeatedPackets != 1 || st.Compressed != 2 || st.TickChecks != 3 || st.TickEqual != 3 || st.WallMs != 240 {
		t.Fatalf("repeated %d compressed %d tick checks %d/%d wall %d",
			st.RepeatedPackets, st.Compressed, st.TickEqual, st.TickChecks, st.WallMs)
	}
	if x.st.ChatSkipped != 1 || chatBytesSeen != 1 {
		t.Fatalf("chat skipped %d", x.st.ChatSkipped)
	}
	if x.st.Starts != 3 || x.st.BlockOK != 3 || x.st.BlockMismatch != 0 || x.st.FinishRepeat != 0 || x.st.HighWords != 1 {
		t.Fatalf("extract stats %+v", x.st)
	}
	byNet := map[int]*buildEvent{}
	for _, ev := range x.events {
		byNet[ev.NetID] = ev
	}
	cmd := byNet[501]
	if cmd.sender != 1 || cmd.Type != 36 || cmd.X != 3840 || cmd.Y != 85 || cmd.Z != 1872 || cmd.Tick != 10 || cmd.TickNext != 11 || cmd.TMs != 100 {
		t.Fatalf("commander start %+v", cmd)
	}
	if other := byNet[1]; other.sender != 2 || other.Tick != 10 || other.TickNext != 11 {
		t.Fatalf("second player's start %+v", other)
	}
	b := byNet[502]
	if b.X != 3900 || b.Hi == nil || b.Hi[0] != 0x7fff || b.Tick != 12 || b.TickNext != 13 {
		t.Fatalf("second start %+v", b)
	}
	if b.FinMs == nil || *b.FinMs != 220 || *b.FinTick != 14 || b.Builder != 501 {
		t.Fatalf("finish %+v", b)
	}
	if b.DiedMs == nil || *b.DiedMs != 240 || b.KillerPlayer != 2 || b.KillerUnit != 1 {
		t.Fatalf("death %+v", b)
	}
}

// TestTruncatedAndCorrupt cuts the fixture at every length and overwrites
// random bytes: decoding must never panic; a cut inside a record must be
// reported as truncation, a cut between records as a clean end; and starts
// decoded before the cut must be kept.
func TestTruncatedAndCorrupt(t *testing.T) {
	full := fixture()
	boundary := map[int]bool{}
	for off := 0; off+2 <= len(full); {
		boundary[off] = true
		off += int(binary.LittleEndian.Uint16(full[off:]))
	}
	for n := 0; n < len(full); n++ {
		var x *extractor
		_, st, err := decode(bytes.NewReader(full[:n]), func(ctx subContext, sp []byte) { x.onSub(ctx, sp) },
			func(su *setup) { x = newExtractor(su) })
		if err != nil {
			continue // cut inside the setup records
		}
		want := errTruncated.Error()
		if boundary[n] {
			want = "eof"
		}
		if st.End != want {
			t.Fatalf("cut at %d: end %q, want %q", n, st.End, want)
		}
	}
	// Cutting into the last packet keeps every earlier start and finish.
	var x *extractor
	_, st, err := decode(bytes.NewReader(full[:len(full)-3]), func(ctx subContext, sp []byte) { x.onSub(ctx, sp) },
		func(su *setup) { x = newExtractor(su) })
	if err != nil || st.End != errTruncated.Error() || len(x.events) != 3 || x.byNet[502].FinMs == nil || x.byNet[502].DiedMs != nil {
		t.Fatalf("cut into last packet: err %v end %q events %d", err, st.End, len(x.events))
	}
	if _, _, err := decode(io.LimitReader(bytes.NewReader(full), 20), nil, nil); err == nil {
		t.Fatal("a cut header decoded without error")
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 3000; i++ {
		b := append([]byte(nil), full...)
		for k := 0; k < 1+rng.Intn(8); k++ {
			b[rng.Intn(len(b))] = byte(rng.Intn(256))
		}
		var x *extractor
		decode(bytes.NewReader(b), func(ctx subContext, sp []byte) {
			if x != nil {
				x.onSub(ctx, sp)
			}
		}, func(su *setup) { x = newExtractor(su) })
	}
}

func TestPlayerInfoLengths(t *testing.T) {
	team := []byte{0x24, 1, 2, 3, 4, 5}
	long := append(make([]byte, playerInfoLongLen), team...)
	long[0] = 0x20
	long[192] = 0x77 // not a known id: 192 cannot be right
	if n := playerInfoLength(long); n != playerInfoLongLen {
		t.Fatalf("long form chose %d", n)
	}
	retail := make([]byte, playerInfoRetailLen)
	retail[0] = 0x20
	if n := playerInfoLength(retail); n != playerInfoRetailLen {
		t.Fatalf("trailing retail form chose %d", n)
	}
	listed := append(make([]byte, playerInfoLen), team...)
	listed[0] = 0x20
	if n := playerInfoLength(listed); n != playerInfoLen {
		t.Fatalf("listed form chose %d", n)
	}
}

func TestChooseTable(t *testing.T) {
	tab := &unitTable{Name: "t", Names: []string{"ARMCOM", "ARMSOLAR", "CORCOM", "ZZZ"}, Real: 3, arm: 1, core: 3}
	players := []extractPlayer{
		{Side: 0, Builds: []*buildEvent{{Type: 1}}},
		{Side: 1, Builds: []*buildEvent{{Type: 3}}},
		{Side: 2},
	}
	su := &setup{UnitData: unitData{Enabled: 3}}
	if got, nm := chooseTable(su, players, []*unitTable{tab}); got != tab || nm.Commanders != 2 {
		t.Fatalf("matching table rejected: %+v", nm)
	}
	su.UnitData.Enabled = 4 // one more catalog unit of unknown sort position
	if got, _ := chooseTable(su, players, []*unitTable{tab}); got != nil {
		t.Fatal("table accepted with a different catalog size")
	}
	su.UnitData.Enabled = 3
	players[1].Builds[0].Type = 2
	if got, _ := chooseTable(su, players, []*unitTable{tab}); got != nil {
		t.Fatal("table accepted with a wrong commander")
	}
}
