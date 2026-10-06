package content

import (
	"fmt"
	"testing"
)

// The definition loader wraps each bound before halving it [02 R-CAT-01 §7].
// Mobile authored definitions bypass yard allocation; this does not assert
// that an oversized footprint can be placed in an ordinary battle.
func TestAuthoredUnitBoundsWrapBeforeDivision(t *testing.T) {
	for _, tc := range []struct {
		foot, min, max int32
	}{
		{2, -1048576, 1048576},
		{2047, -1073217536, 1073217536},
		{2048, -1073741824, -1073741824},
		{2049, 1073217536, -1073217536},
		{-2049, -1073217536, 1073217536},
	} {
		t.Run(fmt.Sprint(tc.foot), func(t *testing.T) {
			u := compileMobilityFixture(t, fmt.Sprintf("bmcode=1\nfootprintx=%d\nfootprintz=%d", tc.foot, tc.foot))
			if u.FootprintX != tc.foot || u.FootprintZ != tc.foot {
				t.Fatalf("authored footprint narrowed unexpectedly: %d,%d", u.FootprintX, u.FootprintZ)
			}
			u.ModelTopFixed = 12345
			min, max := u.BoundingExtents()
			if min != [3]int32{tc.min, 0, tc.min} || max != [3]int32{tc.max, 12345, tc.max} {
				t.Fatalf("bounds = %v,%v; want horizontal %d,%d and unchanged model top", min, max, tc.min, tc.max)
			}
		})
	}
}
