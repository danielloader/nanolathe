package modfetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Preview admission is a host presentation budget, independent of map
// installation and gameplay (docs/DESIGN_MODS_MUTATORS.md §5.6).
const (
	maxPreviewBytes     = 1 << 20
	maxPreviewDimension = 1024
)

// A cancelled worker can still be finishing its response body when the
// player selects the same preview again. Coordinate only that cache path,
// across clients, so the resumed transfer never shares its part with the old
// writer. Entries disappear as soon as their owner finishes.
var previewTransfers = struct {
	sync.Mutex
	active map[string]chan struct{}
}{active: make(map[string]chan struct{})}

func claimPreview(ctx context.Context, name string) (func(), error) {
	key, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		previewTransfers.Lock()
		done := previewTransfers.active[key]
		if done == nil {
			done = make(chan struct{})
			previewTransfers.active[key] = done
			previewTransfers.Unlock()
			return func() {
				previewTransfers.Lock()
				delete(previewTransfers.active, key)
				close(done)
				previewTransfers.Unlock()
			}, nil
		}
		previewTransfers.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

func validatePreview(preview Archive, base *url.URL, allowed origin) (Archive, error) {
	fail := func(what string, cause error) (Archive, error) {
		return Archive{}, &diagError{what: what, logical: preview.URL, providers: []string{allowed.String()}, expected: "a trusted PNG preview of at most 1 MiB with a SHA-256", wrapped: cause}
	}
	target, err := base.Parse(preview.URL)
	if err != nil || preview.URL == "" {
		return fail("map preview URL is unreadable", ErrOriginRefused)
	}
	if !archiveAllowed(target, allowed) {
		return fail("map preview URL is on another origin", ErrOriginRefused)
	}
	if preview.Size <= 0 || preview.Size > maxPreviewBytes || !sha256Pattern.MatchString(preview.SHA256) {
		return fail("map preview has an invalid size or SHA-256", nil)
	}
	preview.URL = target.String()
	preview.SHA256 = strings.ToLower(preview.SHA256)
	return preview, nil
}

// FetchPreview returns a map's optional PNG preview, or nil when none is
// authored. It verifies cached bytes before decoding and makes no request
// for a valid cache hit. Downloads share the archive trust and resumable
// transfer path, but live separately from installable map archives (§5.6).
// Errors concern only this optional presentation asset, never map installation.
func (c *Client) FetchPreview(ctx context.Context, entry Entry) (image.Image, error) {
	if entry.Preview == nil {
		return nil, nil
	}
	base, allowed, err := catalogOrigin(c.catalog())
	if err != nil {
		return nil, err
	}
	preview, err := validatePreview(*entry.Preview, base, allowed)
	if err != nil {
		return nil, err
	}
	fail := func(what string, cause error) error {
		return &diagError{what: what, logical: preview.URL, providers: []string{allowed.String()}, expected: "a verified PNG preview with positive dimensions at most 1024 by 1024", wrapped: cause}
	}
	if err := ctx.Err(); err != nil {
		return nil, fail("map preview cancelled", err)
	}
	dir := filepath.Join(c.CacheDir, ".previews")
	if c.CacheDir == "" {
		dir, err = os.MkdirTemp("", "nanolathe-previews-*")
		if err != nil {
			return nil, fail("creating the preview download directory failed", err)
		}
		defer os.RemoveAll(dir)
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fail("creating the preview cache failed", err)
	}
	dst := filepath.Join(dir, preview.SHA256+".png")
	release, err := claimPreview(ctx, dst)
	if err != nil {
		return nil, fail("waiting for the preview cache failed", err)
	}
	defer release()
	data, err := readPreview(dst, preview)
	if err != nil {
		// An absent or damaged cache is replaced only after Download verifies
		// its new bytes. Its hash-bound part remains resumable on cancellation.
		download := Entry{Archive: preview}
		if err := c.Download(ctx, download, dst, nil); err != nil {
			return nil, err
		}
		data, err = readPreview(dst, preview)
		if err != nil {
			return nil, fail("reading the verified preview failed", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fail("map preview cancelled", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fail("reading the preview PNG dimensions failed", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxPreviewDimension || config.Height > maxPreviewDimension {
		return nil, fail(fmt.Sprintf("map preview dimensions are %d by %d", config.Width, config.Height), nil)
	}
	previewImage, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fail("decoding the preview PNG failed", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fail("map preview cancelled", err)
	}
	return previewImage, nil
}

// Hash and decode the same bounded bytes, including on an offline cache hit.
func readPreview(name string, preview Archive) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != preview.Size {
		return nil, fmt.Errorf("preview cache has an unexpected file size or type")
	}
	data, err := io.ReadAll(io.LimitReader(file, preview.Size+1))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != preview.Size || hex.EncodeToString(sum[:]) != preview.SHA256 {
		return nil, fmt.Errorf("preview cache does not match its size and SHA-256")
	}
	return data, nil
}
