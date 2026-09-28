package ebitenapp

// scanoutSettle is how many host steps in a row must ask for the other route
// before the window is given it, about an eighth of a second. A fullscreen
// transition and a display measured again can each ask for a route for a
// step or two, and changing the window's route is not free.
const scanoutSettle = 4

// scanoutRoute is the route a window has, direct or composited, and how long
// the other has been asked for (nativeScanout).
type scanoutRoute struct {
	composite bool
	// asked counts the host steps in a row that asked for the other route.
	asked int
}

// settle takes one host step's request and reports whether the window is to
// be given the other route now. The caller records the route the window took.
func (r *scanoutRoute) settle(composite bool) bool {
	if composite == r.composite {
		r.asked = 0
		return false
	}
	r.asked++
	return r.asked >= scanoutSettle
}

// took records that the window was given the route.
func (r *scanoutRoute) took(composite bool) {
	r.composite, r.asked = composite, 0
}
