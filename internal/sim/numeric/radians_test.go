package numeric

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

// These vectors were generated from the Go 1.27.1 amd64/v1 library, independently
// of the adapted source. They preserve full bits, not just narrowed headings.
func TestRadiansCommittedVectors(t *testing.T) {
	f, err := os.Open("testdata/radians-amd64.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		if strings.HasPrefix(scanner.Text(), "#") {
			continue
		}
		var b [7]uint64
		if _, err := fmt.Sscanf(scanner.Text(), "%x %x %x %x %x %x %x", &b[0], &b[1], &b[2], &b[3], &b[4], &b[5], &b[6]); err != nil {
			t.Fatalf("line %d: %v", line, err)
		}
		x, y := math.Float64frombits(b[0]), math.Float64frombits(b[1])
		results := [5]float64{SinRadians(x), CosRadians(x), TanRadians(x), Atan2Radians(y, x), AcosRadians(x)}
		for i, v := range results {
			if bits := math.Float64bits(v); bits != b[i+2] {
				t.Fatalf("line %d function %d: %016x want %016x", line, i, bits, b[i+2])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

// Digest generated independently from Go 1.27.1 math on amd64/v1 using
// eachRadiansSample's operand stream. Each row hashes x, y, Sin(x), Cos(x),
// Tan(x), Atan2(y, x), Acos(x), as little-endian binary64 bits, including NaNs.
// This checks the dense sample on every target, including those whose library
// fuses arithmetic and therefore cannot serve as the unfused reference [I2].
func TestRadiansAMD64Digest(t *testing.T) {
	h := sha256.New()
	var row [56]byte
	eachRadiansSample(func(x, y float64) {
		values := [...]float64{x, y, SinRadians(x), CosRadians(x), TanRadians(x), Atan2Radians(y, x), AcosRadians(x)}
		for j, v := range values {
			binary.LittleEndian.PutUint64(row[j*8:], math.Float64bits(v))
		}
		h.Write(row[:])
	})
	const want = "20d451bd1cb4517b3476476f485f8d4fb273f228186d8aa31ee87d8b172b1896"
	if got := fmt.Sprintf("%x", h.Sum(nil)); got != want {
		t.Fatalf("radian sample digest %s want %s", got, want)
	}
}

func eachRadiansSample(check func(x, y float64)) {
	r := xorshift(0x781276321)
	for i := 0; i < 16384; i++ {
		x, y := math.Float64frombits(r.next()), math.Float64frombits(r.next())
		if i%2 == 0 {
			x = float64(int64(r.next())) / 0x1p63
			y = float64(int64(r.next())) / 0x1p63
		}
		check(x, y)
	}
}
