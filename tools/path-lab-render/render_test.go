package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The fixtures below are authored here: a small map, picture, snippet,
// frames and scores, written to a temporary directory.

func tinyGrid() *gridFile {
	return &gridFile{CellW: 4, CellH: 3, ImageW: 64, ImageH: 48,
		Heights: []byte{
			0, 10, 20, 30,
			40, 50, 60, 70,
			80, 90, 100, 110,
		},
		Blocking: make([]byte, 12)}
}

// The terrain picture is screen space: ground of height h at world (x, z)
// is drawn at (x, z - h/2), h being the height of the cell holding the point.
func TestProjectionLiftsGroundByHalfItsHeight(t *testing.T) {
	g := tinyGrid()
	for _, c := range []struct{ x, z, px, py float64 }{
		{8, 8, 8, 8},              // cell (0,0), height 0
		{17, 20, 17, 20 - 25},     // cell (1,1), height 50
		{16, 15.9, 16, 15.9 - 5},  // x on a cell boundary belongs to the cell to its east
		{63.9, 47.9, 63.9, -7.1},  // cell (3,2), height 110
		{-5, 100, -5, 100 - 40},   // off the map: the nearest cell, (0,2), height 80
		{47.5, 31.99, 47.5, 1.99}, // cell (2,1), height 60
	} {
		px, py := g.project(c.x, c.z)
		if math.Abs(px-c.px) > 1e-9 || math.Abs(py-c.py) > 1e-9 {
			t.Errorf("project(%v, %v) = (%v, %v), want (%v, %v)", c.x, c.z, px, py, c.px, c.py)
		}
	}
	// The crop of one cell of height 50 runs from its top edge lifted by 25
	// to its bottom edge lifted by 25, clipped to the picture.
	if got, want := g.pictureCrop([4]float64{16, 16, 32, 32}, 0), image.Rect(16, 0, 32, 7); got != want {
		t.Errorf("pictureCrop = %v, want %v", got, want)
	}
	sc := &scene{grid: g, crop: image.Rect(0, -10, 64, 48), scale: 2}
	if p := sc.toPanel(17, 20); p != (pt{34, 10}) {
		t.Errorf("toPanel(17, 20) = %v, want {34 10}", p)
	}
}

func TestInterpolationBetweenSamples(t *testing.T) {
	l := newLog("x", "", []frame{
		{Tick: 100, Units: [][6]int32{{1, 0, 0, 65000, 0, 1}, {2, 50, 50, 0, 1, 1}}},
		{Tick: 105, Units: [][6]int32{{1, 10, 20, 500, 1, 1}, {2, 50, 50, 0, 1, 1}}},
		{Tick: 110, Units: [][6]int32{{2, 50, 50, 0, 0, 0}}},
		{Tick: 115, Units: [][6]int32{{1, 40, 40, 1000, 0, 1}, {2, 60, 50, 0, 0, 1}, {3, 5, 5, 0, 0, 0}}},
	})
	if l.step != 5 {
		t.Fatalf("step = %d, want 5", l.step)
	}
	type want struct {
		ok       bool
		x, z     float64
		heading  uint16
		blockedW bool
	}
	for _, c := range []struct {
		id int
		t  float64
		w  want
	}{
		// Halfway, and the heading turns the short way across north: 65000
		// to 500 is 1036 units, so halfway is 65518.
		{1, 102.5, want{true, 5, 10, 65518, false}},
		{1, 105, want{true, 10, 20, 500, true}},
		// Unit 1 is missing at 110: it is held one interval, then gone.
		{1, 107, want{true, 10, 20, 500, true}},
		{1, 111, want{ok: false}},
		// At and after the log's last frame the unit stays put.
		{1, 115, want{true, 40, 40, 1000, false}},
		{1, 130, want{true, 40, 40, 1000, false}},
		// Before the log's first frame a unit it shows stands there...
		{2, 90, want{true, 50, 50, 0, true}},
		// ...but one that appears later does not exist yet.
		{3, 112, want{ok: false}},
		{2, 112.5, want{true, 55, 50, 0, false}},
	} {
		s, ok := l.at(c.id, c.t)
		got := want{ok, s.x, s.z, s.heading, s.blocked}
		if !ok {
			got = want{}
		}
		if got != c.w {
			t.Errorf("at(%d, %v) = %+v, want %+v", c.id, c.t, got, c.w)
		}
	}
	runs := l.runs(1)
	if len(runs) != 2 || len(runs[0]) != 2 || len(runs[1]) != 1 || runs[1][0].tick != 115 {
		t.Errorf("runs(1) splits at the gap into [100 105] [115]; got %v", runs)
	}
}

