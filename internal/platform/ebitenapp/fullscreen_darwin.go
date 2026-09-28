//go:build darwin && !ebitenginevmguest

package ebitenapp

import (
	"structs"
	"sync/atomic"

	"github.com/ebitengine/purego/objc"
	"github.com/hajimehoshi/ebiten/v2"
)

// Public NSApplicationPresentationOptions from AppKit/NSApplication.h. Hiding
// the menu bar requires hiding the Dock, and excludes both auto-hide flags and
// autoHideToolbar (which requires autoHideMenuBar).
const (
	presentationAutoHideDock    uintptr = 1 << 0
	presentationHideDock        uintptr = 1 << 1
	presentationAutoHideMenuBar uintptr = 1 << 2
	presentationHideMenuBar     uintptr = 1 << 3
	presentationAutoHideToolbar uintptr = 1 << 11
	presentationEdgeControls            = presentationAutoHideDock | presentationHideDock |
		presentationAutoHideMenuBar | presentationHideMenuBar | presentationAutoHideToolbar
)

// Public NSEventType values from AppKit/NSEvent.h.
const (
	eventMouseEntered = 8
	eventMouseExited  = 9
)

// nsPoint and nsRect are CGPoint and CGRect.
type nsPoint struct {
	_    structs.HostLayout
	X, Y float64
}

type nsRect struct {
	_          structs.HostLayout
	X, Y, W, H float64
}

func (r nsRect) contains(p nsPoint) bool {
	return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

// nativeFullscreenPresentation keeps the top edge available to camera input
// (DESIGN_PRESENTATION_CLIENT §2.1). This is an application policy, not a system
// preference or a replacement for Ebitengine's native fullscreen delegate.
type nativeFullscreenPresentation struct {
	application     objc.ID
	windowedOptions uintptr
	applied         bool
	// withhold is whether pointer exits are withheld (focused fullscreen). The
	// adapter writes it every host step; the event monitor reads it.
	withhold atomic.Bool
	// held is the retained exit the monitor withheld and heldView the content
	// view it was for. Both belong to the main thread.
	held, heldView objc.ID
	stopMonitor    func()
}

// Called before RunGame, on the main thread, while the app is still windowed.
func startNativeFullscreenPresentation() *nativeFullscreenPresentation {
	application := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	p := &nativeFullscreenPresentation{
		application:     application,
		windowedOptions: objc.Send[uintptr](application, objc.RegisterName("presentationOptions")),
	}
	p.stopMonitor = p.monitorPointerCrossings()
	return p
}

func fullscreenPresentationOptions(current uintptr) uintptr {
	return current&^presentationEdgeControls | presentationHideDock | presentationHideMenuBar
}

func restoreFullscreenPresentationOptions(current, windowed uintptr) uintptr {
	return current&^presentationEdgeControls | windowed&presentationEdgeControls
}

// AppKit recalculates its presentation options during fullscreen transitions
// and when returning from another Space. Reconcile while focused rather than
// setting them only once at entry. The setter runs only when they differ.
func (p *nativeFullscreenPresentation) update(fullscreen, focused bool) {
	if p == nil {
		return
	}
	p.withhold.Store(fullscreen && focused)
	if (!fullscreen && !p.applied) || (fullscreen && !focused) {
		return
	}
	ebiten.RunOnMainThread(func() {
		p.apply(fullscreen)
		p.settleHeldExit(fullscreen)
	})
}

// apply runs on AppKit's main thread. Restore only the flags we own, preserving
// native fullscreen and unrelated application options. In particular, never
// add the flags that disable application switching or Force Quit.
func (p *nativeFullscreenPresentation) apply(fullscreen bool) {
	current := objc.Send[uintptr](p.application, objc.RegisterName("presentationOptions"))
	next := restoreFullscreenPresentationOptions(current, p.windowedOptions)
	if fullscreen {
		next = fullscreenPresentationOptions(current)
	}
	if next != current {
		p.application.Send(objc.RegisterName("setPresentationOptions:"), next)
	}
	p.applied = fullscreen
}

// monitorPointerCrossings observes the pointer entering and leaving the
// window's content, counting both for the live trace, and withholds an exit
// while the window is focused and fullscreen.
//
// On a display with a camera housing, native fullscreen places the content
// below a band that the menu bar would occupy, so edge-scrolling up carries
// the pointer out of the content. On that exit the Cocoa backend unhides the
// system pointer, and the unhide stalls presentation
// (DESIGN_PRESENTATION_CLIENT §2.1). The software cursor is the only pointer
// the game shows, so withholding the exit hides nothing the player needs.
//
// Only the tracking area owned by the backend's content view counts; AppKit's
// own fullscreen views track the top edge too. Runs on the main thread.
func (p *nativeFullscreenPresentation) monitorPointerCrossings() func() {
	const mask = uint64(1)<<eventMouseEntered | uint64(1)<<eventMouseExited
	var contentView objc.Class
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		area := event.Send(objc.RegisterName("trackingArea"))
		if area == 0 {
			return event
		}
		if contentView == 0 {
			contentView = objc.GetClass("GLFWContentView")
		}
		view := area.Send(objc.RegisterName("owner"))
		if contentView == 0 || view == 0 || !objc.Send[bool](view, objc.RegisterName("isKindOfClass:"), contentView) {
			return event
		}
		p.releaseHeldExit()
		if objc.Send[uint64](event, objc.RegisterName("type")) == eventMouseEntered {
			nativePointerCrossings.enters.Add(1)
			return event
		}
		nativePointerCrossings.exits.Add(1)
		if !p.withhold.Load() {
			return event
		}
		p.held, p.heldView = event.Send(objc.RegisterName("retain")), view
		return 0
	})
	events := objc.ID(objc.GetClass("NSEvent"))
	monitor := events.Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"), mask, block)
	if monitor == 0 {
		// Without the monitor, exits reach the backend as before and the trace
		// counts stay zero.
		block.Release()
		return func() {}
	}
	return func() {
		events.Send(objc.RegisterName("removeMonitor:"), monitor)
		block.Release()
	}
}

