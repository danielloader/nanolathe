package ai

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Independent vector: 21 owner words appended to an existing word. Distinct
// group lengths fix task-slot order, especially construction/resource/rally.
func TestCheckpointManagerSummaryVector(t *testing.T) {
	m := &Manager{Controller: ControllerModern, countdown: 30,
		GroupResource: make([]pool.Handle, 1), GroupWaveA: make([]pool.Handle, 2), GroupRegroupA: make([]pool.Handle, 3), GroupConstruction: make([]pool.Handle, 4),
		GroupNull: make([]pool.Handle, 5), GroupWaveB: make([]pool.Handle, 6), GroupRegroupB: make([]pool.Handle, 7), GroupExplore: make([]pool.Handle, 8), GroupRally: make([]pool.Handle, 9)}
	for i := range m.Deadlines {
		m.Deadlines[i] = uint32(i)
	}
	var s checkpoint.Summary
	s.Word(37)
	if err := m.AppendCheckpointSummary(&s); err != nil {
		t.Fatal(err)
	}
	if words, sum := s.Result(); words != 22 || sum != 1719 {
		t.Fatalf("summary %d %d", words, sum)
	}
	if got := testing.AllocsPerRun(20, func() {
		var s checkpoint.Summary
		if err := m.AppendCheckpointSummary(&s); err != nil {
			panic(err)
		}
	}); got != 0 {
		t.Fatalf("summary allocations %g", got)
	}
}

func TestCheckpointManagerSummaryBoundariesAndBlindSpots(t *testing.T) {
	m := &Manager{GroupWaveA: []pool.Handle{1, 2}}
	c := aiCheckpointContext(t)
	var before checkpoint.Summary
	if err := m.AppendCheckpointSummary(&before); err != nil {
		t.Fatal(err)
	}
	full := aiCheckpointBytes(t, m, c)
	m.GroupWaveA[0], m.GroupWaveA[1] = m.GroupWaveA[1], m.GroupWaveA[0]
	m.Strategic.CenterX = 1 << 40
	m.ControllerParams = "changed"
	var after checkpoint.Summary
	if err := m.AppendCheckpointSummary(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("unselected fields affected cheap row")
	}
	if bytes.Equal(full, aiCheckpointBytes(t, m, c)) {
		t.Fatal("full writer lost cheap-row blind spots")
	}
	m.Ext = checkpointPanicController{}
	m.SetWeaponMaintenance(func(uint8) { panic("binding called") })
	after = checkpoint.Summary{}
	if err := m.AppendCheckpointSummary(&after); err != nil || before != after {
		t.Fatal("summary inspected bindings")
	}
	m.Deadlines[0]++
	after = checkpoint.Summary{}
	if err := m.AppendCheckpointSummary(&after); err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("deadline missing from cheap row")
	}
	before = after
	if err := (*Manager)(nil).AppendCheckpointSummary(&after); err == nil || before != after {
		t.Fatal("nil receiver was not atomic")
	}
	if err := m.AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator accepted")
	}
}