func TestSubjectColoursFollowTheUnitNotTheOrder(t *testing.T) {
	a := assignColours([]int{19, 4, 250, 7})
	b := assignColours([]int{250, 7, 19, 4})
	if !reflect.DeepEqual(a, b) {
		t.Errorf("colours depend on the order units are listed in: %v vs %v", a, b)
	}
	if a[4] != subjectPalette[0] || a[250] != subjectPalette[3] {
		t.Errorf("colours go by rank of unit ID: 4 -> %v, 250 -> %v", a[4], a[250])
	}
	// Every palette entry is its own colour, and none is the warning red or
	// the grey of other units.
	seen := map[rgba]bool{}
	for _, c := range subjectPalette {
		if seen[c] || c == warn || c == otherUnit {
			t.Errorf("palette colour %s repeats or collides with warn/other", c.hex())
		}
		seen[c] = true
	}
	s1 := &snippet{Units: []snipUnit{{ID: 9, Subject: true}, {ID: 2, Subject: true}, {ID: 5}, {ID: 30, Subject: true, Structure: true}}}
	s2 := &snippet{Units: []snipUnit{{ID: 5}, {ID: 2, Subject: true}, {ID: 9, Subject: true}}}
	if got := subjectIDs(s1); !reflect.DeepEqual(got, []int{2, 9}) || !reflect.DeepEqual(got, subjectIDs(s2)) {
		t.Errorf("subjectIDs = %v and %v, want [2 9] for both", got, subjectIDs(s2))
	}
}

// A reclaim recorded at any cell of a feature removes the whole feature in
// the replay, so the pictures leave all of its cells out.
func TestReclaimedFeaturesAreNotDrawn(t *testing.T) {
	const w, h = 12, 8
	g := &gridFile{CellW: w, CellH: h, ImageW: w * cellSize, ImageH: h * cellSize,
		Heights: make([]byte, w*h), Blocking: make([]byte, w*h)}
	for _, a := range [][2]int{{1, 1}, {6, 2}} { // two 3x2 blocking features
		for dz := 0; dz < 2; dz++ {
			for dx := 0; dx < 3; dx++ {
				g.Blocking[(a[1]+dz)*w+a[0]+dx] = 2
			}
		}
		g.Blocking[a[1]*w+a[0]] = 1
	}
	g.Blocking[6*w+10] = 1 // a lone blocking cell
	sc := &scene{grid: g, crop: image.Rect(0, 0, g.ImageW, g.ImageH),
		snip: &snippet{Reclaimed: [][2]int32{{8, 3}}}} // a fringe cell of the second feature
	got := sc.blockers()
	want := []image.Point{{1, 1}, {2, 1}, {3, 1}, {1, 2}, {2, 2}, {3, 2}, {10, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("blockers = %v, want %v", got, want)
	}
}

func TestPNGCropMatchesFullDecode(t *testing.T) {
	// An opaque RGBA picture is stored as RGB with every row filtered, which
	// the reader must undo; a paletted one of more than sixteen colours is
	// stored eight bits a pixel, unfiltered, as the map exporter writes it.
	rgbaPic := image.NewRGBA(image.Rect(0, 0, 37, 29))
	pal := make(color.Palette, 20)
	for i := range pal {
		pal[i] = color.RGBA{uint8(i * 12), uint8(255 - i*9), uint8(i * i), 255}
	}
	palPic := image.NewPaletted(image.Rect(0, 0, 37, 29), pal)
	seed := uint32(7)
	for y := 0; y < 29; y++ {
		for x := 0; x < 37; x++ {
			seed = seed*1664525 + 1013904223
			rgbaPic.Set(x, y, color.RGBA{uint8(x * 7), uint8(y*9) ^ uint8(seed>>24), uint8(x*y) + uint8(seed>>16&3), 255})
			palPic.SetColorIndex(x, y, uint8((x/3+y/2)%20))
		}
	}
	for _, c := range []struct {
		name string
		pic  image.Image
	}{{"rgba", rgbaPic}, {"paletted", palPic}} {
		var buf bytes.Buffer
		if err := png.Encode(&buf, c.pic); err != nil {
			t.Fatal(err)
		}
		rect := image.Rect(5, 3, 30, 27)
		got, err := decodePNGCrop(bufio.NewReader(bytes.NewReader(buf.Bytes())), rect)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Rect != rect {
			t.Fatalf("%s: bounds %v, want %v", c.name, got.Rect, rect)
		}
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			for x := rect.Min.X; x < rect.Max.X; x++ {
				r, g, b, _ := c.pic.At(x, y).RGBA()
				want := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 255}
				if p := got.RGBAAt(x, y); p != want {
					t.Fatalf("%s: pixel (%d,%d) = %v, want %v", c.name, x, y, p, want)
				}
			}
		}
	}
}

