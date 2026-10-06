package triggers

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// The query owns membership and geometry; the condition owns only its visitor
// and latch. In particular it must consume the entire query after a hit
// [08 R-TRIG-01 §5].
func TestRadiusVisitorContinuesAndRescans(t *testing.T) {
	w := triggerWorld(t)
	match := spawn(t, w, "AIR", 0, 64, 64)
	wrongType := spawn(t, w, "OTHER", 0, 64, 64)
	wrongOwner := spawn(t, w, "AIR", 1, 64, 64)
	unfinished := spawn(t, w, "AIR", 0, 64, 64)
	unfinished.Remaining = 0.5
	c := pollCtx(w, 0)
	cues, deprojections, queries, visits := 0, 0, 0, 0
	c.Celebrate = func() { cues++ }
	c.Deproject = func(x, z int32) (int32, int32, int32) { deprojections++; return x << 16, 0, z << 16 }
	c.VisitRadiusUnits = func(x, z, r int32, visit func(*units.Unit)) {
		queries++
		if x != 64<<16 || z != 64<<16 || r != -1<<31 {
			t.Fatalf("query arguments %d,%d,%d", x, z, r)
		}
		candidates := []*units.Unit{wrongType, wrongOwner, unfinished}
		if queries > 1 && queries < 4 {
			candidates = append([]*units.Unit{match}, candidates...)
		}
		for _, u := range candidates {
			visits++
			visit(u)
		}
	}
	tr := New(KindMoveUnitToRadius, "AIR", 64, 64, 32768)
	if tr.Poll(c) {
		t.Fatal("ineligible candidates completed condition")
	}
	for i := 0; i < 3; i++ {
		if !tr.Poll(c) {
			t.Fatal("matching visit did not preserve completion")
		}
	}
	if cues != 1 || deprojections != 1 || queries != 4 || visits != 14 {
		t.Fatalf("cue/deproject/query/visits = %d/%d/%d/%d", cues, deprojections, queries, visits)
	}
}

func TestRadiusMissingQueryIsIncompleteHostContext(t *testing.T) {
	w := triggerWorld(t)
	spawn(t, w, "AIR", 0, 64, 64)
	c := pollCtx(w, 0)
	tr := New(KindMoveUnitToRadius, "AIR", 64, 64, 100)
	if tr.Poll(c) {
		t.Fatal("missing spatial seam fell back to whole pool")
	}
	if tr.CenterReady {
		t.Fatal("incomplete context converted the centre")
	}
	tr.Completed = true
	if !tr.Poll(c) {
		t.Fatal("missing spatial seam cleared existing latch")
	}
}
