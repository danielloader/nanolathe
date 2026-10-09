package world

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/numeric"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func checkpointTerrainFixture(t *testing.T, tidal string) (*content.SimulationInputs, *vfs.FS, *content.SimulationSources) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "maps"), 0o755); err != nil {
		t.Fatal(err)
	}
	data := legacyTNTBytes(t)
	// Give the row-order tests a distinct authored corner and floor range.
	attrs := int(binary.LittleEndian.Uint32(data[0x10:]))
	data[attrs+(9*16+6)*8] = 24
	for _, name := range []string{"legacy.tnt", "alias.tnt"} {
		if err := os.WriteFile(filepath.Join(dir, "maps", name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ota := "[GlobalHeader]{ gravity=19; tidalstrength=" + tidal + "; waterdoesdamage=1; waterdamage=7; [Schema 0]{type=Network 1; surfacemetal=9;} }"
	if err := os.WriteFile(filepath.Join(dir, "maps", "legacy.ota"), []byte(ota), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.New()
	if err := fs.MountDirectory(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	// The requested display name deliberately differs from the resolved
	// terrain key, as a translated map selection can do.
	sources, err := content.CaptureSimulationSources(fs, "Display Name")
	if err != nil {
		t.Fatal(err)
	}
	maps, err := content.CompileMaps(sources.Filesystem())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sources.Filesystem().ReadFileLimit("maps/alias.tnt", 1<<20); err != nil {
		t.Fatal(err)
	}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{
		Catalog: &content.Catalog{Maps: maps}, MapOTA: "maps/legacy.ota", MapTNT: "maps/legacy.tnt",
	})
	if err != nil {
		t.Fatal(err)
	}
	return inputs, fs, sources
}

func loadCheckpointTerrain(t *testing.T, inputs *content.SimulationInputs, key string) *Terrain {
	t.Helper()
	terrain, err := Load(inputs.Filesystem(), inputs.Catalog(), key)
	if err != nil {
		t.Fatal(err)
	}
	return terrain
}

func TestCheckpointTerrainInputsSealFinalLoad(t *testing.T) {
	inputs, _, _ := checkpointTerrainFixture(t, "0.625")
	if err := inputs.ValidateCheckpointInputs(); err != nil {
		t.Fatal(err)
	}
	terrain := loadCheckpointTerrain(t, inputs, " Legacy ")
	if err := terrain.ValidateCheckpointInputs(inputs, " Legacy "); err != nil {
		t.Fatal(err)
	}
	if terrain.checkpointInputs == nil || terrain.losBuildCount != 1 || !terrain.voidSwept || terrain.PlayRight != 224 || terrain.PlayBottom != 128 {
		t.Fatal("snapshot did not follow complete load post-processing")
	}
	if terrain.AuthoredGravity != 112 || terrain.OTAGravity != 19 || terrain.Gravity != numeric.Fixed(112*65536/900) || terrain.Tidal != 0.625 {
		t.Fatal("load-time environment changed")
	}
	if err := terrain.ValidateCheckpointInputs(inputs, inputs.MapName()); err == nil {
		t.Fatal("display name substituted for requested terrain key")
	}
	if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
		t.Fatal("capture normalized the original requested terrain key")
	}
	copy := *terrain
	if err := copy.ValidateCheckpointInputs(inputs, " Legacy "); err == nil {
		t.Fatal("copied terrain inherited original load proof")
	}
	if err := (&Terrain{CellW: 16, CellH: 16, Plot: slices.Clone(terrain.Plot)}).ValidateCheckpointInputs(inputs, " Legacy "); err == nil {
		t.Fatal("hand-built terrain admitted")
	}
	if err := (*Terrain)(nil).ValidateCheckpointInputs(inputs, " Legacy "); err == nil {
		t.Fatal("nil terrain admitted")
	}
	if err := terrain.ValidateCheckpointInputs(nil, " Legacy "); err == nil {
		t.Fatal("nil inputs admitted")
	}
}

func TestCheckpointTerrainInputsRefuseGeometryAndEnvironmentChanges(t *testing.T) {
	inputs, _, _ := checkpointTerrainFixture(t, "0.625")
	for _, tc := range []struct {
		name   string
		change func(*Terrain)
	}{
		{"same-area dimensions", func(s *Terrain) { s.CellW, s.CellH = 32, 8 }},
		{"version selects schema seed", func(s *Terrain) { s.Version = VersionCanonical }},
		{"sea level", func(s *Terrain) { s.SeaLevel++ }},
		{"gravity high word", func(s *Terrain) { s.Gravity += 1 << 32 }},
		{"authored smoke gravity", func(s *Terrain) { s.AuthoredGravity++ }},
		{"air strike gravity", func(s *Terrain) { s.OTAGravity++ }},
		{"lava explosion selection", func(s *Terrain) { s.LavaWorld = !s.LavaWorld }},
		{"water damage gate", func(s *Terrain) { s.WaterDoesDamage = 0 }},
		{"water damage amount", func(s *Terrain) { s.WaterDamage++ }},
		{"wind minimum", func(s *Terrain) { s.WindMin++ }},
		{"wind maximum", func(s *Terrain) { s.WindMax++ }},
		{"tidal adjacent float", func(s *Terrain) { s.Tidal = math.Nextafter32(s.Tidal, 1) }},
		{"play width", func(s *Terrain) { s.PlayRight++ }},
		{"play height", func(s *Terrain) { s.PlayBottom++ }},
		{"plot missing row", func(s *Terrain) { s.Plot = s.Plot[:len(s.Plot)-1] }},
		{"plot row order", func(s *Terrain) { s.Plot[0], s.Plot[9*16+6] = s.Plot[9*16+6], s.Plot[0] }},
		{"height", func(s *Terrain) { s.Plot[0].SetHeight(11) }},
		{"floor minimum", func(s *Terrain) { s.Plot[0].SetMinHeight(9) }},
		{"floor maximum", func(s *Terrain) { s.Plot[0].SetMaxHeight(11) }},
		{"LOS reader disabled", func(s *Terrain) { s.losBuildCount = 0 }},
		{"LOS build count", func(s *Terrain) { s.losBuildCount++ }},
		{"LOS word", func(s *Terrain) { s.losWords[0] ^= 0x100 }},
		{"LOS length", func(s *Terrain) { s.losWords = s.losWords[:len(s.losWords)-1] }},
		{"LOS absent", func(s *Terrain) { s.losWords = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			terrain := loadCheckpointTerrain(t, inputs, "legacy")
			tc.change(terrain)
			if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
				t.Fatal("changed immutable input admitted")
			}
		})
	}
	// A replacement container with exactly the same values is harmless. The
	// object identity proof applies to Terrain, not its backing allocations.
	terrain := loadCheckpointTerrain(t, inputs, "legacy")
	terrain.Plot, terrain.losWords = slices.Clone(terrain.Plot), slices.Clone(terrain.losWords)
	if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointTerrainInputsAllowMutablePayloadAndExcludedPixels(t *testing.T) {
	inputs, _, _ := checkpointTerrainFixture(t, "0.625")
	terrain := loadCheckpointTerrain(t, inputs, "legacy")
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	before := worldCheckpointBytes(t, terrain, keys)
	if err := terrain.ApplySchema(inputs.Catalog().Maps["legacy"], 0); err != nil {
		t.Fatal(err)
	}
	terrain.RunMissionFeaturePass(func() {
		cell := terrain.PlotAt(6, 9)
		cell.SetFeature(PlotFeatureFringe)
		cell.SetAnchorWord(3)
		cell.SetMetal(41)
		cell.SetOccupantA(-2)
		cell.SetOccupantB(17)
		cell.SetFlagByte(0xff)
	})
	terrain.FeatureNames = append(terrain.FeatureNames, "runtime name")
	terrain.FeatureDefs = append(terrain.FeatureDefs, nil)
	terrain.staticObstacleRevision++
	terrain.TileIndices[0]++
	terrain.TileSet[0][0]++
	if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, worldCheckpointBytes(t, terrain, keys)) {
		t.Fatal("mutable changes disappeared from owner payload")
	}
	if terrain.voidPassActive || !terrain.metalSeeded {
		t.Fatal("ordinary schema/mission entry behavior changed")
	}
}

type checkpointTerrainSource struct {
	vfs.FSOps
	forbid *bool
	// A value of this type cannot be compared through an interface.
	reads []string
}

func (s checkpointTerrainSource) ReadFileLimit(name string, limit int64) ([]byte, error) {
	if *s.forbid {
		panic("capture read the filesystem")
	}
	if len(s.reads) != 0 {
		s.reads[0] = name
	}
	return s.FSOps.ReadFileLimit(name, limit)
}

func TestCheckpointTerrainInputsRequireActualFrozenSource(t *testing.T) {
	inputs, live, sources := checkpointTerrainFixture(t, "0.625")
	terrain := loadCheckpointTerrain(t, inputs, "legacy")
	foreign, _, _ := checkpointTerrainFixture(t, "0.625")
	if err := terrain.ValidateCheckpointInputs(foreign, "legacy"); err == nil {
		t.Fatal("foreign input owner admitted")
	}
	copyInputs := *inputs
	if err := terrain.ValidateCheckpointInputs(&copyInputs, "legacy"); err == nil {
		t.Fatal("copied input owner admitted")
	}
	catCopy := *inputs.Catalog()
	for _, tc := range []struct {
		name string
		fs   vfs.FSOps
		cat  *content.Catalog
	}{
		{"live source", live, inputs.Catalog()},
		{"pre-freeze capture view", sources.Filesystem(), inputs.Catalog()},
		{"copied catalog", inputs.Filesystem(), &catCopy},
		{"nil catalog", inputs.Filesystem(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loaded, err := Load(tc.fs, tc.cat, "legacy")
			if err != nil {
				t.Fatal(err)
			}
			if err := loaded.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
				t.Fatal("foreign source installation admitted")
			}
		})
	}
	alias := loadCheckpointTerrain(t, inputs, "alias")
	if err := alias.ValidateCheckpointInputs(inputs, "alias"); err == nil {
		t.Fatal("unselected TNT path admitted")
	}
	forbid := false
	source := checkpointTerrainSource{FSOps: inputs.Filesystem(), forbid: &forbid, reads: make([]string, 1)}
	wrapped, err := Load(source, inputs.Catalog(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if source.reads[0] != "maps/legacy.tnt" {
		t.Fatal("fixture did not load through actual wrapper")
	}
	forbid = true
	if err := wrapped.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
		t.Fatal("foreign wrapper admitted")
	}
}

func TestCheckpointTerrainInputsRetainExactTidalBits(t *testing.T) {
	for _, tc := range []struct {
		text string
		bits uint32
	}{{"0", 0}, {"-0", 0x80000000}, {"0.625", 0x3f200000}} {
		t.Run(tc.text, func(t *testing.T) {
			inputs, _, _ := checkpointTerrainFixture(t, tc.text)
			terrain := loadCheckpointTerrain(t, inputs, "legacy")
			if math.Float32bits(terrain.Tidal) != tc.bits {
				t.Fatalf("authored tidal bits = %08x, want %08x", math.Float32bits(terrain.Tidal), tc.bits)
			}
			if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err != nil {
				t.Fatal(err)
			}
			terrain.Tidal = math.Float32frombits(tc.bits ^ 0x80000000)
			if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
				t.Fatal("changed sign bit admitted")
			}
		})
	}
	// A malformed supplied catalog can already contain NaN at load. The leaf
	// refuses that value even when it still matches its construction snapshot;
	// root's separate frozen-content validation also refuses this catalog edit.
	inputs, _, _ := checkpointTerrainFixture(t, "0.625")
	inputs.Catalog().Maps["legacy"].TidalStrength = math.NaN()
	terrain := loadCheckpointTerrain(t, inputs, "legacy")
	if math.Float32bits(terrain.Tidal)&0x7fffffff <= 0x7f800000 {
		t.Fatal("fixture did not retain supplied NaN")
	}
	if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
		t.Fatal("unchanged NaN admitted")
	}
}

