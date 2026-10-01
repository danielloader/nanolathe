package main

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
)

// The settings demonstration locks the same host policy as the battle:
// source pages are retained by a lock, including short pages; free flow can
// cross them, but only after reserving the commands (HUD design §3.3/§3.17).
func TestNLSidebarPreviewPagePolicies(t *testing.T) {
	b, _, sources := sidebarRowsFixture(t, 1080)
	sources[1].Gadgets[9].Active = 0
	def, _ := b.cat.Unit("armfav")
	f, _ := b.currentSnapshot()
	c := b.hud.sidebarProductCatalog(b.cat, def, int(f.CommandPage.PageCount))
	if c == nil || !c.safe || len(c.cells) == 0 {
		t.Fatal("no resolved fixture pages")
	}
	original, _ := nlSidebarPageStarts(c, 0, 1080, 12)
	locked, _ := nlSidebarPageStarts(c, 1, 1080, 12)
	if !slices.Equal(locked, original) || len(locked) < 3 {
		t.Fatalf("twelve-cell limit changed authored pages: %v vs %v", locked, original)
	}
	flow, capacity := nlSidebarPageStarts(c, 2, 1080, 0)
	if capacity <= 6 || len(flow) >= len(original) {
		t.Fatalf("tall free flow did not combine pages: %v, capacity %d", flow, capacity)
	}
	short, shortCapacity := nlSidebarPageStarts(c, 2, 480, 0)
	if shortCapacity >= capacity || len(short) <= len(flow) {
		t.Fatalf("short free flow ignored reserved commands: %v, capacity %d", short, shortCapacity)
	}
	fallback, _ := nlSidebarPageStarts(c, 1, 480, 12)
	if !slices.Equal(fallback, original) {
		t.Fatalf("oversized limit should retain source pages: %v", fallback)
	}
	modLocked, _ := nlSidebarPageStarts(c, 2, 1080, 6)
	if !slices.Equal(modLocked, original) {
		t.Fatalf("free-flow preview ignored the mod lock: %v", modLocked)
	}
	sources[0].Rect.H = 640
	for _, mode := range []int{0, 1} {
		if pages, _ := nlSidebarPageStarts(c, mode, 480, 12); len(pages) != 0 {
			t.Fatalf("fitted authored pages must not claim normalized page counts: %v", pages)
		}
	}
	if pages, _ := nlSidebarPageStarts(c, 1, 1080, 12); !slices.Equal(pages, locked) {
		t.Fatalf("fitting lock suppressed a valid normalized preview: %v", pages)
	}
	// Preserve GUI names, duplicate entries and download cells even if the
	// catalog's membership table disagrees with the actual product controls.
	b.cat.BuildMenus = map[string]*content.BuildMenuPage{"armfav": {Buttons: []string{"wrong"}}}
	names := (&nlPicNames{sidebar: c}).order(nil)
	if slices.Contains(names, "wrong") || !slices.Contains(names, "product0") || !slices.Contains(names, "product17") {
		t.Fatalf("preview pictures came from membership: %v", names)
	}
}

func TestNLSidebarLoaderStopsBetweenAssetReads(t *testing.T) {
	cat := nlRenamedCatalog()
	builder, _ := cat.Unit("corehumanbuilder")
	builder.BuildPageCount = 2
	fs := &nlPicFS{t: t, files: map[string][]byte{}}
	fs.hold.Lock()
	var halted atomic.Bool
	done := make(chan *sidebarProductCatalog, 1)
	go func() { done <- loadNLSidebar(fs, cat, builder, cat.Sides[0], halted.Load) }()
	nlWaitFor(t, "the first sidebar asset read", func() bool { return fs.readCount() == 1 })
	halted.Store(true)
	fs.hold.Unlock()
	if c := <-done; c != nil {
		t.Fatal("a cancelled source resolution published a catalog")
	}
	if reads := fs.readCount(); reads != 1 {
		t.Fatalf("cancellation opened more assets: %d reads", reads)
	}
}
