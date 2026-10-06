package content

import (
	"fmt"
	"testing"
)

// Authored files isolate the discovery counter from the selected-page field
// and the final byte store [02 R-CAT-01 §5]. No retail bytes are used.
func TestBuildPageDiscoveryContinuesPastSelectedPageBits(t *testing.T) {
	for _, tc := range []struct {
		last int
		want int32
	}{{8, 9}, {255, 0}, {256, 1}} {
		t.Run(fmt.Sprint(tc.last), func(t *testing.T) {
			var files []fixtureFile
			for page := 1; page <= tc.last; page++ {
				files = append(files, fixtureFile{path: fmt.Sprintf("guis/probe%d.gui", page), data: "authored"})
			}
			u := &UnitDef{UnitName: "probe"}
			fillUnitRecordBuildPages(newFixtureFS(t, files...), []*UnitDef{u})
			if u.BuildPageCount != tc.want || u.HasPageZeroGUI {
				t.Fatalf("through page %d: count=%d pageZero=%v, want %d/false", tc.last, u.BuildPageCount, u.HasPageZeroGUI, tc.want)
			}
		})
	}
}

func TestBuildPageDiscoveryStripsOnlyFinalSuffixAndStopsAtEmptyFile(t *testing.T) {
	fs := newFixtureFS(t,
		fixtureFile{path: "guis/probe.part0.gui", data: "authored"},
		fixtureFile{path: "guis/probe.part1.gui", data: "authored"},
		fixtureFile{path: "guis/probe.part2.gui", data: ""},
		fixtureFile{path: "guis/probe.part3.gui", data: "authored"},
		fixtureFile{path: "guis/probe.part.fbi1.gui", data: "authored"},
		fixtureFile{path: "guis/probe.part.fbi2.gui", data: "authored"},
	)
	u := &UnitDef{UnitName: "Probe.Part.FBI"}
	fillUnitRecordBuildPages(fs, []*UnitDef{u})
	if u.BuildPageCount != 2 || !u.HasPageZeroGUI || u.UnitName != "Probe.Part.FBI" {
		t.Fatalf("discovery changed name or ignored final-suffix/empty-gap contract: %+v", u)
	}
}
