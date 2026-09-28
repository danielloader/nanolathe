//go:build !darwin || ebitenginevmguest

package ebitenapp

// Other hosts choose a fullscreen window's route themselves.
type nativeScanout struct{}

func (*nativeScanout) update(composite bool) {}
func (*nativeScanout) composited() bool      { return false }
