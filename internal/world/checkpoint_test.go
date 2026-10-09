package world

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func worldCheckpointKeys(t *testing.T, defs ...*content.FeatureDef) *content.CheckpointKeys {
	t.Helper()
	cat := &content.Catalog{Features: make(map[string]*content.FeatureDef)}
	for _, def := range defs {
		cat.Features[def.CanonicalKey] = def
	}
	fs := vfs.New()
	if err := fs.MountDirectory(t.TempDir(), 10); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func worldCheckpointBytes(t *testing.T, terrain *Terrain, keys *content.CheckpointKeys) []byte {
	t.Helper()
	c := NewCheckpointContext(keys)
	if added, err := terrain.CollectCheckpointReferences(c); err != nil || added != 0 {
		t.Fatalf("collect = %d, %v", added, err)
	}
	var out bytes.Buffer
	if err := terrain.WriteCheckpoint(checkpoint.NewEncoder(&out), c); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// This independent vector fixes lexical order, signed source widths, raw names,
// nil definition rows, and the exact exclusion mask for plot flags.
func TestCheckpointTerrainVector(t *testing.T) {
	terrain := &Terrain{CellW: 2, CellH: 1, Plot: make([]PlotCell, 2), voidSwept: true, metalSeeded: true,
		FeatureDefs: []*content.FeatureDef{nil}, FeatureNames: []string{"MiXeD", "\xff"}}
	a, b := &terrain.Plot[0], &terrain.Plot[1]
	a.SetAnchorWord(0x1234)
	a.SetFeature(PlotFeatureFringe)
	a.SetFlagByte(0xff)
	a.SetMetal(42)
	a.SetOccupantA(-2)
	a.SetOccupantB(32767)
	b.SetAnchorWord(0xff80)
	b.SetFeature(0xfffb)
	b.SetFlagByte(0x7c)
	b.SetOccupantA(-1)
	got := worldCheckpointBytes(t, terrain, worldCheckpointKeys(t))
	want, err := hex.DecodeString("00" + "0100000000" + "02000000050000004d6958654401000000ff" + "00" + "02000000" + "3412feff832afeffff7f" + "80fffbff0000ffff0000" + "01")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("payload\ngot  %x\nwant %x", got, want)
	}
}

func TestCheckpointTerrainRetainedWordsAndExclusions(t *testing.T) {
	keys := worldCheckpointKeys(t)
	makeTerrain := func() *Terrain { return &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true} }
	baseline := worldCheckpointBytes(t, makeTerrain(), keys)
	for name, edit := range map[string]func(*Terrain){
		"anchor":             func(v *Terrain) { v.Plot[0].SetAnchorWord(1) },
		"feature":            func(v *Terrain) { v.Plot[0].SetFeature(PlotFeatureVoid) },
		"instance flag":      func(v *Terrain) { v.Plot[0].SetFlagByte(1) },
		"yard flag":          func(v *Terrain) { v.Plot[0].SetFlagByte(2) },
		"unclassified flag":  func(v *Terrain) { v.Plot[0].SetFlagByte(0x80) },
		"metal":              func(v *Terrain) { v.Plot[0].SetMetal(1) },
		"ground":             func(v *Terrain) { v.Plot[0].SetOccupantA(-1) },
		"air":                func(v *Terrain) { v.Plot[0].SetOccupantB(-1) },
		"seeded":             func(v *Terrain) { v.metalSeeded = true },
		"definition nil row": func(v *Terrain) { v.FeatureDefs = append(v.FeatureDefs, nil) },
		"name":               func(v *Terrain) { v.FeatureNames = []string{"MiXeD"} },
	} {
		t.Run(name, func(t *testing.T) {
			v := makeTerrain()
			edit(v)
			if bytes.Equal(baseline, worldCheckpointBytes(t, v, keys)) {
				t.Fatal("retained mutation vanished")
			}
		})
	}
	v := makeTerrain()
	v.Plot[0].SetFlagByte(0x7c)
	v.Plot[0].SetHeight(73)
	v.Plot[0].SetMinHeight(71)
	v.Plot[0].SetMaxHeight(78)
	v.staticObstacleRevision = 998
	v.losWords = []uint16{123}
	v.losBuildCount = 7
	v.voidSweepUndo = []voidSweepUndoEntry{{index: 0, feature: PlotFeatureFringe}}
	before := *v
	before.Plot = append([]PlotCell(nil), v.Plot...)
	if got := worldCheckpointBytes(t, v, keys); !bytes.Equal(baseline, got) {
		t.Fatal("excluded state changed payload")
	}
	if !reflect.DeepEqual(before, *v) {
		t.Fatal("capture mutated terrain")
	}
}

