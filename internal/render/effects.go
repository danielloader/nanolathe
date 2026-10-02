package render

// NanolatheColor is the fixed segment color for build/reclaim beams [03 §5.5].
// Every retail emitter passes the same constant; variation is footprint jitter,
// not color.
//
// It is a GUI semantic index, resolved through the GUIPAL-to-display map at
// draw time [03 §4.3] — the same map the build ghost and the queued build-site
// markers index [07 §9]. GUIPAL entry 6 is the sixteen-color set's brown,
// (170, 85, 0), which lands on the display palette's nearest orange. Passing 6
// straight to an indexed primitive would paint some unrelated palette entry.
const NanolatheColor = 6 // [03 §5.5] GUI index, resolve via Client.GUIColor
