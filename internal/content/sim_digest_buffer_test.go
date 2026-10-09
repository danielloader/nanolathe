package content

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"testing"
)

// Independently frame every primitive, crossing the bounded buffer many times.
// Repeated sum and appending after sum retain hash.Hash's ordinary contract.
func TestSemanticDigestBufferedFraming(t *testing.T) {
	d := newSemanticDigest("authored/domain")
	var want []byte
	text := func(s string) { want = binary.AppendUvarint(want, uint64(len(s))); want = append(want, s...) }
	text("authored/domain")
	for i := range 1100 {
		d.u8(uint8(i))
		want = append(want, uint8(i))
		d.boolean(i%2 == 0)
		if i%2 == 0 {
			want = append(want, 1)
		} else {
			want = append(want, 0)
		}
		d.u64(uint64(i) * 0x100000001)
		want = binary.AppendUvarint(want, uint64(i)*0x100000001)
		d.s64(-int64(i))
		want = binary.AppendVarint(want, -int64(i))
		v := strings.Repeat("x", i%513)
		d.text(v)
		text(v)
		f := math.Float32frombits(0x80000000 | uint32(i))
		d.f32(f)
		want = binary.LittleEndian.AppendUint32(want, math.Float32bits(f))
		g := math.Float64frombits(0xfff0000000000000 | uint64(i))
		d.f64(g)
		want = binary.LittleEndian.AppendUint64(want, math.Float64bits(g))
		digest := sha256.Sum256([]byte(v))
		d.digest(digest)
		want = append(want, digest[:]...)
		d.write([]byte{3, 4, 5})
		want = append(want, 3, 4, 5)
		if i%97 == 0 {
			first, second := d.sum(), d.sum()
			expected := sha256.Sum256(want)
			if first != expected || second != expected {
				t.Fatal("intermediate digest mismatch")
			}
		}
	}
	if d.sum() != sha256.Sum256(want) {
		t.Fatal("semantic framing changed")
	}
}

func TestSemanticDigestTextAllocations(t *testing.T) {
	d := newSemanticDigest("authored/domain")
	value := strings.Repeat("authored", 1000)
	if n := testing.AllocsPerRun(10, func() { d.text(value); d.sum() }); n != 0 {
		t.Fatalf("successful streaming text allocates: %v", n)
	}
}
