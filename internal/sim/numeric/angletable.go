package numeric

import "math"

// The table uses exactly the existing angle-to-radian expression [I2].
var angleSinCos = func() [AngleUnitsTurn][2]float64 {
	var table [AngleUnitsTurn][2]float64
	for a := range table {
		r := float64(a) * 2 * math.Pi / 65536
		table[a] = [2]float64{SinRadians(r), CosRadians(r)}
	}
	return table
}()

// AngleSinCos returns the sine and cosine of every 16-bit angle word.
func AngleSinCos(a Angle) (sin, cos float64) {
	return angleSinCos[a][0], angleSinCos[a][1]
}
