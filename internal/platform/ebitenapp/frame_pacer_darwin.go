//go:build darwin && !ebitenginevmguest

package ebitenapp

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// nativeFramePacer is the one pacer the display link feeds. The link's
// delegate is a property of a system class, so the hook is installed once for
// the process and outlives any one window loop.
var nativeFramePacer struct {
	once      sync.Once
	installed bool
	pacer     framePacer
	// delegate is the delegate Ebitengine gave the display link, which the
	// pacer's own delegate passes slots on to.
	delegate atomic.Uintptr
}

// startNativeFramePacer puts the pacer between the window's display link and
// Ebitengine, and returns nil where there is no such link to pace: a macOS
// without CAMetalDisplayLink presents through the older path, which has no
// delegate. It must run before the window is created.
//
// CAMetalDisplayLink reports each refresh to its delegate, and Ebitengine's
// delegate takes a drawable for every one. The link belongs to Ebitengine's
// graphics driver and is created, and created again after a resize, where
// this package cannot reach it; what it can reach is the link's public
// setDelegate:. That method is replaced with one that installs the pacer's
// delegate in front of the one it was given. Nothing else about the link, its
// run loop or Ebitengine's delegate changes, and a refresh the pacer passes on
// arrives exactly as it would have.
func startNativeFramePacer() *framePacer {
	n := &nativeFramePacer
	n.once.Do(func() {
		defer func() { _ = recover() }() // a runtime without the calls leaves the link alone
		link := objc.GetClass("CAMetalDisplayLink")
		if link == 0 {
			return
		}
		// The runtime knows the delegate protocol only once something that
		// adopts it is loaded, and the link asks for the method, not the
		// protocol.
		var protocols []*objc.Protocol
		if protocol := objc.GetProtocol("CAMetalDisplayLinkDelegate"); protocol != nil {
			protocols = append(protocols, protocol)
		}
		runtime, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		var (
			classGetInstanceMethod  func(class objc.Class, name objc.SEL) uintptr
			methodGetImplementation func(method uintptr) objc.IMP
			methodSetImplementation func(method uintptr, implementation objc.IMP) objc.IMP
		)
		purego.RegisterLibFunc(&classGetInstanceMethod, runtime, "class_getInstanceMethod")
		purego.RegisterLibFunc(&methodGetImplementation, runtime, "method_getImplementation")
		purego.RegisterLibFunc(&methodSetImplementation, runtime, "method_setImplementation")

		needsUpdate := objc.RegisterName("metalDisplayLink:needsUpdate:")
		target := objc.RegisterName("targetPresentationTimestamp")
		class, err := objc.RegisterClass(
			"NanolatheDisplayLinkPacer",
			objc.GetClass("NSObject"),
			protocols,
			nil,
			[]objc.MethodDef{{
				Cmd: needsUpdate,
				Fn: func(_ objc.ID, _ objc.SEL, link, update objc.ID) {
					// Seconds on the link's clock.
					shown := time.Duration(objc.Send[float64](update, target) * float64(time.Second))
					if !n.pacer.refresh(time.Now(), shown) {
						return
					}
					// Ebitengine returns once a frame has taken the refresh's
					// drawable and presented.
					if delegate := objc.ID(n.delegate.Load()); delegate != 0 {
						delegate.Send(needsUpdate, link, update)
					}
					n.pacer.returned()
				},
			}},
		)
		if err != nil {
			return
		}
		// The link holds its delegate weakly; this one lives as long as the
		// process does.
		pacer := objc.ID(class).Send(objc.RegisterName("new"))
		setDelegate := objc.RegisterName("setDelegate:")
		method := classGetInstanceMethod(link, setDelegate)
		if pacer == 0 || method == 0 {
			return
		}
		original := methodGetImplementation(method)
		if original == 0 {
			return
		}
		methodSetImplementation(method, objc.NewIMP(func(link objc.ID, cmd objc.SEL, delegate objc.ID) {
			n.delegate.Store(uintptr(delegate))
			if delegate != 0 {
				delegate = pacer
			}
			purego.SyscallN(uintptr(original), uintptr(link), uintptr(cmd), uintptr(delegate))
		}))
		n.installed = true
	})
	if !n.installed {
		return nil
	}
	return &n.pacer
}
