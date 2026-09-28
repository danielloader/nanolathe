package session

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/triggers"
)

// The shipped Example archive supplies an empty use-only file. Battle entry
// must retain the unit catalog so its authored BuildUnitType victory can run.
// [08 R-ENTRY-01 §2][08 R-TRIG-01 §4].
func TestExampleCampaignStartsAndBuildVictory(t *testing.T) {
	f := loadRetailFixture(t)
	if _, err := f.fs.Stat("Example.tdf"); err != nil {
		t.Skipf("the reference install has no Example.tdf: %v", err)
	}
	s, err := NewMissionWithEntryOptions(f.fs, f.cat, "Example.tdf:MISSION0", 0, 7, 7,
		MissionEntryOptions{SelectedSide: 0, SelectedSideSet: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Mission.TerrainKey != "example" || len(s.Mission.Victory) != 1 {
		t.Fatalf("Example mission content: terrain=%q victory=%d", s.Mission.TerrainKey, len(s.Mission.Victory))
	}
	def, ok := s.Catalog.Unit("ARMVP")
	if !ok || def == nil {
		t.Fatal("Example entry excluded its victory unit from the battle catalog")
	}
	win := s.Mission.Victory[0]
	if win.Kind != triggers.KindBuildUnitType || win.Type != "ARMVP" {
		t.Fatalf("Example victory = %s %q", win.Kind, win.Type)
	}
	ctx := triggers.PollContext{World: s.Units, MissionArmed: true}
	if win.Poll(ctx) {
		t.Fatal("Example won before the vehicle plant was finished")
	}
	h, err := s.Units.Create(def, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	plant := s.Units.Unit(h)
	plant.Remaining = 1
	if win.Poll(ctx) {
		t.Fatal("an unfinished vehicle plant satisfied the victory condition")
	}
	plant.Remaining = 0
	if !win.Poll(ctx) {
		t.Fatal("a finished vehicle plant did not satisfy Example's victory condition")
	}
}
