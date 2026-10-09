package effects

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func checkpointDebrisFixture() *DebrisPool {
	p := NewDebrisPoolWithCapacity(2)
	p.count, p.cursor, p.serial = 3, 2, 9
	// The older occupied block's slot was reused. The newer block includes
	// five absorbed charges; its geometry is still exactly one point.
	p.blocks[0] = debrisBlock{occupied: true, start: 0, charge: 134, slot: 0, generation: 5}
	p.blocks[1] = debrisBlock{occupied: true, start: 134, charge: 127, slot: 0, generation: 9}
	p.blocks[2] = debrisBlock{start: 261, charge: 1739}
	p.slots[0] = debrisSlot{live: true, generation: 9, pointStart: 11, pointCount: 1,
		angles: [3]uint16{1, 2, 3}, angularRates: [3]uint16{4, 5, 6}, explodeOnHit: true, fall: true, lifetime: 65535,
		position: [3]numeric.Fixed{7, -8, 1 << 40}, velocity: [3]numeric.Fixed{9, -10, 11}}
	p.slots[1] = debrisSlot{pointStart: -999, pointCount: -999, generation: 77, position: [3]numeric.Fixed{99}}
	p.points[0], p.points[1], p.points[11] = [3]numeric.Fixed{1, -2, 1 << 40}, [3]numeric.Fixed{3, 4, 5}, [3]numeric.Fixed{-6, 7, -8}
	return p
}

func checkpointDebrisBytes(t *testing.T, p *DebrisPool) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := p.WriteCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// Independent schema vector: all numeric literals are authored here, not
// obtained from the service or encoder. This pins stale occupied generation,
// absorbed-charge geometry, live/dead framing and every retained live field.
func TestCheckpointDebrisVector(t *testing.T) {
	p := checkpointDebrisFixture()
	var want []byte
	u8 := func(v byte) { want = append(want, v) }
	u16 := func(v uint16) { want = binary.LittleEndian.AppendUint16(want, v) }
	u32 := func(v uint32) { want = binary.LittleEndian.AppendUint32(want, v) }
	u64 := func(v uint64) { want = binary.LittleEndian.AppendUint64(want, v) }
	u32(3)
	// blocks: charge, generation, occupied, slot, start.
	u64(134)
	u64(5)
	u8(1)
	u64(0)
	u64(0)
	u64(127)
	u64(9)
	u8(1)
	u64(0)
	u64(134)
	u64(1739)
	u64(0)
	u8(0)
	u64(0)
	u64(261)
	u64(3)
	u64(2) // stored count/cursor
	u32(2) // occupied spans
	u64(0)
	u64(0)
	u32(2)
	for _, v := range []uint64{1, 0xfffffffffffffffe, 1 << 40, 3, 4, 5} {
		u64(v)
	}
	u64(1)
	u64(11)
	u32(1)
	for _, v := range []uint64{0xfffffffffffffffa, 7, 0xfffffffffffffff8} {
		u64(v)
	}
	u64(9)
	u32(2) // serial and physical slot count
	u8(1)
	for _, v := range []uint16{1, 2, 3, 4, 5, 6} {
		u16(v)
	}
	u8(1)
	u8(1)
	u64(9)
	u16(65535)
	u64(1)
	u64(11)
	for _, v := range []uint64{7, 0xfffffffffffffff8, 1 << 40, 9, 0xfffffffffffffff6, 11} {
		u64(v)
	}
	u8(0) // dead slot has no payload
	u64(2000)
	if got := checkpointDebrisBytes(t, p); !bytes.Equal(got, want) {
		t.Fatalf("debris payload\ngot  %x\nwant %x", got, want)
	}
}