func TestCheckpointTerrainInputsValidationIsPureAndAllocationFree(t *testing.T) {
	inputs, _, _ := checkpointTerrainFixture(t, "0.625")
	terrain := loadCheckpointTerrain(t, inputs, "legacy")
	snapshot := terrain.checkpointInputs
	plot, words := slices.Clone(terrain.Plot), slices.Clone(terrain.losWords)
	if got := testing.AllocsPerRun(100, func() {
		if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err != nil {
			t.Fatal(err)
		}
	}); got != 0 {
		t.Fatalf("validation allocated %g times", got)
	}
	if !slices.Equal(terrain.Plot, plot) || !slices.Equal(terrain.losWords, words) || terrain.checkpointInputs != snapshot || terrain.losBuildCount != 1 || len(inputs.UncapturedLookups()) != 0 {
		t.Fatal("validation changed terrain, source or load metadata")
	}
	terrain.Plot[0].SetHeight(99)
	terrain.losWords[0]++
	for range 2 {
		if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err == nil {
			t.Fatal("mutated repeated capture admitted")
		}
	}
	if terrain.Plot[0].Height() != 99 || terrain.losWords[0] != words[0]+1 || snapshot.heights[0][0] != plot[0].Height() || snapshot.losWords[0] != words[0] {
		t.Fatal("invalid capture repaired values or snapshot shared their backing")
	}
	terrain.Plot[0], terrain.losWords[0] = plot[0], words[0]
	if err := terrain.ValidateCheckpointInputs(inputs, "legacy"); err != nil {
		t.Fatal(err)
	}
}
