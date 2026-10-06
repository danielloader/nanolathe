package content

import (
	"strings"
	"testing"
)

func TestSideLoaderRetainsStringByteLimits(t *testing.T) {
	name := strings.Repeat("N", 28) + "é"
	prefix := "ARMextra"
	commander := strings.Repeat("C", 32)
	font := strings.Repeat("f", 256)
	var text strings.Builder
	text.WriteString("[SIDE0]{name=" + name + ";nameprefix=" + prefix + ";commander=" + commander + ";font=" + font + ";")
	for _, anchor := range sideAnchorNames {
		text.WriteString("[" + anchor + "]{x1=4;y1=7;x2=2;y2=3;}")
	}
	text.WriteString("}")
	sides, err := CompileSides(newFixtureFS(t, fixtureFile{path: "gamedata/sidedata.tdf", data: text.String()}))
	if err != nil || len(sides) != 1 {
		t.Fatalf("side load: count=%d err=%v", len(sides), err)
	}
	s := sides[0]
	if s.Name != name[:29] || s.NamePrefix != "ARM" || s.Commander != commander[:31] || s.Font != font[:255] {
		t.Fatalf("side byte widths: name=%d prefix=%q commander=%d font=%d", len(s.Name), s.NamePrefix, len(s.Commander), len(s.Font))
	}
}
