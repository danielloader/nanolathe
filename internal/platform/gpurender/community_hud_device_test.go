package gpurender

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/camera"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
)

// Exercise the real committed-frame recorder, not a hand-built approximation
// of its labels. Registered in the existing device loop; no extra window or
// asset dependency. Interface design §3.14 owns the admission prerequisites.
func checkCommunityHUDDevicePixels() error {
	previous := client.DamageBars()
	defer client.SetDamageBars(previous)
	font := &formats.FNT{Height: 5}
	for code, rows := range map[byte]string{
		'1': "010110010010111", '2': "110001010100111", '3': "110001010001110",
		'4': "101101111001001", '+': "000010111010000", '/': "001001010100100",
		' ': "000000000000000",
	} {
		bits := make([]byte, 2)
		for i, value := range rows {
			if value == '1' {
				bits[i/8] |= 1 << (7 - i%8)
			}
		}
		font.Glyphs[code] = &formats.FNTGlyph{Width: 3, Height: 5, Bits: bits}
	}
	buffer := frame.NewBuffer()
	f := buffer.BeginWrite()
	f.Units = []frame.UnitView{
		{Owner: 0, X: numeric.FixedFromInt(80), Z: numeric.FixedFromInt(50), Health: 100, MaxHealth: 100, Group: 1,
			CommunityHUD: frame.UnitHUDView{StockpileCount: 3, StockpileQueued: 2}},
		{Owner: 0, X: numeric.FixedFromInt(160), Z: numeric.FixedFromInt(50), Health: 100, MaxHealth: 100, Group: 2,
			CommunityHUD: frame.UnitHUDView{TransportCount: 2, TransportCapacity: 4}},
		{Owner: 0, X: numeric.FixedFromInt(240), Z: numeric.FixedFromInt(50), Health: 100, MaxHealth: 100,
			CommunityHUD: frame.UnitHUDView{Weapons: [3]frame.UnitWeaponHUDView{{Tagged: true, Reload: 30, ReloadTime: 60}}}},
	}
	if err := buffer.Publish(1); err != nil {
		return err
	}
	pal := fixturePalette()
	for _, scale := range []camera.ViewScale{camera.ViewScaleNative, camera.ViewScaleDetail} {
		w, h := int(scale.Px(320)), int(scale.Px(120))
		c, err := client.New(client.Options{Buffer: buffer, Width: w, Height: h})
		if err != nil {
			return err
		}
		c.SetPalette(&pal)
		c.SetFNT(font)
		c.SetCamera(&camera.Camera{ViewW: int32(w), ViewH: int32(h), MapW: 2048, MapH: 2048, Scale: scale})
		c.SetPresentationPaused(true)
		r, err := NewChecked(&pal, w, h)
		if err != nil {
			return err
		}
		for _, state := range []struct {
			name                 string
			bars, labels, groups bool
		}{
			{"enabled", true, true, true},
			{"labels-off", true, false, true},
			{"health-off", false, true, true},
			{"all-off", false, false, false},
		} {
			client.SetDamageBars(state.bars)
			before, _ := c.PausedWorldDigest()
			old := c.CommunityHUDOptions()
			options := client.CommunityHUDOptions{Counters: state.labels, ReloadBars: state.labels, DisableGroupNumbers: !state.groups}
			c.SetCommunityHUDOptions(options)
			after, _ := c.PausedWorldDigest()
			if old != options && before == after {
				return fmt.Errorf("Community HUD change retained paused world at %s", state.name)
			}
			classic := c.ComposeFrame()
			modern := r.Execute(c.RecordModernFrame(), w, h)
			pixels := make([]byte, w*h*4)
			modern.ReadPixels(pixels)
			if !bytes.Equal(classic.Pix, pixels) {
				return fmt.Errorf("Community HUD differs between Classic and Modern at scale %v, %s", scale, state.name)
			}
			for _, want := range []struct {
				index   byte
				visible bool
			}{
				{255, state.bars && state.labels}, // stockpile and cargo glyphs
				{141, state.bars && state.labels}, // half-reloaded weapon
				{15, state.groups},                // assigned group digits
			} {
				found := false
				for i := 0; i < len(pixels); i += 4 {
					found = found || pixels[i] == want.index
				}
				if found != want.visible {
					return fmt.Errorf("Community HUD palette %d visibility=%v, want %v at %s", want.index, found, want.visible, state.name)
				}
			}
			if dir := os.Getenv("NANOLATHE_COMMUNITY_HUD_SHOTS"); dir != "" && state.name == "enabled" {
				path := filepath.Join(dir, fmt.Sprintf("community-labels-%v.png", scale))
				out, err := os.Create(path)
				if err != nil {
					return err
				}
				err = png.Encode(out, &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)})
				closeErr := out.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
		}
		r.ResetSources()
	}
	return checkCommunityHUDZoomPixels(font)
}