// heldExitAction is what becomes of a withheld pointer exit.
type heldExitAction int

const (
	keepExit heldExitAction = iota
	dropExit
	deliverExit
)

// settleExit decides a withheld exit from where the pointer is now, in screen
// coordinates. Back over the content, the exit is dropped: the system pointer
// was never shown. Once the window has left fullscreen or the pointer has left
// the window's screen, it is delivered, so the backend shows the system
// pointer where it can be used. Otherwise the pointer is still in the band
// above the fullscreen content, and the exit stays withheld.
func settleExit(fullscreen bool, content, screen nsRect, at nsPoint) heldExitAction {
	switch {
	case content.contains(at):
		return dropExit
	case !fullscreen || !screen.contains(at):
		return deliverExit
	}
	return keepExit
}

// settleHeldExit runs on the main thread once per host step while the policy
// is applied, and once more when fullscreen ends.
func (p *nativeFullscreenPresentation) settleHeldExit(fullscreen bool) {
	if p.held == 0 {
		return
	}
	var content, screen nsRect
	if window := p.heldView.Send(objc.RegisterName("window")); window != 0 {
		frame := objc.Send[nsRect](p.heldView, objc.RegisterName("frame"))
		content = objc.Send[nsRect](window, objc.RegisterName("convertRectToScreen:"), frame)
		if display := window.Send(objc.RegisterName("screen")); display != 0 {
			screen = objc.Send[nsRect](display, objc.RegisterName("frame"))
		}
	}
	at := objc.Send[nsPoint](objc.ID(objc.GetClass("NSEvent")), objc.RegisterName("mouseLocation"))
	switch settleExit(fullscreen, content, screen, at) {
	case dropExit:
		p.releaseHeldExit()
	case deliverExit:
		p.heldView.Send(objc.RegisterName("mouseExited:"), p.held)
		p.releaseHeldExit()
	}
}

func (p *nativeFullscreenPresentation) releaseHeldExit() {
	if p.held != 0 {
		p.held.Send(objc.RegisterName("release"))
		p.held, p.heldView = 0, 0
	}
}

// RunGame has returned to the main thread; do not call RunOnMainThread here.
func (p *nativeFullscreenPresentation) close() {
	if p == nil {
		return
	}
	p.stopMonitor()
	p.releaseHeldExit()
	if p.applied {
		p.apply(false)
	}
}
