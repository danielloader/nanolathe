package main

import (
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/settings"
)

func TestSidebarDirectBattleUsesCapturedPreferences(t *testing.T) {
	p := settings.DefaultPresentation()
	p.BuildMenuPageSize, p.SidebarOrders = 12, 0
	b := &battleSession{hostPresentation: &p, modBuildPageSize: 6}
	if b.buildPageLock() != 12 || b.sidebarOrdersEnabled() || !b.expandedSidebarEnabled() {
		t.Fatal("direct battle ignored the saved sidebar preferences")
	}
	p.BuildMenuPageSize = 0
	if b.buildPageLock() != 0 {
		t.Fatal("direct battle's explicit Free flow lost to the mod recommendation")
	}
	p.BuildMenuPageSize = -1
	if b.buildPageLock() != 6 {
		t.Fatal("direct battle stopped inheriting its content recommendation")
	}
	b.shell = &gameShell{presentation: settings.DefaultPresentation()}
	b.shell.presentation.BuildMenuPageSize, b.shell.presentation.SidebarOrders = 24, 1
	if b.buildPageLock() != 24 || !b.sidebarOrdersEnabled() {
		t.Fatal("captured settings overrode the live options owner")
	}
	b.shell, b.hostPresentation = nil, nil
	defaults := settings.DefaultPresentation()
	if b.expandedSidebarEnabled() != (defaults.ExpandedSidebar != 0) || b.sidebarOrdersEnabled() != (defaults.SidebarOrders != 0) || b.buildPageLock() != 6 {
		t.Fatal("detached capture defaults or content fallback changed")
	}
}
