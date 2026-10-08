package checkpoint

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
)

// Authored byte vectors lock widths, two's complement, little-endian order and
// unchanged text/float bits from DESIGN_MULTIPLAYER §16.3.6, not Go layout.
func TestEncoderCanonicalValues(t *testing.T) {
	tests := []struct {
		name string
		put  func(*Encoder)
		hex  string
	}{
		{"false", func(e *Encoder) { e.Bool(false) }, "00"},
		{"true", func(e *Encoder) { e.Bool(true) }, "01"},
		{"u8", func(e *Encoder) { e.U8(0xab) }, "ab"},
		{"u16", func(e *Encoder) { e.U16(0x89ab) }, "ab89"},
		{"u32", func(e *Encoder) { e.U32(0x01234567) }, "67452301"},
		{"u64", func(e *Encoder) { e.U64(0x0123456789abcdef) }, "efcdab8967452301"},
		{"i8", func(e *Encoder) { e.I8(-2) }, "fe"},
		{"i16", func(e *Encoder) { e.I16(-0x1234) }, "cced"},
		{"i32", func(e *Encoder) { e.I32(-0x1234567) }, "99badcfe"},
		{"i64", func(e *Encoder) { e.I64(-0x123456789abcdef) }, "1132547698badcfe"},
		{"f32 positive zero", func(e *Encoder) { e.F32(0) }, "00000000"},
		{"f32 negative zero", func(e *Encoder) { e.F32(math.Float32frombits(0x80000000)) }, "00000080"},
		{"f32 finite", func(e *Encoder) { e.F32(1.5) }, "0000c03f"},
		{"f32 subnormal", func(e *Encoder) { e.F32(math.Float32frombits(1)) }, "01000000"},
		{"f32 positive infinity", func(e *Encoder) { e.F32(math.Float32frombits(0x7f800000)) }, "0000807f"},
		{"f32 negative infinity", func(e *Encoder) { e.F32(math.Float32frombits(0xff800000)) }, "000080ff"},
		{"f64 positive zero", func(e *Encoder) { e.F64(0) }, "0000000000000000"},
		{"f64 negative zero", func(e *Encoder) { e.F64(math.Float64frombits(0x8000000000000000)) }, "0000000000000080"},
		{"f64 finite", func(e *Encoder) { e.F64(1.5) }, "000000000000f83f"},
		{"f64 subnormal", func(e *Encoder) { e.F64(math.Float64frombits(1)) }, "0100000000000000"},
		{"f64 positive infinity", func(e *Encoder) { e.F64(math.Inf(1)) }, "000000000000f07f"},
		{"f64 negative infinity", func(e *Encoder) { e.F64(math.Inf(-1)) }, "000000000000f0ff"},
		{"nil bytes", func(e *Encoder) { e.Bytes(nil) }, "00000000"},
		{"bytes", func(e *Encoder) { e.Bytes([]byte{0xff, 0, 1, 0x80}) }, "04000000ff000180"},
		{"string", func(e *Encoder) { e.String("a\x00\xffé") }, "050000006100ffc3a9"},
		{"empty string", func(e *Encoder) { e.String("") }, "00000000"},
		{"count", func(e *Encoder) { e.Count(257) }, "01010000"},
		{"definition", func(e *Encoder) { e.Definition(Definition{Family: 7, Ordinal: 0x12345678, Key: "Arm\x00"}) }, "07785634120400000041726d00"},
		{"allocation", func(e *Encoder) { e.Allocation(Allocation{Handle: 0x12345678, Serial: 0x0123456789abcdef}) }, "78563412efcdab8967452301"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			e := NewEncoder(&out)
			e.Field("authored.value")
			e.Fail(nil)
			tt.put(e)
			if err := e.Err(); err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(out.Bytes()); got != tt.hex {
				t.Fatalf("bytes = %s, want %s", got, tt.hex)
			}
		})
	}
}

