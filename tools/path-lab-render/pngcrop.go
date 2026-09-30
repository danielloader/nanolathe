package main

import (
	"bufio"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
)

// A map's terrain picture is the whole map at one pixel per world unit —
// 9248x9344 for a large map — and a panel shows a small part of it. The
// reader here inflates the picture's rows in order, keeps only the columns
// of the rows the crop covers, and stops after the crop's last row, so a
// render holds a few megabytes instead of the whole picture. It streams the
// layouts `nanolathe-pathlab map` writes (8-bit, not interlaced); anything
// else falls back to decoding the whole picture with image/png.

var errPNGLayout = errors.New("png layout is not streamed")

// readPNGCrop returns the part of the picture at path inside rect, as an
// opaque RGBA image whose bounds are rect clipped to the picture.
func readPNGCrop(path string, rect image.Rectangle) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("nanolathe: terrain picture is not readable: logical path %s, providers searched [file], expected terrain.png: %w", path, err)
	}
	defer f.Close()
	img, err := decodePNGCrop(bufio.NewReaderSize(f, 1<<20), rect)
	if errors.Is(err, errPNGLayout) {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
		full, err := png.Decode(bufio.NewReader(f))
		if err != nil {
			return nil, fmt.Errorf("nanolathe: terrain picture is not readable: logical path %s, providers searched [file], expected a PNG picture: %w", path, err)
		}
		img = image.NewRGBA(rect.Intersect(full.Bounds()))
		draw.Draw(img, img.Rect, full, img.Rect.Min, draw.Src)
		return img, nil
	}
	if err != nil {
		return nil, fmt.Errorf("nanolathe: terrain picture is not readable: logical path %s, providers searched [file], expected a PNG picture: %w", path, err)
	}
	return img, nil
}

func decodePNGCrop(r *bufio.Reader, want image.Rectangle) (*image.RGBA, error) {
	var sig [8]byte
	if _, err := io.ReadFull(r, sig[:]); err != nil {
		return nil, err
	}
	if string(sig[:]) != "\x89PNG\r\n\x1a\n" {
		return nil, errors.New("not a PNG file")
	}
	var (
		w, h, bpp int
		ctype     byte
		pal       [256][3]uint8
	)
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(r, hdr[:]); err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint32(hdr[:4])
		switch string(hdr[4:8]) {
		case "IHDR":
			b := make([]byte, int(n)+4)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, err
			}
			if n < 13 {
				return nil, errors.New("short IHDR chunk")
			}
			w, h = int(binary.BigEndian.Uint32(b[0:4])), int(binary.BigEndian.Uint32(b[4:8]))
			if b[8] != 8 || b[12] != 0 {
				return nil, errPNGLayout
			}
			ctype = b[9]
			switch ctype {
			case 0, 3:
				bpp = 1
			case 4:
				bpp = 2
			case 2:
				bpp = 3
			case 6:
				bpp = 4
			default:
				return nil, errPNGLayout
			}
		case "PLTE":
			b := make([]byte, int(n)+4)
			if _, err := io.ReadFull(r, b); err != nil {
				return nil, err
			}
			for i := 0; i < int(n)/3 && i < 256; i++ {
				pal[i] = [3]uint8{b[3*i], b[3*i+1], b[3*i+2]}
			}
		case "IDAT":
			if bpp == 0 {
				return nil, errors.New("IDAT before IHDR")
			}
			return decodeRows(&idatReader{r: r, left: n}, w, h, bpp, ctype, &pal, want)
		case "IEND":
			return nil, errors.New("no image data")
		default:
			if _, err := r.Discard(int(n) + 4); err != nil {
				return nil, err
			}
		}
	}
}

func decodeRows(idat io.Reader, w, h, bpp int, ctype byte, pal *[256][3]uint8, want image.Rectangle) (*image.RGBA, error) {
	rect := want.Intersect(image.Rect(0, 0, w, h))
	out := image.NewRGBA(rect)
	if rect.Empty() {
		return out, nil
	}
	zr, err := zlib.NewReader(idat)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	// One filter byte, then the row.
	cur, prev := make([]byte, 1+w*bpp), make([]byte, 1+w*bpp)
	for y := 0; y < rect.Max.Y; y++ {
		if _, err := io.ReadFull(zr, cur); err != nil {
			return nil, fmt.Errorf("row %d: %w", y, err)
		}
		if err := unfilter(cur[0], cur[1:], prev[1:], bpp); err != nil {
			return nil, fmt.Errorf("row %d: %w", y, err)
		}
		if y >= rect.Min.Y {
			row := cur[1:]
			o := out.Pix[(y-rect.Min.Y)*out.Stride:]
			for x := rect.Min.X; x < rect.Max.X; x++ {
				i := (x - rect.Min.X) * 4
				switch ctype {
				case 3:
					c := pal[row[x]]
					o[i], o[i+1], o[i+2] = c[0], c[1], c[2]
				case 0:
					o[i], o[i+1], o[i+2] = row[x], row[x], row[x]
				case 4:
					o[i], o[i+1], o[i+2] = row[2*x], row[2*x], row[2*x]
				default: // 2 and 6: the colour channels lead
					o[i], o[i+1], o[i+2] = row[bpp*x], row[bpp*x+1], row[bpp*x+2]
				}
				o[i+3] = 255
			}
		}
		cur, prev = prev, cur
	}
	return out, nil
}

// unfilter reverses a row's PNG filter in place; prev is the previous row,
// already unfiltered, or zeros for the first row.
func unfilter(ft byte, cur, prev []byte, bpp int) error {
	switch ft {
	case 0:
	case 1:
		for i := bpp; i < len(cur); i++ {
			cur[i] += cur[i-bpp]
		}
	case 2:
		for i := range cur {
			cur[i] += prev[i]
		}
	case 3:
		for i := 0; i < bpp; i++ {
			cur[i] += prev[i] / 2
		}
		for i := bpp; i < len(cur); i++ {
			cur[i] += uint8((int(cur[i-bpp]) + int(prev[i])) / 2)
		}
	case 4:
		for i := range cur {
			var a, c uint8
			if i >= bpp {
				a, c = cur[i-bpp], prev[i-bpp]
			}
			cur[i] += paeth(a, prev[i], c)
		}
	default:
		return fmt.Errorf("unknown filter %d", ft)
	}
	return nil
}

func paeth(a, b, c uint8) uint8 {
	pa, pb, pc := absInt(int(b)-int(c)), absInt(int(a)-int(c)), absInt(int(a)+int(b)-2*int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// idatReader yields the data of consecutive IDAT chunks as one stream.
type idatReader struct {
	r    *bufio.Reader
	left uint32
	done bool
}

func (d *idatReader) Read(p []byte) (int, error) {
	for d.left == 0 {
		if d.done {
			return 0, io.EOF
		}
		// The finished chunk's CRC, then the next chunk's length and type.
		var hdr [12]byte
		if _, err := io.ReadFull(d.r, hdr[:]); err != nil {
			return 0, err
		}
		if string(hdr[8:12]) != "IDAT" {
			d.done = true
			return 0, io.EOF
		}
		d.left = binary.BigEndian.Uint32(hdr[4:8])
	}
	if uint32(len(p)) > d.left {
		p = p[:d.left]
	}
	n, err := d.r.Read(p)
	d.left -= uint32(n)
	return n, err
}
