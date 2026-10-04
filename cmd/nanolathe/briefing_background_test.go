package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/vfs"
)

func TestBriefingBackgroundCacheTracksContentAndSide(t *testing.T) {
	content := func(width int) *contentSet {
		t.Helper()
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "bitmaps"), 0755); err != nil {
			t.Fatal(err)
		}
		for _, side := range []string{"arm", "cor"} {
			if err := os.WriteFile(filepath.Join(root, "bitmaps", "mbrief"+side+".pcx"), nlTestPCX(width, 1), 0644); err != nil {
				t.Fatal(err)
			}
		}
		fs := vfs.New()
		if err := fs.MountDirectory(root, 10); err != nil {
			t.Fatal(err)
		}
		return testContentSet(fs)
	}
	g := &gameShell{cs: content(1)}
	arm := g.briefingBackground()
	if arm == nil || g.briefingBackground() != arm {
		t.Fatal("Arm briefing background decoded again")
	}
	g.missionSide = 1
	core := g.briefingBackground()
	if core == nil || core == arm || g.briefingBackground() != core {
		t.Fatal("Core briefing lost its separate cached background")
	}
	g.missionSide = 0
	if g.briefingBackground() != arm {
		t.Fatal("changing side discarded the other side's background")
	}
	g.cs = content(2)
	if fresh := g.briefingBackground(); fresh == nil || fresh == arm || fresh.Width != 2 {
		t.Fatal("briefing retained art from the replaced content set")
	}
	g.cs = testContentSet(vfs.New())
	if g.briefingBackground() != nil {
		t.Fatal("missing optional background acquired replacement art")
	}
}
