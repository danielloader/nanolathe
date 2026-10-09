package session

import "github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"

// Checkpoint boundaries and value-only history are specified by
// DESIGN_MULTIPLAYER §16.3.6–§16.3.7. Pump and input position are diagnostic
// metadata, while Tick and Boundary also belong to the canonical stream.
const CheckpointOwnerCount = checkpoint.OwnerCount

type CheckpointBoundary uint8

const (
	CheckpointEntry         CheckpointBoundary = 1
	CheckpointInteriorTick  CheckpointBoundary = 2
	CheckpointFinalPumpTick CheckpointBoundary = 3
)

type CheckpointPosition struct {
	Tick          uint32
	Boundary      CheckpointBoundary
	Pump          uint64
	ConsumedInput uint64
}

type OwnerSummary struct{ Words, Sum uint64 }

type CheckpointRingRow struct {
	Position                                                                        CheckpointPosition
	SimulationState, CRTState                                                       uint32
	SimulationDraws, CRTDraws                                                       uint64
	UnitCount, ProjectileCount, EffectCount, FragmentCount, DebrisCount, StripCount uint32
	Owners                                                                          [CheckpointOwnerCount]OwnerSummary
}

type CheckpointRecord struct {
	Position CheckpointPosition
	Digests  checkpoint.Digests
}

type CheckpointHistory struct {
	Records []CheckpointRecord
	Ticks   []CheckpointRingRow
}

type CheckpointCaptureResult struct {
	Pending bool
	Record  CheckpointRecord
	Err     error
}

// Storage retains values only: no unit, queue, geometry, callback, worker or
// byte-buffer reference survives a capture through this ring. Entry's digest
// shares the record ring; runtime ticks alone enter the tick ring (.35).
type checkpointHistoryRing struct {
	records                 [64]CheckpointRecord
	ticks                   [600]CheckpointRingRow
	nextRecord, recordCount int
	nextTick, tickCount     int
}

func (r *checkpointHistoryRing) appendRecord(record CheckpointRecord) {
	r.records[r.nextRecord] = record
	r.nextRecord = (r.nextRecord + 1) % len(r.records)
	if r.recordCount < len(r.records) {
		r.recordCount++
	}
}

func (r *checkpointHistoryRing) appendTick(row CheckpointRingRow) {
	r.ticks[r.nextTick] = row
	r.nextTick = (r.nextTick + 1) % len(r.ticks)
	if r.tickCount < len(r.ticks) {
		r.tickCount++
	}
}

func (r *checkpointHistoryRing) snapshot() CheckpointHistory {
	if r == nil {
		return CheckpointHistory{}
	}
	var out CheckpointHistory
	if r.recordCount != 0 {
		out.Records = make([]CheckpointRecord, r.recordCount)
		start := (r.nextRecord - r.recordCount + len(r.records)) % len(r.records)
		for i := range out.Records {
			out.Records[i] = r.records[(start+i)%len(r.records)]
		}
	}
	if r.tickCount != 0 {
		out.Ticks = make([]CheckpointRingRow, r.tickCount)
		start := (r.nextTick - r.tickCount + len(r.ticks)) % len(r.ticks)
		for i := range out.Ticks {
			out.Ticks[i] = r.ticks[(start+i)%len(r.ticks)]
		}
	}
	return out
}
