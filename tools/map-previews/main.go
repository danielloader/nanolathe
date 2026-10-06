// map-previews exports the authored minimaps from verified catalogue ZIPs.
// It writes small PNGs and their optional catalogue references; map packages
// themselves remain byte-identical. See DESIGN_MODS_MUTATORS §5.6.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

type asset struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func main() {
	manifest := flag.String("manifest", "", "map manifest to update")
	archives := flag.String("archives", "", "directory containing verified catalogue ZIPs")
	root := flag.String("root", "", "reference install supplying the authored palette")
	out := flag.String("out", "", "PNG output directory")
	baseURL := flag.String("url", "https://nanolathe.gg/maps/previews/", "published PNG directory URL")
	flag.Parse()
	if err := run(*manifest, *archives, *root, *out, *baseURL); err != nil {
		fmt.Fprintln(os.Stderr, "map-previews:", err)
		os.Exit(1)
	}
}

func run(manifest, archives, root, out, baseURL string) error {
	if manifest == "" || archives == "" || root == "" || out == "" {
		return fmt.Errorf("--manifest, --archives, --root and --out are required")
	}
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || !strings.HasSuffix(base.Path, "/") {
		return fmt.Errorf("--url must be an HTTPS directory")
	}
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var document map[string]json.RawMessage
	if err = json.Unmarshal(raw, &document); err != nil {
		return err
	}
	var maps []map[string]json.RawMessage
	if err = json.Unmarshal(document["maps"], &maps); err != nil {
		return err
	}
	fs := vfs.New()
	defer fs.Close()
	if err = fs.MountGameDirectory(root); err != nil {
		return err
	}
	tables, err := palette.Load(fs)
	if err != nil {
		return err
	}
	colors := make(color.Palette, 256)
	for i, rgb := range tables.Base {
		colors[i] = color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		return err
	}
	var total int64
	for _, entry := range maps {
		var archive asset
		var logical string
		if err = json.Unmarshal(entry["archive"], &archive); err != nil {
			return err
		}
		if err = json.Unmarshal(entry["map"], &logical); err != nil {
			return err
		}
		u, err := url.Parse(archive.URL)
		if err != nil {
			return err
		}
		filename := path.Base(u.Path)
		payload, err := os.ReadFile(filepath.Join(archives, filename))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(payload)
		if int64(len(payload)) != archive.Size || hex.EncodeToString(digest[:]) != archive.SHA256 {
			return fmt.Errorf("%s: archive identity differs", filename)
		}
		z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
		if err != nil {
			return err
		}
		terrainPath := strings.TrimSuffix(logical, path.Ext(logical)) + ".tnt"
		file, err := z.Open(terrainPath)
		if err != nil {
			return err
		}
		terrainBytes, readErr := io.ReadAll(io.LimitReader(file, (64<<20)+1))
		file.Close()
		if readErr != nil {
			return readErr
		}
		if len(terrainBytes) > 64<<20 {
			return fmt.Errorf("%s: terrain exceeds map library budget", filename)
		}
		terrain, err := formats.LoadTNT(terrainBytes)
		if err != nil {
			return err
		}
		picture, err := minimap(terrain, colors)
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		var encoded bytes.Buffer
		if err = png.Encode(&encoded, picture); err != nil {
			return err
		}
		if encoded.Len() > 1<<20 {
			return fmt.Errorf("%s: PNG exceeds preview download budget", filename)
		}
		pngName := strings.TrimSuffix(filename, path.Ext(filename)) + ".png"
		if err = os.WriteFile(filepath.Join(out, pngName), encoded.Bytes(), 0644); err != nil {
			return err
		}
		digest = sha256.Sum256(encoded.Bytes())
		preview := asset{URL: base.ResolveReference(&url.URL{Path: pngName}).String(), Size: int64(encoded.Len()), SHA256: hex.EncodeToString(digest[:])}
		entry["preview"], err = json.Marshal(preview)
		if err != nil {
			return err
		}
		total += preview.Size
		fmt.Printf("%s: %dx%d, %d bytes\n", pngName, picture.Rect.Dx(), picture.Rect.Dy(), preview.Size)
	}
	document["maps"], err = json.Marshal(maps)
	if err != nil {
		return err
	}
	raw, err = json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(manifest, append(raw, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("%d previews, %d bytes total\n", len(maps), total)
	return nil
}

func minimap(t *formats.TNT, colors color.Palette) (*image.Paletted, error) {
	w, h := int64(t.MinimapWidth), int64(t.MinimapHeight)
	ew, eh := int64(t.Width)*16-32, int64(t.Height)*16-128
	if !t.MiniMapPresent || w <= 0 || h <= 0 || w > 1024 || h > 1024 || ew <= 0 || eh <= 0 {
		return nil, fmt.Errorf("missing or oversized authored minimap")
	}
	// Keep the source crop used by the front-end chooser [07 R-FE-01 §5].
	// This export omits padding and does not resample or invent terrain art.
	cw, ch := w, h
	if ew < eh {
		cw = ew * w / eh
	} else {
		ch = eh * h / ew
	}
	if cw <= 0 || ch <= 0 {
		return nil, fmt.Errorf("empty minimap crop")
	}
	picture := image.NewPaletted(image.Rect(0, 0, int(cw), int(ch)), colors)
	for y := int64(0); y < ch; y++ {
		copy(picture.Pix[y*cw:(y+1)*cw], t.Minimap[y*w:y*w+cw])
	}
	return picture, nil
}
