package numeric

import "math"

// TruncateFloat64ToInt64 implements the signed-64 truncating store, including
// the indefinite result for invalid operands [01 R-DET-01 §1].
func TruncateFloat64ToInt64(v float64) int64 {
	if math.IsNaN(v) || v < -0x1p63 || v >= 0x1p63 {
		return math.MinInt64
	}
	return int64(v)
}

// RoundFloat64ToInt32 implements the nearest-even signed-32 store, including
// the indefinite result for invalid operands [01 R-DET-01 §2].
func RoundFloat64ToInt32(v float64) int32 {
	v = math.RoundToEven(v)
	if math.IsNaN(v) || v < -0x1p31 || v >= 0x1p31 {
		return math.MinInt32
	}
	return int32(v)
}
