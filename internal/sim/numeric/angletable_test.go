package numeric

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"testing"
)

func TestAngleSinCosAllWords(t *testing.T) {
	for a := 0; a < AngleUnitsTurn; a++ {
		radians := float64(a) * 2 * math.Pi / 65536
		sin, cos := AngleSinCos(Angle(a))
		wantSin, wantCos := SinRadians(radians), CosRadians(radians)
		if runtime.GOARCH == "amd64" {
			wantSin, wantCos = math.Sin(radians), math.Cos(radians)
		}
		if math.Float64bits(sin) != math.Float64bits(wantSin) || math.Float64bits(cos) != math.Float64bits(wantCos) {
			t.Fatalf("angle %d: %.17g,%.17g want %.17g,%.17g", a, sin, cos, wantSin, wantCos)
		}
	}
}

// Digest generated independently with Go 1.27.1 math on amd64, over sine then
// cosine binary64 bits in little-endian order for every ascending angle.
func TestAngleSinCosAMD64Digest(t *testing.T) {
	h := sha256.New()
	var row [16]byte
	for a := 0; a < AngleUnitsTurn; a++ {
		sin, cos := AngleSinCos(Angle(a))
		binary.LittleEndian.PutUint64(row[:8], math.Float64bits(sin))
		binary.LittleEndian.PutUint64(row[8:], math.Float64bits(cos))
		h.Write(row[:])
	}
	const want = "3918277d6e2ace7c654cc34dfdefde07450fa2811dbfd8509ec02b6ec5925f77"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Fatalf("angle table digest %s want %s", got, want)
	}
}
