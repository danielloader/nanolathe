package session

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func encodeMatchForTest(t testing.TB, c EffectiveMatchConfig) []byte {
	t.Helper()
	payload, err := EncodeMatchConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

// checkMatchDecode is the codec's one property: a payload either fails to
// decode, or decodes to a value whose encoding is that payload exactly and
// whose identity is stable through another resolution. A decoder never
// accepts one value and hands back another.
func checkMatchDecode(t testing.TB, payload []byte) bool {
	t.Helper()
	c, err := DecodeMatchConfig(payload)
	if err != nil {
		if c.Digest() != ([32]byte{}) {
			t.Fatalf("a refused payload returned a value: %v", err)
		}
		return false
	}
	if again := encodeMatchForTest(t, c); !bytes.Equal(again, payload) {
		t.Fatalf("an accepted payload re-encodes differently:\n got %x\nwant %x", again, payload)
	}
	if r, err := ResolveMatchConfig(c.Request()); err != nil || r.Digest() != c.Digest() {
		t.Fatalf("an accepted payload's value does not resolve to itself: %v", err)
	}
	return true
}

// Every proper prefix of a valid payload is refused, and so is any byte
// after its end.
func TestDecodeMatchConfigRefusesTruncationAndTrailingBytes(t *testing.T) {
	for _, r := range []MatchConfigRequest{validMatchRequest(t), validSurvivalRequest(t)} {
		payload := encodeMatchForTest(t, resolveMatch(t, r))
		for n := 0; n < len(payload); n++ {
			if checkMatchDecode(t, payload[:n]) {
				t.Fatalf("a %d-byte prefix of a %d-byte payload decoded", n, len(payload))
			}
		}
		for _, tail := range [][]byte{{0}, {1}, {0xff}} {
			if checkMatchDecode(t, append(append([]byte(nil), payload...), tail...)) {
				t.Fatalf("a payload with trailing byte %x decoded", tail)
			}
		}
	}
}

// Flipping any bit pattern of any byte of a valid payload is either refused
// or a different, exactly re-encoding configuration. Some flips are both
// valid and different — a seed, a nickname letter — and those must change
// the identity.
func TestDecodeMatchConfigByteMutations(t *testing.T) {
	for _, r := range []MatchConfigRequest{validMatchRequest(t), validSurvivalRequest(t)} {
		c := resolveMatch(t, r)
		payload := encodeMatchForTest(t, c)
		accepted := 0
		for i := range payload {
			for _, mask := range []byte{0x01, 0x02, 0x04, 0x40, 0x80, 0xff} {
				mutated := append([]byte(nil), payload...)
				mutated[i] ^= mask
				if checkMatchDecode(t, mutated) {
					accepted++
					if d, _ := DecodeMatchConfig(mutated); d.Digest() == c.Digest() {
						t.Fatalf("byte %d ^ %#x decoded to the original identity", i, mask)
					}
				}
			}
		}
		if accepted == 0 {
			t.Fatal("no mutation was accepted: the sweep cannot tell a strict decoder from a broken one")
		}
	}
}

// Bounded random edits — replaced, deleted and inserted bytes and cut tails,
// several to a payload — hold the same property. The generator is a fixed
// xorshift so every run edits the same way.
func TestDecodeMatchConfigRandomEdits(t *testing.T) {
	seeds := [][]byte{
		encodeMatchForTest(t, resolveMatch(t, validMatchRequest(t))),
		encodeMatchForTest(t, resolveMatch(t, validSurvivalRequest(t))),
	}
	state := uint64(0x9e3779b97f4a7c15)
	next := func(n int) int {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return int(state % uint64(n))
	}
	for i := 0; i < 20000; i++ {
		p := append([]byte(nil), seeds[i%len(seeds)]...)
		for k := 1 + next(4); k > 0 && len(p) > 0; k-- {
			at := next(len(p))
			switch next(4) {
			case 0:
				p[at] = byte(next(256))
			case 1:
				p = append(p[:at], p[at+1:]...)
			case 2:
				p = append(p[:at], append([]byte{byte(next(256))}, p[at:]...)...)
			case 3:
				p = p[:at]
			}
		}
		checkMatchDecode(t, p)
	}
}

// splice replaces payload[at:at+n] with with.
func splice(payload []byte, at, n int, with ...byte) []byte {
	out := append([]byte(nil), payload[:at]...)
	out = append(out, with...)
	return append(out, payload[at+n:]...)
}

// uvarintBytes is a test-side varint, written independently of the codec.
func uvarintBytes(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

// Structural attacks on known fields: counts over their bound or over the
// bytes that remain, an out-of-range enumeration and overlong integers are
// refused before anything is built from them.
func TestDecodeMatchConfigRefusesHostileFields(t *testing.T) {
	payload := encodeMatchForTest(t, resolveMatch(t, validMatchRequest(t)))
	find := func(pattern []byte) int {
		t.Helper()
		at := bytes.Index(payload, pattern)
		if at < 0 || bytes.Contains(payload[at+1:], pattern) {
			t.Fatalf("pattern %x is not unique in the payload", pattern)
		}
		return at
	}
	// Field 3 ends with the schema, a varint 0, and the seat count follows.
	mapEnd := find([]byte("Great Divide")) + len("Great Divide")
	if payload[mapEnd] != 0 || payload[mapEnd+1] != 3 {
		t.Fatalf("unexpected layout after the map name: %x", payload[mapEnd:mapEnd+2])
	}
	// The table's JSON is between 128 and 16383 bytes, so its length is a
	// two-byte varint just before it.
	table, _ := matchCommunityJSONForTest(matchTestCommunity(t))
	communityLength := find(table) - 2
	if !bytes.Equal(payload[communityLength:communityLength+2], uvarintBytes(uint64(len(table)))) {
		t.Fatalf("unexpected community length bytes %x", payload[communityLength:communityLength+2])
	}
	mutators := find(bytes.Repeat([]byte{3}, 11))
	params := find(append([]byte{2, 6}, "jitter"...))
	directories := find(append([]byte{2, 8}, "gamedata"...))
	restrictions := mutators + 11
	if payload[restrictions] != 2 {
		t.Fatalf("unexpected restriction count %d", payload[restrictions])
	}

	cases := []struct {
		name    string
		payload []byte
	}{
		{"overlong schema", splice(payload, mapEnd, 1, 0x80, 0x00)},
		{"no seats", splice(payload, mapEnd+1, 1, 0)},
		{"one seat", splice(payload, mapEnd+1, 1, 1)},
		{"eleven seats", splice(payload, mapEnd+1, 1, 11)},
		{"255 seats", splice(payload, mapEnd+1, 1, 255)},
		{"community over 16 KiB", splice(payload, communityLength, 2, uvarintBytes(16<<10+1)...)},
		{"community over the payload", splice(payload, communityLength, 2, uvarintBytes(uint64(len(payload)))...)},
		{"community length overflow", splice(payload, communityLength, 2, 0xff, 0xff, 0xff, 0xff, 0x7f)},
		{"mutator index 8", splice(payload, mutators, 1, 8)},
		{"mutator index 255", splice(payload, mutators+10, 1, 255)},
		{"129 AI parameters", splice(payload, params, 1, uvarintBytes(129)...)},
		{"128 AI parameters read from the following fields", splice(payload, params, 1, uvarintBytes(128)...)},
		{"65 directories", splice(payload, directories, 1, 65)},
		{"restrictions over the bound", splice(payload, restrictions, 1, uvarintBytes(65536)...)},
		{"restrictions over the payload", splice(payload, restrictions, 1, uvarintBytes(65535)...)},
		{"overlong restriction count", splice(payload, restrictions, 1, 0x82, 0x00)},
	}
	for _, c := range cases {
		if checkMatchDecode(t, c.payload) {
			t.Errorf("%s: decoded", c.name)
		}
	}
	if checkMatchDecode(t, make([]byte, 2<<20+1)) {
		t.Fatal("a payload over 2 MiB decoded")
	}
}

// The reader's primitives: shortest varints within their width, booleans of
// exactly 0 or 1, and text and key lengths checked before the bytes are
// taken.
func TestMatchReaderPrimitives(t *testing.T) {
	read := func(b []byte, f func(*matchReader)) error {
		r := matchReader{b: b}
		f(&r)
		if r.err == nil && r.off != len(b) {
			t.Fatalf("%x: %d bytes left", b, len(b)-r.off)
		}
		return r.err
	}
	for _, c := range []struct {
		name string
		b    []byte
		bits uint
		want uint64
		ok   bool
	}{
		{"zero", []byte{0}, 16, 0, true},
		{"127", []byte{0x7f}, 16, 127, true},
		{"128", []byte{0x80, 0x01}, 16, 128, true},
		{"u16 max", []byte{0xff, 0xff, 0x03}, 16, 65535, true},
		{"u16 overflow", []byte{0x80, 0x80, 0x04}, 16, 0, false},
		{"u32 max", []byte{0xff, 0xff, 0xff, 0xff, 0x0f}, 32, 1<<32 - 1, true},
		{"u32 overflow", []byte{0x80, 0x80, 0x80, 0x80, 0x10}, 32, 0, false},
		{"u64 max", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}, 64, 1<<64 - 1, true},
		{"u64 overflow", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02}, 64, 0, false},
		{"eleven bytes", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, 64, 0, false},
		{"overlong zero", []byte{0x80, 0x00}, 16, 0, false},
		{"overlong one", []byte{0x81, 0x80, 0x00}, 32, 0, false},
		{"unterminated", []byte{0x80}, 32, 0, false},
		{"empty", nil, 32, 0, false},
	} {
		var got uint64
		err := read(c.b, func(r *matchReader) { got = r.uvarint(c.bits) })
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("uvarint %s: got %d, %v", c.name, got, err)
		}
	}
	for _, c := range []struct {
		b  []byte
		v  int32
		ok bool
	}{{[]byte{0}, 0, true}, {[]byte{1}, -1, true}, {[]byte{2}, 1, true}, {[]byte{0xfe, 0xff, 0xff, 0xff, 0x0f}, 2147483647, true}, {[]byte{0xff, 0xff, 0xff, 0xff, 0x0f}, -2147483648, true}, {[]byte{0x80, 0x80, 0x80, 0x80, 0x10}, 0, false}} {
		var got int32
		err := read(c.b, func(r *matchReader) { got = r.s32() })
		if (err == nil) != c.ok || got != c.v {
			t.Errorf("s32 %x: got %d, %v", c.b, got, err)
		}
		if c.ok {
			var w matchWriter
			w.s32(c.v)
			if !bytes.Equal(w.b, c.b) {
				t.Errorf("s32 %d writes %x, want %x", c.v, w.b, c.b)
			}
		}
	}
	for _, c := range []struct {
		b  byte
		ok bool
	}{{0, true}, {1, true}, {2, false}, {0xff, false}} {
		if err := read([]byte{c.b}, func(r *matchReader) { r.boolean() }); (err == nil) != c.ok {
			t.Errorf("boolean %d: %v", c.b, err)
		}
	}
	if err := read(append([]byte{17}, strings.Repeat("a", 17)...), func(r *matchReader) { r.text(16) }); err == nil {
		t.Error("a 17-byte text(16) was read")
	}
	if err := read([]byte{0xff, 0xff, 0xff, 0xff, 0x0f}, func(r *matchReader) { r.text(1 << 20) }); err == nil {
		t.Error("a text longer than the payload was read")
	}
	if err := read([]byte{0}, func(r *matchReader) { r.key() }); err == nil {
		t.Error("an empty key was read")
	}
	if err := read(append(uvarintBytes(256), strings.Repeat("k", 256)...), func(r *matchReader) { r.key() }); err == nil {
		t.Error("a 256-byte key was read")
	}
	// A failure sticks: nothing after it advances or allocates.
	r := matchReader{b: []byte{2, 0, 0}}
	r.boolean()
	if r.err == nil || r.u8() != 0 || r.text(10) != "" || r.count(10, 1) != 0 || r.off != 0 {
		t.Fatal("a reader kept reading after a failure")
	}
}

