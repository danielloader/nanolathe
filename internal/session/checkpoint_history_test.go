package session

import (
	"reflect"
	"testing"
)

func TestCheckpointHistoryBoundsOrderAndIsolation(t *testing.T) {
	var ring checkpointHistoryRing
	for i := uint64(0); i < 1301; i++ {
		// Cross tick zero. Insertion order is history order: sorting the raw
		// uint32 label would incorrectly move the newer wrapped ticks first.
		position := CheckpointPosition{Tick: uint32(i - 1000), Boundary: CheckpointFinalPumpTick, Pump: i, ConsumedInput: i * 2}
		ring.appendTick(CheckpointRingRow{Position: position, SimulationDraws: i * 3})
		if i%10 == 0 {
			ring.appendRecord(CheckpointRecord{Position: position})
		}
	}
	first := ring.snapshot()
	if len(first.Ticks) != 600 || len(first.Records) != 64 {
		t.Fatalf("retained %d ticks, %d records", len(first.Ticks), len(first.Records))
	}
	for i, row := range first.Ticks {
		want := uint64(701 + i)
		if row.Position.Pump != want || row.SimulationDraws != want*3 {
			t.Fatalf("tick %d lost chronology or values: %+v", i, row)
		}
	}
	for i, record := range first.Records {
		if want := uint64(670 + i*10); record.Position.Pump != want {
			t.Fatalf("record %d pump %d want %d", i, record.Position.Pump, want)
		}
	}
	if !reflect.DeepEqual(first, ring.snapshot()) {
		t.Fatal("reading drained history")
	}
	first.Ticks[0].Owners[0].Sum++
	first.Records[0].Digests.Full[0]++
	if next := ring.snapshot(); next.Ticks[0].Owners[0].Sum != 0 || next.Records[0].Digests.Full[0] != 0 {
		t.Fatal("history read exposed mutable retained storage")
	}
}

func TestCheckpointHistoryAppendDoesNotAllocate(t *testing.T) {
	var ring checkpointHistoryRing
	if got := testing.AllocsPerRun(1000, func() {
		ring.appendTick(CheckpointRingRow{})
		ring.appendRecord(CheckpointRecord{})
	}); got != 0 {
		t.Fatalf("history append allocated %g", got)
	}
	if got := (*checkpointHistoryRing)(nil).snapshot(); got.Records != nil || got.Ticks != nil {
		t.Fatal("absent history allocated rows")
	}
}
