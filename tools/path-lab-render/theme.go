package main

import "sort"

// Colours of the pictures. The terrain is dimmed and desaturated so the
// overlays, which are light and saturated, read on any ground; everything
// drawn on the terrain carries a dark halo for the same reason.

const (
	terrainDesaturate = 0.45
	terrainDim        = 0.62
)

var (
	pageBG      = hex("#141619")
	stripBG     = hex("#1d2025")
	inkFG       = hex("#eef0f2")
	inkMuted    = hex("#a8aeb5")
	halo        = hex("#000000")
	warn        = hex("#ff3b30")
	otherUnit   = hex("#8e959d")
	otherTrack  = hex("#c9ced4").alpha(0.55)
	structFill  = hex("#08090b").alpha(0.6)
	structLine  = hex("#e8eaed").alpha(0.8)
	featureMark = hex("#aebfd0").alpha(0.9)
	featureEdge = hex("#000000").alpha(0.55)
	regionLine  = hex("#ffffff").alpha(0.5)
)

// subjectPalette gives each subject its own colour. The order is a
// farthest-point selection in OKLab over light, saturated colours kept away
// from the warning red and the grey of other units: each colour is the one
// farthest from those before it, judged under normal vision and under
// simulated protanopia and deuteranopia. So a snippet with few subjects gets
// the most distinct colours; the first eight stay at least 17 apart (OKLab
// distance x100) for normal vision and 9.9 under the simulations, thirteen
// at least 12 and 7.3. Many subjects cannot all be told apart by colour
// alone, which is why the pictures also label them by unit ID.
var subjectPalette = []rgba{
	hex("#36c5ff"), // sky blue
	hex("#bbff00"), // lime
	hex("#998800"), // olive
	hex("#bb33ff"), // violet
	hex("#ffaa88"), // peach
	hex("#88ffee"), // pale aqua
	hex("#9977bb"), // lavender grey
	hex("#33cc00"), // green
	hex("#00ddbb"), // aqua
	hex("#ddee99"), // pale yellow
	hex("#7799ff"), // periwinkle
	hex("#33aa99"), // teal
	hex("#dd88cc"), // orchid
	hex("#bbdd44"), // yellow-green
	hex("#77aa55"), // moss
	hex("#ff11cc"), // magenta
}

// assignColours gives subjects their colours by rank of unit ID, so a
// subject has the same colour in every panel and picture of a snippet
// whatever order the snippet or the logs list units in. Past the palette's
// length the colours repeat.
func assignColours(ids []int) map[int]rgba {
	sorted := append([]int(nil), ids...)
	sort.Ints(sorted)
	out := make(map[int]rgba, len(sorted))
	for i, id := range sorted {
		out[id] = subjectPalette[i%len(subjectPalette)]
	}
	return out
}
