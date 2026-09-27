package ebitenapp

import "testing"

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
