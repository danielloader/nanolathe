//go:build darwin

package mtl

import (
	"math"
	"runtime"
	"testing"
	"unsafe"
)

// These round trips go through Foundation, which needs no window or GPU, so
// they exercise each calling path the renderer depends on: integer sends,
// floating-point arguments and results, a two-double aggregate and NSRect,
// which Apple silicon returns in d0..d3 and Intel through objc_msgSend_stret.

func loadFrameworks(t *testing.T) uintptr {
	t.Helper()
	if err := Load(); err != nil {
		t.Fatalf("load system frameworks: %v", err)
	}
	return PoolPush()
}

func TestIntegerSendRoundTrip(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	const want = int64(1) << 40
	number := ID(GetClass("NSNumber").Send(Sel("numberWithLongLong:"), uintptr(want)))
	if number == 0 {
		t.Fatal("numberWithLongLong: returned nil")
	}
	if got := int64(number.Get(Sel("longLongValue"))); got != want {
		t.Fatalf("longLongValue = %d, want %d", got, want)
	}
}

func TestDoubleArgumentAndResult(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	const want = -1234.0625
	c := SendCall(GetClass("NSNumber"), Sel("numberWithDouble:"))
	c.SetDouble(0, want)
	c.Do()
	number := ID(c.R[0])
	if number == 0 {
		t.Fatal("numberWithDouble: returned nil")
	}
	if got := SendDouble(number, Sel("doubleValue")); got != want {
		t.Fatalf("doubleValue = %v, want %v", got, want)
	}
}

// boxed wraps a C value in an NSValue through valueWithBytes:objCType:,
// which takes only pointers, so the result tests the return path alone.
func boxed(t *testing.T, value unsafe.Pointer, encoding string) ID {
	t.Helper()
	typ := append([]byte(encoding), 0)
	v := ID(GetClass("NSValue").Send(Sel("valueWithBytes:objCType:"), uintptr(value), uintptr(unsafe.Pointer(&typ[0]))))
	runtime.KeepAlive(typ)
	if v == 0 {
		t.Fatalf("valueWithBytes:objCType:%s returned nil", encoding)
	}
	return v
}

func TestPointResult(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	want := [2]float64{17.25, -3.5}
	v := boxed(t, unsafe.Pointer(&want), "{CGPoint=dd}")
	runtime.KeepAlive(&want)
	if x, y := SendPoint(v, Sel("pointValue")); x != want[0] || y != want[1] {
		t.Fatalf("pointValue = (%v, %v), want %v", x, y, want)
	}
}

func TestRectResult(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	want := [4]float64{1.5, -2.25, 640, 480}
	v := boxed(t, unsafe.Pointer(&want), "{CGRect={CGPoint=dd}{CGSize=dd}}")
	runtime.KeepAlive(&want)
	if got := SendRect(v, Sel("rectValue")); got != want {
		t.Fatalf("rectValue = %v, want %v", got, want)
	}
}

func TestStringRoundTrip(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	const want = "Nanolathe — Metal ✓"
	if got := GoString(String(want)); got != want {
		t.Fatalf("GoString(String(%q)) = %q", want, got)
	}
}

func TestMediaTimeAdvances(t *testing.T) {
	defer PoolPop(loadFrameworks(t))
	a := MediaTime()
	b := MediaTime()
	if !(a > 0) || math.IsInf(a, 0) || b < a {
		t.Fatalf("CACurrentMediaTime = %v then %v, want positive and nondecreasing", a, b)
	}
}
