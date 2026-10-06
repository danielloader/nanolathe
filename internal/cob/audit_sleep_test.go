package cob

import "testing"

func newAuditYieldVM(t *testing.T, prefix []uint32) *VM {
	t.Helper()
	code := append(append([]uint32{}, prefix...),
		0x10021001, 7, 0x10023004, 0, // completion marker in static 0
		0x10065000)
	prog, err := Load(makeCOBWithStatics(1, code, []string{"Probe"}, []uint32{0}, []string{"base"}))
	if err != nil {
		t.Fatal(err)
	}
	v := NewVM(prog)
	if !v.Start(0, nil) {
		t.Fatal("could not start authored probe")
	}
	return v
}

// The product wraps before signed division, including when a large positive
// duration becomes a negative timer [04 §4.6]. Expected values are authored
// arithmetic cases, not a copy of the implementation's expression.
func TestSleepProductWrapsBeforeDivision(t *testing.T) {
	for _, tc := range []struct {
		name string
		ms   int32
		want int32
	}{
		{"last positive product", 71582788, 2147483},
		{"first wrapped product", 71582789, -2147483},
		{"positive duration", 100000000, -1294967},
		{"negative duration", -100000000, 1294967},
		{"maximum duration", 2147483647, 0},
		{"minimum duration", -2147483648, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newAuditYieldVM(t, []uint32{0x10021001, uint32(tc.ms), 0x10013000})
			v.Drain(1)
			if v.Threads[0].Status != ThreadSleeping || v.Threads[0].Sleep != tc.want || v.getStatic(0) != 0 {
				t.Fatalf("issued sleep: status=%v timer=%d marker=%d, want sleeping timer=%d and no marker",
					v.Threads[0].Status, v.Threads[0].Sleep, v.getStatic(0), tc.want)
			}
			v.Drain(0)
			if tc.want <= 0 {
				if v.getStatic(0) != 7 || v.Threads[0].Status != ThreadIdle {
					t.Fatal("nonpositive timer did not resume on a later delta-zero entry")
				}
			} else if v.Threads[0].Status != ThreadSleeping || v.Threads[0].Sleep != tc.want || v.getStatic(0) != 0 {
				t.Fatal("delta-zero entry advanced a positive timer")
			}
		})
	}
}

// A positive timer needs that many later delta-one entries, with no extra
// decrement. Zero still yields its issuing entry [04 §4.6].
func TestSleepResumesOnLaterEntry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		ms     int32
		visits int
	}{
		{"zero", 0, 1},
		{"rounded to zero", 33, 1},
		{"one", 34, 1},
		{"two", 67, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newAuditYieldVM(t, []uint32{0x10021001, uint32(tc.ms), 0x10013000})
			v.Drain(1)
			if v.getStatic(0) != 0 {
				t.Fatal("sleep completed in its issuing entry")
			}
			for i := 1; i <= tc.visits; i++ {
				v.Drain(1)
				if got := v.getStatic(0); (got == 7) != (i == tc.visits) {
					t.Fatalf("later entry %d: marker=%d, completion entry=%d", i, got, tc.visits)
				}
			}
		})
	}
}

func TestZeroSpeedWaitYieldsUntilLaterEntry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opcode uint32
		status int
	}{
		{"move", 0x10012000, ThreadWaitMove},
		{"turn", 0x10011000, ThreadWaitTurn},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newAuditYieldVM(t, []uint32{tc.opcode, 0, 0})
			v.Drain(0)
			if v.Threads[0].Status != tc.status || v.getStatic(0) != 0 {
				t.Fatal("zero-speed wait did not yield its issuing entry [04 §4.6]")
			}
			v.Drain(0)
			if v.Threads[0].Status != ThreadIdle || v.getStatic(0) != 7 {
				t.Fatal("zero-speed wait did not resume on the later delta-zero entry [04 §4.6]")
			}
		})
	}
}
