package pathlab

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	"github.com/nanolathe-gg/nanolathe/internal/movement"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

// MapGrid is a map's static ground as the offline laboratory reads it: one
// height per 16-world-unit cell and, for every movement class, whether a
// unit of that class may stand with its footprint anchored at each cell.
// It is derived from a composed session's terrain, so the map's own
// features are in it and nothing a battle adds is.
type MapGrid struct {
	Map   string `json:"map"`
	CellW int32  `json:"cell_w"`
	CellH int32  `json:"cell_h"`
	Sea   uint8  `json:"sea"`
	// Heights holds one byte per cell, row-major.
	Heights []byte `json:"heights"`
	// Blocking holds 1 for a cell a blocking feature or a void covers.
	Blocking []byte `json:"blocking"`
	// Classes holds, per movement class name, one byte per anchor cell:
	// 0 blocked, 1 steep, 2 clear [04 §6.1].
	Classes map[string][]byte `json:"classes"`
	// Foot is each class's footprint in cells.
	Foot map[string][2]int32 `json:"foot"`
	// ImageW, ImageH is the size of terrain.png in map pixels, which are
	// world units; a unit on ground of height h is drawn h/2 pixels up.
	ImageW int `json:"image_w"`
	ImageH int `json:"image_h"`
}

// ExportMap writes grid.json.gz for a map into dir, and terrain.png too
// when picture is set.
func ExportMap(c *Content, mapName, dir string, picture bool) (*MapGrid, error) {
	header := c.Catalog.Maps[content.CanonicalKey(mapName)]
	if header == nil {
		return nil, fmt.Errorf("nanolathe: map is not installed: logical path maps/%s, providers searched [%s], expected an installed map", mapName, strings.Join(c.Roots, ", "))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	cfg := session.SkirmishConfig{MapName: header.Name, NumPlayers: 2}
	cfg.ApplyDefaults()
	cfg.Players[0] = session.SkirmishPlayer{Controller: session.SkirmishControllerHuman, AllyGroup: 0}
	cfg.Players[1] = session.SkirmishPlayer{Controller: session.SkirmishControllerComputer, Side: 1, Color: 1, AllyGroup: 1}
	cfg.Location = 1
	cfg.Mapping, cfg.LineOfSight = 0, 0
	fb, err := headless.ComposeFreshBattle(headless.FreshBattleRequest{
		Kind: headless.ScenarioDirectOTA, Map: cfg.MapName, LocalOwner: -1, Gameplay: gameplay.Strict31, Difficulty: 1,
		Skirmish: cfg, SimulationSeed: 7, CRTSeed: 7, FS: c.View, Catalog: c.Catalog,
		CommunitySources: session.CommunitySources{Content: c.Profile.GameplaySources()},
	})
	if err != nil {
		return nil, err
	}
	t := fb.Session.World
	g := &MapGrid{Map: header.Name, CellW: t.CellW, CellH: t.CellH, Sea: t.SeaLevel,
		Heights: make([]byte, int(t.CellW)*int(t.CellH)), Blocking: make([]byte, int(t.CellW)*int(t.CellH)),
		Classes: map[string][]byte{}, Foot: map[string][2]int32{}}
	for z := int32(0); z < t.CellH; z++ {
		for x := int32(0); x < t.CellW; x++ {
			cell := t.PlotAt(x, z)
			i := int(z)*int(t.CellW) + int(x)
			g.Heights[i] = cell.Height()
			if cell.IsVoid() {
				g.Blocking[i] = 1
			} else if cell.IsRealFeature() || cell.IsFringe() {
				if def, ok := t.FeatureDefAt(cell.Feature()); ok && def != nil && def.Blocking {
					g.Blocking[i] = 1
				} else if cell.IsFringe() {
					g.Blocking[i] = 2 // part of a larger feature; its anchor decides
				}
			}
		}
	}
	names := make([]string, 0, len(c.Catalog.Movement))
	for k, mc := range c.Catalog.Movement {
		if mc != nil {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	for _, k := range names {
		mc := c.Catalog.Movement[k]
		p := movement.NewProfile(mc)
		name := strings.ToUpper(k)
		layer := make([]byte, int(t.CellW)*int(t.CellH))
		for z := int32(0); z < t.CellH; z++ {
			for x := int32(0); x < t.CellW; x++ {
				if x+mc.FootprintX > t.CellW || z+mc.FootprintZ > t.CellH {
					continue
				}
				layer[int(z)*int(t.CellW)+int(x)] = byte(p.ClassifyFootprint(t, x, z))
			}
		}
		g.Classes[name] = layer
		g.Foot[name] = [2]int32{mc.FootprintX, mc.FootprintZ}
	}
	tnt, err := formats.LoadTNTFile(c.FS, header.LogicalTNT)
	if err != nil {
		return nil, err
	}
	pal, err := palette.Load(c.View)
	if err != nil {
		return nil, err
	}
	w, h := int(tnt.TileMapWidth)*32, int(tnt.TileMapHeight)*32
	g.ImageW, g.ImageH = w, h
	if !picture {
		return g, WriteJSON(filepath.Join(dir, "grid.json.gz"), g)
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), nil)
	img.Palette = make(color.Palette, 256)
	for i := 0; i < 256; i++ {
		r, gg, b, _ := pal.RGBA(byte(i))
		img.Palette[i] = color.RGBA{r, gg, b, 255}
	}
	for ty := 0; ty < int(tnt.TileMapHeight); ty++ {
		for tx := 0; tx < int(tnt.TileMapWidth); tx++ {
			idx := int(tnt.TileIndices[ty*int(tnt.TileMapWidth)+tx])
			if idx >= int(tnt.Tiles) {
				continue
			}
			src := tnt.TileGraphics[idx*1024 : idx*1024+1024]
			for y := 0; y < 32; y++ {
				copy(img.Pix[(ty*32+y)*img.Stride+tx*32:], src[y*32:y*32+32])
			}
		}
	}
	f, err := os.Create(filepath.Join(dir, "terrain.png"))
	if err != nil {
		return nil, err
	}
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(f, img); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	return g, WriteJSON(filepath.Join(dir, "grid.json.gz"), g)
}
