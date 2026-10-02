package numeric

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

// These vectors were generated from the Go 1.27.1 amd64 library, independently
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

func TestRadiansAgainstAMD64Library(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64 is the reference library")
	}
	r := xorshift(0x781276321)
	for i := 0; i < 16384; i++ {
		x, y := math.Float64frombits(r.next()), math.Float64frombits(r.next())
		if i%2 == 0 {
			x = float64(int64(r.next())) / 0x1p63
			y = float64(int64(r.next())) / 0x1p63
		}
		got := [5]float64{SinRadians(x), CosRadians(x), TanRadians(x), Atan2Radians(y, x), AcosRadians(x)}
		want := [5]float64{math.Sin(x), math.Cos(x), math.Tan(x), math.Atan2(y, x), math.Acos(x)}
		for j := range got {
			if math.Float64bits(got[j]) != math.Float64bits(want[j]) {
				t.Fatalf("(%g,%g) function %d: %016x want %016x", x, y, j, math.Float64bits(got[j]), math.Float64bits(want[j]))
			}
		}
	}
}
