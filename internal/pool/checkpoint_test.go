package pool

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func unitCheckpointBytes(t *testing.T, p *Units) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestUnitCheckpointEmptyArenaVector(t *testing.T) {
	// Schema fields in order: one sentinel alive byte, one sentinel defID,
	// int64 limit, then ten (end, start) pairs. Empty bounds are [-1, 0].
	want := []byte{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0}
	want = binary.LittleEndian.AppendUint64(want, 0)
	for i := 0; i < PlayerCount; i++ {
		want = binary.LittleEndian.AppendUint64(want, ^uint64(0))
		want = binary.LittleEndian.AppendUint64(want, 0)
	}
	if got := unitCheckpointBytes(t, NewUnitsSliced(0)); !bytes.Equal(got, want) {
		t.Fatalf("empty arena vector = %x, want %x", got, want)
	}
}

func TestUnitCheckpointOccupancyReuseAndPermutation(t *testing.T) {
	p := NewUnitsSliced(2)
	empty := unitCheckpointBytes(t, p)
	h, ok := p.AllocForPlayerWithDef(3, 7, false, 0)
	if !ok {
		t.Fatal("allocation refused")
	}
	occupied := unitCheckpointBytes(t, p)
	if bytes.Equal(empty, occupied) || p.Used() != 1 {
		t.Fatal("occupancy was omitted or capture mutated the allocator")
	}
	p.Free(h)
	if got := unitCheckpointBytes(t, p); !bytes.Equal(got, empty) {
		t.Fatal("freed pool retained state owned by the unit world's raw records")
	}
	reused, ok := p.AllocForPlayerWithDef(3, 9, false, 0)
	if !ok || reused != h || bytes.Equal(unitCheckpointBytes(t, p), occupied) {
		t.Fatal("definition identity was lost across immediate slot reuse")
	}
	order := IdentityPlayerPermutation()
	order[0], order[9] = order[9], order[0]
	permuted, err := NewUnitsSlicedWithOrder(2, order)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(unitCheckpointBytes(t, permuted), empty) {
		t.Fatal("player slice ownership was sorted away")
	}
}

func TestUnitCheckpointValidatesDerivedFields(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Units)
		path string
	}{
		{"used", func(p *Units) { p.used++ }, "used count"},
		{"identity", func(p *Units) { p.slotIndex[1] = 0 }, "slotIndex"},
		{"length", func(p *Units) { p.defID = p.defID[:1] }, "lengths"},
		{"sentinel", func(p *Units) { p.alive[0], p.defID[0] = true, 1 }, "slot 0"},
		{"occupancy", func(p *Units) { p.defID[1] = 1 }, "disagree"},
		{"slice", func(p *Units) { p.slices[1] = p.slices[0] }, "repeats"},
		{"sliced", func(p *Units) { p.sliced = false }, "sliced"},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := NewUnitsSliced(1)
			test.edit(p)
			var out bytes.Buffer
			err := p.WriteCheckpoint(checkpoint.NewEncoder(&out))
			if err == nil || !strings.Contains(err.Error(), test.path) || out.Len() != 0 {
				t.Fatalf("invalid allocator capture = %v, %d bytes", err, out.Len())
			}
		})
	}
}
