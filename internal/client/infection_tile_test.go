package client

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// The shipped tile is read back exactly, never resampled: it is the original
// overlay's reduction, proved composite-identical to the full-size source
// before that source left the tree (DESIGN_GPU_RENDERER §38).
func TestInfectionTileIsReadWithoutResampling(t *testing.T) {
	data, err := os.ReadFile("../../cmd/nanolathe/art/infection-overlay-tile.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tile, err := infectionTileOf(img)
	if err != nil {
		t.Fatal(err)
	}
	covered := 0
	for i, c := range tile {
		if c != img.(*image.NRGBA).NRGBAAt(i%InfectionTileSize, i/InfectionTileSize) {
			t.Fatalf("tile pixel %d resampled", i)
		}
		if c.A != 0 {
			covered++
		}
	}
	if covered == 0 || covered == len(tile) {
		t.Fatalf("tile coverage %d of %d: expected sparse art", covered, len(tile))
	}
	// Any other size is art to reduce, not a tile: a 2×2 checker of opaque
	// and clear quadrants reduces to the same pattern at tile scale.
	art := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	art.SetNRGBA(0, 0, color.NRGBA{R: 200, A: 255})
	art.SetNRGBA(1, 1, color.NRGBA{R: 200, A: 255})
	reduced, err := infectionTileOf(art)
	if err != nil {
		t.Fatal(err)
	}
	if reduced[0].A != 255 || reduced[InfectionTileSize-1].A != 0 || reduced[len(reduced)-1].A != 255 {
		t.Fatal("non-tile art was not box-filtered")
	}
}
