package main

// The live shell owns options preview and rollback. Direct battles read their
// captured settings through the same host-preference owner as other controls.
func (b *battleSession) expandedSidebarEnabled() bool {
	return b.hostPreferences().ExpandedSidebar != 0
}

// Every explicit page-size choice, including Free flow, overrides the content
// recommendation (interface design §3.3 "Build page lock").
func (b *battleSession) buildPageLock() int {
	if b == nil {
		return 0
	}
	if p := b.hostPreferences(); p.BuildMenuPageSize >= 0 {
		return p.BuildMenuPageSize
	}
	return max(b.modBuildPageSize, 0)
}

func (b *battleSession) sidebarOrdersEnabled() bool {
	return b.hostPreferences().SidebarOrders != 0
}

// expandedSidebarActive reports whether the flat Expanded sidebar layout is
// selected: the Modern renderer with the preference on.
func (b *battleSession) expandedSidebarActive() bool {
	return b != nil && b.cl != nil && b.cl.Enhanced() && b.expandedSidebarEnabled()
}
