package construction

import "testing"

// The unit viewer's construction-time estimate repeats the established step
// [05 R-WORK-01 §1]; these cases are hand-checked from that arithmetic.
func TestWorkTicksRepeatsTheStoredFractionStep(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		workerTime, buildTime int32
		want                  int
	}{
		// Quantum 90/30 = 3 against buildtime 12: quarters are exact in
		// binary, so 1 -> 0.75 -> 0.5 -> 0.25 -> 0 takes four steps.
		{"exact quarters", 90, 12, 4},
		// 59/30 truncates to quantum 1, the integer division before the float
		// conversion: buildtime 4 again takes four exact quarter steps.
		{"integer quantum", 59, 4, 4},
		// A third is not exact. The second step stores float32(1/3), which is
		// slightly above the subtrahend, so the third step leaves about 1e-8
		// and a fourth is needed: dividing buildtime by the quantum says three.
		{"single-precision carry", 30, 3, 4},
	} {
		if got, ok := WorkTicks(tc.workerTime, tc.buildTime, 1000); !ok || got != tc.want {
			t.Errorf("%s: WorkTicks(%d, %d) = %d, %t; want %d", tc.name, tc.workerTime, tc.buildTime, got, ok, tc.want)
		}
	}
}

func TestWorkTicksRefusesWorkTheStepCannotFinish(t *testing.T) {
	for _, tc := range []struct {
		name                            string
		workerTime, buildTime, maxTicks int32
	}{
		{"zero quantum", 29, 10, 1000},
		{"zero buildtime", 300, 0, 1000},
		{"negative buildtime", 300, -5, 1000},
		// Sixty-fourths are exact, so buildtime 64 takes exactly 64 steps.
		{"bound reached", 30, 64, 63},
		// One step of 1/2^31 is far below float32 spacing just under one,
		// so the stored fraction never moves.
		{"stalled fraction", 30, 1<<31 - 1, 1000},
	} {
		if got, ok := WorkTicks(tc.workerTime, tc.buildTime, int(tc.maxTicks)); ok || got != 0 {
			t.Errorf("%s: WorkTicks = %d, %t; want unavailable", tc.name, got, ok)
		}
	}
	if got, ok := WorkTicks(30, 64, 64); !ok || got != 64 {
		t.Errorf("a bound equal to the step count must finish: %d, %t", got, ok)
	}
}
