package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/clock"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestUnitStateChecksumPlaytestBoundary(t *testing.T) {
	fixture := func() (*Session, *units.Unit) {
		w := newSessionFixtureWorld(1, nil)
		h, err := w.Create(&content.UnitDef{UnitName: "checksum", MaxDamage: 100}, 0, -65536, 0, 131072)
		if err != nil {
			t.Fatal(err)
		}
		s := &Session{Clock: &clock.State{GlobalTick: 30}, Units: w}
		s.SeedSessionRNG(7, 11)
		return s, w.Unit(h)
	}
	a, _ := fixture()
	b, u := fixture()
	want := a.UnitStateChecksum()
	if b.UnitStateChecksum() != want {
		t.Fatal("equal unit values in independent worlds disagree")
	}
	// Deliberate exclusions: client identity, pacing and hidden RNG state do
	// not enter this small check. A later position/health effect can expose it.
	b.LocalOwner, b.ViewingOwner = 1, 1
	b.Clock.ScaledAnchor++
	b.SimRNG().State++
	if b.UnitStateChecksum() != want {
		t.Fatal("host or hidden state entered the unit-only check")
	}
	for name, change := range map[string]func(*units.Unit){
		"position":   func(u *units.Unit) { u.X++ },
		"health":     func(u *units.Unit) { u.Health-- },
		"owner":      func(u *units.Unit) { u.Owner++ },
		"allocation": func(u *units.Unit) { u.AllocationSerial++ },
		"death":      func(u *units.Unit) { u.Dying = true },
		"removed":    func(u *units.Unit) { u.Alive = false },
	} {
		t.Run(name, func(t *testing.T) {
			saved := *u
			change(u)
			if b.UnitStateChecksum() == want {
				t.Fatal("changed unit state was not detected")
			}
			*u = saved
		})
	}
	beforeSim, beforeCRT := *b.SimRNG(), *b.CrtRNG()
	if n := testing.AllocsPerRun(10, func() { b.UnitStateChecksum() }); n != 0 {
		t.Fatalf("small unit check allocates: %v", n)
	}
	if *b.SimRNG() != beforeSim || *b.CrtRNG() != beforeCRT || b.UnitStateChecksum() != want {
		t.Fatal("observation changed state")
	}
}