// The counter bitmap, centering and line spacing must survive every live zoom.
// Compare the real recorder's foreground mask relative to its health-bar anchor
// against native pixels, including a projected anchor beyond the framebuffer
// that zoom brings into view. This catches scaling text along with the world.
func checkCommunityHUDZoomPixels(font *formats.FNT) error {
	const w, h = 512, 256
	buffer := frame.NewBuffer()
	f := buffer.BeginWrite()
	f.Units = []frame.UnitView{{Owner: 0, X: numeric.FixedFromInt(160), Z: numeric.FixedFromInt(60), Health: 100, MaxHealth: 100,
		CommunityHUD: frame.UnitHUDView{StockpileCount: 3, StockpileQueued: 2, TransportCount: 2, TransportCapacity: 4}}}
	view := f.Units[0]
	if err := buffer.Publish(1); err != nil {
		return err
	}
	client.SetDamageBars(true)
	pal := fixturePalette()
	c, err := client.New(client.Options{Buffer: buffer, Width: w, Height: h})
	if err != nil {
		return err
	}
	c.SetPalette(&pal)
	c.SetFNT(font)
	c.SetCommunityHUDOptions(client.CommunityHUDOptions{Counters: true})
	r, err := NewChecked(&pal, w, h)
	if err != nil {
		return err
	}
	var native []byte
	for i, factor := range []float32{1, .6, .9, 1.1, 1.5, 1.9, 2} {
		unit := buffer.BeginWrite()
		unit.Units = append(unit.Units[:0], view)
		if factor == .6 {
			unit.Units[0].X = numeric.FixedFromInt(600)
		}
		if err := buffer.Publish(uint32(i + 2)); err != nil {
			return err
		}
		z := camera.Zoom(factor * float32(camera.ZoomUnit))
		cam := &camera.Camera{ViewW: w, ViewH: h, MapW: 2048, MapH: 2048, Zoom: z}
		c.SetCamera(cam)
		list := c.RecordModernFrame()
		// Exercise the unquantized factor and fractional camera translation too.
		for i := range list.WorldSpaces() {
			region := &list.WorldSpaces()[i]
			if region.Begin {
				region.Factor, region.OffsetX, region.OffsetY = factor, -.375, .625
			}
		}
		pixels := make([]byte, w*h*4)
		r.Execute(list, w, h).ReadPixels(pixels)
		sx, sy := cam.WorldToScreen(unit.Units[0].X, 0, unit.Units[0].Z)
		k := factor / float32(cam.EffectiveScale().Float())
		cx := int(math.Floor(float64(float32(sx-camera.OriginX)*k-.375) + .5))
		cy := int(math.Floor(float64(float32(sy-camera.OriginY+cam.EffectiveScale().Px(10))*k+.625) + .5))
		mask := make([]byte, 48*24)
		for y := -24; y < 0; y++ {
			for x := -24; x < 24; x++ {
				if pixels[((cy+y)*w+cx+x)*4] == 255 {
					mask[(y+24)*48+x+24] = 1
				}
			}
		}
		if factor == 1 {
			native = mask
			if bytes.Count(native, []byte{1}) == 0 {
				return fmt.Errorf("zoom counter fixture drew no native text")
			}
		} else if !bytes.Equal(native, mask) {
			return fmt.Errorf("counter glyphs, centering or spacing changed at zoom %v", factor)
		}
		if dir := os.Getenv("NANOLATHE_COMMUNITY_HUD_SHOTS"); dir != "" {
			if err := saveAircraftCapture(filepath.Join(dir, fmt.Sprintf("community-zoom-%v.png", factor)), &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}); err != nil {
				return err
			}
		}
	}
	return nil
}