// The whole configuration is at most 2 MiB: a request whose encoding would
// be larger does not resolve.
func TestMatchConfigRefusesAnOversizedEncoding(t *testing.T) {
	r := validMatchRequest(t)
	r.UnitRestrictions = nil
	key := strings.Repeat("u", 250)
	for id := 1; id <= 9000; id++ {
		r.UnitRestrictions = append(r.UnitRestrictions, MatchUnitRestriction{DefinitionID: uint16(id), Unit: key + strconv.Itoa(id%10000), Limit: 1})
	}
	if _, err := ResolveMatchConfig(r); err == nil || !strings.Contains(err.Error(), "encoding of at most") {
		t.Fatalf("a configuration over 2 MiB resolved: %v", err)
	}
}

// FuzzDecodeMatchConfig holds the codec property over arbitrary input,
// seeded with the two valid payloads. The fast tier runs the seeds; `go test
// -fuzz` explores further.
func FuzzDecodeMatchConfig(f *testing.F) {
	f.Add(encodeMatchForTest(f, resolveMatch(f, validMatchRequest(f))))
	f.Add(encodeMatchForTest(f, resolveMatch(f, validSurvivalRequest(f))))
	f.Add([]byte{})
	f.Add([]byte{1, 6})
	f.Fuzz(func(t *testing.T, payload []byte) {
		checkMatchDecode(t, payload)
	})
}