func TestCheckpointTerrainDefinitionOrderAndNormalization(t *testing.T) {
	base := &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "bad"}, FootprintX: 0, FootprintZ: -2, Damage: -3, Metal: -4, Energy: -5}
	good := &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "good"}, FootprintX: 2, FootprintZ: 3}
	keys := worldCheckpointKeys(t, base, good)
	normalized := *base
	normalized.FootprintX = 1
	normalized.FootprintZ = 1
	normalized.Damage = 0
	normalized.Metal = 0
	normalized.Energy = 0
	terrain := &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true, FeatureDefs: []*content.FeatureDef{good, nil, &normalized}, FeatureNames: []string{"Good", "missing"}}
	if _, err := terrain.CheckpointFeature(keys, &normalized, nil); err == nil {
		t.Fatal("unrecorded same-name copy admitted")
	}
	terrain.RecordCheckpointFeatureNormalization(base, &normalized)
	ref, err := terrain.CheckpointFeature(keys, &normalized, nil)
	if err != nil || ref.Variant != 2 || ref.FootprintX != 1 || ref.FootprintZ != 1 || ref.Damage != 0 || ref.Metal != 0 || ref.Energy != 0 {
		t.Fatalf("normalized = %+v, %v", ref, err)
	}
	got := worldCheckpointBytes(t, terrain, keys)
	terrain.FeatureDefs[0], terrain.FeatureDefs[2] = terrain.FeatureDefs[2], terrain.FeatureDefs[0]
	if bytes.Equal(got, worldCheckpointBytes(t, terrain, keys)) {
		t.Fatal("definition order vanished")
	}
	if _, err := terrain.CheckpointFeature(keys, &normalized, good); err == nil {
		t.Fatal("conflicting base accepted")
	}
	normalized.Blocking = true
	if _, err := terrain.CheckpointFeature(keys, &normalized, nil); err == nil {
		t.Fatal("changed normalized copy accepted")
	}
	normalized.Blocking = false
	unretained := normalized
	terrain.RecordCheckpointFeatureNormalization(base, &unretained)
	if _, ok := terrain.checkpointFeatureBases[&unretained]; ok {
		t.Fatal("unretained copy kept alive")
	}
	if _, err := terrain.CheckpointFeature(keys, &unretained, base); err != nil {
		t.Fatalf("explicit instance base: %v", err)
	}
	terrain.FeatureDefs = nil
	if _, err := terrain.CheckpointFeature(keys, &normalized, nil); err == nil {
		t.Fatal("stale table relation accepted")
	}
}

type checkpointMobile struct{}

func (checkpointMobile) CellOccupant(cx, cz int32) uint16 { return 0 }

