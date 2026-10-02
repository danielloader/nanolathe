package numeric

import (
	"math"
	"math/big"
	"testing"
)

func bext(v float64) *big.Float   { return new(big.Float).SetPrec(64).SetFloat64(v) }
func bstore(v *big.Float) float64 { f, _ := v.Float64(); return f }

func bextSqrt(v float64) *big.Float {
	frac, exp := math.Frexp(v)
	m := new(big.Int).SetUint64(uint64(frac * (1 << 53)))
	e := exp - 53
	shift := 160
	if (e-shift)%2 != 0 {
		shift++
	}
	n := new(big.Int).Lsh(m, uint(shift))
	r := new(big.Int).Sqrt(n)
	exact := new(big.Int).Mul(r, r).Cmp(n) == 0
	r.Lsh(r, 1)
	if !exact {
		r.Or(r, big.NewInt(1))
	}
	wide := new(big.Float).SetPrec(uint(r.BitLen())).SetInt(r)
	wide.SetMantExp(wide, (e-shift)/2-1)
	return new(big.Float).SetPrec(64).Set(wide)
}

// model is the math/big reference: each step at a 64-bit significand, then a
// binary64 store.
func model(x, y float64) float64 {
	x, y = math.Abs(x), math.Abs(y)
	larger := x
	if y > x {
		larger = y
	}
	if larger == 0 {
		return 0
	}
	qx := bstore(new(big.Float).SetPrec(64).Quo(bext(x), bext(larger)))
	qy := bstore(new(big.Float).SetPrec(64).Quo(bext(y), bext(larger)))
	px := new(big.Float).SetPrec(64).Mul(bext(qx), bext(qx))
	py := new(big.Float).SetPrec(64).Mul(bext(qy), bext(qy))
	sum := bstore(new(big.Float).SetPrec(64).Add(px, py))
	root := bstore(bextSqrt(sum))
	fr, er := math.Frexp(root)
	fl, el := math.Frexp(larger)
	p := bstore(new(big.Float).SetPrec(64).Mul(bext(fr), bext(fl)))
	v := new(big.Float).SetPrec(128).SetFloat64(p)
	v.SetMantExp(v, er+el)
	if v.Cmp(bext(0x1p-1022)) < 0 {
		// Retail shifts the final significand without rounding. Compute the
		// integer number of subnormal units independently [01 R-DET-01 §7].
		v.SetMantExp(v, 1074)
		units, _ := v.Int(nil)
		return math.Float64frombits(units.Uint64())
	}
	return bstore(v)
}

type xorshift uint64

func (x *xorshift) next() uint64 {
	v := uint64(*x)
	v ^= v << 13
	v ^= v >> 7
	v ^= v << 17
	*x = xorshift(v)
	return v
}

func check(t *testing.T, x, y float64) {
	t.Helper()
	got, want := Distance(x, y), model(x, y)
	if truncated := TruncatedDistance(x, y); truncated != TruncateFloat64ToLow32(got) {
		t.Fatalf("TruncatedDistance(%g,%g) = %d, Distance truncates to %d", x, y, truncated, TruncateFloat64ToLow32(got))
	}
	if math.Float64bits(got) != math.Float64bits(want) {
		t.Fatalf("Distance(%v, %v) = %.17g (%016x), model %.17g (%016x)", x, y, got, math.Float64bits(got), want, math.Float64bits(want))
	}
}

func TestIntegerPairs(t *testing.T) {
	limit := 64
	for x := 0; x <= limit; x++ {
		for y := x; y <= limit; y++ {
			check(t, float64(x), float64(y))
			check(t, float64(y), float64(x))
		}
	}
}

func TestRawDeltas(t *testing.T) {
	rng := xorshift(0x9e3779b97f4a7c15)
	n := 4096
	for i := 0; i < n; i++ {
		ex := 8 + rng.next()%21
		ey := 8 + rng.next()%21
		x := float64(rng.next() % (1 << ex))
		y := float64(rng.next() % (1 << ey))
		check(t, x, -y)
	}
}

func TestFractionsAndWideExponents(t *testing.T) {
	rng := xorshift(0x1234567887654321)
	n := 4096
	for i := 0; i < n; i++ {
		// Arbitrary 53-bit mantissas over a wide but non-extreme exponent range.
		mx := float64(rng.next()>>11) / (1 << 53)
		my := float64(rng.next()>>11) / (1 << 53)
		ex := int(rng.next()%2098) - 1073
		ey := int(rng.next()%2098) - 1073
		check(t, math.Ldexp(mx, ex), math.Ldexp(my, ey))
	}
	// Flight-style values: fixed-point words divided by 65536.
	for i := 0; i < n; i++ {
		x := float64(int64(rng.next()%(1<<34))-(1<<33)) / 65536.0
		y := float64(int64(rng.next()%(1<<34))-(1<<33)) / 65536.0
		check(t, x, y)
	}
}

