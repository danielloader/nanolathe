package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/client"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/frame"
	"github.com/nanolathe-gg/nanolathe/internal/headless"
	"github.com/nanolathe-gg/nanolathe/internal/palette"
	"github.com/nanolathe-gg/nanolathe/internal/session"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// Scenario skin preparation precedes adoption. A missing bank can reject a
// Survival entry, while the same optional metadata is inert in ordinary games.
func TestBattleAttackerSkinPreparedForActualOwner(t *testing.T) {
	fs := vfs.New()
	t.Cleanup(func() { _ = fs.Close() })
	if err := fs.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Fatal(err)
	}
	local := t.TempDir()
	if err := os.Mkdir(filepath.Join(local, "skins"), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := formats.EncodeGAF([]formats.GAFWriteEntry{{Name: "authored_skin_fixture", Frames: []formats.GAFWriteFrame{{Width: 1, Height: 1, Pixels: []byte{42}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(local, "skins", "fixture.gaf"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = fs.MountDirectory(local, 1000); err != nil {
		t.Fatal(err)
	}
	cat, err := content.Compile(fs)
	if err != nil {
		t.Fatal(err)
	}
	pal, err := palette.Load(fs)
	if err != nil {
		t.Fatal(err)
	}
	for _, survival := range []bool{false, true} {
		kind := headless.ScenarioSkirmish
		cfg := session.SkirmishConfig{}
		if survival {
			kind = headless.ScenarioSurvival
			cfg = session.SurvivalSkirmishConfig("painted desert", 2, session.SurvivalOptions{})
		}
		composed, err := headless.ComposeFreshBattle(headless.FreshBattleRequest{Kind: kind, Map: "painted desert", Skirmish: cfg, LocalOwner: 0, SimulationSeed: 7, CRTSeed: 7, FS: fs, Catalog: cat})
		if err != nil {
			t.Fatal(err)
		}
		ownCat := composed.Session.Catalog.Clone()
		ownCat.SurvivalRoster = &content.SurvivalRoster{Units: []content.SurvivalRosterEntry{{Unit: "armpw", Tier: 1}}, AttackerSkin: "skins/fixture.gaf"}
		registry, err := client.NewModelTextureRegistry(fs, ownCat, composed.Session.World, 0)
		if err != nil {
			t.Fatal(err)
		}
		b := &battleSession{sess: composed.Session, cat: ownCat, modelTextures: registry}
		if err = b.prepareAttackerSkin(pal); err != nil {
			t.Fatal(err)
		}
		if survival && (b.attackerSkin == nil || b.attackerSkinOwner != 3) {
			t.Fatalf("skin did not follow actual attacker owner: %v %d", b.attackerSkin, b.attackerSkinOwner)
		}
		if !survival && b.attackerSkin != nil {
			t.Fatal("ordinary battle adopted scenario skin")
		}
		f := &frame.Frame{Units: []frame.UnitView{{Slot: 7, Owner: 3, DefName: "armpw", Health: 100}}}
		wantPrefix := ""
		if survival {
			wantPrefix = "Infected "
		}
		if got := b.infectedHoverPrefix(f, 7); got != wantPrefix {
			t.Fatalf("hover prefix = %q, want %q", got, wantPrefix)
		}
		f.Units[0].Owner = 0
		if got := b.infectedHoverPrefix(f, 7); got != "" {
			t.Fatalf("healthy shared definition received prefix: %q", got)
		}
		f.Units[0].Owner = 3
		if got := b.infectedHoverPrefix(f, 7); got != wantPrefix {
			t.Fatalf("capture did not update name on first committed frame: %q", got)
		}
		// Every ordinary/modded Survival roster gets the generated treatment too.
		ownCat.SurvivalRoster = nil
		b.attackerSkin = nil
		if err = b.prepareAttackerSkin(pal); err != nil {
			t.Fatal(err)
		}
		if survival && (b.attackerSkin == nil || b.attackerSkinOwner != 3) {
			t.Fatal("default Survival skin absent")
		}
		if !survival && b.attackerSkin != nil {
			t.Fatal("skirmish received infection")
		}
		ownCat.SurvivalRoster = &content.SurvivalRoster{AttackerSkin: "skins/missing.gaf"}
		err = b.prepareAttackerSkin(pal)
		if survival && err == nil {
			t.Fatal("missing bank accepted before battle adoption")
		}
		if !survival && err != nil {
			t.Fatal("ordinary battle tried to load scenario skin")
		}
	}
}
