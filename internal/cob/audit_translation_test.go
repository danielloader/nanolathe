package cob

import "testing"

// Authored scripts establish both the initial position and the move. The
// signed-speed and wrapped-position cases exercise [04 §4.6]; they do not
// assert that shipped scripts supply these operands.
func TestTranslationSignedArrival(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		current, target, speed int32
		delta                  int
		want                   int32
		busy                   bool
	}{
		{"negative speed upward", 0, 100, -30, 1, 100, false},
		{"negative speed downward", 0, -100, -30, 1, -100, false},
		{"positive position wrap", 2147483646, 2147483647, 60, 1, -2147483648, true},
		{"negative position wrap", -2147483647, -2147483648, 60, 1, 2147483647, true},
		{"inclusive upward", 0, 2, 60, 1, 2, false},
		{"inclusive downward", 0, -2, 60, 1, -2, false},
		{"overshoot upward", 0, 1, 60, 1, 1, false},
		{"overshoot downward", 0, -1, 60, 1, -1, false},
		{"delta scales step", 0, 100, 60, 3, 6, true},
		{"delta scales negative step", 0, -100, 60, 3, -6, true},
		{"maximum speed and drain delta", 0, 2147483647, 2147483647, 5, 357913940, true},
		{"negative speed truncates to zero", 0, 100, -29, 1, 0, false},
		{"already at target", 100, 100, 30, 1, 100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newAuditYieldVM(t, []uint32{
				0x10021001, uint32(tc.current),
				0x1000b000, 0, 0, // move-now sets the current translation
				0x10021001, uint32(tc.speed),
				0x10021001, uint32(tc.target),
				0x10001000, 0, 0,
				0x10012000, 0, 0, // wait-for-move
			})
			v.Drain(tc.delta)
			if got := v.Pieces[0].GetTrans(0).Raw(); got != int64(tc.want) {
				t.Fatalf("translation = %d, want %d", got, tc.want)
			}
			axis := v.anims[0].axes[0]
			if axis.moveBusy != tc.busy || (axis.moveSpeed != 0) != tc.busy || v.dirty != tc.busy {
				t.Fatalf("arrival state: busy=%v speed=%d dirty=%v, want busy=%v", axis.moveBusy, axis.moveSpeed, v.dirty, tc.busy)
			}
			if v.Threads[0].Status != ThreadWaitMove || v.getStatic(0) != 0 {
				t.Fatal("wait resumed within its issuing entry")
			}
			v.Drain(0)
			if tc.busy {
				if v.Threads[0].Status != ThreadWaitMove || v.getStatic(0) != 0 {
					t.Fatal("incomplete move released its waiter")
				}
			} else if v.Threads[0].Status != ThreadIdle || v.getStatic(0) != 7 {
				t.Fatal("completed move did not release its waiter on the later entry")
			}
		})
	}
}
