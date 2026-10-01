package settings

// Zoom styles are host camera preferences (DESIGN_GPU_RENDERER §16).
// Smooth is the default, including for files written before this choice.
const (
	ZoomSmooth  = 0
	ZoomStepped = 1
	ZoomNone    = 2
	// The preferred stop may span the free zoom range. A battle clamps it to
	// its own full-map floor without changing this persisted preference.
	ZoomLockMinPercent     = 1
	ZoomLockMaxPercent     = 200
	ZoomLockDefaultPercent = 100
)

// Strategic icon styles select generated Modern symbols or community art
// (DESIGN_GPU_RENDERER §18). The community art path remains a separate key.
const (
	StrategicIconsModern    = 0
	StrategicIconsCommunity = 1
)

// normalizeZoom keeps supported choices and repairs unsupported values to
// the defaults. These are enumerated preferences, not low-bit switches.
func (p *Presentation) normalizeZoom() {
	if p.ZoomStyle != ZoomStepped && p.ZoomStyle != ZoomNone {
		p.ZoomStyle = ZoomSmooth
	}
	if p.ZoomLockPercent < ZoomLockMinPercent || p.ZoomLockPercent > ZoomLockMaxPercent {
		p.ZoomLockPercent = ZoomLockDefaultPercent
	}
	if p.StrategicIconStyle != StrategicIconsCommunity {
		p.StrategicIconStyle = StrategicIconsModern
	}
}
