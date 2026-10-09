package session

import "fmt"

// CheckpointDifference keeps independent evidence from a common position.
// Owners uses section ID minus one; RNG/Pools are row evidence and Full is
// record evidence (DESIGN_MULTIPLAYER §16.3.80).
type CheckpointDifference struct {
	Position   CheckpointPosition
	Owners     [CheckpointOwnerCount]bool
	RNG, Pools bool
	Full       bool
}

// CheckpointComparison describes only retained, exactly comparable evidence.
// An absent difference does not prove world equality; zero compared counts
// mean no comparable window. UncoveredOwners identifies full-digest differences
// hidden by equal selected summaries at that same position, not between samples.
type CheckpointComparison struct {
	ComparedTicks, ComparedRecords   int
	TickDifference, RecordDifference *CheckpointDifference
	MayPredateTicks                  bool
	UncoveredOwners                  [CheckpointOwnerCount]bool
}

// CompareCheckpointHistories compares detached values without consulting a live
// Session. The caller must establish equal content/configuration identities.
// Exact positions, including pump and input ordinals, must match: different host
// schedules are not simulation-divergence evidence. Bounded scans preserve
// retained order across tick wrap (DESIGN_MULTIPLAYER §16.3.80).
func CompareCheckpointHistories(a, b CheckpointHistory) (CheckpointComparison, error) {
	for i, history := range [2]CheckpointHistory{a, b} {
		if err := validateCheckpointComparisonHistory(history, i); err != nil {
			return CheckpointComparison{}, err
		}
	}
	var out CheckpointComparison
	for _, left := range a.Ticks {
		right, ok := checkpointComparisonRow(b.Ticks, left.Position)
		if !ok {
			continue
		}
		out.ComparedTicks++
		difference := CheckpointDifference{Position: left.Position}
		difference.RNG = left.SimulationState != right.SimulationState || left.CRTState != right.CRTState ||
			left.SimulationDraws != right.SimulationDraws || left.CRTDraws != right.CRTDraws
		difference.Pools = left.UnitCount != right.UnitCount || left.ProjectileCount != right.ProjectileCount ||
			left.EffectCount != right.EffectCount || left.FragmentCount != right.FragmentCount ||
			left.DebrisCount != right.DebrisCount || left.StripCount != right.StripCount
		changed := difference.RNG || difference.Pools
		for owner := range difference.Owners {
			difference.Owners[owner] = left.Owners[owner] != right.Owners[owner]
			changed = changed || difference.Owners[owner]
		}
		if changed && out.TickDifference == nil {
			saved := difference
			out.TickDifference = &saved
			out.MayPredateTicks = out.ComparedTicks == 1
		}
	}
	for _, left := range a.Records {
		for _, right := range b.Records {
			if left.Position != right.Position {
				continue
			}
			out.ComparedRecords++
			difference := CheckpointDifference{Position: left.Position, Full: left.Digests.Full != right.Digests.Full}
			changed := difference.Full
			leftRow, leftPresent := checkpointComparisonRow(a.Ticks, left.Position)
			rightRow, rightPresent := checkpointComparisonRow(b.Ticks, left.Position)
			for owner := range difference.Owners {
				difference.Owners[owner] = left.Digests.Owners[owner] != right.Digests.Owners[owner]
				changed = changed || difference.Owners[owner]
				if difference.Owners[owner] && leftPresent && rightPresent && leftRow.Owners[owner] == rightRow.Owners[owner] {
					out.UncoveredOwners[owner] = true
				}
			}
			if changed && out.RecordDifference == nil {
				saved := difference
				out.RecordDifference = &saved
			}
			break
		}
	}
	return out, nil
}

func checkpointComparisonRow(rows []CheckpointRingRow, position CheckpointPosition) (CheckpointRingRow, bool) {
	for _, row := range rows {
		if row.Position == position {
			return row, true
		}
	}
	return CheckpointRingRow{}, false
}

func validateCheckpointComparisonHistory(history CheckpointHistory, peer int) error {
	if len(history.Records) > 64 || len(history.Ticks) > 600 {
		return checkpointComparisonError(fmt.Sprintf("histories[%d]", peer), "at most 64 records and 600 tick rows")
	}
	// The fixed scratch arrays bound both validation work and storage. Records
	// and rows are independent retained sequences; neither fills the other's gaps.
	var positions [600]CheckpointPosition
	for i := range history.Records {
		positions[i] = history.Records[i].Position
	}
	if err := validateCheckpointComparisonPositions(positions[:len(history.Records)], true, fmt.Sprintf("histories[%d].records", peer)); err != nil {
		return err
	}
	for i := range history.Ticks {
		positions[i] = history.Ticks[i].Position
	}
	return validateCheckpointComparisonPositions(positions[:len(history.Ticks)], false, fmt.Sprintf("histories[%d].ticks", peer))
}

func validateCheckpointComparisonPositions(positions []CheckpointPosition, records bool, path string) error {
	for i, position := range positions {
		switch position.Boundary {
		case CheckpointEntry:
			if !records || i != 0 || position.Tick != 0 || position.Pump != 0 || position.ConsumedInput != 0 {
				return checkpointComparisonError(path, "entry only as the first record at tick, pump and input zero")
			}
		case CheckpointInteriorTick, CheckpointFinalPumpTick:
		default:
			return checkpointComparisonError(path, "a published checkpoint boundary")
		}
		for _, previous := range positions[:i] {
			if position.Tick == previous.Tick {
				return checkpointComparisonError(path, "distinct tick labels and positions within the retained sequence")
			}
		}
		if i != 0 {
			previous := positions[i-1]
			delta := position.Tick - previous.Tick
			if delta == 0 || delta >= uint32(1)<<31 {
				return checkpointComparisonError(path, "nonzero forward tick deltas below half the u32 range")
			}
			if position.Pump < previous.Pump || position.ConsumedInput < previous.ConsumedInput {
				return checkpointComparisonError(path, "nondecreasing pump and consumed-input ordinals")
			}
		}
	}
	return nil
}

func checkpointComparisonError(path, expected string) error {
	return fmt.Errorf("nanolathe: checkpoint comparison failed: logical path %s, providers searched [detached histories], expected %s", path, expected)
}
