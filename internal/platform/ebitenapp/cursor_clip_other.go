//go:build !windows || ebitenginevmguest

package ebitenapp

// Other hosts leave the pointer unconfined; see presentedCursorRect.
// TODO(question): macOS live traces show the pointer reaching the letterbox
// bars and, above the content of a display with a camera housing, leaving the
// window; the software cursor is out of sight there. Neither macOS nor X11 has
// a ClipCursor equivalent behind Ebitengine, so confining it would mean
// driving the pointer natively. Settle whether that is wanted with a manual
// fullscreen check on each host; X11 has not been checked at all.
type nativeCursorClip struct{}

func (*nativeCursorClip) update(fullscreen, focused, captured bool, logicalW, logicalH int) {}
func (*nativeCursorClip) release()                                                          {}
