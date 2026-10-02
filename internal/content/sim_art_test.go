package content

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

// simArtFixture authors two banks, with no retail bytes.
//
// `anims/trees.gaf` holds a burn sequence whose three frames carry distinct
// geometry and distinct authored delays (3, 0, 2) and a one-frame death
// sequence. The zero delay is the interesting one: [05 R-FEAT-01 §10] holds
// every frame for max(delay, 1) visits, so the burn entry lasts 3 + 1 + 2 = 6
// visits, not 5.
//
// `anims/fx.gaf` is the default effect bank, with a five-frame smoke entry and
// a three-frame flame entry. The fixture and the expected numbers below are
// deliberately the same shape the presentation cache's own cadence test uses,
// because the two readings of one entry must never come apart.
func simArtFixture(t *testing.T) *vfs.FS {
	t.Helper()
	pixels := func(w, h int) []byte {
		p := make([]byte, w*h)
		for i := range p {
			p[i] = 1
		}
		return p
	}
	frame := func(w, h int, xoff, yoff int16, dur uint32) formats.GAFWriteFrame {
		return formats.GAFWriteFrame{Width: uint16(w), Height: uint16(h), XOffset: xoff, YOffset: yoff, Duration: dur, Pixels: pixels(w, h)}
	}
	trees, err := formats.EncodeGAF([]formats.GAFWriteEntry{
		{Name: "treeburn", Frames: []formats.GAFWriteFrame{
			frame(20, 12, 7, 5, 3),
			frame(5, 7, 2, 3, 0),
			frame(16, 24, -3, 9, 2),
		}},
		{Name: "treedie", Frames: []formats.GAFWriteFrame{frame(8, 8, 1, 1, 4)}},
		{Name: "treedieshad", Frames: []formats.GAFWriteFrame{frame(8, 8, 1, 1, 4)}},
	})
	if err != nil {
		t.Fatalf("encode feature gaf fixture: %v", err)
	}
	fx, err := formats.EncodeGAF([]formats.GAFWriteEntry{
		{Name: "smoke 1", Frames: []formats.GAFWriteFrame{
			frame(4, 4, 0, 0, 2), frame(4, 4, 0, 0, 2), frame(4, 4, 0, 0, 2),
			frame(4, 4, 0, 0, 2), frame(4, 4, 0, 0, 2),
		}},
		{Name: "flamestream", Frames: []formats.GAFWriteFrame{
			frame(3, 3, 0, 0, 1), frame(3, 3, 0, 0, 1), frame(3, 3, 0, 0, 1),
		}},
	})
	if err != nil {
		t.Fatalf("encode effect gaf fixture: %v", err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "anims"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anims", "trees.gaf"), trees, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anims", "fx.gaf"), fx, 0o644); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err := fs.MountDirectory(dir, 1); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	return fs
}

func simArtFixtureCatalog() *Catalog {
	return &Catalog{Features: map[string]*FeatureDef{
		"tree1": {Filename: "trees", SeqName: "tree1", SeqNameBurn: "treeburn", SeqNameDie: "treedie", SeqNameDieShad: "treedieshad"},
		// A definition whose file does not exist must compile to a miss, not
		// to a load attempt at simulation time.
		"ghost": {Filename: "nosuchfile", SeqNameDie: "ghostdie"},
	}}
}

