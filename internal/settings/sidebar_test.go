package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// The retired Original/Off preference migrates to six items without inline
// Orders-page controls; omission still inherits the ordinary defaults.
func TestSidebarLegacyOriginalMigratesToSixWithoutOrders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, tc := range []struct {
		text         string
		page, orders int
	}{
		{`{"version":1,"presentation":{}}`, -1, 1},
		{`{"version":1,"presentation":{"expandedSidebar":0}}`, 6, 0},
		{`{"version":1,"presentation":{"expandedSidebar":0,"buildMenuPageSize":12,"sidebarOrders":1}}`, 6, 0},
	} {
		if err := os.WriteFile(path, []byte(tc.text), 0o600); err != nil {
			t.Fatal(err)
		}
		for pass := 0; pass < 2; pass++ {
			s, err := LoadFrom(path)
			if err != nil || s.Presentation.ExpandedSidebar != 1 || s.Presentation.BuildMenuPageSize != tc.page || s.Presentation.SidebarOrders != tc.orders {
				t.Fatalf("sidebar migration: %+v, %v", s.Presentation, err)
			}
			if err := s.SaveTo(path); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// Omission inherits content defaults; explicit Free flow and hidden orders
// must survive both restart and the ordinary write-all settings transaction.
func TestSidebarPageSizeAndOrdersPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, tc := range []struct {
		text         string
		page, orders int
	}{
		{`{"version":1,"presentation":{}}`, -1, 1},
		{`{"version":1,"presentation":{"buildMenuPageSize":0,"sidebarOrders":0}}`, 0, 0},
		{`{"version":1,"presentation":{"buildMenuPageSize":6,"sidebarOrders":1}}`, 6, 1},
		{`{"version":1,"presentation":{"buildMenuPageSize":12}}`, 12, 1},
	} {
		if err := os.WriteFile(path, []byte(tc.text), 0o600); err != nil {
			t.Fatal(err)
		}
		for pass := 0; pass < 2; pass++ {
			s, err := LoadFrom(path)
			if err != nil {
				t.Fatal(err)
			}
			if s.Presentation.BuildMenuPageSize != tc.page || s.Presentation.SidebarOrders != tc.orders {
				t.Fatalf("sidebar page/orders = %d/%d, want %d/%d", s.Presentation.BuildMenuPageSize, s.Presentation.SidebarOrders, tc.page, tc.orders)
			}
			if err := s.SaveTo(path); err != nil {
				t.Fatal(err)
			}
		}
	}
}
