package aikit

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/ai"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

func executorCheckpointContext() *ai.CheckpointContext {
	return ai.NewCheckpointContext(&units.CheckpointContext{}, &world.CheckpointContext{})
}

func executorCheckpointBytes(t *testing.T, e *executor) []byte {
	t.Helper()
	return checkpointLeafBytes(t, func(enc *checkpoint.Encoder) error { return e.writeCheckpoint(enc, executorCheckpointContext()) })
}

func TestCheckpointExecutorAuthoredVector(t *testing.T) {
	e := &executor{dedupe: checkpointDedupeFixture(), freeSeq: 0x10203040, freeSlot: (1 << 40) + 3,
		lastFill: 0x12345678, lastTick: 0x87654321, nextPending: -(1 << 40) + 5, tokens: -(1 << 48) + 7,
		selfGrid: exitGrid{cost: []int32{100, 200, 300}, seen: []uint32{3, 9}, stamp: 1}, spotCover: []bool{true, false, true}}
	e.frees[2] = checkpointFreeFixture()
	e.grids[3] = checkpointGuardFixture()
	e.pending[15] = pendingSite{cx: -1, cz: 2, fx: 3, fz: 4, g: 0xfe, tick: ^uint32(0)}
	want := checkpointDecode(t, checkpointDedupeVector+"40302010"+"0300000000010000")
	// Fixed physical arrays have no count and retain their empty early rows.
	want = append(want, make([]byte, 2*65)...)
	want = append(want, checkpointDecode(t, checkpointFreeVector)...)
	want = append(want, make([]byte, 3*42)...)
	want = append(want, checkpointDecode(t, checkpointGuardVector+"7856341221436587"+"00"+"0500000000ffffff")...)
	want = append(want, make([]byte, 15*21)...)
	want = append(want, checkpointDecode(t, checkpointPendingVector+checkpointSelfVector+"03000000010001"+"00"+"070000000000ffff")...)
	if got := executorCheckpointBytes(t, e); !bytes.Equal(got, want) {
		t.Fatalf("got  %x\nwant %x", got, want)
	}
}

func TestCheckpointPersonaStoredValues(t *testing.T) {
	p := Persona{Name: "ignored", Async: true, APM: -1, Ambition: -2, Attention: 0, Burst: 0x010203, Omniscient: true, Reaction: 1 << 31, Skill: 4}
	before := p
	want := checkpointDecode(t, "fffffffffeffffff000000000302010001000000800400000000000000")
	got := checkpointLeafBytes(t, p.writeCheckpoint)
	if !bytes.Equal(got, want) || p != before {
		t.Fatalf("persona changed or normalized: %+v, bytes %x, want %x", p, got, want)
	}
	p.Name, p.Async = "another label", false
	if !bytes.Equal(got, checkpointLeafBytes(t, p.writeCheckpoint)) {
		t.Fatal("persona label/scheduling changed payload")
	}
}

func TestCheckpointPersonaRetainedMutations(t *testing.T) {
	p := Persona{}
	before := checkpointLeafBytes(t, p.writeCheckpoint)
	for _, tc := range []struct {
		name string
		edit func(*Persona)
	}{
		{"APM", func(p *Persona) { p.APM = -1 }},
		{"Ambition", func(p *Persona) { p.Ambition = 1 }},
		{"Attention", func(p *Persona) { p.Attention = 2 }},
		{"Burst", func(p *Persona) { p.Burst = 3 }},
		{"Omniscient", func(p *Persona) { p.Omniscient = true }},
		{"Reaction", func(p *Persona) { p.Reaction = 4 }},
		{"Skill", func(p *Persona) { p.Skill = 5 }},
		{"ThinkEvery", func(p *Persona) { p.ThinkEvery = 6 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Persona{}
			tc.edit(&p)
			if bytes.Equal(before, checkpointLeafBytes(t, p.writeCheckpoint)) {
				t.Fatal("effective persona mutation vanished")
			}
		})
	}
}

func TestCheckpointExecutorRetainedMutations(t *testing.T) {
	baseline := executorCheckpointBytes(t, &executor{})
	for _, tc := range []struct {
		name string
		edit func(*executor)
	}{
		{"dedupe", func(e *executor) { e.dedupe.unitIdx = []int32{-1} }},
		{"freeSeq", func(e *executor) { e.freeSeq = ^uint32(0) }},
		{"freeSlot", func(e *executor) { e.freeSlot = -1 }},
		{"unusedFree", func(e *executor) { e.frees[2].comp = []int32{1} }},
		{"invalidatedGrid", func(e *executor) { e.grids[3].reach = []uint32{1} }},
		{"lastFill", func(e *executor) { e.lastFill = 1 }},
		{"lastTick", func(e *executor) { e.lastTick = 1 }},
		{"nextPending", func(e *executor) { e.nextPending = 15 }},
		{"expiredPending", func(e *executor) { e.pending[15].cx = 1 }},
		{"selfCostLength", func(e *executor) { e.selfGrid.cost = []int32{1} }},
		{"selfSeen", func(e *executor) { e.selfGrid.seen = []uint32{1} }},
		{"selfStamp", func(e *executor) { e.selfGrid.stamp = ^uint32(0) }},
		{"spotCover", func(e *executor) { e.spotCover = []bool{false} }},
		{"tokens", func(e *executor) { e.tokens = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &executor{}
			tc.edit(e)
			if bytes.Equal(baseline, executorCheckpointBytes(t, e)) {
				t.Fatal("retained mutation vanished")
			}
		})
	}
	for _, name := range []string{"pending", "grids", "frees", "spotCover"} {
		t.Run(name+"Order", func(t *testing.T) {
			e := &executor{spotCover: []bool{true, false}}
			e.pending[0].cx, e.grids[0].built, e.frees[0].tick = 1, 2, 3
			before := executorCheckpointBytes(t, e)
			switch name {
			case "pending":
				e.pending[0], e.pending[15] = e.pending[15], e.pending[0]
			case "grids":
				e.grids[0], e.grids[3] = e.grids[3], e.grids[0]
			case "frees":
				e.frees[0], e.frees[2] = e.frees[2], e.frees[0]
			case "spotCover":
				e.spotCover[0], e.spotCover[1] = e.spotCover[1], e.spotCover[0]
			}
			if bytes.Equal(before, executorCheckpointBytes(t, e)) {
				t.Fatal("physical order lost")
			}
		})
	}
}

