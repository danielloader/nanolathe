package hud

import "testing"

// Auto steps at 1440 and 2160 rows; a fixed choice is reduced until the rail
// keeps 480 virtual rows (DESIGN_INTERFACE_HUD_INPUT "Modern sidebar scale").
func TestChromeScale(t *testing.T) {
	cases := []struct {
		pref    int
		screenH int32
		want    int32
	}{
		{0, 480, 1}, {0, 1080, 1}, {0, 1439, 1}, {0, 1440, 2}, {0, 2159, 2}, {0, 2160, 3}, {0, 4320, 3},
		{1, 2160, 1},
		{2, 959, 1}, {2, 960, 2},
		{3, 1350, 2}, {3, 1439, 2}, {3, 1440, 3},
		{4, 2160, 3},
	}
	for _, c := range cases {
		if got := ChromeScale(c.pref, c.screenH, 3); got != c.want {
			t.Errorf("ChromeScale(%d, %d) = %d, want %d", c.pref, c.screenH, got, c.want)
		}
	}
}
