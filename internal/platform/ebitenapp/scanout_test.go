package ebitenapp

import "testing"

// A route is changed once it has been asked for at several host steps in a
// row; a request that comes and goes changes nothing.
func TestScanoutRouteSettlesBeforeItChanges(t *testing.T) {
	var r scanoutRoute
	for i := range scanoutSettle - 1 {
		if r.settle(true) {
			t.Fatalf("the route changed at request %d", i+1)
		}
	}
	if r.settle(false) {
		t.Fatalf("a request for the route in force changed it")
	}
	for i := range scanoutSettle - 1 {
		if r.settle(true) {
			t.Fatalf("the route changed at request %d after an interruption", i+1)
		}
	}
	if !r.settle(true) {
		t.Fatalf("the route did not change after %d requests in a row", scanoutSettle)
	}
	// A window that could not be given the route is asked again at once.
	if !r.settle(true) {
		t.Fatalf("a route that was not taken was not asked for again")
	}
	r.took(true)
	if r.settle(true) {
		t.Fatalf("the route in force was changed")
	}
	for i := range scanoutSettle - 1 {
		if r.settle(false) {
			t.Fatalf("the route changed back at request %d", i+1)
		}
	}
	if !r.settle(false) {
		t.Fatalf("the route did not change back")
	}
}
