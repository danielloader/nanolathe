package session

import "testing"

// Flame's window comparison is signed, its due comparison unsigned, and
// segment expiry signed and strict [03 R-STRIP-01 §3]. Sibling families keep
// their existing predicates until their own contracts are verified.
func TestFlameSpawnUsesSignedWindowAndUnsignedDueTick(t *testing.T) {
	const boundary = uint32(1 << 31)
	for _, tc := range []struct {
		name            string
		next, end, tick uint32
		want            bool
	}{
		{"ordinary inclusive", 10, 10, 10, true},
		{"signed window refuses", boundary - 10, boundary + 10, boundary - 10, false},
		{"signed window admits", boundary + 10, boundary - 10, boundary + 10, true},
		{"unsigned due refuses", boundary + 10, boundary + 20, boundary - 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, crt := newStripTestSession(31, 31)
			o := stripObject{family: stripFamilyFlame, src: teleportFlameFrom, dst: teleportFlameTo, nextSpawn: tc.next, windowEnd: tc.end, spawnInterval: 10, frameCountBase: 19}
			before := crt.Draws()
			o.spawnGate(tc.tick, crt)
			wantDraws := uint64(0)
			if tc.want {
				wantDraws = 1
			}
			if got := crt.Draws() - before; got != wantDraws {
				t.Fatalf("spawn CRT draws=%d, want %d", got, wantDraws)
			}
			if got := len(o.particles) > 0; got != tc.want {
				t.Fatalf("spawned=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestFlameExpiryUsesSignedStrictComparison(t *testing.T) {
	const boundary = uint32(1 << 31)
	for _, tc := range []struct {
		name         string
		expiry, tick uint32
		retained     bool
	}{
		{"equal", 10, 10, true},
		{"past", 10, 11, false},
		{"signed expiry removes", boundary + 2, boundary - 1, false},
		{"signed expiry retains", boundary - 2, boundary + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := stripObject{family: stripFamilyFlame, particles: []stripParticle{{expiry: tc.expiry}}}
			o.expireParticles(tc.tick)
			if got := len(o.particles) != 0; got != tc.retained {
				t.Fatalf("retained=%v, want %v", got, tc.retained)
			}
		})
	}
	sibling := stripObject{family: stripFamilyNano, particles: []stripParticle{{expiry: boundary + 2}}}
	sibling.expireParticles(boundary - 1)
	if len(sibling.particles) != 1 {
		t.Fatal("changed nano's existing unsigned expiry predicate")
	}
}
