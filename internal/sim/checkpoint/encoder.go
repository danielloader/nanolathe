// Package checkpoint encodes diagnostic simulation state according to
// DESIGN_MULTIPLAYER §16.3.6. It owns no gameplay state or restore path.
package checkpoint

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
)

// Encoder streams the canonical little-endian representation. Its first error
// is sticky; Field supplies diagnostic context without changing the encoding.
// Encoders and their captures are used only on the simulation thread.
type Encoder struct {
	w          io.Writer
	err        error
	path       string
	pathSuffix string
	pathIndex  int
	pathKind   uint8
	owner      Owner
	capture    *Capture
	closed     bool
	absent     bool
	word       [8]byte
}

// NewEncoder creates a standalone encoder. A nil writer is an error; use
// NewCapture with a nil output for a hash-only capture.
func NewEncoder(w io.Writer) *Encoder {
	e := &Encoder{w: w}
	if w == nil {
		e.Fail(errors.New("nil writer"))
	}
	return e
}

// Field sets the logical path used by subsequent errors; it emits no bytes.
func (e *Encoder) Field(path string) {
	e.path, e.pathSuffix, e.pathKind = path, "", 0
}

// FieldChild is Field(prefix + "." + name) without allocating on success.
func (e *Encoder) FieldChild(prefix, name string) {
	e.path, e.pathSuffix, e.pathKind = prefix, name, 1
}

// FieldIndex formats an indexed logical path only when an error occurs.
func (e *Encoder) FieldIndex(prefix string, index int, suffix string) {
	e.path, e.pathIndex, e.pathSuffix, e.pathKind = prefix, index, suffix, 2
}

func (e *Encoder) errorPath() string {
	switch e.pathKind {
	case 1:
		return e.path + "." + e.pathSuffix
	case 2:
		return e.path + "[" + strconv.Itoa(e.pathIndex) + "]" + e.pathSuffix
	default:
		return e.path
	}
}

func (e *Encoder) Bool(v bool) {
	var b uint8
	if v {
		b = 1
	}
	e.U8(b)
}

func (e *Encoder) U8(v uint8) {
	e.word[0] = v
	e.write(e.word[:1])
}

func (e *Encoder) U16(v uint16) {
	binary.LittleEndian.PutUint16(e.word[:2], v)
	e.write(e.word[:2])
}

func (e *Encoder) U32(v uint32) {
	binary.LittleEndian.PutUint32(e.word[:4], v)
	e.write(e.word[:4])
}

func (e *Encoder) U64(v uint64) {
	binary.LittleEndian.PutUint64(e.word[:], v)
	e.write(e.word[:])
}

func (e *Encoder) I8(v int8)   { e.U8(uint8(v)) }
func (e *Encoder) I16(v int16) { e.U16(uint16(v)) }
func (e *Encoder) I32(v int32) { e.U32(uint32(v)) }
func (e *Encoder) I64(v int64) { e.U64(uint64(v)) }

// F32 preserves every non-NaN bit pattern, including signed zero and infinities.
func (e *Encoder) F32(v float32) {
	bits := math.Float32bits(v)
	if bits&0x7f800000 == 0x7f800000 && bits&0x007fffff != 0 {
		e.Fail(errors.New("NaN binary32"))
		return
	}
	e.U32(bits)
}

// F64 preserves every non-NaN bit pattern, including signed zero and infinities.
func (e *Encoder) F64(v float64) {
	bits := math.Float64bits(v)
	if bits&0x7ff0000000000000 == 0x7ff0000000000000 && bits&0x000fffffffffffff != 0 {
		e.Fail(errors.New("NaN binary64"))
		return
	}
	e.U64(bits)
}

// Bytes writes a u32 byte count followed by the unchanged bytes.
func (e *Encoder) Bytes(v []byte) {
	e.Count(len(v))
	e.write(v)
}

// String uses Bytes framing without text normalization or UTF-8 validation.
func (e *Encoder) String(v string) {
	e.Count(len(v))
	if e.Err() == nil {
		e.write([]byte(v))
	}
}

// Count rejects lengths that cannot be represented by the schema's u32 count.
func (e *Encoder) Count(n int) {
	if n < 0 || uint64(n) > math.MaxUint32 {
		e.Fail(fmt.Errorf("length %d is outside u32", n))
		return
	}
	e.U32(uint32(n))
}

func (e *Encoder) Definition(v Definition) {
	e.U8(v.Family)
	e.U32(v.Ordinal)
	e.String(v.Key)
}

func (e *Encoder) Allocation(v Allocation) {
	e.U32(v.Handle)
	e.U64(v.Serial)
}

// Fail records an owner's validation failure. Nil is a no-op. The first
// failure invalidates the whole capture, including bytes already sent to out.
func (e *Encoder) Fail(err error) {
	if err == nil || e.Err() != nil {
		return
	}
	e.err = contextualError(e.owner, e.errorPath(), err)
	if e.capture != nil {
		e.capture.err = e.err
	}
}

func (e *Encoder) Err() error {
	if e.err != nil {
		return e.err
	}
	if e.capture != nil {
		return e.capture.err
	}
	return nil
}

func (e *Encoder) write(p []byte) {
	if e.Err() != nil {
		return
	}
	switch {
	case e.closed || (e.capture != nil && e.owner != 0 && (e.capture.finished || e.capture.current != e)):
		e.Fail(errors.New("section is closed"))
		return
	case e.absent:
		e.Fail(errors.New("absent section has no payload"))
		return
	case e.w == nil:
		e.Fail(errors.New("nil writer"))
		return
	}
	if len(p) == 0 {
		return
	}
	n, err := e.w.Write(p)
	if err != nil {
		e.Fail(err)
	} else if n != len(p) {
		e.Fail(io.ErrShortWrite)
	}
}

func contextualError(owner Owner, path string, err error) error {
	if path == "" {
		path = "<unspecified>"
	}
	return fmt.Errorf("nanolathe: checkpoint owner %s: logical path %s, providers searched [], expected canonical checkpoint: %w", ownerName(owner), path, err)
}
