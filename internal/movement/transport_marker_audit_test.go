package movement

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/model"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// Authored callbacks return without changing pose; the pad query selects COB
// piece zero, mapped to the model's descendant rather than its root.
func auditTransportBinding(u *units.Unit, offset [3]numeric.Fixed) *cob.Binding {
	vm := cob.NewVM(&cob.Program{
		Pieces:      []string{"pad", "root"},
		Code:        []uint32{0x10021001, 0, 0x10065000, 0x10021001, 0, 0x10023002, 0, 0x10021001, 0, 0x10065000},
		Scripts:     map[string]int{"BeginTransport": 0, "QueryLandingPad": 3},
		ScriptsByID: []int{0, 3},
	})
	bridge := cob.NewCallbackBridge(vm)
	b := &cob.Binding{VM: vm, Callbacks: bridge, PieceMap: []int{1, 0}, Model: &model.Model{Root: 0, Pieces: []model.Piece{
		{Name: "root", Parent: -1, Children: []int{1}},
		{Name: "pad", Parent: 0, Translate: offset},
	}}}
	u.ScriptState = &units.ScriptState{VM: vm, Bridge: bridge, Binding: b}
	u.Script = vm
	return b
}

// Lowering keeps the cargo-heading gate even though it follows the origin,
// not a cargo piece [04 R-AIR-01 §9]. Exercise the ordinary queue wake too.
func TestPickupMarkerAuditHeadingArrival(t *testing.T) {
	s, w, carrier, cargo, sim, kinds := transportFixture(t)
	b := auditTransportBinding(carrier, [3]numeric.Fixed{})
	starts, finishes := 0, 0
	b.Callbacks.SetLifecycleSink(func(e cob.LifecycleEvent) {
		if e.Name == "BeginTransport" {
			if e.Phase == "start" {
				starts++
			}
			if e.Phase == "finish" {
				finishes++
			}
		}
	})
	carrier.X, carrier.Y, carrier.Z = cargo.X, cargo.Y, cargo.Z
	carrier.Move.Heading, cargo.Move.Heading = 0, 0x4000
	carrier.Pending = 0
	n := pushAirOrder(t, carrier, "VTOL_Pickup", cargo.X, cargo.Z)
	n.Target, n.Phase, n.Param1, n.Deadline = cargo.Handle, 3, 0, -1
	before := *sim
	pump := &orders.Pump{World: w}
	pump.PumpUnit(carrier.Handle, 1)
	m, ok := s.AirGoalPayload(carrier.Handle).(*airMarker)
	if !ok || m == nil || n.Phase != 4 {
		t.Fatalf("lowering marker/phase = %T/%d", s.AirGoalPayload(carrier.Handle), n.Phase)
	}
	s.StepFlightCommand(carrier, nil, nil)
	if got := s.FlightCommandFor(carrier.Handle, carrier).Heading; got != cargo.Move.Heading {
		t.Fatalf("command heading = %d, want cargo %d", got, cargo.Move.Heading)
	}
	if n.Satisfied&airGoalArrivedBit != 0 {
		t.Fatal("unequal headings reported arrival")
	}
	pump.PumpUnit(carrier.Handle, 2)
	if n.Phase != 4 || cargo.Attachment.Carrier != 0 {
		t.Fatal("unequal headings released the attach phase")
	}
	carrier.Move.Heading = cargo.Move.Heading
	s.StepFlightCommand(carrier, nil, nil)
	if n.Satisfied&airGoalArrivedBit == 0 {
		t.Fatal("matching pose did not report arrival")
	}
	pump.PumpUnit(carrier.Handle, 3)
	if cargo.Attachment.Carrier != carrier.Handle || transportCountKind(*kinds, 12) != 1 {
		t.Fatal("matching heading did not attach exactly once")
	}
	if starts != 1 || finishes != 1 || *sim != before {
		t.Fatalf("callback starts/finishes = %d/%d, RNG changed = %v", starts, finishes, *sim != before)
	}
}

// The selected pad must use the live mapped hierarchy, including subsequent
// animation and orientation, rather than the pad owner's origin [04 R-AIR-01 §14.1].
func TestLandingMarkerAuditMappedLivePiece(t *testing.T) {
	s, _, lander, pad, sim, _ := transportFixture(t)
	pad.Def.IsAirBase = true
	pad.Move.Heading, pad.Move.Bank, pad.Move.Pitch = 0, 0, 0
	b := auditTransportBinding(pad, [3]numeric.Fixed{64 << 16, 8 << 16, 0})
	n := pushAirOrder(t, lander, "VTOL_Landing", pad.X, pad.Z)
	n.Target, n.Phase = pad.Handle, 3
	before := *sim
	if got := s.legPadLanding(lander, n, 0, 1, nil); got != 1 {
		t.Fatalf("approach result = %d", got)
	}
	m := s.AirGoalPayload(lander.Handle).(*airMarker)
	var goal Vec3
	m.UpdateGoal(lander, &goal)
	if want := (Vec3{X: pad.X + 64<<16, Y: pad.Y + 8<<16, Z: pad.Z}); goal != want {
		t.Fatalf("mapped goal = %+v, want %+v", goal, want)
	}
	lander.X, lander.Y, lander.Z = pad.X, pad.Y, pad.Z
	if m.Arrived(lander) {
		t.Fatal("owner origin incorrectly reached offset pad within radius 48")
	}
	b.VM.Pieces[0].Trans[0] = 16 << 16
	pad.Move.Heading = 0x8000
	m.UpdateGoal(lander, &goal)
	if want := (Vec3{X: pad.X - 80<<16, Y: pad.Y + 8<<16, Z: pad.Z}); goal != want {
		t.Fatalf("animated oriented goal = %+v, want %+v", goal, want)
	}
	if *sim != before {
		t.Fatal("pad goal updates drew RNG")
	}
}