// TestSimArtWalksTheCursorCadence locks the table against [05 R-FEAT-01 §10]:
// each frame holds for max(delay, 1) visits, the entry's lifetime is the sum of
// those holds, and a visit at or past the end reports the last frame — where
// the cursor sits on the visit that finishes it.
func TestSimArtWalksTheCursorCadence(t *testing.T) {
	art := CompileSimArt(simArtFixture(t), simArtFixtureCatalog())
	type geom struct{ w, h, xoff, yoff int32 }
	first := geom{20, 12, 7, 5}
	second := geom{5, 7, 2, 3}
	third := geom{16, 24, -3, 9}
	for _, tc := range []struct {
		visit int32
		want  geom
	}{
		{-1, first}, {0, first}, {1, first}, {2, first},
		{3, second},
		{4, third}, {5, third},
		// Past the end: the cursor sits on the frame that finishes the record.
		{6, third}, {99, third},
	} {
		w, h, xoff, yoff, visits, ok := art.FeatureSequence("trees", "treeburn", tc.visit)
		if !ok {
			t.Fatalf("visit %d: burn sequence did not resolve", tc.visit)
		}
		if got := (geom{w, h, xoff, yoff}); got != tc.want {
			t.Fatalf("visit %d geometry %+v, want %+v", tc.visit, got, tc.want)
		}
		if visits != 6 {
			t.Fatalf("visit %d lifetime %d, want 3+1+2 = 6", tc.visit, visits)
		}
	}
	if _, _, _, _, visits, ok := art.FeatureSequence("treeS", "TreeDie", 0); !ok || visits != 4 {
		t.Fatalf("death lifetime %d ok=%v, want 4 and a case-insensitive hit", visits, ok)
	}
}

// TestSimArtReportsUnknownRatherThanAFallback locks rule 1 at the seam: a file,
// a sequence or an entry that does not resolve reports ok=false, so the
// consumers keep their documented "unknown" behaviour instead of receiving an
// invented lifetime [I9].
func TestSimArtReportsUnknownRatherThanAFallback(t *testing.T) {
	art := CompileSimArt(simArtFixture(t), simArtFixtureCatalog())
	for _, tc := range []struct{ file, seq string }{
		{"trees", "nosuchseq"},
		{"nosuchfile", "ghostdie"},
		{"trees", "treereclamate"}, // authored by no definition, so not compiled
		{"", ""},
		{"trees", ""},
	} {
		if _, _, _, _, _, ok := art.FeatureSequence(tc.file, tc.seq, 0); ok {
			t.Fatalf("%q|%q resolved, want unknown", tc.file, tc.seq)
		}
	}
	var nilArt *SimArt
	if _, _, _, _, _, ok := nilArt.FeatureSequence("trees", "treeburn", 0); ok {
		t.Fatal("a nil table resolved a sequence")
	}
	if _, ok := nilArt.EffectEntryFrameCount("", "smoke 1"); ok {
		t.Fatal("a nil table resolved an effect entry")
	}
}

func TestSimArtCompilesEventShadowsWithoutGivingThemCursorTiming(t *testing.T) {
	art := CompileSimArt(simArtFixture(t), simArtFixtureCatalog())
	if _, _, _, _, _, ok := art.FeatureSequence("trees", "treedieshad", 0); !ok {
		t.Fatal("authored event shadow did not resolve")
	}
	if _, _, _, _, _, ok := art.FeatureSequence("trees", "missing-shadow", 0); ok {
		t.Fatal("missing event shadow resolved")
	}
}

