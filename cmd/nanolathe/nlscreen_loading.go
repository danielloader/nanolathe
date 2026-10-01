package main

import (
	"sync"

	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// nlPreviewContent belongs to one mounted content set. The screen and its
// staging goroutine may request the catalog together; neither can observe an
// incomplete compile. A reload constructs a new content set and cache.
type nlPreviewContent struct {
	catalogOnce sync.Once
	catalog     *content.Catalog
	catalogErr  error
	assetsOnce  sync.Once
	assets      *client.PreviewModelTextureAssets
}

// nlPreviewCatalog is the settings screen's immutable authored catalog seam.
// Scenes and mutator examples take the same content set through this method;
// battle entry remains responsible for its own rule and mutator preparation
// (docs/DESIGN_INTERFACE_HUD_INPUT.md §3.17).
func (c *contentSet) nlPreviewCatalog() (*content.Catalog, error) {
	if c == nil {
		return c.compileCatalog(nil)
	}
	c.preview.catalogOnce.Do(func() {
		c.preview.catalog, c.preview.catalogErr = c.compileCatalog(nil)
	})
	return c.preview.catalog, c.preview.catalogErr
}

func (c *contentSet) nlPreviewModelAssets() *client.PreviewModelTextureAssets {
	c.preview.assetsOnce.Do(func() {
		c.preview.assets = client.NewPreviewModelTextureAssets(c.unmappedMount, c.presentation.TeamLogos)
	})
	return c.preview.assets
}
