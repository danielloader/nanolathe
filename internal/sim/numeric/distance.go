package numeric

import (
	"math"
	"math/bits"
)

// ext is a non-negative value mant * 2^exp with mant either zero or
// normalised so that bit 63 is set (a 64-bit significand).
type ext struct {
	mant uint64
	exp  int
}

// decompose splits a finite, non-negative binary64 into a 53-bit integer
// mantissa with bit 52 set and a binary exponent: v = mant * 2^exp.
// A zero returns mant 0.
func decompose(v float64) (mant uint64, exp int) {
	b := math.Float64bits(v)
	frac := b & (1<<52 - 1)
	e := int(b >> 52 & 0x7ff)
	if e == 0 {
		if frac == 0 {
			return 0, 0
		}
		// Subnormal: normalise so bit 52 is set.
		shift := bits.LeadingZeros64(frac) - 11
		return frac << uint(shift), -1074 - shift
	}
	return frac | 1<<52, e - 1075
}

// roundNearestEven rounds a 64-bit truncated significand up by one unit when
// the discarded part is above half, or exactly half with an odd significand.
// It reports the significand and whether it carried out of 64 bits.
func roundNearestEven(mant uint64, round, sticky bool) (uint64, bool) {
	if round && (sticky || mant&1 == 1) {
		mant++
		if mant == 0 {
			return 1 << 63, true
		}
	}
	return mant, false
}

// store rounds a 64-bit-significand value to binary64, nearest even, as an
// x87 store to a double slot does: overflow gives infinity and a result below
// the normal range is rounded once onto the subnormal grid.
func store(v ext) float64 {
	if v.mant == 0 {
		return 0
	}
	// v = mant * 2^exp with bit 63 set; the binary64 exponent of the leading
	// bit is exp + 63.
	e := v.exp + 63
	if e > 1023 {
		return math.Inf(1)
	}
	drop := uint(11)
	if e < -1022 {
		// Subnormal: more bits fall off the bottom.
		extra := -1022 - e
		if extra > 64 {
			return 0
		}
		drop += uint(extra)
		e = -1023 // encodes with a zero exponent field
	}
	var kept, round, sticky uint64
	if drop >= 64 {
		// Everything is below the last place; only the rounding decides.
		if drop == 64 {
			round = v.mant >> 63
			sticky = v.mant & (1<<63 - 1)
		} else {
			sticky = v.mant
		}
	} else {
		kept = v.mant >> drop
		round = v.mant >> (drop - 1) & 1
		sticky = v.mant & (1<<(drop-1) - 1)
	}
	if round == 1 && (sticky != 0 || kept&1 == 1) {
		kept++
	}
	if e == -1023 {
		// kept is the subnormal fraction; a carry into bit 52 is the smallest
		// normal number, which the same encoding expresses.
		return math.Float64frombits(kept)
	}
	if kept == 1<<53 {
		kept >>= 1
		e++
		if e > 1023 {
			return math.Inf(1)
		}
	}
	return math.Float64frombits(uint64(e+1023)<<52 | kept&(1<<52-1))
}

// quotient returns RN64(s / m) for 53-bit mantissas with bit 52 set and
// s <= m in value.
func quotient(sm uint64, se int, mm uint64, me int) ext {
	if sm == 0 {
		return ext{}
	}
	// Make the mantissa ratio lie in [1/2, 1) so the 64-bit quotient of
	// sm * 2^64 by the divisor has its top bit set.
	div := mm
	e := se - me - 64
	if sm >= mm {
		div = mm << 1
		e++
	}
	q, r := bits.Div64(sm, 0, div)
	// One more quotient bit and the sticky remainder decide the rounding.
	round := r<<1 >= div
	var sticky bool
	if round {
		sticky = r<<1-div != 0
	} else {
		sticky = r != 0
	}
	q, carried := roundNearestEven(q, round, sticky)
	if carried {
		e++
	}
	return ext{q, e}
}

