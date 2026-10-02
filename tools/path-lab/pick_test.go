package main

import "testing"

// Lottery classes include unsuccessful groups and use the same observation
// window whether nobody arrives, most arrive, or arrival outlasts the window.
// Deliberately selected hard cases retain their recorded-arrival filters.
func TestRandomGroupsDoNotSelectOnArrival(t *testing.T) {
	for _, tc := range []struct {
		name     string
		arrived  int
		last     int32
		hardCase bool
	}{
		{"above former cutoff", 20, 600, true},
		{"below former cutoff", 19, 600, false},
		{"no arrivals", 0, 0, false},
		{"arrivals beyond window", 32, 3000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &minedFile{Rec: "authored/sample", Map: "authored", LastTick: 5000,
				Cohorts: []cohortRec{{Index: 1, T0: 100, N: 32, Distance: 800, Arrived: tc.arrived, LastArr: tc.last}}}
			found := map[string]pickCandidate{}
			for _, c := range candidatesOf(m, 1800) {
				found[c.class] = c
			}
			for _, class := range []string{"group-random", "group-large", "army"} {
				c, ok := found[class]
				if !ok {
					t.Fatalf("%s excludes a group with %d/32 arrivals", class, tc.arrived)
				}
				if c.t0 != 99 || c.t1 != 1899 {
					t.Fatalf("%s window [%d,%d] depends on arrival", class, c.t0, c.t1)
				}
			}
			if _, ok := found["group"]; ok != tc.hardCase {
				t.Fatalf("hard-case admission = %v, want %v", ok, tc.hardCase)
			}
		})
	}
}

// Calm recording-comparison groups exclude losses and substantial combat.
// Army samples compare replays with combat disabled and admit both. The
// recording's end may shorten a fixed window, independently of arrival.
func TestRandomGroupsKeepCalmAndArmyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		combat int32
		died   int
		calm   bool
	}{
		{"calm boundary", 96, 0, true},
		{"combat", 97, 0, false},
		{"loss", 0, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &minedFile{Rec: "authored/sample", Map: "authored", LastTick: 800,
				Cohorts: []cohortRec{{Index: 1, T0: 100, N: 32, Distance: 800, Combat: tc.combat, Died: tc.died}}}
			found := map[string]pickCandidate{}
			for _, c := range candidatesOf(m, 1800) {
				found[c.class] = c
				if c.t0 != 99 || c.t1 != 800 {
					t.Fatalf("%s window [%d,%d] extends past the recording", c.class, c.t0, c.t1)
				}
			}
			for _, class := range []string{"group-random", "group-large", "army"} {
				_, ok := found[class]
				if want := class == "army" || tc.calm; ok != want {
					t.Fatalf("%s admitted = %v, want %v", class, ok, want)
				}
			}
		})
	}
}

// Static shortest paths were mined only for arrivals. A long solo move is
// sampled by its commanded distance, without requiring that measurement,
// distance actually travelled, or completion within the observation window.
func TestRandomTripsDoNotRequireArrivalMeasurements(t *testing.T) {
	for _, tc := range []struct {
		name, end string
		combat    int32
		admitted  bool
	}{
		{"arrived", "arrived", 0, true},
		{"stopped short", "short", 0, true},
		{"superseded", "superseded", 0, true},
		{"unfinished", "open", 0, true},
		{"loss", "died", 0, false},
		{"combat", "arrived", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := episodeRec{Unit: 7, Name: "authored", Purpose: "move", Combat: tc.combat,
				episodeOut: episodeOut{T0: 100, TEnd: 3000, GoalX: 1000, HasGoal: true, Cohort: -1, End: tc.end}}
			m := &minedFile{Rec: "authored/sample", Map: "authored", LastTick: 5000, Episodes: []episodeRec{e}}
			cs := candidatesOf(m, 1800)
			if !tc.admitted {
				if len(cs) != 0 {
					t.Fatalf("non-calm trip admitted: %+v", cs)
				}
				return
			}
			if len(cs) != 1 || cs[0].class != "trip-random" || cs[0].t0 != 99 || cs[0].t1 != 1899 {
				t.Fatalf("unsuccessful or unmeasured trip not sampled in the fixed window: %+v", cs)
			}
		})
	}
}
