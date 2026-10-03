package session

import (
	"encoding/binary"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/hud"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

// savedFlags reads a projected base record's packed status word back into the
// unit flags it carries: word bits 4..15 are flags 0..11 and word bits 20..31
// are flags 14..25 [08 R-SAVE-02 §6].
func savedFlags(rec save.UnitRecord) uint32 {
	word := binary.LittleEndian.Uint32(rec.Data[0xb4:])
	return (word>>4)&0x0fff | (word>>6)&0x03ffc000
}

// A retail save writes the client's selection and moved page into the status
// words, as retail saved them, without writing the live words; a load hands
// them back, and the host seeds its fresh local state from the restored words
// (hud.LocalInterface; DESIGN_MULTIPLAYER §7.3).
func TestRetailSaveCarriesTheLocalSelectionAndPages(t *testing.T) {
	bank, deps := restoreRNGFixture(t)
	loaded, err := LoadRetailSaveWithDeps(bank, deps)
	if err != nil {
		t.Fatal(err)
	}
	src := loaded.Battle.Session
	var us []*units.Unit
	for slot := 1; slot < src.Units.TotalRecords(); slot++ {
		if u := src.Units.Unit(pool.Handle(slot)); u != nil {
			us = append(us, u)
		}
	}
	if len(us) < 2 {
		t.Fatalf("fixture has %d units, want two", len(us))
	}
	ref := func(u *units.Unit) pool.UnitRef { return pool.UnitRef{Handle: u.Handle, Serial: u.AllocationSerial} }
	// A stale bit on the second unit — what a loaded file could carry — is
	// replaced by the client's selection.
	us[1].Flags |= units.SelectedStatus
	live := []uint32{us[0].Flags, us[1].Flags}
	const page3 = hud.PagePagedBit | 3<<23

	in, err := src.RetailBattleSaveInputs(loaded.Summary, save.Camera{})
	if err != nil {
		t.Fatal(err)
	}
	plainIn := in
	in.LocalInterface = &RetailLocalInterface{
		Selected: []pool.UnitRef{ref(us[0]), {Handle: us[1].Handle, Serial: us[1].AllocationSerial + 99}},
		Pages:    []RetailPageField{{Ref: ref(us[1]), Flags: page3}},
	}
	projection, err := src.RetailProjection(in)
	if err != nil {
		t.Fatal(err)
	}
	if us[0].Flags != live[0] || us[1].Flags != live[1] {
		t.Fatal("taking a save wrote the live status words")
	}
	saved := map[uint16]uint32{}
	for _, rec := range projection.Units.Records {
		saved[rec.StableID] = savedFlags(rec)
	}
	first, second := saved[uint16(us[0].Handle)], saved[uint16(us[1].Handle)]
	if first&units.SelectedStatus == 0 {
		t.Fatal("the selected unit was saved without its selected bit")
	}
	if second&units.SelectedStatus != 0 {
		t.Fatal("a stale reference or a stale word bit was saved as selected")
	}
	if second&hud.PageFieldMask != page3 || first&hud.PageFieldMask != live[0]&hud.PageFieldMask {
		t.Fatalf("page fields %#x/%#x, want the moved page on the builder and the word's own elsewhere", first&hud.PageFieldMask, second&hud.PageFieldMask)
	}

	// No client: each word is written as the session holds it.
	plain, err := src.RetailProjection(plainIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range plain.Units.Records {
		if rec.StableID == uint16(us[1].Handle) && savedFlags(rec)&units.SelectedStatus == 0 {
			t.Fatal("a save with no client dropped the session word's selected bit")
		}
	}

	// Load the client's save: the restored words carry the bits, and a fresh
	// local state adopts the selection from the load's publication.
	data, err := projection.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := save.OpenBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	again, err := LoadRetailSaveWithDeps(reopened, deps)
	if err != nil {
		t.Fatal(err)
	}
	dst := again.Battle.Session
	r0, r1 := dst.Units.Unit(us[0].Handle), dst.Units.Unit(us[1].Handle)
	if r0 == nil || r1 == nil || r0.Flags&units.SelectedStatus == 0 || r1.Flags&units.SelectedStatus != 0 || r1.Flags&hud.PageFieldMask != page3 {
		t.Fatal("the restored status words lost the saved selection or page")
	}
	local := hud.NewLocalInterface()
	f := dst.Snapshot.Current()
	if !local.AdoptStatusWords(f) {
		t.Fatal("the load's publication was already composed")
	}
	if !local.Selected(ref(r0)) || local.Selected(ref(r1)) || local.SelectionCount() != 1 {
		t.Fatalf("seeded selection %v, want the saved unit", local.SelectedRefs())
	}
	for _, v := range f.Units {
		if v.Slot == r1.Handle && local.PageFlags(ref(r1), v.Flags) != page3 {
			t.Fatal("the builder's page did not come back from its restored word")
		}
	}
}
