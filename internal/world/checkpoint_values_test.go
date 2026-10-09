package world

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func TestCheckpointFootprintStoredVector(t *testing.T) {
	extent, err := NewFootprintExtent(3, 4)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewFootprintRect(NewFootprintAnchor(-1, 2), extent)
	if err != nil {
		t.Fatal(err)
	}
	want, err := hex.DecodeString("ffffffff020000000400000001030000000200000006000000")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := r.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("payload = %x, want %x", b.Bytes(), want)
	}
	// A stored residual endpoint must not be silently regenerated at capture.
	r.maxX = -1
	b.Reset()
	if err := r.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	copy(want[17:21], []byte{255, 255, 255, 255})
	if !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("stored endpoint = %x, want %x", b.Bytes(), want)
	}
}

func TestCheckpointFootprintEmptyInitialization(t *testing.T) {
	var zero FootprintRect
	extent, err := NewFootprintExtent(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := NewFootprintRect(NewFootprintAnchor(0, 0), extent)
	if err != nil {
		t.Fatal(err)
	}
	var a, b bytes.Buffer
	if err := zero.WriteCheckpoint(checkpoint.NewEncoder(&a)); err != nil {
		t.Fatal(err)
	}
	if err := empty.WriteCheckpoint(checkpoint.NewEncoder(&b)); err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 25)
	want[12] = 1
	if !bytes.Equal(a.Bytes(), make([]byte, 25)) || !bytes.Equal(b.Bytes(), want) {
		t.Fatalf("initialization lost: zero=%x empty=%x", a.Bytes(), b.Bytes())
	}
}