func TestEncoderRejectsNaNWithoutWriting(t *testing.T) {
	for _, bits := range []uint32{0x7f800001, 0x7fc00000, 0xff800001, 0xffffffff} {
		var out bytes.Buffer
		e := NewEncoder(&out)
		e.Field("stock.metal")
		e.F32(math.Float32frombits(bits))
		assertNaNFailure(t, e, &out)
	}
	for _, bits := range []uint64{0x7ff0000000000001, 0x7ff8000000000000, 0xfff0000000000001, 0xffffffffffffffff} {
		var out bytes.Buffer
		e := NewEncoder(&out)
		e.Field("stock.metal")
		e.F64(math.Float64frombits(bits))
		assertNaNFailure(t, e, &out)
	}
}

func assertNaNFailure(t *testing.T, e *Encoder, out *bytes.Buffer) {
	t.Helper()
	first := e.Err()
	if first == nil || !strings.Contains(first.Error(), "stock.metal") || !strings.Contains(first.Error(), "NaN") {
		t.Fatalf("missing NaN field context: %v", first)
	}
	e.Field("later.field")
	e.Fail(errors.New("later error"))
	e.String("ignored")
	if out.Len() != 0 || e.Err() != first {
		t.Fatalf("failure was not sticky: %x, %v", out.Bytes(), e.Err())
	}
}

func TestEncoderCountBounds(t *testing.T) {
	bad := []int{-1}
	if strconv.IntSize == 64 {
		tooLarge := uint64(math.MaxUint32) + 1
		bad = append(bad, int(tooLarge))
		var out bytes.Buffer
		e := NewEncoder(&out)
		largest := uint64(math.MaxUint32)
		e.Count(int(largest))
		if e.Err() != nil || !bytes.Equal(out.Bytes(), []byte{255, 255, 255, 255}) {
			t.Fatalf("u32 maximum count: %x, %v", out.Bytes(), e.Err())
		}
	}
	for _, n := range bad {
		var out bytes.Buffer
		e := NewEncoder(&out)
		e.Field("records.length")
		e.Count(n)
		e.U8(1)
		if e.Err() == nil || out.Len() != 0 || !strings.Contains(e.Err().Error(), "records.length") {
			t.Fatalf("count %d accepted or missing context: %x, %v", n, out.Bytes(), e.Err())
		}
	}
}

// faultSink models both an ordinary partial failure and a broken writer that
// returns a short count with no error. Neither permits a later write.
type faultSink struct {
	bytes.Buffer
	remaining int
	err       error
	calls     int
}

func (w *faultSink) Write(p []byte) (int, error) {
	w.calls++
	if len(p) <= w.remaining {
		w.remaining -= len(p)
		return w.Buffer.Write(p)
	}
	n := w.remaining
	w.remaining = 0
	_, _ = w.Buffer.Write(p[:n])
	return n, w.err
}

func TestEncoderSinkAndValidationFailures(t *testing.T) {
	sinkError := errors.New("authored sink failure")
	for _, cause := range []error{nil, sinkError} {
		w := &faultSink{remaining: 2, err: cause}
		e := NewEncoder(w)
		e.Field("records[1].serial")
		e.U64(0x0807060504030201)
		want := cause
		if want == nil {
			want = io.ErrShortWrite
		}
		if !errors.Is(e.Err(), want) || !strings.Contains(e.Err().Error(), "records[1].serial") {
			t.Fatalf("sink failure = %v, want %v with context", e.Err(), want)
		}
		e.Bytes([]byte{3})
		if w.calls != 1 || !bytes.Equal(w.Bytes(), []byte{1, 2}) {
			t.Fatalf("wrote after failure: calls %d, bytes %x", w.calls, w.Bytes())
		}
	}
	var out bytes.Buffer
	e := NewEncoder(&out)
	e.Field("program")
	e.Fail(sinkError)
	e.U32(42)
	if !errors.Is(e.Err(), sinkError) || out.Len() != 0 {
		t.Fatalf("validation error lost: %v, %x", e.Err(), out.Bytes())
	}
	if e := NewEncoder(nil); e.Err() == nil {
		t.Fatal("nil writer accepted")
	}
	var zero Encoder
	zero.U8(1)
	if zero.Err() == nil {
		t.Fatal("uninitialized encoder accepted a write")
	}
}
