//go:build retail

package session

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport"
	"github.com/nanolathe-gg/nanolathe/internal/testsupport/retailcat"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

const frozenSkirmishMap = "ashap plateau"

func frozenSkirmishConfig() SkirmishConfig {
	cfg := DirectSkirmishConfig(frozenSkirmishMap)
	cfg.RNGSimSeed, cfg.RNGCrtSeed = 7, 7
	return cfg
}

func writeOverlayFile(t *testing.T, root, logical string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(logical))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mountWithOverlay mounts the reference install with a loose directory above
// it, the way a developer's edited files shadow the archives.
func mountWithOverlay(t *testing.T, overlay string) *vfs.FS {
	t.Helper()
	fs := vfs.New()
	if err := fs.MountGameDirectory(testsupport.RetailRoot(t)); err != nil {
		t.Fatal(err)
	}
	if err := fs.MountDirectory(overlay, 1000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

// M2-C8 at battle level. A model is edited on disk during a battle, before
// the unit that uses it has ever been built: the battle's frozen identity does
// not move, the unit built afterwards binds the model the battle admitted, and
// the battle's composition and play read nothing its capture had not frozen.
// A second battle after the edit freezes the edited model and a new identity.
func TestFrozenSkirmishKeepsAdmittedContentRetail(t *testing.T) {
	cat, shared := retailcat.Shared(t)
	chain := retailcat.SelectOpeningChain(t, cat, 0)
	def, ok := cat.Unit(chain.LabProduct)
	if !ok {
		t.Fatalf("opening lab product %q missing", chain.LabProduct)
	}
	modelPath := "objects3d/" + content.CanonicalKey(strings.TrimSpace(def.ObjectName)) + ".3do"
	original, err := shared.ReadFileLimit(modelPath, 1<<22)
	if err != nil {
		t.Fatal(err)
	}
	overlay := t.TempDir()
	writeOverlayFile(t, overlay, modelPath, original)
	fs := mountWithOverlay(t, overlay)

	first, err := NewSkirmishWithEntryOptions(fs, cat, frozenSkirmishConfig(), SkirmishEntryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	inputs := first.frozenSimulationInputs()
	if inputs == nil {
		t.Fatal("skirmish composed without frozen inputs")
	}
	if first.Catalog != inputs.Catalog() || first.simArt != inputs.SimArt() {
		t.Fatal("composition does not run on the frozen catalog and animation table")
	}
	admitted := inputs.Digest()
	frozenModel, ok := inputs.Model(def.ObjectName)
	if !ok {
		t.Fatalf("lab product %q model was not frozen", def.UnitName)
	}
	for _, u := range first.Units.IterSliced() {
		if u != nil && u.Alive && u.Def == def {
			t.Fatalf("%s exists at entry; the test needs a unit no one has built", def.UnitName)
		}
	}
	for step := 0; step < 300; step++ {
		first.Step(first.Clock.ScaledAnchor + 1)
	}
	if got := inputs.UncapturedLookups(); len(got) != 0 {
		t.Fatalf("battle read content its capture had not frozen: %v", got)
	}

	// Move the root piece without changing its height, so the edited file is
	// still the model the catalog was compiled with.
	edited := append([]byte(nil), original...)
	x := int32(binary.LittleEndian.Uint32(edited[16:])) + 5<<16
	binary.LittleEndian.PutUint32(edited[16:], uint32(x))
	writeOverlayFile(t, overlay, modelPath, edited)

	var owner uint8
	var at *units.Unit
	for _, u := range first.Units.IterSliced() {
		if u != nil && u.Alive && u.Def != nil && u.Def.Commander {
			owner, at = u.Owner, u
			break
		}
	}
	if at == nil {
		t.Fatal("no commander to build beside")
	}
	h, err := first.Units.Create(def, owner, at.X, at.Y, at.Z)
	if err != nil {
		t.Fatalf("create the never-built unit: %v", err)
	}
	built := first.Units.Unit(h)
	if built == nil || built.COBBinding() == nil || built.COBBinding().Model != frozenModel {
		t.Fatal("a unit first built after the edit did not bind the admitted model")
	}
	if inputs.Digest() != admitted {
		t.Fatal("the running battle's identity moved")
	}
	if got := inputs.UncapturedLookups(); len(got) != 0 {
		t.Fatalf("late creation read content its capture had not frozen: %v", got)
	}

	second, err := NewSkirmishWithEntryOptions(fs, cat, frozenSkirmishConfig(), SkirmishEntryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	next := second.frozenSimulationInputs()
	if next.Digest() == admitted {
		t.Fatal("a battle admitted after the edit kept the old identity")
	}
	editedModel, ok := next.Model(def.ObjectName)
	if !ok || editedModel.Pieces[editedModel.Root].Translate == frozenModel.Pieces[frozenModel.Root].Translate {
		t.Fatal("the second battle did not freeze the edited model")
	}
}

// A host's precompiled animation table with the battle's content is used as
// is and freezes to the same identity as a table compiled at entry. Once a
// bank it read has been edited, battle entry compiles its own instead.
func TestFrozenSkirmishPrecompiledSimArtRetail(t *testing.T) {
	cat, shared := retailcat.Shared(t)
	bank, err := shared.ReadFileLimit(content.EffectBankPath(content.DefaultEffectBank), content.EffectBankMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	overlay := t.TempDir()
	logical := content.EffectBankPath(content.DefaultEffectBank)
	writeOverlayFile(t, overlay, logical, bank)
	fs := mountWithOverlay(t, overlay)

	compiled, err := NewSkirmishWithEntryOptions(fs, cat, frozenSkirmishConfig(), SkirmishEntryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	precompiled := content.CompileSimArt(fs, cat)
	supplied, err := NewSkirmishWithEntryOptions(fs, cat, frozenSkirmishConfig(), SkirmishEntryOptions{SimArt: precompiled})
	if err != nil {
		t.Fatal(err)
	}
	if supplied.simArt != precompiled {
		t.Fatal("a matching precompiled table was not used")
	}
	if compiled.frozenSimulationInputs().Digest() != supplied.frozenSimulationInputs().Digest() {
		t.Fatal("compiled and precompiled tables with equal content disagree")
	}

	// Lengthen the first frame of the bank's first entry: the GAF entry
	// header is 40 bytes, then each frame reference is an offset and a hold
	// [fmt gaf].
	edited := append([]byte(nil), bank...)
	entry := binary.LittleEndian.Uint32(edited[12:])
	hold := entry + 40 + 4
	binary.LittleEndian.PutUint32(edited[hold:], binary.LittleEndian.Uint32(edited[hold:])+7)
	writeOverlayFile(t, overlay, logical, edited)

	stale, err := NewSkirmishWithEntryOptions(fs, cat, frozenSkirmishConfig(), SkirmishEntryOptions{SimArt: precompiled})
	if err != nil {
		t.Fatal(err)
	}
	if stale.simArt == precompiled {
		t.Fatal("a table compiled before the bank was edited was used")
	}
	if stale.frozenSimulationInputs().Digest() == supplied.frozenSimulationInputs().Digest() {
		t.Fatal("an edited effect bank did not change the identity")
	}
}
