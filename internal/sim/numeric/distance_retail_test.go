//go:build retail

package numeric

import "testing"

// Exhaustive ordered pairs include all 722 truncations below the exact integer
// root documented by [01 R-DET-01 §7]. The reference is the rounded sequence.
func TestDistanceAllOrderedIntegerPairsRetail(t *testing.T) {
	for x := 0; x <= 3000; x++ {
		for y := 0; y <= 3000; y++ {
			check(t, float64(x), float64(y))
		}
	}
}
