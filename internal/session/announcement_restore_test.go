package session

import (
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/combat"
	"github.com/nanolathe-gg/nanolathe/internal/community"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/gameplay"
	"github.com/nanolathe-gg/nanolathe/internal/save"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func roundTripEntryState(t *testing.T, loaded RetailLoadResult, deps RetailLoadDeps) *Session {
	t.Helper()
	src := loaded.Battle.Session
	sc := SaveSidecar(src)
	in, err := src.RetailBattleSaveInputs(loaded.Summary, save.Camera{})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := src.RetailProjection(in)
	if err != nil {
		t.Fatal(err)
	}
	deps.Gameplay = gameplay.Mode(sc.Rules)
	deps.CommunitySources = SidecarCommunitySources(sc.Community)
	deps.EntryCommunity = &sc.Community.Entry
	restored, err := LoadRetailSaveWithDeps(recursiveRestoreBank(t, projection), deps)
	if err != nil {
		t.Fatal(err)
	}
	return restored.Battle.Session
}

// Entry transforms survive a rule switch in either direction. Restoring the
// current rule selection must not recompute the immutable reload word
// (DESIGN_COMMUNITY_PATCH §4.1, DESIGN_MODS_MUTATORS §7.3).
func TestSavePreservesEntryStockpileClampAcrossRuleSwitch(t *testing.T) {
	for _, entry := range []gameplay.Mode{gameplay.Modern, gameplay.Strict31} {
		t.Run(string(entry), func(t *testing.T) {
			bank, deps := restoreRNGFixture(t)
			weapon := &content.WeaponDef{ID: 1, Stockpile: true, MetalPerShot: 100}
			weapon.CanonicalKey = "stock"
			deps.Catalog.Weapons = map[string]*content.WeaponDef{"stock": weapon}
			deps.Catalog.Units["armcom"].Weapon1 = "stock"
			deps.Catalog.Units["armcom"].Weapon1Def = weapon
			enabled := true
			deps.CommunitySources.Content = []community.Overrides{{BuildWeaponSlotGuard: &enabled}}
			deps.Gameplay = entry
			loaded, err := LoadRetailSaveWithDeps(bank, deps)
			if err != nil {
				t.Fatal(err)
			}
			src := loaded.Battle.Session
			next, wantReload, wantCost := gameplay.Strict31, int32(1), float32(100)
			if entry == gameplay.Strict31 {
				next, wantReload, wantCost = gameplay.Modern, 0, 0
			}
			if err := src.SetRules(string(next)); err != nil {
				t.Fatal(err)
			}
			dst := roundTripEntryState(t, loaded, deps)
			for _, s := range []*Session{src, dst} {
				w := s.Catalog.Weapons["stock"]
				var cost float32
				combat.TickStockpile(&combat.StockpileEntry{Weapon: w, Count: 1}, &combat.Slot{}, 0,
					func(_, metal float32) bool { cost += metal; return true })
				if w.ReloadTime != wantReload || cost != wantCost || s.Gameplay != next {
					t.Fatalf("entry %s, current %s: reload %d, round cost %v; want %d, %v", entry, s.Gameplay, w.ReloadTime, cost, wantReload, wantCost)
				}
			}
			if weapon.ReloadTime != 0 {
				t.Fatal("entry transform changed the shared authored catalog")
			}
		})
	}
}

// The heading persists an existing building's geometry even when the current
// rules prohibit placing another at that facing (DESIGN_COMMUNITY_PATCH §4.3).
func TestSavePreservesRotatedGeometryAfterRuleSwitch(t *testing.T) {
	bank, deps := restoreRNGFixture(t)
	root := t.TempDir()
	writeCompositionCOBProgram(t, root, "lab", []uint32{0x10065000}, []string{"Create"}, []uint32{0}, []string{"modelroot", "modelchild"})
	if err := deps.FS.(*vfs.FS).MountDirectory(root, 20); err != nil {
		t.Fatal(err)
	}
	lab := *deps.Catalog.Units["armcom"]
	lab.UnitName, lab.CanonicalKey = "lab", "lab"
	lab.BMCode, lab.FootprintX, lab.FootprintZ = 0, 2, 3
	lab.CanMove, lab.CanFly, lab.CanHover = false, false, false
	lab.YardMap, lab.Rotations = "cooooo", content.FacingSouth|content.FacingEast
	deps.Catalog.Units["lab"] = &lab
	deps.Catalog = deps.Catalog.Clone()
	enabled := true
	deps.CommunitySources.Content = []community.Overrides{{StructureRotation: &enabled}}
	deps.Gameplay = gameplay.Modern
	loaded, err := LoadRetailSaveWithDeps(bank, deps)
	if err != nil {
		t.Fatal(err)
	}
	src := loaded.Battle.Session
	h, err := src.Units.CreateFacing(src.Catalog.Units["lab"], 0, numeric.FixedFromInt(320), numeric.FixedFromInt(10), numeric.FixedFromInt(320), units.FacingEast)
	if err != nil {
		t.Fatal(err)
	}
	u := src.Units.Unit(h)
	src.Movement.EnsureUnit(u)
	if err := src.SetRules(string(gameplay.Strict31)); err != nil {
		t.Fatal(err)
	}
	before, err := src.Build.StructureGeometryForUnit(u)
	if err != nil {
		t.Fatal(err)
	}
	dst := roundTripEntryState(t, loaded, deps)
	got := dst.Units.Unit(h)
	after, err := dst.Build.StructureGeometryForUnit(got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Move.Heading != u.Move.Heading || after.Facing != units.FacingEast ||
		after.FootprintX != before.FootprintX || after.FootprintZ != before.FootprintZ || !slices.Equal(after.Yard, before.Yard) ||
		int32(got.FootprintSizeX) != after.FootprintX || int32(got.FootprintSizeZ) != after.FootprintZ {
		t.Fatalf("saved geometry %+v, restored %+v; instance footprint %dx%d", before, after, got.FootprintSizeX, got.FootprintSizeZ)
	}
	if dst.Build.ResolveStructureFacing(got.Def, units.FacingEast) != units.FacingSouth {
		t.Fatal("restoring a facing enabled it for new Strict placements")
	}
}
