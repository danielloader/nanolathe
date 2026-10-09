package visibility

import (
	"reflect"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/world"
)

// These are online composition contracts of DESIGN_MULTIPLAYER §16.4.1;
// ordinary retail's one-viewer status remains the fallback [03 R-VIS-01 §4].
func TestPerspectiveStatusAllocationAndOverlay(t *testing.T) {
	s := New(nil, ModeHistoryEnabled|ModeCurrentEnabled)
	const fallback uint32 = 0xa5a517d5
	if got := s.StatusForPerspective(1, 9, 11, 1, fallback); got != fallback {
		t.Fatalf("disabled status = %#x, want %#x", got, fallback)
	}
	s.EnableOwnerPerspectives()
	for _, serial := range []uint64{0, 11} {
		if got := s.StatusForPerspective(1, 9, serial, 1, fallback); got != 0xa5a502d5 {
			t.Fatalf("own initial status for serial %d = %#x", serial, got)
		}
		if got := s.StatusForPerspective(0, 9, serial, 1, fallback); got != 0xa5a500d5 {
			t.Fatalf("foreign initial status for serial %d = %#x", serial, got)
		}
	}
	if len(s.perspectiveStatus[1]) != 0 {
		t.Fatal("reading constructor status allocated a retained row")
	}
	flags := fallback
	u := []SensorUnit{{ID: 9, AllocationSerial: 11, Owner: 1, Alive: true, Hidden: true, Status: &flags}}
	before := u[0]
	s.SensorTickForPerspective(1, false, 30, 2, u)
	if flags != fallback || u[0] != before {
		t.Fatal("banked pass mutated its caller's status or descriptor")
	}
	if got := s.StatusForPerspective(1, 9, 11, 1, 0x40001000); got != 0x40000300 {
		t.Fatalf("latched friendly pair with current non-sensor flags = %#x", got)
	}
	s.EnableOwnerPerspectives()
	if got := s.StatusForPerspective(1, 9, 11, 1, 0); got != 0x300 {
		t.Fatalf("repeated enable reset retained status: %#x", got)
	}
	for _, serial := range []uint64{0, 12} {
		if got := s.StatusForPerspective(1, 9, serial, 1, fallback); got != 0xa5a502d5 {
			t.Fatalf("replacement serial %d inherited old contact: %#x", serial, got)
		}
	}
	// Reuse the slot for a foreign unit. Until its first pass the constructor
	// seed applies, even while the old occupant's bank row remains present.
	if got := s.StatusForPerspective(1, 9, 12, 0, fallback); got != 0xa5a500d5 {
		t.Fatalf("foreign replacement inherited contact: %#x", got)
	}
	u[0].AllocationSerial, u[0].Owner = 12, 0
	s.SensorTickForPerspective(1, false, 60, 2, u)
	if got := s.StatusForPerspective(1, 9, 12, 0, fallback); got != 0xa5a500d5 {
		t.Fatalf("foreign hidden replacement after pass = %#x", got)
	}
	if got := s.StatusForPerspective(1, 9, 11, 1, fallback); got != 0xa5a502d5 {
		t.Fatalf("old allocation read replacement's bank: %#x", got)
	}
}

func TestPerspectiveSensorOwnersAndSharedDeadline(t *testing.T) {
	for _, local := range []PlayerID{0, 1} {
		s := newTestService(&world.Terrain{CellW: 64, CellH: 64}, ModeHistoryEnabled|ModeCurrentEnabled)
		s.SetLocal(local)
		s.SetViewerDefeated(true) // neither new pass may borrow this legacy flag
		s.EnableOwnerPerspectives()
		flags := [2]uint32{0x80001700, 0x40001700}
		deadlines := [2]uint32{7, 9}
		units := []SensorUnit{
			{ID: 1, AllocationSerial: 101, Owner: 0, Status: &flags[0], X: 64 << 16, Z: 64 << 16,
				Alive: true, Hidden: true, Active: true, CanCloak: true, OwnerLocallySimulated: true,
				RadarDistance: 100, SonarDistance: 100, MinCloakDistance: 100, PrimaryCandidateOf: 1 << 1, DecloakDeadline: &deadlines[0]},
			{ID: 2, AllocationSerial: 102, Owner: 1, Status: &flags[1], X: 96 << 16, Z: 64 << 16,
				Alive: true, Hidden: true, Active: true, CanCloak: true, OwnerLocallySimulated: true,
				RadarDistance: 100, SonarDistance: 100, MinCloakDistance: 100, PrimaryCandidateOf: 1 << 0, DecloakDeadline: &deadlines[1]},
		}
		s.SensorTickForPerspective(0, false, 30, 2, units)
		if deadlines != [2]uint32{120, 9} {
			t.Fatalf("local %d: owner-0 pass wrote another owner's deadline: %v", local, deadlines)
		}
		if got := s.StatusForPerspective(0, 1, 101, 0, flags[0]); got != 0x80001300 {
			t.Fatalf("local %d: owner-0 source status = %#x", local, got)
		}
		if got := s.StatusForPerspective(0, 2, 102, 1, flags[1]); got != 0x40000300 {
			t.Fatalf("local %d: owner-0 radar/sonar target = %#x", local, got)
		}
		if got := s.StatusForPerspective(1, 1, 101, 0, flags[0]); got != 0x80000000 {
			t.Fatalf("local %d: untouched bank borrowed owner-0 contact: %#x", local, got)
		}
		s.SensorTickForPerspective(1, false, 60, 2, units)
		if deadlines != [2]uint32{120, 150} {
			t.Fatalf("local %d: owner-1 pass wrote another owner's deadline: %v", local, deadlines)
		}
		if got := s.StatusForPerspective(1, 2, 102, 1, flags[1]); got != 0x40001300 {
			t.Fatalf("local %d: owner-1 source status = %#x", local, got)
		}
		if got := s.StatusForPerspective(0, 1, 101, 0, flags[0]); got != 0x80001300 {
			t.Fatalf("local %d: later pass cleared an earlier owner's latch: %#x", local, got)
		}
		// An own source still needs its original active-controller gate.
		units[0].OwnerLocallySimulated = false
		s.SensorTickForPerspective(0, false, 90, 2, units)
		if deadlines != [2]uint32{120, 150} || s.StatusForPerspective(0, 1, 101, 0, 0) != 0x300 {
			t.Fatalf("local %d: source gate or first-pass clear changed", local)
		}
		if flags != [2]uint32{0x80001700, 0x40001700} || s.local != local || !s.viewerDefeated {
			t.Fatal("perspective pass altered caller flags or the legacy viewer")
		}
	}
}

