package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/input"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
)

type panelAuditClock struct {
	samples []uint32
	reads   int
}

func (c *panelAuditClock) Millis32() uint32 {
	i := c.reads
	c.reads++
	if i >= len(c.samples) {
		i = len(c.samples) - 1
	}
	return c.samples[i]
}

// The ordinary viewer uses the injected battle source for both panel reads.
// Later consumers take their own reads, which return the final supplied value;
// the strip does not mutate the authoritative clock [07 §6][I6].
func TestPanelViewerUsesBattleClockBeforeCue(t *testing.T) {
	b := newTestBattle(testCatalogON05(), testWorldON05(40, 40))
	cl := b.cl
	cl.SetFocused(true)
	cl.Input().Mouse.SetPosition(320, 240)
	cl.Input().Kbd.SetKey(input.KeySpace, true)
	s := b.battleState()
	s.PanelOffset = ui.PanelVisible
	s.PanelTarget = ui.PanelVisible
	s.PanelDeadline = 100
	clock := &panelAuditClock{samples: []uint32{101, 107}}
	b.millisSource = clock
	cues := 0
	schedulerBefore := *b.sess.Clock
	s.SetPanelCue(func(cue string) {
		cues++
		if *b.sess.Clock != schedulerBefore {
			t.Fatal("panel admission changed authoritative scheduler state")
		}
		if cue != "Panel" || clock.reads != 2 || s.PanelDeadline != 122 {
			t.Fatalf("cue=%q reads=%d deadline=%d, want Panel/2/122", cue, clock.reads, s.PanelDeadline)
		}
	})
	b.viewerStep(0, cl)
	if cues != 1 || s.PanelOffset != -10 || s.PanelDeadline != 122 {
		t.Fatalf("viewer cues=%d offset=%d deadline=%d", cues, s.PanelOffset, s.PanelDeadline)
	}

	// The next host pass exactly on the deadline does not move the strip;
	// compositor draws likewise do not consume clock samples or advance it.
	clock.samples = []uint32{122}
	clock.reads = 0
	b.viewerStep(0, cl)
	if cues != 1 || s.PanelOffset != -10 || s.PanelDeadline != 122 {
		t.Fatalf("equal-deadline viewer cues=%d offset=%d deadline=%d", cues, s.PanelOffset, s.PanelDeadline)
	}
	reads := clock.reads
	cl.ComposeFrameSnapshot()
	if clock.reads != reads || s.PanelOffset != -10 {
		t.Fatal("composition sampled or advanced the panel clock")
	}
}
