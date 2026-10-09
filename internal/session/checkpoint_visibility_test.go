package session

import (
	"bytes"
	"io"
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

func visibilityTailCheckpointBytes(t *testing.T, s *Session) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := s.writeCheckpointVisibilityTail(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func visibilityTailCheckpointFixture() *Session {
	return &Session{
		postLoop: &postLoopState{eyeballs: eyeballList{records: []eyeballRecord{
			{cx: -1, cz: 2, emitter: 0x83, expiry: 0x12345678, heightByte: 0x84, owner: 9, published: true, sightDistance: -3, x: -0x100000001, y: 4, z: -5},
			{cx: 6, cz: -7, emitter: 8, expiry: 9, heightByte: 10, owner: 1, sightDistance: 0x1234, x: 11, y: -12, z: 13},
		}}},
		visStamps: map[int]visStamp{10: {cx: -14, cz: 15, radius: -16}, 2: {cx: 17, cz: -18, radius: 19}},
	}
}

func TestCheckpointVisibilityTailVector(t *testing.T) {
	s := visibilityTailCheckpointFixture()
	want := runtimeCheckpointHex(t,
		"01 02000000",
		"ffffffff 02000000 83 78563412 84 09 01 fdff ffffffff feffffff 0400000000000000 fbffffffffffffff",
		"06000000 f9ffffff 08 09000000 0a 01 00 3412 0b00000000000000 f4ffffffffffffff 0d00000000000000",
		"02000000 02000000 11000000 eeffffff 13000000 0a000000 f2ffffff 0f000000 f0ffffff",
	)
	before := visibilityTailCheckpointFixture()
	if got := visibilityTailCheckpointBytes(t, s); !bytes.Equal(got, want) {
		t.Fatalf("tail=%x want=%x", got, want)
	}
	if !reflect.DeepEqual(s, before) {
		t.Fatal("tail capture mutated state")
	}
	// A different map insertion order produces the same numeric-key stream.
	s.visStamps = map[int]visStamp{}
	s.visStamps[2] = before.visStamps[2]
	s.visStamps[10] = before.visStamps[10]
	if !bytes.Equal(visibilityTailCheckpointBytes(t, s), want) {
		t.Fatal("stamp order depends on map insertion")
	}
	s.postLoop.eyeballs.records[0], s.postLoop.eyeballs.records[1] = s.postLoop.eyeballs.records[1], s.postLoop.eyeballs.records[0]
	if bytes.Equal(visibilityTailCheckpointBytes(t, s), want) {
		t.Fatal("eyeballs were sorted instead of preserving list order")
	}
}

func TestCheckpointVisibilityTailPresenceAndResiduals(t *testing.T) {
	s := &Session{}
	absent := runtimeCheckpointHex(t, "00 00000000")
	if !bytes.Equal(visibilityTailCheckpointBytes(t, s), absent) || s.postLoop != nil {
		t.Fatal("absent tail capture allocated or changed framing")
	}
	s.visStamps = map[int]visStamp{}
	if !bytes.Equal(visibilityTailCheckpointBytes(t, s), absent) {
		t.Fatal("nil and empty stamp maps differ")
	}
	s.postLoop = &postLoopState{}
	if got := visibilityTailCheckpointBytes(t, s); !bytes.Equal(got, runtimeCheckpointHex(t, "01 00000000 00000000")) {
		t.Fatalf("present empty tail=%x", got)
	}
	s = visibilityTailCheckpointFixture()
	want := visibilityTailCheckpointBytes(t, s)
	records := make([]eyeballRecord, 3)
	copy(records, s.postLoop.eyeballs.records)
	records[2] = eyeballRecord{x: 99, owner: 8, published: true}
	s.postLoop.eyeballs.records = records[:2]
	s.postLoop.trace = []string{"ignored"}
	s.postLoop.traceEnabled = true
	s.postLoop.traceLimit = 17
	s.postLoop.traceDropped = 18
	s.postLoop.publicationCount = 19
	s.postLoop.messageRetire = func(uint32) bool { panic("capture retired a message") }
	s.postLoop.hooks.Barrier = func(int, uint32) { panic("capture called a barrier") }
	if !bytes.Equal(visibilityTailCheckpointBytes(t, s), want) {
		t.Fatal("residual or excluded tail state entered payload")
	}
	for _, tc := range []struct {
		name string
		edit func(*eyeballRecord)
	}{
		{"cell", func(r *eyeballRecord) { r.cx++ }}, {"cell z", func(r *eyeballRecord) { r.cz++ }},
		{"emitter", func(r *eyeballRecord) { r.emitter++ }}, {"expiry", func(r *eyeballRecord) { r.expiry++ }},
		{"height", func(r *eyeballRecord) { r.heightByte++ }}, {"owner", func(r *eyeballRecord) { r.owner++ }},
		{"published", func(r *eyeballRecord) { r.published = !r.published }}, {"radius", func(r *eyeballRecord) { r.sightDistance++ }},
		{"x", func(r *eyeballRecord) { r.x++ }}, {"y", func(r *eyeballRecord) { r.y++ }}, {"z", func(r *eyeballRecord) { r.z++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := visibilityTailCheckpointFixture()
			tc.edit(&s.postLoop.eyeballs.records[0])
			if bytes.Equal(visibilityTailCheckpointBytes(t, s), want) {
				t.Fatal("retained field omitted")
			}
		})
	}
}

func TestCheckpointVisibilityTailRefusals(t *testing.T) {
	for _, h := range []int{-1, 1 << 32} {
		s := &Session{visStamps: map[int]visStamp{h: {}}}
		e := checkpoint.NewEncoder(io.Discard)
		if err := s.writeCheckpointVisibilityTail(e); err == nil || e.Err() == nil {
			t.Fatalf("out-of-range stamp key %d accepted", h)
		}
	}
	s := &Session{visStamps: map[int]visStamp{int(^uint32(0)): {}}}
	if err := s.writeCheckpointVisibilityTail(checkpoint.NewEncoder(io.Discard)); err != nil {
		t.Fatalf("u32 edge rejected: %v", err)
	}
	if err := s.writeCheckpointVisibilityTail(nil); err == nil {
		t.Fatal("nil encoder accepted")
	}
	e := checkpoint.NewEncoder(io.Discard)
	if err := (*Session)(nil).writeCheckpointVisibilityTail(e); err == nil || e.Err() == nil {
		t.Fatal("nil session accepted")
	}
}

func TestCheckpointVisibilityTailSummaryVectorAndPurity(t *testing.T) {
	s := visibilityTailCheckpointFixture()
	var got checkpoint.Summary
	got.Word(17)
	// Prefix 17; count 2; first 9/-4294967297/4/-5/305419896/1;
	// second 1/11/-12/13/9/0. The signed weighted sum wraps modulo 2^64.
	before := visibilityTailCheckpointFixture()
	if err := s.appendCheckpointVisibilityTailSummary(&got); err != nil {
		t.Fatal(err)
	}
	if n, sum := got.Result(); n != 14 || sum != 18446744058667622006 || !reflect.DeepEqual(s, before) {
		t.Fatalf("summary=%d,%d or capture purity mismatch", n, sum)
	}
	want := got
	if n := testing.AllocsPerRun(100, func() {
		var sum checkpoint.Summary
		if err := s.appendCheckpointVisibilityTailSummary(&sum); err != nil {
			panic(err)
		}
	}); n != 0 {
		t.Fatalf("summary allocations=%v", n)
	}
	r := &s.postLoop.eyeballs.records[0]
	r.cx++
	r.cz++
	r.heightByte++
	r.emitter++
	r.sightDistance++
	s.visStamps[-1] = visStamp{radius: 99} // Even full-capture-only validation is absent.
	var blind checkpoint.Summary
	blind.Word(17)
	if err := s.appendCheckpointVisibilityTailSummary(&blind); err != nil || blind != got {
		t.Fatal("summary read unselected fields/map")
	}
	if err := (*Session)(nil).appendCheckpointVisibilityTailSummary(&got); err == nil || got != want {
		t.Fatal("nil session did not fail atomically")
	}
	if err := s.appendCheckpointVisibilityTailSummary(nil); err == nil {
		t.Fatal("nil summary accepted")
	}
	empty := &Session{}
	var sum checkpoint.Summary
	sum.Word(17)
	if err := empty.appendCheckpointVisibilityTailSummary(&sum); err != nil {
		t.Fatal(err)
	}
	if n, v := sum.Result(); n != 2 || v != 17 || empty.postLoop != nil {
		t.Fatal("absent tail did not append exactly zero count")
	}
}
