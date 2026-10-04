//go:build darwin && !ebitenginevmguest

package ebitenapp

import (
	"fmt"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nanolathe-gg/nanolathe/internal/input"
)

const coreGraphicsLibrary = "/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics"

// scrollWheelDeltaY is kCGScrollWheelEventDeltaAxis1 from CGEventTypes.h.
const scrollWheelDeltaY uint32 = 11

// Observe AppKit scroll and magnify events without consuming them (§16.6 of
// DESIGN_GPU_RENDERER). Ebitengine's Wheel API loses precision and momentum
// metadata and does not expose native pinch gestures. Event masks and phases
// below are the public AppKit constants from NSEvent.h.
func startNativeScrollMonitor() (func(), error) {
	lib, err := purego.Dlopen(coreGraphicsLibrary, purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("nanolathe: load wheel input: logical path CoreGraphics, providers searched [CoreGraphics], expected scroll event fields: %w", err)
	}
	var integerValue func(uintptr, uint32) int64
	purego.RegisterLibFunc(&integerValue, lib, "CGEventGetIntegerValueField")
	const (
		scrollEvent    = 22
		magnifyEvent   = 30
		phaseBegan     = 1
		phaseEnded     = 8
		phaseCancelled = 16
		gestureMask    = uint64(1)<<scrollEvent | uint64(1)<<magnifyEvent
	)
	events := objc.ID(objc.GetClass("NSEvent"))
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		switch objc.Send[uint64](event, objc.RegisterName("type")) {
		case scrollEvent:
			recordNativeScroll(event, integerValue)
		case magnifyEvent:
			phase := objc.Send[uint64](event, objc.RegisterName("phase"))
			nativeScroll.pinch(input.PinchEvent{
				Delta: objc.Send[float64](event, objc.RegisterName("magnification")),
				Began: phase&phaseBegan != 0, Ended: phase&phaseEnded != 0, Cancelled: phase&phaseCancelled != 0,
			})
		}
		return event
	})
	monitor := events.Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"), gestureMask, block)
	if monitor == 0 {
		block.Release()
		_ = purego.Dlclose(lib)
		return nil, fmt.Errorf("nanolathe: install scroll monitor: logical path AppKit/NSEvent, providers searched [AppKit], expected local scroll event monitor")
	}
	nativeScroll.setActive(true)
	return func() {
		events.Send(objc.RegisterName("removeMonitor:"), monitor)
		nativeScroll.setActive(false)
		block.Release()
		_ = purego.Dlclose(lib)
	}, nil
}

func recordNativeScroll(event objc.ID, integerValue func(uintptr, uint32) int64) {
	x := objc.Send[float64](event, objc.RegisterName("scrollingDeltaX"))
	y := objc.Send[float64](event, objc.RegisterName("scrollingDeltaY"))
	precise := objc.Send[bool](event, objc.RegisterName("hasPreciseScrollingDeltas"))
	momentum := objc.Send[uint64](event, objc.RegisterName("momentumPhase")) != 0
	var wheelY float64
	if !precise && !momentum {
		// AppKit's line distance can be fractional or accelerated independently
		// of the wheel count. Only camera zoom spends the integer count; GUI
		// controls retain the scrolling distance (DESIGN_GPU_RENDERER §16.6).
		if cg := event.Send(objc.RegisterName("CGEvent")); cg != 0 {
			wheelY = float64(integerValue(uintptr(cg), scrollWheelDeltaY))
		}
		// Keep AppKit's user-preference-adjusted direction when a line delta
		// supplies it. The integer field supplies only the notch magnitude then.
		if y > 0 && wheelY < 0 || y < 0 && wheelY > 0 {
			wheelY = -wheelY
		}
		// An event without a discrete count still represents deliberate wheel
		// input. Spend one signed notch rather than the scroll-distance magnitude.
		if wheelY == 0 {
			if y > 0 {
				wheelY = 1
			} else if y < 0 {
				wheelY = -1
			}
		}
	}
	nativeScroll.add(x, y, wheelY, precise, momentum)
}
