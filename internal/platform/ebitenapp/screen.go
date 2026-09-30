package ebitenapp

import "github.com/hajimehoshi/ebiten/v2"

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

// screenLayout is the device-pixel canvas the screen draws at.
func screenLayout(outsideWidth, outsideHeight int) (int, int) {
	scale := ebiten.Monitor().DeviceScaleFactor()
	if scale <= 0 {
		scale = 1
	}
	return max(1, int(float64(outsideWidth)*scale)), max(1, int(float64(outsideHeight)*scale))
}
