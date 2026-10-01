package main

import "testing"

// Authored synthetic heights lock the approved host policy, including the
// exact six-slot boundary. These are not retail panel measurements.
func TestSidebarBuildCapacityPrecedesOrders(t *testing.T) {
	c := &sidebarProductCatalog{
		upperHeight: 20, lowerHeight: 148, navigationHeight: 20,
		spacing:           []sidebarCommandGap{{at: -1, pixels: 4}},
		navigationSpacing: []sidebarCommandGap{{at: -1, pixels: 4}},
	}
	for _, tc := range []struct {
		name          string
		height, limit int
		orders        bool
		capacity      int
		inline        bool
	}{
		{"short flow hides orders", 480, 0, true, 8, false},
		{"short six hides orders", 480, 6, true, 6, false},
		{"tall flow includes orders", 800, 0, true, 14, true},
		{"tall flow without orders", 800, 0, false, 18, false},
		{"twelve includes orders", 800, 12, true, 12, true},
		{"twelve hides orders before reducing count", 600, 12, true, 12, false},
		{"twelve physically limited", 480, 12, true, 8, false},
		{"fixed count can fall below six if physically constrained", 360, 12, true, 4, false},
		{"Free flow cannot fall below six", 360, 0, true, 0, false},
		{"six-slot orders boundary fits", 489, 0, true, 6, true},
		{"six-slot orders boundary fails", 488, 0, true, 8, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := c.sidebarLayout(tc.height, tc.limit, tc.orders)
			if p.capacity != tc.capacity || p.inlineOrders != tc.inline {
				t.Fatalf("capacity/orders = %d/%v, want %d/%v", p.capacity, p.inlineOrders, tc.capacity, tc.inline)
			}
		})
	}
	c.navigationSpacing[0].pixels = 100
	if p := c.sidebarLayout(600, 12, true); p.capacity != 12 || p.inlineOrders || p.spacing[0].pixels != 48 {
		t.Fatalf("twelve should compress navigation gaps before reducing count: %+v", p)
	}
	if p := c.sidebarLayout(540, 12, true); p.capacity != 10 || p.inlineOrders || p.spacing[0].pixels != 52 {
		t.Fatalf("constrained fixed count should retain every achievable complete row: %+v", p)
	}
	if p := c.sidebarLayout(480, int(^uint(0)>>1), true); p.capacity < 6 || p.inlineOrders {
		t.Fatalf("large legacy count overflowed layout arithmetic: %+v", p)
	}
}
