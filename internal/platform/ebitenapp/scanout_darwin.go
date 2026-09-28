//go:build darwin && !ebitenginevmguest

package ebitenapp

import (
	"github.com/ebitengine/purego/objc"
	"github.com/hajimehoshi/ebiten/v2"
)

// nativeScanout chooses how a fullscreen window reaches the display
// (DESIGN_GPU_RENDERER §13.5 "Composited fullscreen").
//
// The display scans out an opaque fullscreen window directly, from a queue
// with no refresh to spare when every refresh carries a frame, and anything
// that disturbs the window server then shows frames a refresh late
// (framePacer). A window the window server composites is shown from a longer
// queue that absorbs the same disturbances, two refreshes later. The window
// server composites a window whose layer is not declared opaque, so the
// declaration is withdrawn while the window is fullscreen and its frames are
// crowded, and restored otherwise: windowed play is composited anyway, and
// the direct route is the quicker one wherever the pacer keeps refreshes
// free.
//
// A layer that is not opaque is blended over the window behind it. Ebitengine
// clears the screen to transparent black, which an opaque layer shows as
// black, so the window's background is made black for as long as the layer
// is blended: the border around the game's picture, and any pixel the game
// left less than opaque, then look as they do on the direct route.
//
// The zero value leaves the window as Ebitengine made it.
type nativeScanout struct {
	// route is the route the window was last given. The game loop owns it.
	route scanoutRoute
	// background is the window's own background colour, retained while the
	// window is composited. It belongs to the main thread.
	background objc.ID
}

// update gives the window the route wanted once it has been wanted for long
// enough (scanoutRoute), crossing to the main thread only then. A window with
// no layer to change is asked again at the next host step.
func (s *nativeScanout) update(composite bool) {
	if !s.route.settle(composite) {
		return
	}
	found := false
	ebiten.RunOnMainThread(func() {
		found = s.apply(composite)
	})
	if found {
		s.route.took(composite)
	}
}

// composited reports whether the window was last given the composited route.
func (s *nativeScanout) composited() bool { return s.route.composite }

// apply changes the window whose content is a Metal layer, and reports
// whether there was one. It runs on the main thread, so the background and
// the layer change in one Core Animation transaction.
func (s *nativeScanout) apply(composite bool) bool {
	window, layer := metalContentWindow()
	if layer == 0 {
		return false
	}
	if composite {
		if s.background == 0 {
			s.background = window.Send(objc.RegisterName("backgroundColor")).Send(objc.RegisterName("retain"))
		}
		black := objc.ID(objc.GetClass("NSColor")).Send(objc.RegisterName("blackColor"))
		window.Send(objc.RegisterName("setBackgroundColor:"), black)
	}
	layer.Send(objc.RegisterName("setOpaque:"), !composite)
	if !composite && s.background != 0 {
		window.Send(objc.RegisterName("setBackgroundColor:"), s.background)
		s.background.Send(objc.RegisterName("release"))
		s.background = 0
	}
	return true
}

// metalContentWindow is the application's window whose content view is
// backed by a Metal layer, and that layer; both are zero before Ebitengine
// has given its window one.
func metalContentWindow() (window, layer objc.ID) {
	metal := objc.GetClass("CAMetalLayer")
	application := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	windows := application.Send(objc.RegisterName("windows"))
	if metal == 0 || windows == 0 {
		return 0, 0
	}
	for i := range objc.Send[uint64](windows, objc.RegisterName("count")) {
		window := windows.Send(objc.RegisterName("objectAtIndex:"), i)
		layer := window.Send(objc.RegisterName("contentView")).Send(objc.RegisterName("layer"))
		if layer != 0 && objc.Send[bool](layer, objc.RegisterName("isKindOfClass:"), metal) {
			return window, layer
		}
	}
	return 0, 0
}