func TestBundleShape(t *testing.T) {
	o := writeFixture(t)
	sc, err := loadScene(&o)
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildBundle(sc, 70)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	jsonPath, jsPath := filepath.Join(dir, "b.json"), filepath.Join(dir, "b.js")
	if err := writeBundleJSON(jsonPath, b); err != nil {
		t.Fatal(err)
	}
	if err := writeBundleJS(jsPath, b); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(jsonPath)
	js, _ := os.ReadFile(jsPath)
	const head = "window.PATHLAB_BUNDLES = window.PATHLAB_BUNDLES || [];\nwindow.PATHLAB_BUNDLES.push("
	if !bytes.HasPrefix(js, []byte(head)) || !bytes.HasSuffix(js, []byte(");\n")) ||
		!bytes.Equal(js[len(head):len(js)-3], bytes.TrimSpace(raw)) {
		t.Errorf("the script form is not the JSON bundle pushed onto window.PATHLAB_BUNDLES")
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "map", "class", "t0", "t1", "scale", "width", "height", "background", "structures", "goals", "units", "logs"} {
		if _, ok := got[k]; !ok {
			t.Errorf("bundle lacks %q", k)
		}
	}
	if b.Width != sc.w || b.Height != sc.h {
		t.Errorf("bundle size %dx%d, panel %dx%d", b.Width, b.Height, sc.w, sc.h)
	}
	const prefix = "data:image/jpeg;base64,"
	if !strings.HasPrefix(b.Background, prefix) {
		t.Fatalf("background is not a JPEG data URL")
	}
	jpg, err := base64.StdEncoding.DecodeString(b.Background[len(prefix):])
	if err != nil {
		t.Fatal(err)
	}
	if cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpg)); err != nil || cfg.Width != b.Width || cfg.Height != b.Height {
		t.Errorf("background %v %dx%d, want %dx%d", err, cfg.Width, cfg.Height, b.Width, b.Height)
	}
	// Subjects lead the unit list, with the pictures' colours.
	if len(b.Units) != 3 || b.Units[0].ID != 3 || b.Units[1].ID != 7 || b.Units[2].Subject || b.Units[0].Color != subjectPalette[0].hex() {
		t.Errorf("units = %+v", b.Units)
	}
	if len(b.Goals) != 2 || b.Goals[0][0] != 0 || b.Goals[1][0] != 1 {
		t.Errorf("goals = %v, want one per subject, indexed in unit order", b.Goals)
	}
	if len(b.Structures) != 1 || len(b.Structures[0]) != 4 {
		t.Errorf("structures = %v, want one [x y w h]", b.Structures)
	}
	if len(b.Logs) != 2 || b.Logs[0].Label != "recorded" || b.Logs[1].Label != "replay" {
		t.Fatalf("logs are not in the order given")
	}
	samples := 0
	for _, l := range b.Logs {
		if len(l.Frames) != len(l.Ticks) {
			t.Errorf("%s: %d frames for %d ticks", l.Label, len(l.Frames), len(l.Ticks))
		}
		for _, f := range l.Frames {
			if len(f)%5 != 0 {
				t.Fatalf("%s: a frame is not whole [unit, x, y, heading, flags] groups", l.Label)
			}
			for i := 0; i < len(f); i += 5 {
				if f[i] < 0 || f[i] >= len(b.Units) || f[i+3] < 0 || f[i+3] > 255 || f[i+4] < 0 || f[i+4] > 3 {
					t.Fatalf("%s: bad entry %v", l.Label, f[i:i+5])
				}
			}
			samples += len(f) / 5
		}
		if _, ok := l.Score["subjects"]; ok {
			t.Errorf("%s: per-subject scores belong in the score file, not the bundle", l.Label)
		}
	}
	if b.Logs[0].Score == nil || b.Logs[1].Score == nil {
		t.Errorf("scores were not matched to their logs")
	}
	// The frames stay compact: a sample costs a few bytes of integers.
	var frames bytes.Buffer
	for _, l := range b.Logs {
		json.NewEncoder(&frames).Encode(l.Frames)
	}
	if per := frames.Len() / samples; per > 24 {
		t.Errorf("frames cost %d bytes a sample, want at most 24", per)
	}
}

func TestPicturesAreDeterministic(t *testing.T) {
	o := writeFixture(t)
	render := func() ([]byte, []byte) {
		sc, err := loadScene(&o)
		if err != nil {
			t.Fatal(err)
		}
		ticks, err := sheetTicks("", 3, sc.snip.T0, sc.snip.T1)
		if err != nil {
			t.Fatal(err)
		}
		return renderTracks(sc).Pix, renderSheet(sc, ticks).Pix
	}
	t1, s1 := render()
	t2, s2 := render()
	if !bytes.Equal(t1, t2) || !bytes.Equal(s1, s2) {
		t.Errorf("the same inputs drew different pictures")
	}
}

