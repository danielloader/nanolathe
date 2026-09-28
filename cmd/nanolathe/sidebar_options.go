package main

import "github.com/nanolathe-gg/nanolathe/internal/settings"

// The live shell owns options preview and rollback. Detached captures use the
// same deterministic defaults as their other presentation preferences.
func (b *battleSession) expandedSidebarEnabled() bool {
	if b.shell != nil {
		return b.shell.presentation.ExpandedSidebar != 0
	}
	return settings.DefaultPresentation().ExpandedSidebar != 0
}

// buildPageLock is the expanded sidebar's fixed build page size, or zero for
// auto-flow. The player's settings file wins over the running mod, and the mod
// over auto-flow (interface design §3.3 "Build page lock").
func (b *battleSession) buildPageLock() int {
	if b == nil {
		return 0
	}
	if b.shell != nil && b.shell.presentation.BuildMenuPageSize > 0 {
		return b.shell.presentation.BuildMenuPageSize
	}
	return max(b.modBuildPageSize, 0)
}

// expandedSidebarActive reports whether the flat Expanded sidebar layout is
// selected: the Modern renderer with the preference on.
func (b *battleSession) expandedSidebarActive() bool {
	return b != nil && b.cl != nil && b.cl.Enhanced() && b.expandedSidebarEnabled()
}