func TestEdges(t *testing.T) {
	for _, p := range [][2]float64{
		{0, 0}, {0, 5}, {5, 0}, {1, 1}, {165, 52}, {20, 99}, {3, 4}, {1, 1e-30}, {1e-30, 1},
		{math.MaxInt32, math.MaxInt32}, {math.MaxInt32, 1}, {0x1p-1022, 0x1p-1022}, {0x1p-1050, 0x1p-1060},
		{0x1p1000, 0x1p1000}, {1.5, 0x1p-60}, {1, 0x1p-32}, {1, 0x1p-33}, {1, 0x1p-31},
	} {
		check(t, p[0], p[1])
	}
}

var sink float64

func BenchmarkDistance(b *testing.B) {
	rng := xorshift(0x9e3779b97f4a7c15)
	var xs, ys [1024]float64
	for i := range xs {
		xs[i] = float64(rng.next() % (1 << 26))
		ys[i] = float64(rng.next() % (1 << 26))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = Distance(xs[i&1023], ys[i&1023])
	}
}

func BenchmarkTruncatedDistance(b *testing.B) {
	rng := xorshift(0x9e3779b97f4a7c15)
	var xs, ys [1024]float64
	for i := range xs {
		xs[i] = float64(rng.next() % (1 << 26))
		ys[i] = float64(rng.next() % (1 << 26))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = float64(TruncatedDistance(xs[i&1023], ys[i&1023]))
	}
}

// Cases lock the published near-integer failures and the final exponent exit,
// whose underflow truncates while quotient stores round [01 R-DET-01 §7].
func TestDistanceRoundingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		x, y float64
		bits uint64
	}{
		{20, 99, math.Float64bits(math.Nextafter(101, 0))},
		{165, 52, math.Float64bits(math.Nextafter(173, math.Inf(1)))},
		{math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, 1},
		{2 * math.SmallestNonzeroFloat64, 2 * math.SmallestNonzeroFloat64, 2},
		{3 * math.SmallestNonzeroFloat64, 4 * math.SmallestNonzeroFloat64, 5},
		{math.MaxFloat64, math.MaxFloat64, math.Float64bits(math.Inf(1))},
		{math.MaxFloat64, 0, math.Float64bits(math.MaxFloat64)},
	} {
		check(t, tc.x, tc.y)
		if got := math.Float64bits(Distance(tc.x, tc.y)); got != tc.bits {
			t.Fatalf("(%g,%g): %016x want %016x", tc.x, tc.y, got, tc.bits)
		}
	}
	for _, v := range []float64{math.SmallestNonzeroFloat64, 0x1p-1022, 0x1p-1021, 1, 101, 173, 0x1p31, 0x1p32, 0x1p50, 0x1p63, math.MaxFloat64} {
		for _, w := range []float64{0, v, math.Nextafter(v, 0), math.Nextafter(v, math.Inf(1))} {
			if !math.IsInf(w, 0) {
				check(t, v, w)
			}
		}
	}
	// Perturb Pythagorean triples on both sides of integer thresholds.
	for i := 1; i <= 128; i++ {
		for _, scale := range []float64{1, 0x1p16, 0x1p30} {
			x, y := float64(3*i)*scale, float64(4*i)*scale
			for j := 0; j < 8; j++ {
				check(t, x, y)
				check(t, -x, y)
				x = math.Nextafter(x, 0)
				y = math.Nextafter(y, math.Inf(1))
			}
		}
	}
}

func TestDistanceSpecialOperands(t *testing.T) {
	for _, x := range []float64{0, math.Copysign(0, -1)} {
		for _, y := range []float64{0, math.Copysign(0, -1)} {
			if math.Float64bits(Distance(x, y)) != 0 {
				t.Fatal("distance zero must be positive")
			}
		}
	}
	values := []float64{0, 1, math.Inf(1), math.Inf(-1), math.Float64frombits(0x7ff8000000000042), math.Float64frombits(0xfff0000000000042)}
	for _, x := range values {
		for _, y := range values {
			d := Distance(x, y)
			switch {
			case math.IsNaN(x) || math.IsNaN(y):
				// Payload/sign are Unknown; assert only the established result class.
				if !math.IsNaN(d) {
					t.Fatalf("NaN must win over all operands: %g,%g", x, y)
				}
			case math.IsInf(x, 0) || math.IsInf(y, 0):
				if !math.IsInf(d, 1) {
					t.Fatalf("infinity distance: %g,%g", x, y)
				}
			}
			if got := TruncatedDistance(x, y); got != TruncateFloat64ToLow32(d) {
				t.Fatal("special truncation differs")
			}
		}
	}
}
