package construction

// WorkTicks counts the unhindered construction steps one builder needs to
// finish one target from a fresh nanoframe: the remaining fraction starts at
// one and the shared step runs until the stored single-precision fraction is
// exactly zero [05 "Construction arithmetic"][05 R-WORK-01 §1]. It reuses that
// step rather than dividing buildtime by the quantum, because each step
// narrows the fraction to float32 and that rounding can add a step.
//
// The fraction does not depend on cost or health, so those terms are passed
// as zero; resource admission is assumed to accept every step. Nothing here
// models approach, factory opening, build-stance waits, nanoframe creation or
// stalls; callers that show the count as time must say so.
//
// ok is false, and the count is unavailable, when the worker quantum is zero
// (`workertime` below thirty makes the step do nothing), when buildtime is not
// positive (a malformed definition [05 R-WORK-01 §11]), when a step no longer
// changes the stored fraction, or when maxTicks steps do not finish it.
//
// The helper is pure: it reads no unit, service, economy or random stream.
func WorkTicks(workerTime, buildTime int32, maxTicks int) (ticks int, ok bool) {
	quantum := float32(WorkerQuantum(workerTime))
	if quantum == 0 || buildTime <= 0 {
		return 0, false
	}
	remaining := float32(1)
	for ticks < maxTicks {
		next, _, _, _ := wideConstructionStepQuantum(remaining, quantum, buildTime, 0, 0, 0)
		ticks++
		if next == 0 {
			return ticks, true
		}
		if next >= remaining {
			// A step too small to change the stored fraction repeats forever.
			return 0, false
		}
		remaining = next
	}
	return 0, false
}