// Radar precedes jam, then direct sight restores only seen; sonar stays jammed
// [03 R-VIS-01 §4 passes 2, 3, 5]. The active viewer must be explicit at all
// three sites, even when the service presents the other human's view.
func TestPerspectiveJammingAndSeenProbeOrder(t *testing.T) {
	s := newTestService(&world.Terrain{CellW: 64, CellH: 64}, ModeHistoryEnabled|ModeCurrentEnabled)
	s.SetLocal(1)
	s.EnableOwnerPerspectives()
	s.byteGrids[0][2*s.W+3] = 1
	flags := [2]uint32{}
	units := []SensorUnit{
		{ID: 1, AllocationSerial: 1, Owner: 0, Alive: true, Active: true, Hidden: true,
			Status: &flags[0], X: 64 << 16, Z: 64 << 16, RadarDistance: 100, SonarDistance: 100},
		{ID: 2, AllocationSerial: 2, Owner: 1, Alive: true, Active: true,
			Status: &flags[1], X: 96 << 16, Z: 64 << 16, RadarJam: 100, SonarJam: 100},
	}
	s.SensorTickForPerspective(0, false, 30, 2, units)
	if got := s.StatusForPerspective(0, 2, 2, 1, 0); got != 0x500 {
		t.Fatalf("jammed target after seen probe = %#x, want seen+jammed only", got)
	}
	if got := s.StatusForPerspective(0, 1, 1, 0, 0); got != 0x400 {
		t.Fatalf("jammed own hidden unit = %#x, want jammed only", got)
	}
	s.SensorTickForPerspective(1, false, 60, 2, units)
	if got := s.StatusForPerspective(1, 2, 2, 1, 0); got != 0x300 {
		t.Fatalf("own jammer suppressed its own perspective: %#x", got)
	}
	if got := s.StatusForPerspective(1, 1, 1, 0, 0); got != 0 {
		t.Fatalf("other perspective borrowed radar/LOS: %#x", got)
	}
	s.SensorTickForPerspective(1, true, 90, 2, units)
	if got := s.StatusForPerspective(1, 1, 1, 0, 0); got != 0x300 {
		t.Fatalf("explicit defeated viewer did not mark a foreign unit friendly: %#x", got)
	}
}

func TestPerspectiveSparseBankAndUncommittedStatus(t *testing.T) {
	s := New(nil, ModeHistoryEnabled|ModeCurrentEnabled)
	s.EnableOwnerPerspectives()
	flags := [2]uint32{}
	units := []SensorUnit{
		{ID: 1, AllocationSerial: 10, Owner: 1, Alive: true, Hidden: true, Status: &flags[0]},
		{ID: 60000, AllocationSerial: 20, Owner: 1, Alive: true, Hidden: true, Status: &flags[1]},
	}
	s.SensorTickForPerspective(0, true, 30, 2, units)
	for _, u := range units {
		if got := s.StatusForPerspective(0, u.ID, u.AllocationSerial, u.Owner, 0); got != 0x300 {
			t.Fatalf("sparse row %d lost its writable bank pointer: %#x", u.ID, got)
		}
	}
	// A not-yet-committed replacement must not overwrite the old slot's row,
	// and its temporary sensor result must not become a zero-serial identity.
	units[0].AllocationSerial = 0
	s.SensorTickForPerspective(0, false, 60, 2, units[:1])
	if got := s.StatusForPerspective(0, 1, 10, 1, 0); got != 0x300 {
		t.Fatalf("uncommitted replacement overwrote the old bank: %#x", got)
	}
	if got := s.StatusForPerspective(0, 1, 0, 1, 0); got != 0 {
		t.Fatalf("zero-serial lookup retained temporary contact: %#x", got)
	}
	units[0].ID = 3
	s.SensorTickForPerspective(0, true, 90, 2, units[:1])
	if got := s.perspectiveStatus[0][3]; got != (perspectiveSensorStatus{}) {
		t.Fatalf("stored a zero-serial row: %+v", got)
	}
	if got := s.StatusForPerspective(0, 3, 0, 1, 0); got != 0 {
		t.Fatalf("zero-serial friendly result escaped its pass: %#x", got)
	}
}

