package combat

import (
	"bytes"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
)

// Independent scalar vector, including composition onto an existing summary,
// negative signed values and Fixed above 32 bits. A row contributes 16 words;
// the owner contributes another 35. The beyond-count row is included.
func TestCheckpointCombatSummaryVector(t *testing.T) {
	s := NewServiceWithProjectileCapacity(2)
	s.Slots.Reserve()
	s.Records[0] = Projectile{Dead: true, WeaponID: -2, Pos: Vec3{-3, 1<<40 + 4, 5}, TargetUnit: 0xf123, TargetProjectile: 0xabcd,
		TargetPos: Vec3{6, -7, 8}, Shooter: 0xfedc, Velocity: Vec3{9, -10, 11}, ExpiryTick: 0xdeadbeef, BurstRemaining: -12}
	s.Records[1] = Projectile{WeaponID: -13, ExpiryTick: 17, BurstRemaining: -18}
	for i := range s.targets.lastRebuild {
		s.targets.lastRebuild[i] = uint32(i + 1)
		s.targets.gate[i] = i%2 == 0
		s.scanCursor.next[i] = i + 1
	}
	s.scanCursor.next[0] = -1
	s.scanCursor.next[9] = -10
	s.modernNextProjectileTick = 99
	s.communityAreaGenCounter = 100
	var got checkpoint.Summary
	got.Word(37)
	if err := s.AppendCheckpointSummary(&got); err != nil {
		t.Fatal(err)
	}
	if words, sum := got.Result(); words != 68 || sum != 8867077762462 {
		t.Fatalf("summary = %d %d", words, sum)
	}
	if allocations := testing.AllocsPerRun(20, func() {
		var row checkpoint.Summary
		if err := s.AppendCheckpointSummary(&row); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("cheap summary allocated %g", allocations)
	}
}

func TestCheckpointCombatSummaryBlindSpots(t *testing.T) {
	s := NewServiceWithProjectileCapacity(2)
	c := combatCheckpointContext(t)
	var before checkpoint.Summary
	if err := s.AppendCheckpointSummary(&before); err != nil {
		t.Fatal(err)
	}
	full := combatCheckpointBytes(t, s, c)
	s.targets.primary[0] = []pool.Handle{9, 1, 9}
	s.targets.secondary[9] = []pool.Handle{0xffff}
	s.modernTick = 99
	var after checkpoint.Summary
	if err := s.AppendCheckpointSummary(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("cheap-row blind spot changed")
	}
	if bytes.Equal(full, combatCheckpointBytes(t, s, c)) {
		t.Fatal("full capture omitted cheap-row blind spot")
	}
	s.Records[1].ExpiryTick++
	after = checkpoint.Summary{}
	if err := s.AppendCheckpointSummary(&after); err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("cheap row omitted residual expiry")
	}
	// Summary requires neither initialized storage nor capture contexts. Its row
	// reports actual length; full capture separately enforces its admitted arena.
	var zero checkpoint.Summary
	if err := (&Service{}).AppendCheckpointSummary(&zero); err != nil {
		t.Fatal(err)
	}
	if words, sum := zero.Result(); words != 35 || sum != 600 {
		t.Fatalf("zero owner = %d %d", words, sum)
	}
	before = after
	if err := (*Service)(nil).AppendCheckpointSummary(&after); err == nil || before != after {
		t.Fatal("nil receiver was not atomic")
	}
	if err := s.AppendCheckpointSummary(nil); err == nil {
		t.Fatal("nil accumulator accepted")
	}
}