// product returns RN64(a * b) for two integer mantissas and their exponents.
func product(am uint64, ae int, bm uint64, be int) ext {
	if am == 0 || bm == 0 {
		return ext{}
	}
	hi, lo := bits.Mul64(am, bm)
	e := ae + be
	// Normalise the 128-bit product so bit 127 is set.
	var shift uint
	if hi == 0 {
		hi, lo = lo, 0
		e -= 64
	}
	shift = uint(bits.LeadingZeros64(hi))
	if shift != 0 {
		hi = hi<<shift | lo>>(64-shift)
		lo <<= shift
	}
	e += 64 - int(shift)
	round := lo>>63 == 1
	sticky := lo<<1 != 0
	m, carried := roundNearestEven(hi, round, sticky)
	if carried {
		e++
	}
	return ext{m, e}
}

// onePlus returns RN64(1 + p) for 0 <= p <= 1.
func onePlus(p ext) ext {
	if p.mant == 0 {
		return ext{1 << 63, -63}
	}
	// p = mant * 2^exp. In units of 2^-63 (the last place of a value in
	// [1, 2)) it is mant >> shift with shift = -63 - exp.
	shift := -63 - p.exp
	if shift <= 0 {
		// p is exactly one (it cannot exceed it): the sum is two.
		return ext{1 << 63, -62}
	}
	var add uint64
	var round, sticky bool
	switch {
	case shift > 64:
		sticky = true
	case shift == 64:
		round = p.mant>>63 == 1
		sticky = p.mant<<1 != 0
	default:
		add = p.mant >> uint(shift)
		round = p.mant>>uint(shift-1)&1 == 1
		sticky = p.mant&(1<<uint(shift-1)-1) != 0
	}
	sum := uint64(1<<63) + add // add < 2^63, so this cannot wrap
	sum, carried := roundNearestEven(sum, round, sticky)
	if carried {
		return ext{1 << 63, -62}
	}
	return ext{sum, -63}
}

// root returns RN64(sqrt(s)) for a binary64 s in [1, 2].
func root(s float64) ext {
	mant, exp := decompose(s) // s = mant * 2^exp, mant in [2^52, 2^53)
	// Scale to an even power so the integer root carries 64 bits:
	// s = mant * 2^exp; choose k with exp-k even and mant<<k in [2^126, 2^128).
	k := 74
	if (exp-k)%2 != 0 {
		k = 75
	}
	hi := mant << uint(k-64) // k >= 64, so the low word of mant<<k is zero
	lo := uint64(0)
	r := sqrt128(hi, lo)
	// remainder = n - r*r, compared with r to round to nearest (a tie is
	// impossible: (r + 1/2)^2 is not an integer).
	sqHi, sqLo := bits.Mul64(r, r)
	remLo, borrow := bits.Sub64(lo, sqLo, 0)
	remHi, _ := bits.Sub64(hi, sqHi, borrow)
	e := (exp - k) / 2
	if remHi != 0 || remLo > r {
		r++
		if r == 0 {
			return ext{1 << 63, e + 1}
		}
	}
	return ext{r, e}
}

// sqrt128 returns the floor of the square root of the 128-bit integer hi:lo,
// which must be at least 2^126 so that the root has its top bit set.
func sqrt128(hi, lo uint64) uint64 {
	// A binary64 estimate is within a few thousand units; one integer Newton
	// step brings it within one, and the final adjustment makes it exact.
	est := math.Sqrt(float64(float64(hi)*0x1p64) + float64(lo))
	var r uint64
	if est >= 0x1p64 {
		r = math.MaxUint64
	} else {
		r = uint64(est)
	}
	if r < 1<<63 {
		r = 1 << 63
	}
	// Newton: r = (r + n/r) / 2, with n/r formed by a 128/64 division (hi < r
	// holds because hi < 2^64 and r >= 2^63 > hi/2... guard the overflow case).
	if hi < r {
		q, _ := bits.Div64(hi, lo, r)
		sum, carry := bits.Add64(r, q, 0)
		r = sum>>1 | carry<<63
	}
	for {
		sqHi, sqLo := bits.Mul64(r, r)
		if sqHi > hi || (sqHi == hi && sqLo > lo) {
			r--
			continue
		}
		// (r+1)^2 = r^2 + 2r + 1 <= n ?
		addLo, c := bits.Add64(sqLo, r<<1|1, 0)
		addHi, c2 := bits.Add64(sqHi, r>>63, c)
		if c2 == 0 && (addHi < hi || (addHi == hi && addLo <= lo)) {
			r++
			continue
		}
		return r
	}
}

