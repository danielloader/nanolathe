package ebitenapp

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A flight snapshot is for a frame at least 12 ms later than the interval the
// window presents at — two refreshes at 120 Hz — whatever the cap: a 60 cap
// on a 120 Hz panel presents every 16.7 ms, so one missed refresh (25 ms) is
// not a spike there but two (33.3 ms) are (docs/BATTLE_BENCHMARK.md "Live
// window trace").
func TestFlightSpikeIsLatenessPastTheNominalInterval(t *testing.T) {
	for _, c := range []struct {
		capUS, refresh, interval int64
		want                     bool
	}{
		{0, 8333, 16667, false},
		{0, 8333, 25000, true},
		{16667, 8333, 25000, false},
		{16667, 8333, 33333, true},
		{16667, 16667, 33333, true},
		{0, 0, 16667, false},
		{0, 0, 25000, true},
	} {
		tr := &frameTrace{lastDraw: 1_000_000}
		row := frameRow{due: true, drawStart: 1_000_000 + c.interval, capUS: c.capUS, refresh: c.refresh}
		if got := tr.flightSpike(row); got != c.want {
			t.Errorf("cap %d refresh %d interval %d: spike %v, want %v", c.capUS, c.refresh, c.interval, got, c.want)
		}
	}
	if !(&frameTrace{}).flightSpike(frameRow{simWait: 7000}) {
		t.Error("a host step held 7 ms by the simulation is not a spike")
	}
}

// While the present schedule's cadence is uneven, a flight snapshot is for a
// frame at least 12 ms later than the spacing the schedule planned from the
// frame before: at a 120 cap on 144 Hz a frame planned one refresh after the
// last (6.9 ms) is a spike 18.9 ms after it, though that is not 12 ms past
// the cap's 8.3 ms, and one planned two refreshes after (13.9 ms) is not a
// spike 20.8 ms after it.
func TestFlightSpikeIsLatenessPastThePlannedSpacing(t *testing.T) {
	for _, c := range []struct {
		plan, interval int64
		want           bool
	}{
		{6944, 18944, true},
		{6944, 18000, false},
		{13889, 20833, false},
		{13889, 25889, true},
	} {
		tr := &frameTrace{lastDraw: 1_000_000, lastPlan: c.plan}
		row := frameRow{due: true, drawStart: 1_000_000 + c.interval, capUS: 8333, refresh: 6944}
		if got := tr.flightSpike(row); got != c.want {
			t.Errorf("plan %d interval %d: spike %v, want %v", c.plan, c.interval, got, c.want)
		}
	}
}

// Every row has the header's columns, and plan_us carries the planned spacing
// on a presented row and zero on a skipped one.
func TestFrameTraceWritesThePlanOnPresentedRows(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "trace")
	tr, err := newFrameTrace(&FrameTraceOptions{Directory: dir, ProfileFrom: -1})
	if err != nil {
		t.Fatal(err)
	}
	tr.begin()
	tr.beginUpdate()
	tr.markDraw(time.Second/144, time.Second/120, 2*time.Second/144, true)
	tr.row.drawStart, tr.row.due = 100, true
	tr.beginUpdate()
	tr.markDraw(time.Second/144, time.Second/120, 2*time.Second/144, true)
	tr.row.drawStart = 200
	tr.close()
	f, err := os.Open(filepath.Join(dir, "frames.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("%d lines, want a header and two rows", len(records))
	}
	column := slices.Index(records[0], "plan_us")
	if column < 0 {
		t.Fatalf("no plan_us column in %v", records[0])
	}
	if got := records[1][column]; got != "13888" {
		t.Errorf("presented row plan_us %s, want 13888", got)
	}
	if got := records[2][column]; got != "0" {
		t.Errorf("skipped row plan_us %s, want 0", got)
	}
}
