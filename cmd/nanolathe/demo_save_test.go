package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nanolathe-gg/nanolathe/internal/ui"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Independently authored windows distinguish resource selection without
// copying demo bytes. The fallback retains the existing SAV controller,
// although retail uses these resource names for LST files [08 R-SAVE-02 §5].
func demoSaveFixture(x int) []byte {
	return []byte(fmt.Sprintf(`
[GADGET0]{[COMMON]{id=0;name=fixture;xpos=%d;ypos=20;width=400;height=350;active=1;}totalgadgets=5;panel=NULL;crdefault=LOAD;}
[GADGET1]{[COMMON]{id=2;name=GAMES;xpos=20;ypos=30;width=200;height=100;active=1;}}
[GADGET2]{[COMMON]{id=3;name=GAMENAME;xpos=20;ypos=150;width=200;height=20;active=1;}maxchars=20;}
[GADGET3]{[COMMON]{id=1;name=LOAD;xpos=230;ypos=220;width=96;height=20;active=1;}text=OK;}
[GADGET4]{[COMMON]{id=1;name=CANCEL;xpos=230;ypos=250;width=96;height=20;active=1;}text=Cancel;}
[GADGET5]{[COMMON]{id=1;name=DELETE;xpos=230;ypos=180;width=96;height=20;active=1;}text=Delete List;}
`, x))
}

func demoSaveShell(t *testing.T, files map[string][]byte) *gameShell {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	fs := vfs.New()
	t.Cleanup(func() { _ = fs.Close() })
	if err := fs.MountDirectory(root, 0); err != nil {
		t.Fatal(err)
	}
	return &gameShell{cs: testContentSet(fs), opts: Options{SaveDir: t.TempDir()}, frontend: ui.NewFrontend(modeMenuSingle)}
}

func TestSaveLoadGUIFallback(t *testing.T) {
	for _, tc := range []struct {
		name, guiPath, errorPath string
		mode                     saveLoadMode
		retail                   []byte
		backdrop, fallback       bool
		corruptFallback          bool
	}{
		{name: "save uses demo name", guiPath: "guis/savelist.gui", fallback: true},
		{name: "load uses demo name", mode: loadScreenMode, guiPath: "guis/loadlist.gui", fallback: true},
		{name: "retail does not need demo files", retail: demoSaveFixture(5), backdrop: true, guiPath: retailSaveLoadGUI},
		{name: "missing save fallback", errorPath: "guis/savelist.gui"},
		{name: "missing load fallback", mode: loadScreenMode, errorPath: "guis/loadlist.gui"},
		{name: "corrupt retail does not fall back", retail: []byte("invalid GUI"), fallback: true, errorPath: retailSaveLoadGUI},
		{name: "present retail requires its backdrop", retail: demoSaveFixture(5), fallback: true, errorPath: "bitmaps/dsavegame2.pcx"},
		{name: "corrupt selected fallback", guiPath: "guis/savelist.gui", errorPath: "guis/savelist.gui", corruptFallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetSaveLoadScreenState(t)
			files := map[string][]byte{}
			if tc.fallback {
				files["guis/savelist.gui"] = demoSaveFixture(10)
				files["guis/loadlist.gui"] = demoSaveFixture(30)
			}
			if tc.retail != nil {
				files[retailSaveLoadGUI] = tc.retail
			}
			if tc.backdrop {
				files[tc.mode.backdrop()] = nlTestPCX(2, 1)
			}
			if tc.corruptFallback {
				files[tc.guiPath] = []byte("invalid GUI")
			}
			panel, err := demoSaveShell(t, files).loadSaveLoadPanel(tc.mode)
			if tc.errorPath != "" {
				if err == nil || !strings.Contains(err.Error(), "logical path "+tc.errorPath) || saveLoadAssets != nil {
					t.Fatalf("error=%v assets=%v, want failure naming %s before installation", err, saveLoadAssets, tc.errorPath)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if panel.Window.Name != tc.guiPath || (saveLoadAssets.background != nil) != tc.backdrop {
				t.Fatalf("window=%s background=%v", panel.Window.Name, saveLoadAssets.background)
			}
			if tc.fallback {
				i := panel.Index("DELETE")
				if panel.TextOf("DELETE") != "Delete Game" || retailGadgetText(panel, i, panel.Window.Gadgets[i]) != "Delete Game" {
					t.Fatal("fallback still labels game deletion as list deletion")
				}
			}
		})
	}
}

func TestDemoSavePanelsKeepGameFilesAndDirectionLayout(t *testing.T) {
	resetSaveLoadScreenState(t)
	shell := demoSaveShell(t, map[string][]byte{
		"guis/savelist.gui":     demoSaveFixture(10),
		"guis/loadlist.gui":     demoSaveFixture(30),
		"gamedata/sidedata.tdf": []byte("[SIDE0]{name=ARM;font=fixture;" + authoredSideAnchors() + "}"),
	})
	path := writeContinuationSlot(t, shell.opts.SaveDir, "game slot", time.Unix(1_700_000_000, 0))
	if err := os.WriteFile(filepath.Join(shell.opts.SaveDir, "restriction.LST"), []byte("list"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := shell.openSaveLoadScreen(saveScreenMode, saveLoadFromBattle); err != nil {
		t.Fatal(err)
	}
	defer shell.closeSaveLoadScreen()
	if !saveLoadPanel.EditorCaptured() || len(saveLoadUI.Entries()) != 1 || saveLoadUI.Entries()[0].Path != path {
		t.Fatal("fallback lost initial name focus or enumerated restriction files")
	}
	shell.activateSaveLoadGadget("LoadGame")
	if saveLoadUI.Mode() != loadScreenMode || saveLoadPanel.Window.Rect.X != 30 || saveLoadPanel.EditorCaptured() || shell.activePanel() != saveLoadPanel {
		t.Fatal("load direction did not install its authored layout and hide the editor")
	}
	shell.activateSaveLoadGadget("SaveGame")
	if saveLoadUI.Mode() != saveScreenMode || saveLoadPanel.Window.Rect.X != 10 || !saveLoadPanel.EditorCaptured() {
		t.Fatal("save direction did not restore its authored layout and name focus")
	}
	shell.activateSaveLoadGadget("DELETE")
	if _, err := os.Stat(path); !os.IsNotExist(err) || len(saveLoadUI.Entries()) != 0 {
		t.Fatalf("game deletion did not remove the selected SAV: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shell.opts.SaveDir, "restriction.LST")); err != nil {
		t.Fatal("game deletion touched a restriction list")
	}
}