// Distance implements the rounding sequence in [01 R-DET-01 §7].
// Its integer significands preserve each 64-bit intermediate and binary64 store.
func Distance(x, y float64) float64 {
	if math.IsNaN(x) || math.IsNaN(y) {
		// TODO(question): retail NaN payload/sign selection is not established
		// by [01 R-DET-01 §7]; processor NaN propagation rules and an
		// independently authored arithmetic probe would settle it.
		// Use a canonical quiet NaN until then; NaN wins over infinity.
		return math.Float64frombits(0x7ff8000000000000)
	}
	if math.IsInf(x, 0) || math.IsInf(y, 0) {
		return math.Inf(1)
	}
	a := math.Float64frombits(math.Float64bits(x) &^ (1 << 63))
	b := math.Float64frombits(math.Float64bits(y) &^ (1 << 63))
	m, s := a, b
	if b > a {
		m, s = b, a
	}
	if m == 0 {
		return 0
	}
	mm, me := decompose(m)
	sm, se := decompose(s)
	q := store(quotient(sm, se, mm, me))
	qm, qe := decompose(q)
	sum := store(onePlus(product(qm, qe, qm, qe)))
	r := store(root(sum))
	// Mantissas in [1/2, 1): a 53-bit integer times 2^-53.
	rm, re := decompose(r)
	p := store(product(rm, -53, mm, -53))
	pm, pe := decompose(p)
	// exponent(p) + exponent(r) + exponent(m), each in the [1/2, 1) convention.
	e := pe + (re + 53) + (me + 53)
	if e+52 < -1022 {
		// The final exponent replacement truncates subnormals, unlike the
		// earlier quotient store's nearest-even rounding [01 R-DET-01 §7].
		return math.Float64frombits(pm >> uint(-1074-e))
	}
	return store(ext{pm << 11, e - 11})
}

// TruncatedDistance equals TruncateFloat64ToLow32(Distance(x, y)).
//
// Proof of the shortcut: let u=2^-53, h=sqrt(x*x+y*y) in exact arithmetic,
// and 1 <= max(abs(x),abs(y)) <= 2^50. The unfused binary64 expression d
// below has |d/h-1| < 4u: two separately rounded squares, one rounded sum,
// and the correctly rounded root. Any subnormal square contributes an
// absolute error <= 2^-1075, hence less than u relative to h*h >= 1.
//
// For Distance, each RN53(RN64(v)) of a normal v has relative error less
// than 2u. The quotient q differs from the exact ratio by at most 2u
// absolutely (including subnormal underflow). Thus its squared value differs
// by less than 5u; RN64 and addition to one, then RN53, bring the error
// in s below 7u relative to 1+qExact*qExact. Taking the root halves relative
// perturbations (with a factor < 1+8u); its two roundings and the final
// product's two roundings give |Distance/h-1| < 10u. Final exponent scaling
// is exact in this domain. Therefore |Distance-d| < 15u*d.
//
// The margin 2^-45*d is 256u*d, exact binary scaling. The tested gaps d-n
// and n+1-d are exact by Sterbenz's lemma (d >= 1 and n=floor(d)); n and
// n+1 are exactly represented because d < 2^51. If both gaps exceed the
// margin, Distance is strictly in (n,n+1), proving its integer truncation
// is n. The low-word conversion also preserves wrapping. All other inputs,
// including every near-integer boundary and special value, use Distance.
func TruncatedDistance(x, y float64) int32 {
	m := max(math.Abs(x), math.Abs(y))
	if m >= 1 && m <= 0x1p50 {
		d := math.Sqrt(float64(x*x) + float64(y*y))
		n := int64(d) // d < 2^51, so this conversion always fits.
		margin := d * 0x1p-45
		if d-float64(n) > margin && float64(n+1)-d > margin {
			return int32(n)
		}
	}
	return TruncateFloat64ToLow32(Distance(x, y))
}
