package client

import "github.com/nanolathe-gg/nanolathe/internal/palette"

// Original disease colours, resolved once against the active content palette.
// Both renderers receive the same palette indices; there are no draw-time
// colour searches or random draws (DESIGN_GPU_RENDERER §38).
func infectionNanoColors(p *palette.Tables) (ramp [7]uint8) {
	if p == nil {
		return ramp
	}
	for i, rgb := range [7][3]int{{106, 29, 49}, {153, 43, 66}, {195, 68, 91}, {228, 115, 134}, {132, 143, 49}, {173, 177, 74}, {209, 205, 110}} {
		ramp[i] = nearestPaletteIndex(p, rgb[0], rgb[1], rgb[2])
	}
	return ramp
}
