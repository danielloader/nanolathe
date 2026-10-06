package main

// The desktop community map library is content, independent of the selected
// mod and gameplay rules (docs/DESIGN_MODS_MUTATORS.md §5.6).

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/gui"
	"github.com/nanolathe-gg/nanolathe/internal/maplibrary"
	"github.com/nanolathe-gg/nanolathe/internal/modfetch"
	"github.com/nanolathe-gg/nanolathe/internal/modlibrary"
	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// installedMapRoots returns the downloaded packages that pass the mount-time
// audit and those it leaves out; mountContent reports the latter instead of
// failing (docs/DESIGN_CONTENT_VFS.md "Downloaded community maps").
func installedMapRoots(opts Options) ([]string, []maplibrary.Skipped, error) {
	if runtime.GOOS == "js" || opts.ignoresSavedSelection() {
		return nil, nil, nil
	}
	root, err := maplibrary.DefaultRoot()
	if err != nil {
		return nil, nil, err
	}
	roots, skipped, err := maplibrary.Roots(root)
	if opts.excludedMapRoot != "" {
		excluded := func(dir string) bool { return modDirectoryMounted(opts.excludedMapRoot, []string{dir}) }
		roots = slices.DeleteFunc(roots, excluded)
		skipped = slices.DeleteFunc(skipped, func(s maplibrary.Skipped) bool { return excluded(s.Dir) })
	}
	return roots, skipped, err
}

// mapSkipNote names a downloaded package a mount left out, why, and where it
// is, so the player can remove it (DESIGN_CONTENT_VFS "Downloaded community maps").
func mapSkipNote(s maplibrary.Skipped) string {
	return fmt.Sprintf("nanolathe: downloaded map package not mounted: %s; remove it with its X under More maps, or delete %s", noticeReason(s.Err), s.Dir)
}

// mapSkipped reports the mount-time audit's reason for leaving dir out, or nil
// when the running content mounted it or never considered it.
func (c *contentSet) mapSkipped(dir string) error {
	if c == nil {
		return nil
	}
	for _, s := range c.skippedMaps {
		if modDirectoryMounted(dir, []string{s.Dir}) {
			return s.Err
		}
	}
	return nil
}

// Authored profile redirects remain authoritative. Community packages carry
// standard map-support paths, so they cannot supply a renamed family.
func mapLibraryLayoutSupported(layout vfs.Layout) bool {
	for _, row := range layout.Names() {
		switch strings.ToLower(row[0]) {
		case "maps", "features", "anims", "objects3d", "textures":
			return false
		}
	}
	return true
}

// Keep the authored map preview and navigation. Put discovery above the
// minimap, using its layout instead of the Load button position.
func installGetMapsButton(w *gui.Window) {
	if runtime.GOOS == "js" || w == nil {
		return
	}
	i := w.GadgetIndex("LOAD")
	if i < 0 {
		return
	}
	b := w.Gadgets[i]
	b.Name, b.SourceName, b.Text, b.QuickKey = "GETMAPS", "GETMAPS", "More maps", 0
	preview := w.GadgetIndex("MAPPIC")
	if preview < 0 {
		return
	}
	r := w.Gadgets[preview].Rect
	b.Rect.X, b.Rect.Y = r.X+(r.W-b.Rect.W)/2, r.Y-b.Rect.H-8
	w.Gadgets = append(w.Gadgets, b)
}

type mapsFetch struct {
	mu                    sync.Mutex
	status                string
	entries, dependencies []modfetch.Entry
	installed             []modlibrary.Mod
	selected              int
	cancel                context.CancelFunc
	dirty                 bool
	lib                   *modlibrary.Library
	previewKey            string
	previewSerial         uint64
	previewCancel         context.CancelFunc
	preview               image.Image
	previewStatus         string
	previewPixels         []byte
	previewPalette        [256][4]byte
	// inInstall caches catalogueInInstall for inInstallContent.
	inInstall        []bool
	inInstallContent *contentSet
}

var (
	mapsFetchUI     *mapsFetch
	mapsFetchPanel  *ui.Panel
	mapsFetchAssets *retailPanelAssets
	mapDownload     modDownloadJob
	mapInstallsSeen int
)

func (g *gameShell) mapsFetchActive() bool {
	return mapsFetchUI != nil && mapsFetchPanel != nil && g.activePanel() == mapsFetchPanel
}

