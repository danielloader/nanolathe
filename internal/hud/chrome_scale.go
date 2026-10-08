package hud

// Modern sidebar scale (DESIGN_INTERFACE_HUD_INPUT "Modern sidebar scale").
// This is Nanolathe host presentation policy, not retail: retail always
// draws the rail at one framebuffer pixel per authored pixel [07 R-HUD-05].

// AutoChromeScaleHeight is the surface height per step of the Auto scale:
// 1x below 1440 rows, 2x from 1440, 3x from 2160.
const AutoChromeScaleHeight = 720

// MinChromeScaleHeight is the smallest virtual rail height a scale may leave.
// The stock rail art and every stock command page are authored for 480 rows,
// so a fixed scale that would leave fewer is reduced.
const MinChromeScaleHeight = 480

// MaxChromeScale is the largest integer magnification offered.
const MaxChromeScale = 3

// ChromeScale resolves a sidebar scale preference — zero for Auto, otherwise a
// fixed factor — against a surface screenH rows tall.
func ChromeScale(pref int, screenH int32) int32 {
	k := int32(pref)
	if k <= 0 {
		k = screenH / AutoChromeScaleHeight
	}
	if k > MaxChromeScale {
		k = MaxChromeScale
	}
	for k > 1 && screenH/k < MinChromeScaleHeight {
		k--
	}
	if k < 1 {
		k = 1
	}
	return k
}
