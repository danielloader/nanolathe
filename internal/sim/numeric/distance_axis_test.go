package numeric

import (
	"math"
	"testing"
)

// One zero operand makes every retail distance rounding exact. Exercise every
// finite exponent, both operand orders/signs and both zeros, especially the
// subnormal normalization and final truncating denormalization [01 R-DET-01 §7].
func TestDistanceAxisExponentBoundaries(t *testing.T) {
	for exponent := uint64(0); exponent < 0x7ff; exponent++ {
		for _, fraction := range []uint64{0, 1, 1 << 51, (1 << 52) - 1} {
			v := math.Float64frombits(exponent<<52 | fraction)
			for _, sign := range []float64{1, -1} {
				for _, zero := range []float64{0, math.Copysign(0, -1)} {
					check(t, sign*v, zero)
					check(t, zero, sign*v)
				}
			}
		}
	}
}

func BenchmarkDistanceAxis(b *testing.B) {
	for _, value := range []struct {
		name string
		v    float64
	}{
		{"zero", 0}, {"raw-delta", 65536}, {"fractional", 0.3}, {"subnormal", math.SmallestNonzeroFloat64},
	} {
		b.Run(value.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				sink = Distance(value.v, 0)
			}
		})
	}
}

func BenchmarkTruncatedDistanceAxis(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sink = float64(TruncatedDistance(float64(i&1023)*65536, 0))
	}
}