func TestCheckpointTerrainRefusals(t *testing.T) {
	keys := worldCheckpointKeys(t)
	for name, edit := range map[string]func(*Terrain, *CheckpointContext){
		"foreign definition": func(v *Terrain, c *CheckpointContext) { v.FeatureDefs = []*content.FeatureDef{{}} },
		"dimensions":         func(v *Terrain, c *CheckpointContext) { v.CellW = 2 },
		"sweep":              func(v *Terrain, c *CheckpointContext) { v.voidSwept = false },
		"restamp": func(v *Terrain, c *CheckpointContext) {
			v.SetClassRestamp(func(int32, int32, int16, int16) { t.Error("called restamp") })
		},
		"movers":          func(v *Terrain, c *CheckpointContext) { v.SetMovers(checkpointMobile{}) },
		"foreign terrain": func(v *Terrain, c *CheckpointContext) { c.Terrain = &Terrain{} },
		"keys":            func(v *Terrain, c *CheckpointContext) { c.Keys = nil },
	} {
		t.Run(name, func(t *testing.T) {
			v := &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true}
			c := NewCheckpointContext(keys)
			edit(v, c)
			if _, err := v.CollectCheckpointReferences(c); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint capture failed: logical path ") {
				t.Fatalf("collector error %v", err)
			}
			var out bytes.Buffer
			e := checkpoint.NewEncoder(&out)
			if err := v.WriteCheckpoint(e, c); err == nil || !strings.HasPrefix(err.Error(), "nanolathe: checkpoint owner stream: logical path ") || !strings.Contains(err.Error(), "expected") || e.Err() == nil {
				t.Fatalf("writer error %v", err)
			}
			if out.Len() != 0 {
				t.Fatal("failed preflight wrote bytes")
			}
		})
	}
}

func TestCheckpointTerrainEntryTransactionMarker(t *testing.T) {
	keys := worldCheckpointKeys(t)
	terrain := &Terrain{CellW: 4, CellH: 16, Plot: ExpandPlot(flat(4, 16, 0), 4, 16)}
	terrain.applyVoidSweep(false)
	c := NewCheckpointContext(keys)
	terrain.RunMissionFeaturePass(func() {
		if _, err := terrain.CollectCheckpointReferences(c); err == nil || !strings.Contains(err.Error(), "voidPassActive") {
			t.Fatalf("entry callback accepted: %v", err)
		}
		terrain.RunMissionFeaturePass(func() {
			if !terrain.voidPassActive {
				t.Fatal("nested pass lost marker")
			}
		})
		if !terrain.voidPassActive {
			t.Fatal("nested pass cleared outer marker")
		}
	})
	if terrain.voidPassActive {
		t.Fatal("pass kept marker")
	}
	worldCheckpointBytes(t, terrain, keys)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("callback panic swallowed")
			}
		}()
		terrain.RunMissionFeaturePass(func() { panic("fixture") })
	}()
	if terrain.voidPassActive {
		t.Fatal("panic unwinding kept marker")
	}
}

func TestCheckpointTerrainNormalizedDefinitionVector(t *testing.T) {
	base := &content.FeatureDef{DefinitionHeader: content.DefinitionHeader{CanonicalKey: "bad"}, FootprintX: 0, FootprintZ: 3, Damage: -2, Metal: 7, Energy: 8}
	keys := worldCheckpointKeys(t, base)
	normalized := *base
	normalized.FootprintX = 1
	normalized.Damage = 0
	terrain := &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true, FeatureDefs: []*content.FeatureDef{&normalized}}
	terrain.RecordCheckpointFeatureNormalization(base, &normalized)
	got := worldCheckpointBytes(t, terrain, keys)
	// ClassRestamp; one present normalized row; catalog base key; the five
	// transform words in API order, then empty names, Movers, plot, seeded.
	want, err := hex.DecodeString("00" + "010000000102" + "01000000000b000000666561747572652f626164" +
		"0100000003000000000000000700000008000000" + "00000000" + "00" + "0100000000000000000000000000" + "00")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("normalized payload\ngot  %x\nwant %x", got, want)
	}
}

func TestCheckpointTerrainCollectorBindsOnce(t *testing.T) {
	keys := worldCheckpointKeys(t)
	first := &Terrain{CellW: 1, CellH: 1, Plot: []PlotCell{{}}, voidSwept: true}
	second := *first
	c := NewCheckpointContext(keys)
	if _, err := first.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	if c.Terrain != first {
		t.Fatal("collector did not bind terrain")
	}
	if _, err := first.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	if _, err := second.CollectCheckpointReferences(c); err == nil {
		t.Fatal("collector rebound terrain")
	}
	if c.Terrain != first {
		t.Fatal("failed collection changed binding")
	}
}
