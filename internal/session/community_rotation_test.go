package session

import (
	"encoding/binary"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/orders"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/units"
)

func TestCommunityRestoreRotatesBeforeBinding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		bmcode    uint8
		rotations content.FacingMask
		footX     int32
		wantX     int16
	}{
		{"building", 0, content.FacingSouth | content.FacingEast, 2, 3},
		{"unauthored-facing", 0, content.FacingSouth, 2, 2},
		{"mobile", 1, content.FacingSouth | content.FacingEast, 2, 2},
		{"oversized", 0, content.FacingSouth | content.FacingEast, 33, 33},
	} {
		def := &content.UnitDef{UnitName: "lab", BMCode: tc.bmcode, FootprintX: tc.footX, FootprintZ: 3, YardMap: "cooooo", Rotations: tc.rotations, Limit: -1}
		def.CanonicalKey = "lab"
		def.Script = &cob.Program{Code: []uint32{0x10065000}, Scripts: map[string]int{}}
		cat := &content.Catalog{Units: map[string]*content.UnitDef{"lab": def}}
		w := units.NewSliced(4, cat)
		seen := false
		w.SetCOBBinder(func(u *units.Unit) error {
			seen = true
			if u.FootprintSizeX != tc.wantX {
				t.Fatalf("%s binder footprint=%d want=%d", tc.name, u.FootprintSizeX, tc.wantX)
			}
			return nil
		})
		data := make([]byte, save.UnitBoxSize)
		copy(data, "lab")
		binary.LittleEndian.PutUint16(data[0x39:], 32768+16384)
		_, err := allocateRetailUnit(w, cat, save.UnitRecord{StableID: 1, Data: data})
		if err != nil {
			t.Fatal(err)
		}
		if !seen {
			t.Fatal("creator did not bind the reserved unit")
		}
	}
}

func TestQueuedRotationSnapshotRetainsFootprint(t *testing.T) {
	def := &content.UnitDef{UnitName: "lab", FootprintX: 2, FootprintZ: 3}
	cat := &content.Catalog{Units: map[string]*content.UnitDef{"lab": def}}
	q := orders.SnapshotQueue{Primary: []orders.SnapshotNode{{BuildProduct: "lab", BuildFacing: units.FacingEast}}}
	got := appendOrderQueueView(nil, q, cat)[0].Primary[0]
	if got.FootX != 3 || got.FootZ != 2 || got.BuildFacing != uint8(units.FacingEast) {
		t.Fatalf("queued footprint=%dx%d facing=%d", got.FootX, got.FootZ, got.BuildFacing)
	}
}
