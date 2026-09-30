//go:build retail

package session

import "testing"

// Nanolathe Modern policy: docs/DESIGN_MOVEMENT_PATH.md "Modern traffic" and
// "Modern arrival places". The scenes are the pocket release's
// (modern_pocket_release_retail_test.go): a block of parked fleas round one
// slot, and a flea outside the block ordered into the slot. They are
// authored here and say nothing about retail beyond the Strict retry they
// lock [04 R-ORD-01 §4].

// A unit sent to a free place it cannot reach, because parked units stand all
// round the place, settles where it stands. It never shares a cell with the
// block, the block does not move, and the move ends. Under Strict 3.1 the
// same move is retried for as long as the block stands. A hostile unit in the
// block changes nothing: no policy takes a unit through another.
func TestModernSealedOutUnitSettlesWhereItStands(t *testing.T) {
	for _, variant := range []string{"free", "enemy"} {
		run := pocketScene(t, ModernRuleSetName, nil, variant)
		if run.passedRing || run.passedEnemy || run.endOverlap || run.ringMoved {
			t.Fatalf("%s: shared a cell with the block %v (the enemy %v, at the end %v), block moved %v; want every unit in its own cells", variant, run.passedRing, run.passedEnemy, run.endOverlap, run.ringMoved)
		}
		if run.done == 0 || run.final == pocketHole {
			t.Fatalf("%s: move done at tick %d with the mover at %v; want it settled outside the block", variant, run.done, run.final)
		}
		t.Logf("%s: the sealed-out mover settled at %v; move done at tick %d", variant, run.final, run.done)
	}
	strict := pocketScene(t, StrictRuleSetName, nil, "free")
	if strict.done != 0 || strict.passedRing || strict.ringMoved {
		t.Fatalf("Strict 3.1: move done at tick %d, shared a cell %v, block moved %v; want the retry", strict.done, strict.passedRing, strict.ringMoved)
	}
}

// A unit sent to a place a parked unit holds is given the nearest free
// footprint for its place when the order begins, goes there and finishes: it
// neither waits on the held cell nor enters the block.
func TestModernHeldGoalIsGivenTheNearestFreePlace(t *testing.T) {
	run := pocketScene(t, ModernRuleSetName, nil, "held")
	if run.passedRing || run.endOverlap || run.ringMoved {
		t.Fatalf("shared a cell with the block %v (at the end %v), block moved %v; want every unit in its own cells", run.passedRing, run.endOverlap, run.ringMoved)
	}
	// The block covers the footprints within two cells of the slot, so the
	// nearest free ones lie four cells from it along an axis.
	dx, dz := run.final.X-pocketHoleX, run.final.Z-pocketHoleZ
	if run.done == 0 || dx*dx+dz*dz != 16 {
		t.Fatalf("move done at tick %d with the mover at %v; want one of the free footprints nearest the slot %v", run.done, run.final, pocketHole)
	}
	t.Logf("the mover was given %v; move done at tick %d", run.final, run.done)
}