func TestCheckpointExecutorExclusionsAndPurity(t *testing.T) {
	e := &executor{dedupe: checkpointDedupeFixture(), spotCover: []bool{true, false}}
	before := executorCheckpointBytes(t, e)
	e.mapInfo, e.obs = &MapInfo{}, &Obs{}
	e.places = []*placeDef{{ok: true, footX: 7, footZ: 8}}
	e.stats = ApplyStats{Applied: 91, Failed: 92}
	e.guardFacs[3] = OwnUnit{H: 94}
	e.nGuard, e.keep, e.rowNear = 4, true, 95
	e.cutBuf, e.blkBuf = []int32{96}, []int32{97}
	e.rows = rowState{vGen: 98, vStamp: []uint32{99}, vOK: []bool{true}, rStamp: []uint32{100}, rOK: []bool{true}}
	e.lanes, e.allyTowers = []rowBld{{x0: 101}}, []towerSpace{{x: 102}}
	e.featDist, e.featOrder = []int64{103}, []int32{104}
	snapshot := *e
	for i := 0; i < 2; i++ {
		if !bytes.Equal(before, executorCheckpointBytes(t, e)) || !reflect.DeepEqual(*e, snapshot) {
			t.Fatal("excluded state was encoded or capture changed executor")
		}
		if !reflect.DeepEqual(e.dedupe, checkpointDedupeFixture()) || !reflect.DeepEqual(e.spotCover, []bool{true, false}) ||
			!reflect.DeepEqual(e.rows.vStamp, []uint32{99}) || e.places[0].footX != 7 {
			t.Fatal("capture mutated backing storage")
		}
	}
}

// A race-enabled run also checks that capture never samples the worker-owned
// pointer fields, not even for presence. No producer or worker is joined.
func TestCheckpointExecutorDoesNotReadWorkerPointers(t *testing.T) {
	e := &executor{}
	before := executorCheckpointBytes(t, e)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		m, o := &MapInfo{}, &Obs{}
		for {
			select {
			case <-stop:
				return
			default:
				e.mapInfo, e.obs = m, o
				e.mapInfo, e.obs = nil, nil
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := 0; i < 10; i++ {
		if !bytes.Equal(before, executorCheckpointBytes(t, e)) {
			t.Fatal("worker scheduling changed capture")
		}
	}
}

func TestCheckpointExecutorRefusals(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		e          *executor
		c          *ai.CheckpointContext
	}{
		{"nilExecutor", "aikit.executor", nil, executorCheckpointContext()},
		{"nilContext", "aikit.context", &executor{}, nil},
		{"missingUnits", "aikit.context", &executor{}, ai.NewCheckpointContext(nil, &world.CheckpointContext{})},
		{"missingWorld", "aikit.context", &executor{}, ai.NewCheckpointContext(&units.CheckpointContext{}, nil)},
		{"manager", "aikit.executor.m", &executor{m: &ai.Manager{}}, executorCheckpointContext()},
		{"table", "aikit.executor.table", &executor{table: &Table{}}, executorCheckpointContext()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := tc.e.writeCheckpoint(checkpoint.NewEncoder(&out), tc.c)
			if err == nil || !strings.HasPrefix(err.Error(), "nanolathe: ") || !strings.Contains(err.Error(), "logical path "+tc.path) || out.Len() != 0 {
				t.Fatalf("capture error = %v, bytes = %x", err, out.Bytes())
			}
		})
	}
	for _, write := range []func(*checkpoint.Encoder) error{
		(*Persona)(nil).writeCheckpoint,
		func(e *checkpoint.Encoder) error { return (*gridDedupe)(nil).writeCheckpoint(e, "dedupe") },
		func(e *checkpoint.Encoder) error { return (*exitGrid)(nil).writeGuardCheckpoint(e, "guard") },
		func(e *checkpoint.Encoder) error { return (*exitGrid)(nil).writeSelfCheckpoint(e, "self") },
		func(e *checkpoint.Encoder) error { return (*freeCache)(nil).writeCheckpoint(e, "free") },
		func(e *checkpoint.Encoder) error { return (*pendingSite)(nil).writeCheckpoint(e, "pending") },
	} {
		if err := write(checkpoint.NewEncoder(io.Discard)); err == nil {
			t.Fatal("nil leaf accepted")
		}
	}
}

type checkpointExecutorFailWriter struct{ err error }

func (w checkpointExecutorFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCheckpointExecutorWriterFailure(t *testing.T) {
	cause := errors.New("authored output failure")
	e := &executor{dedupe: checkpointDedupeFixture(), tokens: -4}
	enc := checkpoint.NewEncoder(checkpointExecutorFailWriter{cause})
	if err := e.writeCheckpoint(enc, executorCheckpointContext()); !errors.Is(err, cause) || !strings.Contains(err.Error(), "aikit.executor.dedupe.cellIdx") {
		t.Fatalf("writer error lost cause/path: %v", err)
	}
	if !reflect.DeepEqual(e.dedupe, checkpointDedupeFixture()) || e.tokens != -4 {
		t.Fatal("failed capture changed state")
	}
}