// The no-piece word widens to −1, never to a real piece of a large model
// [04 R-AIR-01 §9]: map COB piece 255 to the offset pad and the marker must
// still follow the origin.
func TestFollowMarkerAuditNoPieceIsNotPiece255(t *testing.T) {
	s, _, u, target, _, _ := transportFixture(t)
	b := auditTransportBinding(target, [3]numeric.Fixed{64 << 16, 8 << 16, 3 << 16})
	b.PieceMap = make([]int, 256)
	b.PieceMap[255] = 1
	want := Vec3{X: target.X, Y: target.Y, Z: target.Z}
	if got := s.newFollowPieceMarker(u, target.Handle, airNoPiece).targetAttachPosition(target); got != want {
		t.Fatalf("no-piece marker resolved a real piece: %+v, want origin %+v", got, want)
	}
	if got := s.newFollowPieceMarker(u, target.Handle, 255).targetAttachPosition(target); got == want {
		t.Fatal("fixture piece 255 does not resolve; the test cannot distinguish the sentinel")
	}
}

func TestFollowMarkerAuditNoPieceAndInvalidFallback(t *testing.T) {
	s, _, u, target, _, _ := transportFixture(t)
	b := auditTransportBinding(target, [3]numeric.Fixed{64 << 16, 8 << 16, 3 << 16})
	want := Vec3{X: target.X, Y: target.Y, Z: target.Z}
	for _, piece := range []uint16{airNoPiece, 0xfffe, 2, 0x7fff} {
		m := s.newFollowPieceMarker(u, target.Handle, piece)
		if got := m.targetAttachPosition(target); got != want {
			t.Fatalf("piece %d = %+v, want origin %+v", piece, got, want)
		}
	}
	plain := s.newFollowUnitMarker(u, target.Handle)
	if got := plain.targetAttachPosition(target); got != want {
		t.Fatalf("plain follow resolved a real piece: %+v", got)
	}
	b.PieceMap[0] = 99
	if got := s.newFollowPieceMarker(u, target.Handle, 0).targetAttachPosition(target); got != want {
		t.Fatal("invalid mapped piece did not fall back to origin")
	}
	target.ScriptState = nil
	target.Script = nil
	if got := s.newFollowPieceMarker(u, target.Handle, 0).targetAttachPosition(target); got != want {
		t.Fatal("missing binding did not fall back to origin")
	}
}

// Consumer-only pose fixture: this does not claim a complete flight history
// producing a quarter-turn bank. The caller must preserve any supplied unit
// orientation and sample it once [04 R-AIR-01 §9][04 R-REV-02].
func TestPickupMarkerAuditTiltedOffsetSampledOnce(t *testing.T) {
	s, _, u, cargo, _, _ := transportFixture(t)
	auditTransportBinding(u, [3]numeric.Fixed{64 << 16, 8 << 16, 0})
	u.Move.Heading, u.Move.Pitch, u.Move.Bank = 0, 0, 0x4000
	n := pushAirOrder(t, u, "VTOL_Pickup", cargo.X, cargo.Z)
	n.Target, n.Phase, n.Param1 = cargo.Handle, 3, 0
	if got := s.legVTOLPickup(u, n, 0, 1); got != 1 {
		t.Fatalf("lowering result = %d", got)
	}
	m := s.AirGoalPayload(u.Handle).(*airMarker)
	if m.altOffset != -64 {
		t.Fatalf("oriented lowering offset = %d, want -64", m.altOffset)
	}
	u.Move.Bank = 0
	var goal Vec3
	m.UpdateGoal(u, &goal)
	if m.altOffset != -64 || goal.Y != cargo.Y-64<<16 {
		t.Fatalf("installed offset was resampled: %d, goal %+v", m.altOffset, goal)
	}
	if got := s.transportHangOffset(u, 0); got != -8 {
		t.Fatalf("new pose sample = %d, want -8", got)
	}
	for _, piece := range []int32{-1, 2} {
		if got := s.transportHangOffset(u, piece); got != 0 {
			t.Fatalf("invalid piece %d offset = %d", piece, got)
		}
	}
}

// A finite authored descendant translation can cross the signed coordinate
// boundary even when the pad origin is inside the map [04 R-REV-02].
func TestLandingMarkerAuditWorldPointWrapsBeforeAltitude(t *testing.T) {
	s, _, u, pad, _, _ := transportFixture(t)
	pad.Move.Heading, pad.Move.Pitch, pad.Move.Bank = 0, 0, 0
	const reach numeric.Fixed = 32760 << 16
	auditTransportBinding(pad, [3]numeric.Fixed{reach, reach, -reach})
	m := s.newFollowPieceMarker(u, pad.Handle, 0)
	want := Vec3{X: pad.X + reach - 1<<32, Y: pad.Y + reach - 1<<32, Z: pad.Z + reach - 1<<32}
	if got := m.targetAttachPosition(pad); got != want {
		t.Fatalf("wrapped world point = %+v, want %+v", got, want)
	}
	m.setAltitudeOffset(2)
	want.Y += 2 << 16
	var goal Vec3
	m.UpdateGoal(u, &goal)
	if goal != want {
		t.Fatalf("goal after altitude = %+v, want %+v: wrap precedes the ceiling", goal, want)
	}
}
