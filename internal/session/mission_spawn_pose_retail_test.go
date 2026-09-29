//go:build retail

package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// TestRetailMissionUnitsLeaveCreationOnTheSurface locks the allocator's
// creation-time post-move correction [04 R-MOV-01 §5] on the stock missions
// where its absence showed (issue #29). The spawner keeps a mobile unit's
// authored position [08 R-ENTRY-01 §6], and those positions are wrong in the
// maps: Gelidus authors its Arm units 115 world units above the snow, and Arm
// mission 23 authors its ships at `YPos=0`, on the sea floor. The allocator
// corrects both before anything else sees the unit, so at battle entry every
// grounded `upright` mover stands on the terrain and every floater sits at sea
// level less its waterline.
func TestRetailMissionUnitsLeaveCreationOnTheSurface(t *testing.T) {
	f := loadRetailFixture(t)
	for _, tc := range []struct {
		mission          string
		upright, floater bool // which branch the mission must exercise
	}{
		{"camps/Arm Campaign - Core Contingency .tdf:MISSION9", true, false}, // Gelidus
		{"camps/Arm Campaign.tdf:MISSION22", true, true},                     // Crossing Aqueous Body 397
	} {
		t.Run(tc.mission, func(t *testing.T) {
			s, err := NewMissionWithEntryOptions(f.fs, f.cat, tc.mission, 1, 7, 7, MissionEntryOptions{}, nil)
			if err != nil {
				t.Skipf("stock mission unavailable: %v", err)
			}
			var uprights, floaters int
			for _, u := range s.Units.Iter() {
				d := u.Def
				if d.BMCode != 1 || u.Move.ModeMirror != 1 || d.CanHover {
					continue
				}
				switch {
				case d.Upright:
					want := s.World.HeightAt(u.X, u.Z)
					if want == -1 {
						continue
					}
					uprights++
					if u.Y != want {
						t.Errorf("%s h%d stands at y %v, want the terrain's %v", d.UnitName, u.Handle, u.Y, want)
					}
				case d.Floater:
					floaters++
					want := numeric.Fixed((int64(s.World.SeaLevel) - int64(d.Waterline)) << 16)
					if u.Y != want {
						t.Errorf("%s h%d floats at y %v, want sea level less waterline %v", d.UnitName, u.Handle, u.Y, want)
					}
				}
			}
			if (tc.upright && uprights == 0) || (tc.floater && floaters == 0) {
				t.Fatalf("mission exercised %d upright and %d floater movers, want both branches it authors", uprights, floaters)
			}
		})
	}
}
