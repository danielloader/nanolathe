package ebitenapp

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// FullScreen is a host-owned screen that replaces the client's frame while it
// is active: the Nanolathe screen (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17).
// It is drawn at the display's own pixel size rather than the authored
// 640×480 canvas, it receives the pointer and keyboard (the client sees an
// idle sample, so nothing underneath reacts). Screens may own a software
// pointer through FullScreenPointer; otherwise the system pointer is shown.
// The client keeps stepping, so host work such as a requested content reload
// still runs.
type FullScreen interface {
	// Active reports whether the screen owns the window this frame.
	Active() bool
	// Update runs once per display frame on the game goroutine.
	Update()
	// Draw presents the screen; it owns every pixel.
	Draw(screen *ebiten.Image)
}

// FullScreenPointer optionally supplies the active screen's software pointer.
// OwnsPointer is true only while usable art is installed; the host keeps the
// native pointer visible when the screen cannot draw its own.
type FullScreenPointer interface {
	OwnsPointer() bool
}

func (a *app) screenActive() bool {
	return a.options.Screen != nil && a.options.Screen.Active()
}

// screenScale caches the monitor's device scale for screenLayout. Asking the
// monitor is a synchronous round trip to the OS main thread, and Layout runs
// every display frame while the screen is open; the answer changes only when
// the window moves to another display, which also changes its outside size,
// and is otherwise re-read twice a second.
type screenScale struct {
	scale         float64
	outside       [2]int
	at            time.Time
	monitorScaleF func() float64
}

func (s *screenScale) get(outsideWidth, outsideHeight int, now time.Time) float64 {
	outside := [2]int{outsideWidth, outsideHeight}
	if s.scale <= 0 || outside != s.outside || now.Sub(s.at) >= 500*time.Millisecond {
		read := s.monitorScaleF
		if read == nil {
			read = func() float64 { return ebiten.Monitor().DeviceScaleFactor() }
		}
		s.scale, s.outside, s.at = read(), outside, now
	}
	if s.scale <= 0 {
		return 1
	}
	return s.scale
}

// screenLayout is the device-pixel canvas the screen draws at.
func (a *app) screenLayout(outsideWidth, outsideHeight int) (int, int) {
	scale := a.screenScale.get(outsideWidth, outsideHeight, time.Now())
	return max(1, int(float64(outsideWidth)*scale)), max(1, int(float64(outsideHeight)*scale))
}
