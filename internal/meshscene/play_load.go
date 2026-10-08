package meshscene

import (
	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
)

func playOpeningCamera(sess *session.Session) (float32, float32) {
	if f := sess.Snapshot.Current(); f != nil {
		for _, u := range f.Units {
			if def, ok := sess.Catalog.Unit(u.DefName); ok && def.Commander && u.Owner == sess.LocalOwner {
				return float32(u.X) / 65536, (float32(u.Z) - float32(u.Y)/2) / 65536
			}
		}
	}
	return float32(sess.World.CellW * 8), float32(sess.World.CellH * 8)
}

// A playable camera needs the whole map. Large maps retain a nearest-sampled
// backdrop capped at 8192 on either axis; world/placement heights stay exact.
func bakePlayTerrain(t *formats.TNT, pal *palette.Tables) (Texture, [4]float32) {
	fullW, fullH := int(t.TileMapWidth)*32, int(t.TileMapHeight)*32
	scale := max(1, (max(fullW, fullH)+8191)/8192)
	width, height := (fullW+scale-1)/scale, (fullH+scale-1)/scale
	texture := Texture{Width: width, Height: height, RGBA: make([]byte, width*height*4)}
	for z := 0; z < height; z++ {
		for x := 0; x < width; x++ {
			sx, sz := min(x*scale, fullW-1), min(z*scale, fullH-1)
			tile := int(t.TileIndices[(sz/32)*int(t.TileMapWidth)+sx/32])
			index := t.TileGraphics[tile*1024+(sz%32)*32+sx%32]
			rgba := pal.Base[index]
			offset := (z*width + x) * 4
			copy(texture.RGBA[offset:offset+3], rgba[:3])
			texture.RGBA[offset+3] = 255
		}
	}
	return texture, [4]float32{0, 0, float32(fullW), float32(fullH)}
}
