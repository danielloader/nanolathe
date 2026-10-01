package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/pool"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// Clearing selection exposes the authored root panel, including its faction
// emblem. It must remain native-sized over a stretched rail and must not
// become an input-bearing command window [07 §6][07 R-HUD-05].
func TestEmptyBattlePanelShowsAuthoredFactionArt(t *testing.T) {
	opts := Options{Root: probeRetail(t), Map: "ashap plateau", Seed: 7}
	cs, err := openContent(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for side, prefix := range []string{"ARM", "COR"} {
		t.Run(prefix, func(t *testing.T) {
			cfg := session.SkirmishConfig{MapName: opts.Map, NumPlayers: 2}
			cfg.Players[0].Side, cfg.Players[0].Color = side, 4+side
			cfg.Players[1].Side = 1 - side
			cfg.Players[1].Controller = session.SkirmishControllerComputer
			cfg.ApplyDefaults()
			sess, cat, err := newBattleSessionWithConfig(opts, cs, cfg)
			if err != nil {
				t.Fatal(err)
			}
			pal := retailPaletteForTest(t, cs)
			h, err := loadRetailBattleHUD(cs.fs, sess, cat, pal, nil, newBattleWindowContext(cs, nil))
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := h.common.Find(prefix + "PAN2")
			if !ok || len(entry.Frames) != 1 || entry.Frames[0].Frame == nil {
				t.Fatal("reference root panel unavailable")
			}
			art := entry.Frames[0].Frame
			if art.Width != 128 || art.Height != 352 {
				t.Fatal("reference root art has an unexpected size")
			}
			for _, layout := range []struct {
				w, h     int
				expanded bool
			}{{640, 480, false}, {1920, 1080, false}, {1920, 1080, true}} {
				t.Run(fmt.Sprintf("%dx%d-expanded%t", layout.w, layout.h, layout.expanded), func(t *testing.T) {
					c, err := client.New(client.Options{Buffer: &frame.Buffer{}, Width: layout.w, Height: layout.h})
					if err != nil {
						t.Fatal(err)
					}
					c.SetPalette(pal)
					c.SetFNT(h.console)
					c.SetEnhanced(layout.expanded)
					b := &battleSession{sess: sess, cat: cat, hud: h, cl: c}
					h.applyDisplaySize(layout.w, layout.h)
					current := &frame.Frame{}
					c.SetUIStage(painterBindingStage(func(c *client.Client) {
						h.drawRailBackdrop(c, b, current, layout.h)
						h.drawSidePage(c, b, current)
					}))
					shot := c.ComposeFrameSnapshot()
					if dir := os.Getenv("NANOLATHE_EMPTY_PANEL_SHOTS"); dir != "" {
						if err := os.MkdirAll(dir, 0o755); err != nil {
							t.Fatal(err)
						}
						path := filepath.Join(dir, fmt.Sprintf("%s-%dx%d-expanded%t.png", prefix, layout.w, layout.h, layout.expanded))
						file, err := os.Create(path)
						if err != nil {
							t.Fatal(err)
						}
						img := image.NewRGBA(image.Rect(0, 0, shot.Width, shot.Height))
						copy(img.Pix, shot.RGBA)
						err = png.Encode(file, img)
						closeErr := file.Close()
						if err != nil || closeErr != nil {
							t.Fatalf("root panel capture: %v / %v", err, closeErr)
						}
					}
					for y := 0; y < int(art.Height); y++ {
						for x := 0; x < int(art.Width); x++ {
							source := y*int(art.Width) + x
							if art.Transparent[source] {
								continue
							}
							if got := shot.Indexed[(128+y)*shot.Width+x]; got != art.Pixels[source] {
								t.Fatalf("native root pixel (%d,%d) = %d, want authored %d", x, y+128, got, art.Pixels[source])
							}
						}
					}
					rootName := strings.ToLower(prefix) + "main2"
					if !h.windowBuilt[rootName] || h.commandWindowInput.window != nil {
						t.Fatal("empty root was not built or admitted command input")
					}
					// Open the general page, then clear again: the same root
					// raster must return and the command capture must retire.
					current.Selection = frame.SelectionView{Handles: []pool.Handle{1, 2}, Primary: 1, Count: 2}
					c.ComposeFrameSnapshot()
					if h.commandWindowInput.window == nil {
						t.Fatal("multiple selection did not open its general command window")
					}
					current.Selection = frame.SelectionView{}
					cleared := c.ComposeFrameSnapshot()
					if h.commandWindowInput.window != nil {
						t.Fatal("clearing selection retained command input")
					}
					for y := 128; y < 480; y++ {
						for x := 0; x < 128; x++ {
							if cleared.Indexed[y*cleared.Width+x] != shot.Indexed[y*shot.Width+x] {
								t.Fatal("clearing selection did not restore the original root panel")
							}
						}
					}
				})
			}
		})
	}
}