// writeFixture writes a small map, snippet, two logs, a unit table and a
// score file, and returns the options that load them.
func writeFixture(t *testing.T) options {
	t.Helper()
	dir := t.TempDir()
	mapDir := filepath.Join(dir, "map")
	if err := os.Mkdir(mapDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const cw, ch = 16, 12
	g := gridFile{Map: "fixture", CellW: cw, CellH: ch, ImageW: cw * cellSize, ImageH: ch * cellSize,
		Heights: make([]byte, cw*ch), Blocking: make([]byte, cw*ch)}
	for z := 0; z < ch; z++ {
		for x := 0; x < cw; x++ {
			g.Heights[z*cw+x] = byte(4 * (x + z))
		}
	}
	// A blocking 2x2 feature, and one the recording shows reclaimed.
	for _, a := range [][2]int{{4, 4}, {10, 7}} {
		g.Blocking[a[1]*cw+a[0]] = 1
		g.Blocking[a[1]*cw+a[0]+1] = 2
		g.Blocking[(a[1]+1)*cw+a[0]] = 2
		g.Blocking[(a[1]+1)*cw+a[0]+1] = 2
	}
	writeGz(t, filepath.Join(mapDir, "grid.json.gz"), g)
	pal := color.Palette{color.RGBA{180, 140, 100, 255}, color.RGBA{150, 120, 90, 255}, color.RGBA{90, 110, 130, 255}}
	pic := image.NewPaletted(image.Rect(0, 0, g.ImageW, g.ImageH), pal)
	for y := 0; y < g.ImageH; y++ {
		for x := 0; x < g.ImageW; x++ {
			pic.SetColorIndex(x, y, uint8((x/8+y/8)%3))
		}
	}
	f, err := os.Create(filepath.Join(mapDir, "terrain.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, pic); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s := snippet{Schema: 1, ID: "fixture-1", Map: "Fixture", Class: "group", T0: 1000, T1: 1100,
		Region: [4]float64{32, 32, 200, 160},
		Units: []snipUnit{
			{ID: 7, Name: "TANK", X: 40, Z: 60, Spawn: 1000, Die: -1, Subject: true},
			{ID: 3, Name: "TANK", X: 40, Z: 100, Spawn: 1000, Die: -1, Subject: true},
			{ID: 9, Name: "TANK", X: 180, Z: 60, Spawn: 1000, Die: -1},
			{ID: 50, Name: "TOWER", X: 120, Z: 40, Structure: true, Spawn: 1000, Die: -1},
		},
		Orders: []snipOrder{
			{Tick: 1000, Unit: 7, GoalX: 500, GoalZ: 500, Carry: true},
			{Tick: 1001, Unit: 7, GoalX: 180, GoalZ: 140},
			{Tick: 1001, Unit: 3, GoalX: 160, GoalZ: 150},
		},
		Reclaimed: [][2]int32{{10, 7}},
	}
	writeGz(t, filepath.Join(dir, "snippet.json"), s)
	var logs []logSpec
	for li, label := range []string{"recorded", "replay"} {
		var fr []frame
		for k := 0; k <= 20; k++ {
			tick := int32(1000 + 5*k)
			d := int32(k * (6 + li))
			blocked := int32(0)
			if k >= 8 && k <= 10 {
				blocked = 1
			}
			fr = append(fr, frame{Tick: tick, Units: [][6]int32{
				{7, 40 + d, 60 + d/2, 49152, blocked, 1},
				{3, 40 + d, 100 + d/3, 45000, 0, 1},
				{9, 180 - d/2, 60 + d, 32768, 0, 1},
			}})
		}
		path := filepath.Join(dir, "fixture-1__"+label+".frames.json.gz")
		writeGz(t, path, fr)
		logs = append(logs, logSpec{label: label, path: path})
	}
	writeGz(t, filepath.Join(dir, "units.json"), []map[string]any{
		{"unitname": "TANK", "foot_x": 2, "foot_z": 2}, {"unitname": "TOWER", "foot_x": 3, "foot_z": 3}})
	writeGz(t, filepath.Join(dir, "score.json"), map[string]any{"snippet": "fixture-1", "scores": []map[string]any{
		{"log": "recorded", "n": 2, "reached": 1, "mean_near_ticks": 80.5, "blocked_ticks": 15, "overlap_unit_ticks": 0, "subjects": []any{map[string]any{"unit": 3}}},
		{"log": "replay", "rules": "replay", "n": 2, "reached": 2, "mean_near_ticks": 60, "blocked_ticks": 0, "overlap_unit_ticks": 4},
	}})
	return options{snippet: filepath.Join(dir, "snippet.json"), mapDir: mapDir, units: filepath.Join(dir, "units.json"),
		score: filepath.Join(dir, "score.json"), out: filepath.Join(dir, "out.png"), logs: logs, margin: 16}
}

// writeGz writes v as gzip-compressed JSON; the readers accept either form.
func writeGz(t *testing.T, path string, v any) {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(v); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
