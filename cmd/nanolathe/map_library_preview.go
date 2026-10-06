package main

import (
	"context"
	"image"
	"image/color"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/modfetch"
)

// This host-owned catalogue keeps the ordinary map chooser's preview intact.
// Transfer status uses the upper-right space, above the picture.
func buildMapsPreviewWindow(w *gui.Window) {
	for i := range w.Gadgets {
		gad := &w.Gadgets[i]
		switch gad.Name {
		case "STATUS":
			gad.Rect = gui.Rect{X: 352, Y: 86, W: 116, H: 48}
		case "STATUS2", "STATUS3":
			gad.Active = 0
		}
	}
	w.Gadgets = append(w.Gadgets,
		gui.Gadget{Name: "CATALOGUEPIC", Kind: gui.KindSurface, Active: 1, Rect: gui.Rect{X: 352, Y: 150, W: 116, H: 116}},
		gui.Gadget{Name: "PREVIEWSTATUS", Kind: gui.KindLabel, Active: 1, Attribs: gui.AttribInert, ColorF: 15, Rect: gui.Rect{X: 352, Y: 185, W: 116, H: 32}},
	)
}

func (state *mapsFetch) stopPreview() {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.previewSerial++
	if state.previewCancel != nil {
		state.previewCancel()
	}
}

// Called with the state lock. Each selection owns a generation, even when a
// cancelled transport completes late or the player returns to the same map.
func (state *mapsFetch) startPreviewLocked() {
	if state.selected < 0 || state.selected >= len(state.entries) {
		return
	}
	entry := state.entries[state.selected]
	key := entry.ID + "@" + entry.Version
	if entry.Preview != nil {
		key += ":" + entry.Preview.SHA256 + ":" + entry.Preview.URL
	}
	if key == state.previewKey {
		return
	}
	if state.previewCancel != nil {
		state.previewCancel()
	}
	state.previewSerial++
	serial := state.previewSerial
	state.previewKey, state.preview, state.previewPixels = key, nil, nil
	state.previewStatus = "No preview available"
	if entry.Preview == nil {
		return
	}
	state.previewStatus = "Loading preview..."
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	state.previewCancel = cancel
	c := &modfetch.Client{CatalogURL: modfetch.MapCatalogURL(), CacheDir: state.lib.Root}
	go func() {
		defer cancel()
		picture, err := c.FetchPreview(ctx, entry)
		state.finishPreview(serial, picture, err)
	}()
}

func (state *mapsFetch) finishPreview(serial uint64, picture image.Image, err error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.previewSerial != serial {
		return
	}
	state.preview, state.previewPixels, state.dirty = picture, nil, true
	switch {
	case err != nil:
		state.previewStatus = "Preview unavailable"
		state.preview = nil
	case picture == nil:
		state.previewStatus = "No preview available"
	default:
		state.previewStatus = ""
	}
}

func (g *gameShell) drawMapsPreview(c *client.Client, r gui.Rect) {
	state := mapsFetchUI
	if state == nil || c.PaletteTables() == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.preview == nil {
		return
	}
	pal := c.PaletteTables().Base
	if state.previewPixels == nil || state.previewPalette != pal {
		state.previewPixels = mapPreviewIndexed(state.preview, pal, int(r.W), int(r.H))
		state.previewPalette = pal
	}
	c.UIBlitIndexed(state.previewPixels, int(r.W), int(r.H), int(r.X), int(r.Y), int(r.W), int(r.H))
}

// Preview PNG indices belong to their own palette. Match their actual RGB
// colours into the active display palette, including a mod-provided palette.
func mapPreviewIndexed(src image.Image, pal [256][4]byte, width, height int) []byte {
	if src == nil || width <= 0 || height <= 0 || src.Bounds().Empty() {
		return nil
	}
	pixels := make([]byte, width*height)
	cache := make(map[color.RGBA]byte)
	nearest := func(c color.RGBA) byte {
		if index, ok := cache[c]; ok {
			return index
		}
		best, distance := byte(0), int(^uint(0)>>1)
		for i, p := range pal {
			r, g, b := int(c.R)-int(p[0]), int(c.G)-int(p[1]), int(c.B)-int(p[2])
			d := r*r + g*g + b*b
			if d < distance {
				best, distance = byte(i), d
			}
		}
		cache[c] = best
		return best
	}
	black := nearest(color.RGBA{A: 255})
	for i := range pixels {
		pixels[i] = black
	}
	bounds := src.Bounds()
	w, h := width, height
	if bounds.Dx()*height > bounds.Dy()*width {
		h = max(1, bounds.Dy()*width/bounds.Dx())
	} else {
		w = max(1, bounds.Dx()*height/bounds.Dy())
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBAModel.Convert(src.At(bounds.Min.X+x*bounds.Dx()/w, bounds.Min.Y+y*bounds.Dy()/h)).(color.RGBA)
			pixels[(y+(height-h)/2)*width+x+(width-w)/2] = nearest(c)
		}
	}
	return pixels
}
