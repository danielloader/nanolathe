package ebitenapp

import "sync/atomic"

// pointerCrossings counts the native pointer leaving and entering the window's
// content, for the live trace. Only hosts whose window system reports the
// crossings to the adapter count them (macOS); elsewhere both stay zero.
type pointerCrossings struct{ exits, enters atomic.Int64 }

var nativePointerCrossings pointerCrossings

// take returns the crossings since the previous call.
func (c *pointerCrossings) take() (exits, enters int64) {
	return c.exits.Swap(0), c.enters.Swap(0)
}
