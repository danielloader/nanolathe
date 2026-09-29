package units

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/rng"
)

// recordingPose captures the unit state the allocator hands the post-move
// correction [04 R-MOV-01 §5].
type recordingPose struct {
	calls   int
	heading uint16
	mirror  uint8
}

func (p *recordingPose) CorrectCreatedPose(u *Unit) {
	p.calls++
	p.heading = u.Move.Heading
	p.mirror = u.Move.ModeMirror
}

// TestAllocatorGivesMoversTheirBuildAngle locks the allocator's second heading
// write [04 §2.3b]: a definition that receives a mover (`bmcode 1`) leaves
// the allocator facing its `buildangle` word, while a structure keeps the
// drawn heading. The draw sequence is the same for both, and the post-move
// correction runs once, after the heading write and the grounded mode install
// [04 R-MOV-01 §5].
func TestAllocatorGivesMoversTheirBuildAngle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bmcode uint8
		want   uint16
	}{
		{"mover", 1, 4096},
		{"structure", 0, 33048}, // seed 7's first bounded draw, 2328, recentred [04 §2.3b]
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim := rng.NewSimulation(7)
			w := newFixtureWorld(1, nil)
			w.SetSimulationRNG(&sim)
			pose := &recordingPose{}
			w.SetCreationPose(pose)
			def := p28AngleDef(tc.name, 4096)
			def.BMCode = tc.bmcode
			if got := p28CreateHeading(t, w, def); got != tc.want {
				t.Fatalf("heading = %d, want %d", got, tc.want)
			}
			if got := sim.Draws(); got != 2 {
				t.Fatalf("draw count = %d, want the initializer's 2 for either class", got)
			}
			if pose.calls != 1 || pose.heading != tc.want || pose.mirror != CreatedMoverMode {
				t.Fatalf("correction saw %+v, want one call at heading %d with the grounded mirror", pose, tc.want)
			}
		})
	}
}