func TestCheckpointDebrisRetainedMutationsAndExclusions(t *testing.T) {
	baseline := checkpointDebrisBytes(t, checkpointDebrisFixture())
	for _, test := range []struct {
		name string
		edit func(*DebrisPool)
	}{
		{"cursor", func(p *DebrisPool) { p.cursor = 0 }},
		{"serial", func(p *DebrisPool) { p.serial = 1 << 50 }},
		{"stale generation", func(p *DebrisPool) { p.blocks[0].generation++ }},
		{"stale slot", func(p *DebrisPool) { p.blocks[0].slot = 1 }},
		{"stale geometry", func(p *DebrisPool) { p.points[1][2] = 1 << 45 }},
		{"current geometry", func(p *DebrisPool) { p.points[11][0]-- }},
		{"occupied", func(p *DebrisPool) { p.blocks[0].occupied = false }},
		{"partition boundary", func(p *DebrisPool) { p.blocks[1].charge++; p.blocks[2].start++; p.blocks[2].charge-- }},
		{"live", func(p *DebrisPool) { p.slots[0].live = false }},
		{"angles", func(p *DebrisPool) { p.slots[0].angles[2]++ }},
		{"rates", func(p *DebrisPool) { p.slots[0].angularRates[1]++ }},
		{"explode", func(p *DebrisPool) { p.slots[0].explodeOnHit = false }},
		{"fall", func(p *DebrisPool) { p.slots[0].fall = false }},
		{"generation", func(p *DebrisPool) { p.slots[0].generation = 3 }},
		{"lifetime", func(p *DebrisPool) { p.slots[0].lifetime = 0 }},
		{"point count", func(p *DebrisPool) { p.slots[0].pointCount++ }},
		{"point start", func(p *DebrisPool) { p.slots[0].pointStart++ }},
		{"position", func(p *DebrisPool) { p.slots[0].position[2] = -(1 << 44) }},
		{"velocity", func(p *DebrisPool) { p.slots[0].velocity[1] = 1 << 44 }},
		{"physical slots", func(p *DebrisPool) { p.slots = append(p.slots, debrisSlot{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := checkpointDebrisFixture()
			test.edit(p)
			if bytes.Equal(checkpointDebrisBytes(t, p), baseline) {
				t.Fatal("retained mutation lost")
			}
		})
	}
	p := checkpointDebrisFixture()
	p.slots[1] = debrisSlot{generation: 1 << 50, pointStart: -9999, pointCount: -8888, position: [3]numeric.Fixed{-1}, lifetime: 5, fall: true, explodeOnHit: true}
	s := &p.slots[0]
	s.model, s.pieceIndex, s.geometryName, s.source, s.defID, s.defName = &model.Model{Name: "model"}, 3, "geometry", 65535, 65535, "definition"
	s.renderFlags, s.smoke, s.fire = 0xff, true, true
	p.points[2] = [3]numeric.Fixed{17, 18, 19} // outside both occupied geometry spans
	p.blocks[3] = debrisBlock{occupied: true, charge: -1, slot: -1, generation: 19}
	slots, blocks, points := append([]debrisSlot(nil), p.slots...), append([]debrisBlock(nil), p.blocks...), append([][3]numeric.Fixed(nil), p.points...)
	if got := checkpointDebrisBytes(t, p); !bytes.Equal(got, baseline) || !reflect.DeepEqual(slots, p.slots) || !reflect.DeepEqual(blocks, p.blocks) || !reflect.DeepEqual(points, p.points) {
		t.Fatal("excluded debris state changed bytes or capture mutated arena")
	}
	zero := &DebrisPool{}
	before := *zero
	checkpointDebrisBytes(t, zero)
	if !reflect.DeepEqual(before, *zero) {
		t.Fatal("capture initialized zero debris storage")
	}
}

func TestCheckpointDebrisExpiredSlotChargedPartition(t *testing.T) {
	p := NewDebrisPoolWithCapacity(1)
	if !p.Admit(DebrisRequest{Points: [][3]numeric.Fixed{{1, 2, 3}}, Position: [3]numeric.Fixed{0, 10 << 16, 0}}) {
		t.Fatal("admit fixture")
	}
	p.Step(DebrisStepContext{}, nil) // zero lifetime expires, leaving the block charged
	if p.slots[0].live || !p.blocks[0].occupied {
		t.Fatal("fixture did not retain expired partition")
	}
	before := checkpointDebrisBytes(t, p)
	p.points[0][0] = 9
	if bytes.Equal(before, checkpointDebrisBytes(t, p)) {
		t.Fatal("charged expired geometry omitted")
	}
	var summary checkpoint.Summary
	if err := p.AppendCheckpointSummary(&summary); err != nil {
		t.Fatal(err)
	}
	if n, sum := summary.Result(); n != 1 || sum != 0 {
		t.Fatalf("summary must count live slots, got (%d,%d)", n, sum)
	}
	oldGeneration := p.blocks[0].generation
	if !p.Admit(DebrisRequest{Points: [][3]numeric.Fixed{{4, 5, 6}}, Lifetime: 7}) || p.blocks[0].generation != oldGeneration || p.slots[0].generation == oldGeneration {
		t.Fatal("fixture did not reuse a slot before clearing its old partition")
	}
	checkpointDebrisBytes(t, p) // stale partition need not own today's generation
}

func TestCheckpointDebrisBoundsRefusal(t *testing.T) {
	for _, test := range []struct {
		field string
		edit  func(*DebrisPool)
	}{
		{"storageCharge", func(p *DebrisPool) { p.storageCharge = -1 }},
		{"storageCharge", func(p *DebrisPool) { p.blocks = p.blocks[:1] }},
		{"storageCharge", func(p *DebrisPool) { p.points = nil }},
		{"count", func(p *DebrisPool) { p.count = -1 }},
		{"count", func(p *DebrisPool) { p.count = len(p.blocks) + 1 }},
		{"cursor", func(p *DebrisPool) { p.cursor = p.count }},
		{"blocks[1]", func(p *DebrisPool) { p.blocks[1].start++ }},
		{"blocks[1]", func(p *DebrisPool) { p.blocks[1].charge = int(^uint(0) >> 1) }},
		{"blocks[0]", func(p *DebrisPool) { p.blocks[0].slot = -1 }},
		{"blocks[0]", func(p *DebrisPool) { p.blocks[0].charge = 100; p.blocks[1].start = 100 }},
		{"blocks", func(p *DebrisPool) { p.blocks[2].charge-- }},
		{"slots[0]", func(p *DebrisPool) { p.slots[0].pointStart = -1 }},
		{"slots[0]", func(p *DebrisPool) { p.slots[0].pointCount = -1 }},
		{"slots[0]", func(p *DebrisPool) { p.slots[0].pointCount = int(^uint(0) >> 1) }},
	} {
		p := checkpointDebrisFixture()
		test.edit(p)
		var out bytes.Buffer
		e := checkpoint.NewEncoder(&out)
		err := p.WriteCheckpoint(e)
		if err == nil || !strings.HasPrefix(err.Error(), "nanolathe:") || !strings.Contains(err.Error(), "effects.DebrisPool."+test.field) || out.Len() != 0 {
			t.Fatalf("refusal %s = %v, bytes=%d", test.field, err, out.Len())
		}
		e.U8(1)
		if e.Err() != err || out.Len() != 0 {
			t.Fatal("failure not sticky")
		}
	}
	var out bytes.Buffer
	if err := (*DebrisPool)(nil).WriteCheckpoint(checkpoint.NewEncoder(&out)); err == nil {
		t.Fatal("nil debris pool accepted")
	}
}