func (g *gameShell) openMapsFetch() error {
	if g.cs.config != nil && !mapLibraryLayoutSupported(g.cs.config.Content.Layout()) {
		return &missingProductError{what: "community maps are unavailable with this content layout", logical: "maps", providers: g.cs.baseRoots, expected: "standard map and feature directories"}
	}
	root, err := maplibrary.DefaultRoot()
	if err != nil {
		return err
	}
	lib, err := maplibrary.Open(root)
	if err != nil {
		return err
	}
	installed, err := lib.Installed()
	if err != nil {
		return err
	}
	// Use the base game's compact template even when the active mod's map
	// chooser has its own layout. No mod configuration is applied here.
	base := vfs.New()
	defer base.Close()
	if err := base.MountGameDirectories(g.cs.baseRoots); err != nil {
		return err
	}
	w, err := gui.LoadWithTranslation(base, modsTemplateGUI, g.cs.translations)
	if err != nil {
		return err
	}
	bg, err := modsBackdrop(base)
	if err != nil {
		return err
	}
	buildModsWindow(w, modsWindowFetch)
	w.Gadgets[w.GadgetIndex("NTITLE")].Text = "GET MORE MAPS"
	g.installRetailWindowButtonArt(w, nil)
	g.installRetailListScrollbars(w, nil)
	buildMapsPreviewWindow(w)
	panel := ui.NewPanel(w)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	state := &mapsFetch{status: "Fetching community maps...", lib: lib, installed: installed, cancel: cancel}
	mapsFetchUI, mapsFetchPanel, mapsFetchAssets = state, panel, &retailPanelAssets{window: w, background: bg}
	g.frontend.Panels.Push(panel)
	flushWindowTokens(clPtr)
	c := &modfetch.Client{CatalogURL: modfetch.MapCatalogURL(), CacheDir: root}
	go func() {
		defer cancel()
		result, err := c.FetchManifest(ctx)
		state.mu.Lock()
		defer state.mu.Unlock()
		state.dirty = true
		if err != nil {
			state.status = "Catalogue unavailable: " + err.Error()
			return
		}
		state.entries, state.dependencies = result.Manifest.Maps, result.Manifest.Dependencies
		slices.SortStableFunc(state.entries, func(a, b modfetch.Entry) int {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
		state.status = fmt.Sprintf("%d community maps", len(state.entries))
		if result.FromCache {
			state.status = "Offline: catalogue from " + result.FetchedAt.Local().Format("2 Jan 15:04")
		}
	}()
	g.refreshMapsFetch()
	return nil
}

func (g *gameShell) closeMapsFetch() {
	if mapsFetchUI != nil {
		mapsFetchUI.cancel()
		mapsFetchUI.stopPreview()
	}
	// A map request belongs to this chooser. Cancel network work on close;
	// verified installations already publishing finish atomically.
	mapDownload.cancelNetwork()
	mapDownload.acknowledge()
	if g.mapsFetchActive() {
		g.frontend.Panels.Pop()
	}
	p := g.activePanel()
	var top int
	keepTop := false
	if p != nil {
		items, selected, oldTop, ok := p.ListValues("MAPNAMES")
		keepTop = ok && selected >= 0 && selected < len(items) && g.mapIdx >= 0 && g.mapIdx < len(g.maps) && items[selected] == g.maps[g.mapIdx]
		top = oldTop
	}
	mapsFetchUI, mapsFetchPanel, mapsFetchAssets = nil, nil, nil
	g.refreshMapPanel()
	if keepTop {
		p.SetListTopAt(p.Index("MAPNAMES"), top, p.ListMaxTopAt(p.Index("MAPNAMES")))
	}
	flushWindowTokens(clPtr)
}

func (g *gameShell) commitMapsListSelection(name string, index int) bool {
	if !g.mapsFetchActive() || name != "MAPNAMES" {
		return false
	}
	mapsFetchUI.mu.Lock()
	mapsFetchUI.selected = index
	mapsFetchUI.mu.Unlock()
	g.refreshMapsFetch()
	return true
}

func (g *gameShell) activateMapsGadget(name string) bool {
	if !g.mapsFetchActive() {
		return false
	}
	switch name {
	case "PREVMENU":
		g.closeMapsFetch()
	case "LOAD", "MAPNAMES":
		job := mapDownload.view()
		if job.running {
			mapDownload.cancelNetwork()
			g.refreshMapsFetch()
			return true
		}
		state := mapsFetchUI
		state.mu.Lock()
		if state.selected < 0 || state.selected >= len(state.entries) {
			state.mu.Unlock()
			return true
		}
		entry := state.entries[state.selected]
		deps := append([]modfetch.Entry(nil), state.dependencies...)
		installed := modInstalled(state.installed, entry) && !mapUpdateAvailable(state.installed, entry, deps)
		state.mu.Unlock()
		var err error
		if installed {
			err = g.selectLibraryMap(entry)
		} else {
			err = g.startMapDownload(state.lib, entry, deps)
		}
		if err != nil {
			state.mu.Lock()
			state.status, state.dirty = noticeReason(err), true
			state.mu.Unlock()
			mapDownload.acknowledge()
		}
		if g.mapsFetchActive() {
			g.refreshMapsFetch()
		}
	}
	return true
}

// Resolve only declared dependencies, in the requested order, without fetching
// other maps. A missing requirement refuses the request before any writes.
func mapDownloadEntries(entry modfetch.Entry, dependencies []modfetch.Entry) ([]modfetch.Entry, error) {
	var entries []modfetch.Entry
	seen := map[string]bool{}
	for _, id := range entry.Requires {
		if seen[id] {
			continue
		}
		found := false
		for _, dep := range dependencies {
			if dep.ID == id {
				entries = append(entries, dep)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("nanolathe: missing map dependency: logical path %s, providers searched [catalogue], expected a declared dependency", id)
		}
		seen[id] = true
	}
	return append(entries, entry), nil
}

// installedVersion finds the installed package with e's id and version,
// whatever bytes it was installed from.
func installedVersion(installed []modlibrary.Mod, e modfetch.Entry) (modlibrary.Mod, bool) {
	for _, mod := range installed {
		if mod.ID == e.ID && mod.Version == e.Version {
			return mod, true
		}
	}
	return modlibrary.Mod{}, false
}

// mapUpdateAvailable reports an installed map whose request no longer matches
// the library: the catalogue republished it or one of its declared
// dependencies under the same version, or added a dependency.
func mapUpdateAvailable(installed []modlibrary.Mod, entry modfetch.Entry, dependencies []modfetch.Entry) bool {
	if _, ok := installedVersion(installed, entry); !ok {
		return false
	}
	entries, err := mapDownloadEntries(entry, dependencies)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e modfetch.Entry) bool { return !modInstalled(installed, e) })
}

// baseAndModRoots is the mounted base install and selected mod, without
// downloaded map packages.
func (c *contentSet) baseAndModRoots() []string {
	roots := append([]string(nil), c.baseRoots...)
	if c.mod != nil {
		roots = append(roots, c.mod.Dir)
	}
	return roots
}

// mapInInstall reports whether the mounted base install or selected mod,
// rather than a downloaded package, supplies the map at logical.
func (g *gameShell) mapInInstall(libRoot, logical string) bool {
	if logical == "" || g.cs == nil || g.cs.unmappedMount == nil {
		return false
	}
	info, err := g.cs.unmappedMount.Stat(logical)
	return err == nil && !pathInsideMapPackage(libRoot, info.Source.SourcePath)
}

// mapInstallRequest is one catalogue request's install plan. It is captured
// on the render thread, so the worker never reads the live shell.
type mapInstallRequest struct {
	lib        *modlibrary.Library
	entries    []modfetch.Entry // declared dependencies first, then the map
	pending    []modfetch.Entry // the entries to install, in that order
	baseAndMod []string         // the mounted base install and selected mod
	mounted    []string         // the mounted downloaded packages
}

// pendingMapReplacement is a downloaded request that replaces an installed
// package. The front end installs it between remounts (installMapReplacement).
var pendingMapReplacement *mapInstallRequest

func (r *mapInstallRequest) dir(e modfetch.Entry) string {
	return filepath.Join(r.lib.Root, e.ID, e.Version)
}

// validator resolves e's features from the base stack and its own declared
// dependencies only. Every other mounted package keeps its feature
// definitions; e's own previous version is not one of them.
func (r *mapInstallRequest) validator(e modfetch.Entry) func(string, modlibrary.Metadata) error {
	var required, others []string
	if e.Map != "" {
		for _, dep := range r.entries {
			if dep.Map == "" {
				required = append(required, r.dir(dep))
			}
		}
	}
	required = append(required, r.baseAndMod...)
	for _, dir := range r.mounted {
		if modDirectoryMounted(dir, required) || filepath.Base(filepath.Dir(dir)) == e.ID {
			continue
		}
		others = append(others, dir)
	}
	return maplibrary.InstallValidator(required, others, e.Map)
}

// install installs the verified downloads in order. unmount, when set, runs
// before each package so a mounted previous version is never replaced under
// its readers.
func (r *mapInstallRequest) install(unmount func(dir string) error) error {
	for _, e := range r.pending {
		if unmount != nil {
			if err := unmount(r.dir(e)); err != nil {
				return err
			}
		}
		dst := filepath.Join(r.lib.Root, ".downloads", e.ArchiveName())
		options := e.InstallOptions()
		options.Validate = r.validator(e)
		if _, err := r.lib.InstallArchive(dst, options); err != nil {
			return err
		}
		_ = os.Remove(dst)
	}
	return nil
}

// installMapReplacement publishes a same-version update on the render thread:
// remount without the old package, install the verified archive (the library
// keeps the old bytes if that fails), then remount whatever is installed.
func (g *gameShell) installMapReplacement(r *mapInstallRequest) error {
	err := r.install(func(dir string) error {
		if !modDirectoryMounted(dir, g.cs.roots) {
			return nil
		}
		return g.refreshMapLibraryExcluding(dir)
	})
	if refreshErr := g.refreshMapLibrary(); refreshErr != nil {
		return errors.Join(err, refreshErr)
	}
	return err
}

func (g *gameShell) startMapDownload(lib *modlibrary.Library, entry modfetch.Entry, dependencies []modfetch.Entry) error {
	entries, err := mapDownloadEntries(entry, dependencies)
	if err != nil {
		return err
	}
	if g.mapInInstall(lib.Root, entry.Map) {
		return &missingProductError{what: maplibrary.ErrMapInInstall.Error(), logical: entry.Map, providers: g.cs.baseAndModRoots(), expected: "a map your installed game and selected mod do not supply"}
	}
	installed, err := lib.Installed()
	if err != nil {
		return err
	}
	// Capture roots on the render thread; workers never read the live shell.
	request := &mapInstallRequest{lib: lib, entries: entries, baseAndMod: g.cs.baseAndModRoots()}
	for _, root := range g.cs.roots {
		if pathInsideMapPackage(lib.Root, root) {
			request.mounted = append(request.mounted, root)
		}
	}
	replacing := false
	for _, e := range entries {
		if modInstalled(installed, e) {
			continue
		}
		if _, ok := installedVersion(installed, e); ok {
			replacing = true
		}
		request.pending = append(request.pending, e)
	}
	c := &modfetch.Client{CatalogURL: modfetch.MapCatalogURL(), CacheDir: lib.Root}
	var total int64
	for _, e := range request.pending {
		total += e.Archive.Size
	}
	if replacing {
		pendingMapReplacement = request
	}
	started := mapDownload.start(entry, func(ctx context.Context, progress func(int64, int64)) error {
		if err := os.MkdirAll(filepath.Join(lib.Root, ".downloads"), 0755); err != nil {
			return err
		}
		var previous int64
		for _, e := range request.pending {
			dst := filepath.Join(lib.Root, ".downloads", e.ArchiveName())
			if err := c.Download(ctx, e, dst, func(done, _ int64) { progress(previous+done, total) }); err != nil {
				return err
			}
			previous += e.Archive.Size
		}
		return ctx.Err()
	}, func() error {
		if replacing {
			return nil // pollMapsFetch installs it between remounts
		}
		return request.install(nil)
	})
	if !started {
		if replacing {
			pendingMapReplacement = nil
		}
		return &missingProductError{what: "another map download is running", logical: entry.ID, expected: "the current download to finish"}
	}
	return nil
}

func (g *gameShell) pollMapsFetch() {
	if g == nil || g.cs == nil || g.frontend == nil {
		return
	}
	dirty := mapDownload.takeDirty()
	job := mapDownload.view()
	// A closed chooser can finish installation during another screen, but a
	// battle keeps its original mount for its full lifetime.
	if !job.running && job.installs != mapInstallsSeen && g.battle == nil && g.frontend.Mode != modeLoading {
		mapInstallsSeen = job.installs
		outcome := ""
		if r := pendingMapReplacement; r != nil {
			pendingMapReplacement = nil
			if err := g.installMapReplacement(r); err != nil {
				outcome = "Failed: " + noticeReason(err)
			}
		} else if err := g.refreshMapLibrary(); err != nil {
			outcome = "Installed; could not refresh maps: " + noticeReason(err)
		}
		if outcome != "" {
			mapDownload.mu.Lock()
			mapDownload.outcome = outcome
			mapDownload.unseen = true
			mapDownload.mu.Unlock()
		}
		dirty = true
	}
	if !job.running && job.installs == mapInstallsSeen {
		pendingMapReplacement = nil // its download failed or was cancelled
	}
	state := mapsFetchUI
	if state == nil {
		return
	}
	state.mu.Lock()
	dirty = dirty || state.dirty
	state.dirty = false
	state.mu.Unlock()
	if dirty {
		installed, err := state.lib.Installed()
		state.mu.Lock()
		if err == nil {
			state.installed = installed
		}
		state.mu.Unlock()
		if g.mapsFetchActive() {
			g.refreshMapsFetch()
		}
	}
}

// Remount only content and its readers. The shell, panels, scroll positions,
// live setup, active mod and preferences survive. No settings are written.
func (g *gameShell) refreshMapLibrary() error {
	return g.refreshMapLibraryExcluding("")
}

func (g *gameShell) refreshMapLibraryExcluding(exclude string) error {
	if g.battle != nil {
		return &missingProductError{what: "map refresh requires the front end", logical: "maps", expected: "the running battle to finish"}
	}
	old := g.cs
	opts := g.opts
	opts.ModConfig = old.configPath
	opts.excludedMapRoot = exclude
	fresh, err := mountContent(opts, old.baseRoots, modSelection{mod: old.mod, saved: old.savedMod, manual: old.manualRoots, notice: old.modNotice})
	if err != nil {
		return err
	}
	census, err := censusSkirmishMaps(fresh.unmappedMount)
	if err != nil {
		fresh.Close()
		return err
	}
	keep := ""
	if g.mapIdx >= 0 && g.mapIdx < len(g.maps) {
		keep = g.maps[g.mapIdx]
	}
	if nlScreenInst != nil {
		nlScreenInst.releasePreview(true)
	}
	g.releaseAudio()
	g.audioOwner, g.frontendAliasesBound = nil, false
	g.cs = fresh
	g.maps, g.mapLabels, g.mapCensus = census.names, append([]string(nil), census.names...), census
	g.mapData = nil
	g.mapIdx = max(0, slices.Index(g.maps, keep))
	g.opts.Root, g.opts.Roots = fresh.root, append([]string(nil), fresh.roots...)
	if clPtr != nil {
		clPtr.SetModelFS(fresh.unmappedMount, fresh.presentation.TeamLogos)
	}
	g.ensureFrontendAudio()
	g.armMenuBGM()
	return old.Close()
}

func (g *gameShell) selectLibraryMap(entry modfetch.Entry) error {
	if err := g.refreshMapLibrary(); err != nil {
		return err
	}
	name := strings.TrimSuffix(path.Base(entry.Map), path.Ext(entry.Map))
	i := slices.IndexFunc(g.maps, func(s string) bool { return strings.EqualFold(s, name) })
	if i < 0 {
		return &missingProductError{what: "installed map is unavailable", logical: entry.Map, providers: g.cs.roots, expected: "a selectable Network map"}
	}
	g.mapIdx = i
	g.closeMapsFetch()
	// Selection returns to the ordinary map preview; Load keeps its existing
	// explicit commit to the Skirmish or Survival setup.
	g.refreshMapPanel()
	return nil
}

// The authored retail font consumes bytes. Catalogue text is UTF-8, so use
// readable equivalents for common publishing punctuation before measuring it.
func mapCatalogueText(text string) string {
	return strings.NewReplacer("’", "'", "‘", "'", "“", "\"", "”", "\"", "–", "-", "—", "-", "×", "x", "…", "...", "\u00a0", " ").Replace(text)
}

func (g *gameShell) refreshMapsFetch() {
	state, p := mapsFetchUI, mapsFetchPanel
	if state == nil || p == nil {
		return
	}
	job := mapDownload.view()
	state.mu.Lock()
	defer func() { state.mu.Unlock(); g.refreshMapRemovalRows(p) }()
	state.startPreviewLocked()
	inInstall := g.catalogueInInstall(state)
	rows := make([]mapCatalogueRow, len(state.entries))
	items := make([]string, len(state.entries))
	for i, e := range state.entries {
		var skipped error
		if _, ok := installedVersion(state.installed, e); ok && state.lib != nil {
			skipped = g.cs.mapSkipped(filepath.Join(state.lib.Root, e.ID, e.Version))
		}
		rows[i] = catalogueRow(e, state.installed, state.dependencies, inInstall[i], skipped)
		items[i] = mapCatalogueText(e.Name) + "  (" + rows[i].suffix + ")"
	}
	g.setListItems("MAPNAMES", items, state.selected)
	description, detail, action, blocked := "", "", "Download", false
	if state.selected >= 0 && state.selected < len(state.entries) {
		e, row := state.entries[state.selected], rows[state.selected]
		description, detail, action, blocked = e.Summary, e.Homepage, row.action, row.blocked
		if len(e.Requires) > 0 {
			description += fmt.Sprintf(" Includes %d feature pack(s).", len(e.Requires))
		}
		if row.detail != "" {
			detail = row.detail
		}
	}
	status, percent, bytes := state.status, "", ""
	switch {
	case job.installing:
		status = "Installing " + job.entry.Name
	case job.running:
		status, action = "Downloading "+job.entry.Name, "Cancel"
		if job.total > 0 {
			percent = fmt.Sprintf("%d%%", job.done*100/job.total)
			bytes = fmt.Sprintf("%.1f / %.1f MB", float64(job.done)/1e6, float64(job.total)/1e6)
		}
	case job.outcome != "":
		status = job.outcome
	}
	if percent != "" {
		detail = percent + "   " + bytes
	}
	p.SetText("DESCRIPTION", g.fitDetail(mapCatalogueText(description), 230, 2))
	p.SetText("SIZE", g.fitDetail(mapCatalogueText(detail), 230, 1))
	p.SetText("STATUS", g.fitDetail(mapCatalogueText(status), 116, 3))
	p.SetText("PREVIEWSTATUS", g.fitDetail(state.previewStatus, 116, 2))
	p.SetText("STATUS2", percent)
	p.SetText("STATUS3", bytes)
	p.SetText("LOAD", action)
	p.SetText("PREVMENU", "Close")
	retailGreyGadget(p.Window, "LOAD", job.installing || (!job.running && (len(items) == 0 || blocked)))
}

// mapCatalogueRow is one catalogue entry's list suffix, Load action and the
// detail line that replaces its homepage when its state needs explaining.
// blocked greys Load: the install already supplies that map.
type mapCatalogueRow struct {
	suffix, action, detail string
	blocked                bool
}

// catalogueRow describes e against the installed library and the running
// mount (docs/DESIGN_INTERFACE_HUD_INPUT.md "Community map downloads").
func catalogueRow(e modfetch.Entry, installed []modlibrary.Mod, dependencies []modfetch.Entry, inInstall bool, skipped error) mapCatalogueRow {
	row := mapCatalogueRow{suffix: fmt.Sprintf("%.1f MB", float64(e.Archive.Size)/1e6), action: "Download"}
	switch {
	case inInstall:
		row.suffix, row.detail, row.blocked = "in your install", "Already in your install: choose it from the map list", true
	case mapUpdateAvailable(installed, e, dependencies):
		row.suffix, row.action = "update available", "Update"
	case !modInstalled(installed, e):
	case skipped != nil:
		row.suffix, row.action, row.detail = "not loaded", "Select", "Not loaded: "+noticeReason(skipped)
	default:
		row.suffix, row.action = "installed", "Select"
	}
	return row
}

// catalogueInInstall caches, per mounted content, which catalogue maps the
// base install or selected mod already supplies. The caller holds state.mu.
func (g *gameShell) catalogueInInstall(state *mapsFetch) []bool {
	if state.inInstallContent == g.cs && len(state.inInstall) == len(state.entries) {
		return state.inInstall
	}
	state.inInstall, state.inInstallContent = make([]bool, len(state.entries)), g.cs
	if state.lib != nil {
		for i, e := range state.entries {
			state.inInstall[i] = g.mapInInstall(state.lib.Root, e.Map)
		}
	}
	return state.inInstall
}
