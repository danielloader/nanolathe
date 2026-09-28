//go:build !darwin || ebitenginevmguest

package ebitenapp

// Other hosts present through their own swap chains, which this package has
// not measured; they keep the window's own cap (presentDue).
func startNativeFramePacer() *framePacer { return nil }
