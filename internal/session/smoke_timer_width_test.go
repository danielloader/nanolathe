package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// The ordinary producers admit both sides of the signed-window boundary.
// Valid frame metadata keeps the initial puff alive through the first due
// visit; that visit has no animation draw, isolating spawn CRT admission
// [03 R-STRIP-01 §3]. This is not a full tick-wrap or long-match claim.
func TestSmokeProducerSweepUsesSignedWindow(t *testing.T) {
	const boundary = uint32(1 << 31)
	for _, tc := range []struct {
		name  string
		start uint32
		init  SmokePuffInit
		spawn bool
	}{
		{"ordinary one shot", 100, SmokePuffTrail, false},
		{"ordinary land dust", 100, SmokePuffLandDust, true},
		{"one shot next crosses signed boundary", boundary - 1, SmokePuffTrail, true},
		{"dust window crosses signed boundary", boundary - 8, SmokePuffLandDust, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, crt := newStripTestSession(1, 1)
			s.SetEffectEntryFrameCount(func(bank, entry string) (int, bool) { return 12, true })
			s.Clock.GlobalTick = tc.start
			before := crt.Draws()
			s.appendStripSmokePuffer(9, [3]numeric.Fixed{}, tc.init)
			if got := crt.Draws() - before; got != 1 {
				t.Fatalf("initial spawn draws=%d, want 1", got)
			}
			due := tc.start + uint32(tc.init.SpawnInterval)
			for elapsed := uint32(0); elapsed < uint32(tc.init.SpawnInterval); elapsed++ {
				tick := tc.start + elapsed
				s.Clock.GlobalTick = tick
				s.phaseObjectSweeps(tick)
			}
			obj := &s.strips.strips[9][0]
			if obj.readyToSpawn(due - 1) {
				t.Fatal("spawn admitted before unsigned due tick")
			}
			before = crt.Draws()
			s.Clock.GlobalTick = due
			s.phaseObjectSweeps(due)
			wantDraws := uint64(0)
			wantParticles := 1
			if tc.spawn {
				wantDraws = 1
				wantParticles++
			}
			if got := crt.Draws() - before; got != wantDraws {
				t.Fatalf("due visit draws=%d, want %d", got, wantDraws)
			}
			if got := len(s.strips.strips[9][0].particles); got != wantParticles {
				t.Fatalf("puffs=%d, want %d", got, wantParticles)
			}
		})
	}
}

func TestSmokeRemovalKeepsUnsignedStrictExpiry(t *testing.T) {
	const boundary = uint32(1 << 31)
	for _, tc := range []struct {
		name      string
		end, tick uint32
		remove    bool
	}{
		{"equality survives", 10, 10, false},
		{"past deadline removes", 10, 11, true},
		{"unsigned future survives", boundary + 1, boundary - 1, false},
		{"unsigned past removes", boundary - 1, boundary + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := stripObject{family: stripFamilySmoke, windowEnd: tc.end}
			if got := o.removalVerdict(tc.tick); got != tc.remove {
				t.Fatalf("empty removal=%v, want %v", got, tc.remove)
			}
			o.particles = []stripParticle{{}}
			if o.removalVerdict(tc.tick) {
				t.Fatal("nonempty smoke container removed")
			}
		})
	}
}

// The independently verified vent gate has no window term [03 R-FX-02 §3].
func TestSmokeWindowCorrectionPreservesVentDueGate(t *testing.T) {
	const boundary = uint32(1 << 31)
	o := stripObject{family: stripFamilyVentSteam, nextSpawn: boundary - 1, windowEnd: boundary + 1}
	if !o.readyToSpawn(boundary - 1) {
		t.Fatal("vent incorrectly acquired a signed window gate")
	}
	o.nextSpawn = boundary + 1
	if o.readyToSpawn(boundary - 1) {
		t.Fatal("vent due comparison became signed")
	}
	if !o.readyToSpawn(boundary + 1) {
		t.Fatal("vent lost inclusive due comparison")
	}
	if o.removalVerdict(boundary + 2) {
		t.Fatal("vent acquired an expiry verdict")
	}
}
