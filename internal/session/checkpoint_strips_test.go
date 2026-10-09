package session

import (
	"bytes"
	"encoding/binary"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

func checkpointStripFixture() *stripTable {
	t := newStripTableWithSfxLimit(0)
	t.live = 1
	t.strips[2] = []stripObject{{
		family: stripFamilySmoke,
		dst:    [3]numeric.Fixed{-1, 1 << 40, 3}, dstExtent: [3]numeric.Fixed{4, 5, 6},
		frameCountBase: 7, frameDelayParam: -8, nextSpawn: 9, particleLife: 10,
		particles: []stripParticle{{expiry: 11, frame: -12, frameDelay: 13, lastFrame: 14,
			lastFrameDraw: 15, lastFrameDrawn: true, phase: 16, vx: -17, vy: 18, vz: 19, x: 20, y: -(1 << 41), z: 22}},
		phaseModulus: 23, smokeSelector: 1, spawnInterval: 25,
		src: [3]numeric.Fixed{26, 27, 28}, srcExtent: [3]numeric.Fixed{29, 30, 31}, windowEnd: 32,
	}}
	return t
}

func checkpointStripBytes(t *testing.T, strips *stripTable) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := strips.writeCheckpoint(checkpoint.NewEncoder(&out)); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestCheckpointStripByteVector(t *testing.T) {
	var want bytes.Buffer
	// Independent literal schema vector: table scalars, ten lists, one object
	// in list two and one particle. Fixed arrays have no count prefix.
	for _, v := range []any{int64(1), int64(1000), int64(400), uint32(0), uint32(0), uint32(1),
		int64(-1), int64(1 << 40), int64(3), int64(4), int64(5), int64(6), uint8(2),
		int32(7), int32(-8), uint32(9), int32(10), uint32(1),
		uint32(11), int32(-12), int32(13), int32(14), int32(15), uint8(1), int32(16),
		int64(-17), int64(18), int64(19), int64(20), int64(-(1 << 41)), int64(22),
		int32(23), uint8(1), int32(25), int64(26), int64(27), int64(28), int64(29), int64(30), int64(31), uint32(32),
		uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0), uint32(0)} {
		if err := binary.Write(&want, binary.LittleEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	if got := checkpointStripBytes(t, checkpointStripFixture()); !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("strip bytes\ngot  %x\nwant %x", got, want.Bytes())
	}
}

func TestCheckpointStripLifetimeOrderingAndExclusions(t *testing.T) {
	base := checkpointStripBytes(t, checkpointStripFixture())
	for name, change := range map[string]func(*stripTable){
		"deferred draw":      func(s *stripTable) { s.strips[2][0].particles[0].lastFrameDraw++ },
		"draw presence":      func(s *stripTable) { s.strips[2][0].particles[0].lastFrameDrawn = false },
		"lifetime input":     func(s *stripTable) { s.strips[2][0].frameCountBase++ },
		"high velocity bits": func(s *stripTable) { s.strips[2][0].particles[0].vx += 1 << 40 },
		"capacity":           func(s *stripTable) { s.poolCapacity++ },
		"strip index":        func(s *stripTable) { s.strips[3], s.strips[2] = s.strips[2], nil },
	} {
		t.Run(name, func(t *testing.T) {
			s := checkpointStripFixture()
			change(s)
			if bytes.Equal(base, checkpointStripBytes(t, s)) {
				t.Fatal("retained future state lost")
			}
		})
	}
	s := checkpointStripFixture()
	s.nanoColorCursors[0] = 71
	s.particleFree = [][]stripParticle{{{x: 999}}}
	o := &s.strips[2][0]
	o.nanoOwnerColor, o.nanoOwnerColorKnown, o.nanoInfected, o.colorSel = 9, true, true, 5
	o.nanoColorCursor = &s.nanoColorCursors[0]
	p := &o.particles[0]
	p.color, p.colorSample, p.colorSequence, p.reservedWord = 8, 9, 10, 11
	if !bytes.Equal(base, checkpointStripBytes(t, s)) || s.nanoColorCursors[0] != 71 {
		t.Fatal("presentation or recycled storage entered capture")
	}
	o.particles = append(o.particles, stripParticle{x: 99})
	first := checkpointStripBytes(t, s)
	o.particles[0], o.particles[1] = o.particles[1], o.particles[0]
	if bytes.Equal(first, checkpointStripBytes(t, s)) {
		t.Fatal("particle order lost")
	}
	// An over-steady list is retained, not evicted or refused at capture.
	s.steadyCap = 0
	_ = checkpointStripBytes(t, s)
}

func TestCheckpointStripSummaryVectorPurityAndBlindSpots(t *testing.T) {
	s := checkpointStripFixture()
	var got checkpoint.Summary
	got.Word(17)
	if err := s.appendCheckpointSummary(&got); err != nil {
		t.Fatal(err)
	}
	words := []int64{17, 1, 2, 9, 32, 1, 20, -(1 << 41), 22, 11, -12, 13, 16}
	var want uint64
	for i, v := range words {
		want += uint64(i+1) * uint64(v)
	}
	if n, sum := got.Result(); n != uint64(len(words)) || sum != want {
		t.Fatalf("summary (%d,%x), want (%d,%x)", n, sum, len(words), want)
	}
	before := checkpointStripBytes(t, s)
	s.strips[2][0].particles[0].lastFrameDraw++
	var blind checkpoint.Summary
	blind.Word(17)
	if err := s.appendCheckpointSummary(&blind); err != nil || blind != got || bytes.Equal(before, checkpointStripBytes(t, s)) {
		t.Fatal("deferred draw must be retained in full, but a deliberate summary blind spot")
	}
	s.strips[2][0].particles[0].lastFrameDraw--
	other := checkpointStripFixture()
	if allocs := testing.AllocsPerRun(20, func() {
		var summary checkpoint.Summary
		if err := s.appendCheckpointSummary(&summary); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("summary allocated %g", allocs)
	}
	_ = checkpointStripBytes(t, s)
	if !reflect.DeepEqual(s, other) {
		t.Fatal("capture modified strip state")
	}
	// Full capture refuses invalid capacities; the selected-only summary
	// neither reads them nor walks unrelated validation state.
	s.poolCapacity, s.steadyCap = 0, -1
	var unselected checkpoint.Summary
	unselected.Word(17)
	if err := s.appendCheckpointSummary(&unselected); err != nil || unselected != got {
		t.Fatal("unselected capacities changed cheap summary", err)
	}
	s.strips[2][0].particles[0].x++
	var selected checkpoint.Summary
	selected.Word(17)
	if err := s.appendCheckpointSummary(&selected); err != nil || selected == got {
		t.Fatal("selected coordinate change lost")
	}
}

func TestCheckpointStripRefusalsAreAtomic(t *testing.T) {
	for name, change := range map[string]func(*stripTable) *stripTable{
		"absent":   func(*stripTable) *stripTable { return nil },
		"live":     func(s *stripTable) *stripTable { s.live++; return s },
		"capacity": func(s *stripTable) *stripTable { s.poolCapacity = 0; return s },
		"steady":   func(s *stripTable) *stripTable { s.steadyCap = -1; return s },
		"family":   func(s *stripTable) *stripTable { s.strips[2][0].family = 7; return s },
	} {
		t.Run(name, func(t *testing.T) {
			s := change(checkpointStripFixture())
			var out bytes.Buffer
			if err := s.writeCheckpoint(checkpoint.NewEncoder(&out)); err == nil || !strings.Contains(err.Error(), "effects.strips") || out.Len() != 0 {
				t.Fatalf("invalid strip state admitted or partially emitted: %v", err)
			}
			var summary checkpoint.Summary
			summary.Word(99)
			before := summary
			if s == nil {
				if err := s.appendCheckpointSummary(&summary); err == nil || summary != before {
					t.Fatal("absent strip table changed summary")
				}
			} else if err := s.appendCheckpointSummary(&summary); err != nil {
				t.Fatal("cheap summary validated full-only constraints", err)
			}
		})
	}
	s := checkpointStripFixture()
	if err := s.appendCheckpointSummary(nil); err == nil {
		t.Fatal("absent summary accepted")
	}
	e := checkpoint.NewEncoder(io.Discard)
	e.Fail(io.ErrClosedPipe)
	before := e.Err()
	if err := s.writeCheckpoint(e); err != before {
		t.Fatalf("earlier encoder error replaced: %v", err)
	}
}