// TestSimArtSuppressesAValidSequenceWhenAnotherEntryIsCorrupt locks the
// whole-bank policy at the content boundary. Metadata has no pixel planes, but
// it still validates every frame payload, so it cannot let one requested entry
// silently bypass an unrelated malformed raw frame [fmt gaf].
func TestSimArtSuppressesAValidSequenceWhenAnotherEntryIsCorrupt(t *testing.T) {
	for _, tc := range []struct {
		name      string
		unrelated formats.GAFWriteFrame
		corrupt   func([]byte, uint32)
	}{
		{
			name:      "raw",
			unrelated: formats.GAFWriteFrame{Width: 1, Height: 1, Pixels: []byte{3}},
			corrupt: func(bank []byte, frameOffset uint32) {
				binary.LittleEndian.PutUint32(bank[frameOffset+16:frameOffset+20], uint32(len(bank)))
			},
		},
		{
			name:      "rle",
			unrelated: formats.GAFWriteFrame{Width: 1, Height: 1, Pixels: []byte{3}, Transparent: []bool{true}},
			corrupt: func(bank []byte, frameOffset uint32) {
				dataOffset := binary.LittleEndian.Uint32(bank[frameOffset+16 : frameOffset+20])
				bank[dataOffset+2] = 1 // a zero-length RLE skip run
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bank, err := formats.EncodeGAF([]formats.GAFWriteEntry{
				{Name: "treeburn", Frames: []formats.GAFWriteFrame{{Width: 2, Height: 1, XOffset: 3, YOffset: -4, Duration: 7, Pixels: []byte{1, 2}}}},
				{Name: "unrelated", Frames: []formats.GAFWriteFrame{tc.unrelated}},
			})
			if err != nil {
				t.Fatal(err)
			}
			entryOffset := binary.LittleEndian.Uint32(bank[16:20])
			frameOffset := binary.LittleEndian.Uint32(bank[entryOffset+40 : entryOffset+44])
			tc.corrupt(bank, frameOffset)
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, "anims"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "anims", "trees.gaf"), bank, 0o644); err != nil {
				t.Fatal(err)
			}
			fs := vfs.New()
			if err := fs.MountDirectory(dir, 1); err != nil {
				t.Fatal(err)
			}
			defer fs.Close()
			art := CompileSimArt(fs, &Catalog{Features: map[string]*FeatureDef{
				"tree": {Filename: "trees", SeqNameBurn: "treeburn"},
			}})
			if _, _, _, _, _, ok := art.FeatureSequence("trees", "treeburn", 0); ok {
				t.Fatal("valid sequence survived an unrelated corrupt entry")
			}
		})
	}
}

// TestSimArtEffectEntryFrameCount locks the length the strip families draw a
// puff's own last frame against [03 R-STRIP-01 §2][06 R-WFX-01 §5], and the
// default bank an entry published without one names [06 R-WFX-01 §1].
func TestSimArtEffectEntryFrameCount(t *testing.T) {
	art := CompileSimArt(simArtFixture(t), simArtFixtureCatalog())
	for _, tc := range []struct {
		bank, entry string
		want        int
	}{
		{"", "smoke 1", 5},
		{"fx", "smoke 1", 5},
		{"FX", "SMOKE 1", 5},
		{"", "flamestream", 3},
	} {
		n, ok := art.EffectEntryFrameCount(tc.bank, tc.entry)
		if !ok || n != tc.want {
			t.Fatalf("%q|%q frame count %d ok=%v, want %d", tc.bank, tc.entry, n, ok, tc.want)
		}
	}
	for _, tc := range []struct{ bank, entry string }{
		{"", ""},
		{"", "smoke 2"},   // absent from this fixture bank
		{"other", "fire"}, // a bank the simulation never names, so never compiled
	} {
		if _, ok := art.EffectEntryFrameCount(tc.bank, tc.entry); ok {
			t.Fatalf("%q|%q resolved, want unknown", tc.bank, tc.entry)
		}
	}
}

// TestSimArtRetainsNoFilesystem locks the property that makes the table safe to
// read from an authoritative phase: compilation happens up front and nothing
// afterwards can turn a miss into a load (I4). Unmounting the fixture must not
// change a single answer.
func TestSimArtRetainsNoFilesystem(t *testing.T) {
	fs := simArtFixture(t)
	art := CompileSimArt(fs, simArtFixtureCatalog())
	fs.Close()
	if _, _, _, _, visits, ok := art.FeatureSequence("trees", "treeburn", 0); !ok || visits != 6 {
		t.Fatalf("burn lifetime %d ok=%v after unmount, want 6", visits, ok)
	}
	if n, ok := art.EffectEntryFrameCount("", "smoke 1"); !ok || n != 5 {
		t.Fatalf("smoke frame count %d ok=%v after unmount, want 5", n, ok)
	}
	if _, _, _, _, _, ok := art.FeatureSequence("trees", "nosuchseq", 0); ok {
		t.Fatal("a miss resolved after unmount")
	}
}

