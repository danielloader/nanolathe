package session

import (
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"testing"
)

func TestPublishedUnitIdentityChangesOnSameSlotReplacement(t *testing.T) {
	def := &content.UnitDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "identity"}, MaxDamage: 1}
	w := newSessionFixtureWorld(2, nil)
	h, err := w.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{Units: w, Snapshot: frame.NewBuffer()}
	s.publishSnapshot(1)
	first := s.Snapshot.Current().Units[0].InstanceID
	s.publishSnapshot(2)
	if first == 0 || s.Snapshot.Current().Units[0].InstanceID != first {
		t.Fatal("unchanged unit lost its identity")
	}
	prior := s.Snapshot.Current()
	w.FreeImmediate(h)
	replacement, err := w.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if replacement != h {
		t.Fatalf("fixture did not reuse slot: %d != %d", replacement, h)
	}
	s.publishSnapshot(3)
	if got := s.Snapshot.Current().Units[0].InstanceID; got == 0 || got == first {
		t.Fatal("replacement inherited retired cache identity")
	}
	if prior.Units[0].InstanceID != first {
		t.Fatal("replacement changed prior committed identity")
	}
	w.FreeImmediate(replacement)
	s.publishSnapshot(4)
	if s.publication.unitIdentities[h].unit != nil {
		t.Fatal("publication retained retired object")
	}
}

// M2-C1: allocations between publications still invalidate commands; the
// presentation cache's counter cannot supply this authoritative identity.
func TestPublishedAllocationSerialSurvivesMissedPublications(t *testing.T) {
	def := &content.UnitDef{UnitName: "reference", MaxDamage: 1}
	w := newSessionFixtureWorld(2, nil)
	s := &Session{Units: w, Snapshot: frame.NewBuffer()}
	var retired pool.UnitRef
	for i := 0; i < 3; i++ {
		h, err := w.Create(def, 0, 0, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		retired = w.Reference(h)
		w.FreeImmediate(h)
	}
	h, err := w.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	ref := w.Reference(h)
	if h != retired.Handle || w.LookupReference(retired) != nil {
		t.Fatal("fixture did not reject reused allocation")
	}
	s.publishSnapshot(1)
	prior := s.Snapshot.Current()
	view := prior.Units[0]
	if view.AllocationSerial != ref.Serial || view.InstanceID == view.AllocationSerial || view.InstanceID == 0 {
		t.Fatalf("publication confused command and cache identity: %+v", view)
	}
	s.publishSnapshot(2)
	if got := s.Snapshot.Current().Units[0]; got.AllocationSerial != ref.Serial || got.InstanceID != view.InstanceID {
		t.Fatal("publication changed an unchanged allocation's identity")
	}
	w.FreeImmediate(h)
	if _, err := w.Create(def, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if prior.Units[0].AllocationSerial != ref.Serial {
		t.Fatal("allocation mutated the committed serial")
	}
	s.publishSnapshot(3)
	if got := s.Snapshot.Current().Units[0]; got.AllocationSerial != w.Reference(h).Serial || got.AllocationSerial == ref.Serial {
		t.Fatal("replacement did not publish its own allocation serial")
	}
}

func TestAllocationReferenceSurvivesInPlaceOwnerChange(t *testing.T) {
	s := newLoopTestSession(t, 2)
	u := s.Units.IterSliced()[0]
	ref, last := s.Units.Reference(u.Handle), s.Units.LastAllocationSerial()
	newOwner := (u.Owner + 1) % 2
	s.CaptureUnit(u.Handle, newOwner)
	if u.Owner != newOwner || s.Units.Reference(u.Handle) != ref || s.Units.LastAllocationSerial() != last {
		t.Fatal("in-place owner change allocated a new command identity")
	}
}
