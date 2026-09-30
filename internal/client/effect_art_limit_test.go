package client

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
)

// A host's effect art limit refuses a whole entry once any of its frames is
// larger, for drawing and for the blast size that sizes its light, and leaves
// entries within it alone; zero, the battle's setting, refuses nothing.
func TestEffectArtLimitRefusesOversizedEntries(t *testing.T) {
	c := stripTestClient(t)
	c.effectBanks["fx"] = &formats.GAF{Entries: []formats.GAFEntry{
		{Name: "small", Frames: []formats.GAFFrameRef{{Frame: &formats.GAFFrame{Width: 64, Height: 48}}}},
		{Name: "bubble", Frames: []formats.GAFFrameRef{
			{Frame: &formats.GAFFrame{Width: 40, Height: 40}},
			{Frame: &formats.GAFFrame{Width: 760, Height: 760}},
		}},
	}}
	view := func(entry string) frame.EffectView { return frame.EffectView{Graphic: entry, AssetID: "fx"} }
	for _, limit := range []int{0, 512} {
		c.SetEffectArtLimit(limit)
		if _, ok := c.resolveEffectFrame(view("small"), 0); !ok {
			t.Fatalf("limit %d: small entry refused", limit)
		}
		_, drawn := c.resolveEffectFrame(view("bubble"), 0)
		size := c.resolveBlastSize(view("bubble"))
		if limit == 0 && (!drawn || size != 760) {
			t.Fatalf("no limit: bubble drawn %v size %v, want drawn at 760", drawn, size)
		}
		if limit > 0 && (drawn || size != 0) {
			t.Fatalf("limit %d: bubble drawn %v size %v, want refused", limit, drawn, size)
		}
	}
}
