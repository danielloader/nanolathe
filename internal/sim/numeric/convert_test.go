package numeric

import (
	"math"
	"testing"
)

func TestTruncateFloat64ToInt64(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		want int64
	}{
		{0, 0}, {math.Copysign(0, -1), 0}, {1.9, 1}, {-1.9, -1},
		{math.SmallestNonzeroFloat64, 0}, {-math.SmallestNonzeroFloat64, 0},
		{0x1p31, 1 << 31}, {0x1p32 + 1, 1<<32 + 1},
		{math.Nextafter(0x1p63, 0), math.MaxInt64 - 1023}, {-0x1p63, math.MinInt64},
		{math.Nextafter(-0x1p63, 0), math.MinInt64 + 1024},
		{0x1p63, math.MinInt64}, {math.Nextafter(-0x1p63, math.Inf(-1)), math.MinInt64},
		{math.NaN(), math.MinInt64}, {math.Inf(1), math.MinInt64}, {math.Inf(-1), math.MinInt64},
	} {
		if got := TruncateFloat64ToInt64(tc.v); got != tc.want {
			t.Errorf("%g: %d want %d", tc.v, got, tc.want)
		}
	}
}
func TestRoundFloat64ToInt32(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		want int32
	}{
		{0, 0}, {math.Copysign(0, -1), 0}, {0.5, 0}, {1.5, 2}, {2.5, 2},
		{-0.5, 0}, {-1.5, -2}, {-2.5, -2},
		{math.Nextafter(0.5, 0), 0}, {math.Nextafter(0.5, 1), 1},
		{0x1p31 - 1, math.MaxInt32}, {0x1p31 - 0.5, math.MinInt32},
		{math.Nextafter(0x1p31-0.5, 0), math.MaxInt32},
		{-0x1p31 - 0.5, math.MinInt32}, {-0x1p31 + 0.5, math.MinInt32},
		{math.Nextafter(-0x1p31+0.5, 0), math.MinInt32 + 1},
		{math.NaN(), math.MinInt32}, {math.Inf(1), math.MinInt32}, {math.Inf(-1), math.MinInt32},
		{0x1p40, math.MinInt32}, {-0x1p40, math.MinInt32},
	} {
		if got := RoundFloat64ToInt32(tc.v); got != tc.want {
			t.Errorf("%g: %d want %d", tc.v, got, tc.want)
		}
	}
}