// The fixed pool reads all default and weapon-named bank entries, not just
// entries selected by a particular weapon [06 R-WFX-01 §1].
func TestSimArtEffectHoldsFromSortedBanks(t *testing.T) {
	frame := func(hold uint32) formats.GAFWriteFrame {
		return formats.GAFWriteFrame{Width: 1, Height: 1, Duration: hold, Pixels: []byte{1}}
	}
	data, err := formats.EncodeGAF([]formats.GAFWriteEntry{
		{Name: "primary", Frames: []formats.GAFWriteFrame{frame(0), frame(3), frame(^uint32(0))}},
		{Name: "other", Frames: []formats.GAFWriteFrame{frame(2)}},
		{Name: "PRIMARY", Frames: []formats.GAFWriteFrame{frame(9)}},
		{Name: "empty", Frames: []formats.GAFWriteFrame{frame(1)}},
		{Name: "EMPTY", Frames: []formats.GAFWriteFrame{frame(9)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Author a zero-frame first match; the second same-name entry must not
	// supply a substitute [fmt gaf][06 R-WFX-01 §1].
	emptyOffset := binary.LittleEndian.Uint32(data[12+3*4:])
	binary.LittleEndian.PutUint16(data[emptyOffset:], 0)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "anims"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, bank := range []string{"fx", "land", "water", "lava", "unused"} {
		if err := os.WriteFile(filepath.Join(root, "anims", bank+".gaf"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	fs := vfs.New()
	if err := fs.MountDirectory(root, 0); err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	ordered := &simArtReadOrder{FSOps: fs}
	art := CompileSimArt(ordered, &Catalog{Weapons: map[string]*WeaponDef{
		"z":   {ExplosionGaf: " LAND ", WaterExplosionGaf: "water", LavaExplosionGaf: "lava"},
		"a":   {ExplosionGaf: "land", WaterExplosionGaf: "missing"},
		"nil": nil,
	}})
	if want := []string{"anims/fx.gaf", "anims/land.gaf", "anims/lava.gaf", "anims/missing.gaf", "anims/water.gaf"}; !slices.Equal(ordered.names, want) {
		t.Fatalf("compiled banks = %v, want sorted unique %v", ordered.names, want)
	}
	fs.Close() // no per-tick VFS reads
	for _, bank := range []string{"", "FX", "land", "water", "LAVA"} {
		holds, ok := art.EffectEntryHolds(bank, "PRIMARY")
		if !ok || !slices.Equal(holds, []int32{1, 3, 1}) {
			t.Fatalf("%q holds = %v, ok=%v", bank, holds, ok)
		}
		holds[0] = 900
		again, _ := art.EffectEntryHolds(bank, "primary")
		if again[0] != 1 {
			t.Fatal("caller altered immutable content")
		}
		if other, ok := art.EffectEntryHolds(bank, "other"); !ok || !slices.Equal(other, []int32{2}) {
			t.Fatal("unnamed bank entry omitted")
		}
		if _, ok := art.EffectEntryHolds(bank, "empty"); ok {
			t.Fatal("empty first match replaced by duplicate")
		}
	}
	for _, key := range [][2]string{{"unused", "primary"}, {"missing", "primary"}, {"fx", "absent"}, {"fx", ""}, {"fx", " primary "}} {
		if holds, ok := art.EffectEntryHolds(key[0], key[1]); ok || holds != nil {
			t.Fatalf("%v resolved to %v", key, holds)
		}
	}
	for _, empty := range []*SimArt{nil, {}, CompileSimArt(nil, nil)} {
		if _, ok := empty.EffectEntryHolds("", "primary"); ok {
			t.Fatal("empty table invented timing")
		}
	}
}

// simArtEmptyCanvasBank authors one entry of `frames` distinct side×side RLE
// frames whose rows are all zero-length records [fmt gaf]. Each frame costs
// 2·side bytes of file but counts side² pixels against the validation budget,
// so a file of a few kilobytes can exceed the eager loader's pixel budget.
func simArtEmptyCanvasBank(name string, frames int, side uint16, hold uint32) []byte {
	const header, entryHeader, frameHeader = 12, 40, 24
	le := binary.LittleEndian
	entry := header + 4
	refs := entry + entryHeader
	first := refs + 8*frames
	frameBytes := frameHeader + 2*int(side)
	data := make([]byte, first+frames*frameBytes)
	le.PutUint32(data[0:], 0x00010100)
	le.PutUint32(data[4:], 1)
	le.PutUint32(data[header:], uint32(entry))
	le.PutUint16(data[entry:], uint16(frames))
	copy(data[entry+8:entry+entryHeader], name)
	for i := range frames {
		at := first + i*frameBytes
		le.PutUint32(data[refs+8*i:], uint32(at))
		le.PutUint32(data[refs+8*i+4:], hold)
		le.PutUint16(data[at:], side)
		le.PutUint16(data[at+2:], side)
		data[at+9] = 1 // row-length-prefixed records; every length stays zero
		le.PutUint32(data[at+16:], uint32(at+frameHeader))
	}
	return data
}

// A bank above the eager default budget but within the shared effect-bank
// policy compiles its holds, as the client draws it; a bank that is absent or
// refused is recorded as a diagnostic in sorted bank order instead of vanishing.
func TestSimArtEffectBankPolicyAndDiagnostics(t *testing.T) {
	// Nine distinct 4096×4096 frames: 144 Mi pixels, over the default 128 Mi
	// and under the policy's 512 Mi.
	big := simArtEmptyCanvasBank("Boom", 9, 4096, 3)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "anims"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"big.gaf":    big,
		"broken.gaf": big[:len(big)/2],
		"fx.gaf":     simArtEmptyCanvasBank("smoke 1", 1, 1, 2),
	} {
		if err := os.WriteFile(filepath.Join(root, "anims", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs := vfs.New()
	defer fs.Close()
	if err := fs.MountDirectory(root, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := formats.LoadGAFMetadataFile(fs, "anims/big.gaf"); err == nil || !strings.Contains(err.Error(), "aggregate decoded pixels exceed limit") {
		t.Fatalf("fixture must exceed the eager default budget; default load error = %v", err)
	}
	art := CompileSimArt(fs, &Catalog{Weapons: map[string]*WeaponDef{
		"a": {ExplosionGaf: "missing", WaterExplosionGaf: "BIG"},
		"b": {ExplosionGaf: "broken"},
	}})
	if holds, ok := art.EffectEntryHolds("big", "boom"); !ok || !slices.Equal(holds, []int32{3, 3, 3, 3, 3, 3, 3, 3, 3}) {
		t.Fatalf("large bank holds = %v, ok=%v; want nine holds of 3 under the shared policy", holds, ok)
	}
	if holds, ok := art.EffectEntryHolds("", "smoke 1"); !ok || !slices.Equal(holds, []int32{2}) {
		t.Fatalf("default bank holds = %v, ok=%v", holds, ok)
	}
	diagnostics := art.Diagnostics()
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v, want the broken and the missing bank", diagnostics)
	}
	broken, missing := diagnostics[0], diagnostics[1]
	if broken.Bank != "broken" || broken.Path != "anims/broken.gaf" || len(broken.Providers) != 1 || broken.Reason == "" {
		t.Fatalf("broken bank diagnostic = %+v", broken)
	}
	if missing.Bank != "missing" || missing.Path != "anims/missing.gaf" || len(missing.Providers) != 0 || missing.Reason == "" {
		t.Fatalf("missing bank diagnostic = %+v", missing)
	}
	if got := missing.String(); !strings.HasPrefix(got, "nanolathe: effect bank timing unavailable: logical path anims/missing.gaf, providers searched [], expected ") {
		t.Fatalf("diagnostic text %q is not in the engine's diagnostic shape", got)
	}
	diagnostics[0].Bank = "changed"
	if art.Diagnostics()[0].Bank != "broken" {
		t.Fatal("caller altered the compiled diagnostics")
	}
	if (*SimArt)(nil).Diagnostics() != nil || CompileSimArt(fs, nil).Diagnostics() != nil {
		t.Fatal("a table with nothing unreadable reported diagnostics")
	}
}

type simArtReadOrder struct {
	vfs.FSOps
	names []string
}

func (fs *simArtReadOrder) ReadFileLimit(name string, limit int64) ([]byte, error) {
	fs.names = append(fs.names, name)
	return fs.FSOps.ReadFileLimit(name, limit)
}