func TestPerspectiveHistoryUsesQueryOwner(t *testing.T) {
	s := newTestService(&world.Terrain{CellW: 8, CellH: 8}, ModeHistoryEnabled)
	s.SetLocal(1)
	s.wordMask[0] = 1 << 0
	probe := func(viewer PlayerID) [4]bool {
		return [4]bool{
			s.VisiblePoint(viewer, 0, 0, 0),
			s.IsVisible(viewer, Target{Owner: 2}),
			s.VisibleExtents(viewer, Box{Owner: 2}),
			s.sampleClampedOrigin(viewer, -numeric.Fixed(1<<16), 0, 0),
		}
	}
	if got := probe(0); got != [4]bool{} {
		t.Fatalf("single-player query stopped using local history: %v", got)
	}
	word := append([]uint16(nil), s.wordMask...)
	var grids [10][]uint8
	for owner := range grids {
		grids[owner] = append([]uint8(nil), s.byteGrids[owner]...)
	}
	mode, mapping, fog := s.mode, s.mappingVersion, s.fogVersion
	s.EnableOwnerPerspectives()
	if got := probe(0); got != [4]bool{true, true, true, true} {
		t.Fatalf("online query did not use owner history: %v", got)
	}
	if got := probe(1); got != [4]bool{} {
		t.Fatalf("other owner borrowed history: %v", got)
	}
	if !reflect.DeepEqual(word, s.wordMask) || !reflect.DeepEqual(grids, s.byteGrids) ||
		s.mode != mode || s.mappingVersion != mapping || s.fogVersion != fog || s.local != 1 {
		t.Fatal("enabling or querying perspectives changed grids or presentation")
	}
	// Current-coverage mode already selects the query owner in both forms.
	s.SetMode(ModeHistoryEnabled | ModeCurrentEnabled)
	s.byteGrids[0][0] = 0
	s.byteGrids[1][0] = 1
	if s.VisiblePoint(0, 0, 0, 0) || !s.VisiblePoint(1, 0, 0, 0) {
		t.Fatal("current coverage stopped selecting the query owner")
	}
}

func TestPerspectiveSkippedPassAndScratchReuse(t *testing.T) {
	s := New(nil, ModeHistoryEnabled|ModeCurrentEnabled)
	var flags uint32
	units := []SensorUnit{{ID: 1, AllocationSerial: 1, Owner: 0, Alive: true, Hidden: true, Status: &flags}}
	s.SensorTickForPerspective(0, true, 1, 2, units)
	if s.ownerPerspectives || len(s.perspectiveStatus[0]) != 0 || flags != 0 {
		t.Fatal("disabled perspective call enabled or wrote state")
	}
	s.EnableOwnerPerspectives()
	s.SensorTickForPerspective(0, false, 30, 2, units)
	before := s.perspectiveStatus[0][1]
	units[0].AllocationSerial = 2
	s.SensorTickForPerspective(0, true, 60, 1, units)
	if s.perspectiveStatus[0][1] != before || len(s.SensorInputs()) != 0 {
		t.Fatal("one-active-player pass changed retained status or kept diagnostics")
	}
	s.SensorTickForPerspective(10, true, 61, 2, units)
	if s.perspectiveStatus[0][1] != before {
		t.Fatal("invalid owner wrote a bank")
	}
	units[0].Alive = false
	s.SensorTickForPerspective(0, false, 90, 2, units)
	units[0].Alive, units[0].Status = true, nil
	s.SensorTickForPerspective(0, false, 120, 2, units)
	if s.perspectiveStatus[0][1] != before {
		t.Fatal("dead or nil-status input replaced a live bank row")
	}
	units[0].Status = &flags
	s.SensorTickForPerspective(0, false, 150, 2, units)
	if allocations := testing.AllocsPerRun(20, func() {
		s.SensorTickForPerspective(0, false, 180, 2, units)
		_ = s.StatusForPerspective(0, 1, 2, 0, flags)
	}); allocations != 0 {
		t.Fatalf("warmed perspective pass/lookup allocated %g times", allocations)
	}
	var absent *Service
	absent.EnableOwnerPerspectives()
	absent.SensorTickForPerspective(0, false, 1, 2, units)
	if got := absent.StatusForPerspective(0, 1, 1, 0, 0xdeadbeef); got != 0xdeadbeef {
		t.Fatalf("nil service changed fallback: %#x", got)
	}
}
