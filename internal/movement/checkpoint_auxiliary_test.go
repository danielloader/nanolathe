package movement

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/nanolathe-gg/nanolathe/formats"
	"github.com/nanolathe-gg/nanolathe/internal/cob"
	"github.com/nanolathe-gg/nanolathe/internal/content"
	"github.com/nanolathe-gg/nanolathe/internal/sim/checkpoint"
	"github.com/nanolathe-gg/nanolathe/internal/units"
	"github.com/nanolathe-gg/nanolathe/internal/world"
	"github.com/nanolathe-gg/nanolathe/vfs"
)

func movementAuxiliaryFixture(t *testing.T) (*System, *CheckpointContext, *checkpoint.BindingAuthority) {
	t.Helper()
	s := &System{Grid: NewOccupancyGrid()}
	c, a := movementCheckpointContext(), checkpoint.NewBindingAuthority()
	s.AttachOverlapBindingWithCheckpointBinding(func(uint8) uint8 { panic("capture called owner state") }, a)
	if err := c.SetAuxiliaryBindings(s, nil, a); err != nil {
		t.Fatal(err)
	}
	return s, c, a
}

func movementAuxiliaryBytes(t *testing.T, s *System, c *CheckpointContext) []byte {
	t.Helper()
	collectMovementCheckpoint(t, s, c)
	return movementCheckpointWrite(t, func(e *checkpoint.Encoder) { _ = s.WriteCheckpoint(e, c) })
}

func movementAuxiliaryRefused(t *testing.T, s *System, c *CheckpointContext) {
	t.Helper()
	if _, err := s.CollectCheckpointReferences(c); err == nil {
		t.Fatal("collector accepted unproved auxiliary binding")
	}
	var out bytes.Buffer
	e := checkpoint.NewEncoder(&out)
	if err := s.WriteCheckpoint(e, c); err == nil || out.Len() != 0 {
		t.Fatalf("writer error=%v bytes=%d", err, out.Len())
	}
	e.U8(1)
	if out.Len() != 0 {
		t.Fatal("refusal was not sticky")
	}
}

func TestMovementCheckpointAuxiliaryCallbackVectorAndPurity(t *testing.T) {
	s, c, a := movementAuxiliaryFixture(t)
	// Independent lexical grid vector: empty planes, present claim callback,
	// empty sector links, absent off-map head, exact overlap and owner callback.
	want := movementCheckpointVector(t, uint32(0), uint32(0), uint8(1), uint64(0), uint32(0), uint8(0), uint8(1), uint8(1), int32(0), int32(0), uint8(0), int32(0), uint32(0), int32(0))
	gridBefore, bindingBefore := *s.Grid, c.auxiliaryBindings
	got := movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementGrid(e, c, s, s.Grid, "grid") })
	if !bytes.Equal(got, want) {
		t.Fatalf("callback vector\ngot %x\nwant %x", got, want)
	}
	before := movementAuxiliaryBytes(t, s, c)
	if err := c.SetAuxiliaryBindings(s, nil, a); err != nil {
		t.Fatal(err)
	}
	if after := movementAuxiliaryBytes(t, s, c); !bytes.Equal(before, after) {
		t.Fatal("capture changed bytes")
	}
	if s.Grid.rev != gridBefore.rev || s.Grid.cells != nil || s.Grid.air != nil || c.auxiliaryBindings != bindingBefore || s.Grid.checkpointOwnerState != gridBefore.checkpointOwnerState || s.Grid.checkpointClaimConflict != gridBefore.checkpointClaimConflict {
		t.Fatal("capture changed grid or proof")
	}
	if allocs := testing.AllocsPerRun(20, func() {
		if err := validateMovementGrid(s, c, s.Grid); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("grid preflight allocated %v", allocs)
	}
	// Ordinary AttachOverlap only touches ownerState. Clearing that callback
	// keeps the independently attested claim method usable.
	claimProof := s.Grid.checkpointClaimConflict
	s.Grid.AttachOverlap(s, nil)
	if s.Grid.checkpointOwnerState != (checkpointGridProof{}) || s.Grid.checkpointClaimConflict != claimProof {
		t.Fatal("ordinary grid setter cleared the wrong proofs")
	}
	want[23] = 0 // ownerState follows the exact overlap byte.
	got = movementCheckpointWrite(t, func(e *checkpoint.Encoder) { writeMovementGrid(e, c, s, s.Grid, "grid") })
	if !bytes.Equal(got, want) {
		t.Fatalf("absent owner-state vector\ngot %x\nwant %x", got, want)
	}
	s.AttachOverlapBindingWithCheckpointBinding(nil, a)
	if s.Grid.checkpointOwnerState != (checkpointGridProof{}) || !s.Grid.checkpointClaimConflict.matches(s.Grid, s, c) {
		t.Fatal("nil owner state acquired proof or claim method lost proof")
	}
	movementAuxiliaryBytes(t, s, c)
}

func TestMovementCheckpointAuxiliaryCallbackRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*System, *CheckpointContext, *checkpoint.BindingAuthority)
	}{
		{"ordinary owner reinstall", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) {
			s.Grid.AttachOverlap(s, s.Grid.ownerState)
		}},
		{"ordinary system reinstall", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) {
			s.AttachOverlapBinding(s.Grid.ownerState)
		}},
		{"nil authority", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) {
			s.AttachOverlapBindingWithCheckpointBinding(s.Grid.ownerState, nil)
		}},
		{"foreign authority", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) {
			s.AttachOverlapBindingWithCheckpointBinding(s.Grid.ownerState, checkpoint.NewBindingAuthority())
		}},
		{"foreign overlap", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) { s.Grid.overlap = &System{} }},
		{"typed nil overlap", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) { s.Grid.overlap = (*System)(nil) }},
		{"absent overlap", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) { s.Grid.overlap = nil }},
		{"copied grid", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) {
			copied := *s.Grid
			s.Grid = &copied
		}},
		{"world replacement", func(s *System, _ *CheckpointContext, _ *checkpoint.BindingAuthority) { s.world = &units.World{} }},
		{"registration only", func(s *System, c *CheckpointContext, a *checkpoint.BindingAuthority) {
			s.AttachOverlapBinding(s.Grid.ownerState)
			if err := c.SetAuxiliaryBindings(s, nil, a); err != nil {
				panic(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, a := movementAuxiliaryFixture(t)
			tc.change(s, c, a)
			movementAuxiliaryRefused(t, s, c)
		})
	}
	s, _, a := movementAuxiliaryFixture(t)
	copied := *s
	// Even with a fresh context and an overlap pointer moved to the copy, its
	// copied callbacks still capture the original System.
	copied.Grid.overlap = &copied
	c := movementCheckpointContext()
	if err := c.SetAuxiliaryBindings(&copied, nil, a); err != nil {
		t.Fatal(err)
	}
	movementAuxiliaryRefused(t, &copied, c)
	// Reinstallation on the true owner makes precisely those slots admissible.
	s.AttachOverlapBindingWithCheckpointBinding(s.Grid.ownerState, a)
	c = movementCheckpointContext()
	if err := c.SetAuxiliaryBindings(s, nil, a); err != nil {
		t.Fatal(err)
	}
	movementAuxiliaryBytes(t, s, c)
	var absent *System
	absent.AttachOverlapBindingWithCheckpointBinding(nil, a)
	(&System{}).AttachOverlapBindingWithCheckpointBinding(nil, a)
}

func TestMovementCheckpointAuxiliaryPathRegistrationBothOrders(t *testing.T) {
	for _, pathFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "auxiliary first", true: "path first"}[pathFirst], func(t *testing.T) {
			a := checkpoint.NewBindingAuthority()
			s := NewSystemWithCheckpointBindings(nil, Profile{}, nil, a)
			w := &units.World{}
			s.world, s.pathProvider.world = w, w
			c := movementCheckpointContext()
			registerPath := func(w *units.World, a *checkpoint.BindingAuthority) error { return c.SetPathBindings(s, w, a) }
			registerAux := func(w *units.World, a *checkpoint.BindingAuthority) error { return c.SetAuxiliaryBindings(s, w, a) }
			first, second := registerAux, registerPath
			if pathFirst {
				first, second = registerPath, registerAux
			}
			if err := first(w, a); err != nil {
				t.Fatal(err)
			}
			beforeSystem, beforeAux, beforePath, beforePaths := c.system, c.auxiliaryBindings, c.pathBindings, *c.Paths
			for _, bad := range []struct {
				world     *units.World
				authority *checkpoint.BindingAuthority
			}{{w, checkpoint.NewBindingAuthority()}, {nil, a}} {
				// Match live world edges so the cross-context disagreement, not an
				// incidental alias failure, must reject the second registration.
				s.world, s.pathProvider.world = bad.world, bad.world
				if err := second(bad.world, bad.authority); err == nil {
					t.Fatal("conflicting registration accepted")
				}
				if c.system != beforeSystem || c.auxiliaryBindings != beforeAux || c.pathBindings != beforePath || !reflect.DeepEqual(*c.Paths, beforePaths) {
					t.Fatal("conflict mutated either context")
				}
			}
			s.world, s.pathProvider.world = w, w
			if err := second(w, a); err != nil {
				t.Fatal(err)
			}
			movementPathBindingBytes(t, s, c)
		})
	}
	s, c, a := movementAuxiliaryFixture(t)
	before := c.auxiliaryBindings
	for _, bad := range []struct {
		context   *CheckpointContext
		system    *System
		world     *units.World
		authority *checkpoint.BindingAuthority
	}{
		{nil, s, nil, a}, {c, nil, nil, a}, {c, s, nil, nil}, {c, s, &units.World{}, a}, {c, &System{}, nil, a}, {c, s, nil, checkpoint.NewBindingAuthority()},
	} {
		if err := bad.context.SetAuxiliaryBindings(bad.system, bad.world, bad.authority); err == nil {
			t.Fatal("invalid registration accepted")
		}
		if c.auxiliaryBindings != before || c.system != s {
			t.Fatal("invalid registration changed context")
		}
	}
}

func movementAirAuxiliaryFixture(t *testing.T) (*System, *CheckpointContext, *checkpoint.BindingAuthority) {
	t.Helper()
	terrain := &world.Terrain{CellW: 17, CellH: 9, Plot: make([]world.PlotCell, 17*9), SeaLevel: 7}
	terrain.Plot[0].SetMaxHeight(91)
	a := checkpoint.NewBindingAuthority()
	s := NewSystemWithCheckpointBindings(terrain, Profile{}, NewOccupancyGrid(), a)
	c := movementCheckpointContext()
	if err := c.SetTerrainBindings(s, world.NewCheckpointContext(nil), s.Grid, a); err != nil {
		t.Fatal(err)
	}
	if err := c.SetPathBindings(s, nil, a); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAuxiliaryBindings(s, nil, a); err != nil {
		t.Fatal(err)
	}
	return s, c, a
}

func TestMovementCheckpointAuxiliaryAirSnapshotPresenceAndPurity(t *testing.T) {
	s, c, _ := movementAirAuxiliaryFixture(t)
	g, p := s.AirSectors, s.checkpointAirSectors
	if len(g.records) != 8 || g.Columns*g.Rows != 6 || &p.records[0] == &g.records[0] {
		t.Fatal("fixture did not retain independent padded snapshot")
	}
	before := movementAuxiliaryBytes(t, s, c)
	if before[0] != 1 || s.Grid.cells != nil || s.Grid.air != nil {
		t.Fatal("air presence or lazy occupancy differs")
	}
	records, sentinel := slices.Clone(g.records), g.sentinel
	// Current mutable terrain is not the admitted constructor input. Changing
	// it must not recompute the immutable air grid at capture [04 R-AIR-01 §5].
	s.Terrain.Plot[0].SetMaxHeight(1)
	s.Terrain.SeaLevel = 100
	if after := movementAuxiliaryBytes(t, s, c); !bytes.Equal(before, after) {
		t.Fatal("capture rebuilt constructor grid from mutable terrain")
	}
	if !slices.Equal(records, g.records) || g.sentinel != sentinel || s.Grid.cells != nil || s.Grid.air != nil {
		t.Fatal("capture mutated grid storage")
	}
	if allocs := testing.AllocsPerRun(20, func() {
		if err := s.validateCheckpointAirSectors(c); err != nil {
			panic(err)
		}
	}); allocs != 0 {
		t.Fatalf("air snapshot validation allocated %v", allocs)
	}
	// A separate absent fixture context preserves the old byte framing. The
	// only encoded change is the first presence byte, never snapshot metadata.
	s.AirSectors = nil
	absent := movementCheckpointContext()
	absent.terrainBindings = c.terrainBindings
	absent.pathBindings = c.pathBindings
	without := movementAuxiliaryBytes(t, s, absent)
	expected := slices.Clone(before)
	expected[0] = 0
	if !bytes.Equal(without, expected) {
		t.Fatal("air snapshot added a payload or moved existing fields")
	}
	s.AirSectors = g
	movementAuxiliaryBytes(t, s, c)
}

func TestMovementCheckpointAuxiliaryAirSnapshotRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*System)
	}{
		{"grid removed", func(s *System) { s.AirSectors = nil }},
		{"equal grid replacement", func(s *System) { g := *s.AirSectors; s.AirSectors = &g }},
		{"proof removed", func(s *System) { s.checkpointAirSectors = nil }},
		{"columns", func(s *System) { s.AirSectors.Columns++ }},
		{"rows", func(s *System) { s.AirSectors.Rows++ }},
		{"width", func(s *System) { s.AirSectors.cellW++ }},
		{"height", func(s *System) { s.AirSectors.cellH++ }},
		{"record height", func(s *System) { s.AirSectors.records[0].Height++ }},
		{"record smooth", func(s *System) { s.AirSectors.records[0].Smoothed++ }},
		{"record edge", func(s *System) { s.AirSectors.records[0].Edge ^= 0x80000000 }},
		{"padding", func(s *System) { s.AirSectors.records[7].Height++ }},
		{"record length", func(s *System) { s.AirSectors.records = s.AirSectors.records[:7] }},
		{"sentinel height", func(s *System) { s.AirSectors.sentinel.Height++ }},
		{"sentinel smooth", func(s *System) { s.AirSectors.sentinel.Smoothed++ }},
		{"sentinel edge", func(s *System) { s.AirSectors.sentinel.Edge++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := movementAirAuxiliaryFixture(t)
			tc.change(s)
			movementAuxiliaryRefused(t, s, c)
		})
	}
	s, c, a := movementAirAuxiliaryFixture(t)
	for _, copyOwner := range []bool{false, true} {
		copy := *s
		target := s
		if copyOwner {
			target = &copy
		}
		ctx := movementCheckpointContext()
		authority := checkpoint.NewBindingAuthority()
		if copyOwner {
			authority = a
		}
		if err := ctx.SetAuxiliaryBindings(target, nil, authority); err != nil {
			t.Fatal(err)
		}
		if err := target.validateCheckpointAirSectors(ctx); err == nil {
			t.Fatal("foreign authority or copied System inherited air proof")
		}
	}
	s.AirSectors = NewAirSectorGrid(s.Terrain)
	if err := s.validateCheckpointAirSectors(c); err == nil {
		t.Fatal("rebuilding an equal air grid forged constructor proof")
	}
	// Ordinary construction has exactly the same values, but no proof.
	ordinary := NewSystem(s.Terrain, Profile{}, NewOccupancyGrid())
	ctx := movementCheckpointContext()
	if err := ctx.SetAuxiliaryBindings(ordinary, nil, a); err != nil {
		t.Fatal(err)
	}
	if err := ordinary.validateCheckpointAirSectors(ctx); err == nil {
		t.Fatal("context registration attested ordinary constructor")
	}
}

func TestMovementCheckpointAuxiliaryBindWorldUsesExactObserverInstaller(t *testing.T) {
	fs := vfs.New()
	t.Cleanup(func() { _ = fs.Close() })
	catalog := movementAuxiliaryCatalog(t, fs)
	sources, err := content.CaptureSimulationSources(fs, "")
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := content.FreezeSimulationInputs(sources, content.SimulationInputRequest{Catalog: catalog})
	if err != nil {
		t.Fatal(err)
	}
	keys, err := inputs.CheckpointKeys()
	if err != nil {
		t.Fatal(err)
	}
	a := checkpoint.NewBindingAuthority()
	w := units.NewSliced(2, catalog)
	s := NewSystemWithCheckpointBindings(nil, Profile{}, nil, a)
	c := units.NewCheckpointContext(keys)
	if err := c.SetWorldBindings(w, inputs, nil, nil, nil, a); err != nil {
		t.Fatal(err)
	}
	s.BindWorldWithCheckpointBinding(w, a)
	if w.AttachmentObserver() != s || s.world != w || s.pathProvider.world != w || s.layerRegistry.world != w || len(s.Routes) != w.Capacity()+1 {
		t.Fatal("admitted bind changed ordinary owner or sizing behavior")
	}
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	s.pathProvider.started[2] = true
	s.BindWorldWithCheckpointBinding(w, a)
	if !s.pathProvider.started[2] {
		t.Fatal("same-world bind reset cursor")
	}
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	s.BindWorld(w)
	if _, err := w.CollectCheckpointReferences(c); err == nil {
		t.Fatal("ordinary BindWorld retained observer proof")
	}
	if !s.pathProvider.started[2] || w.AttachmentObserver() != s {
		t.Fatal("ordinary BindWorld changed gameplay binding")
	}
	s.BindWorldWithCheckpointBinding(w, a)
	if _, err := w.CollectCheckpointReferences(c); err != nil {
		t.Fatal(err)
	}
	copied := *s
	copied.BindWorldWithCheckpointBinding(w, a)
	if w.AttachmentObserver() != &copied {
		t.Fatal("copied System installed original observer")
	}
	// An old System leaving a shared world does not erase its successor.
	s.BindWorldWithCheckpointBinding(nil, a)
	if w.AttachmentObserver() != &copied || s.world != nil || s.pathProvider.world != nil || s.layerRegistry.world != nil || s.pathProvider.started[2] {
		t.Fatal("leaving world erased successor or retained old world state")
	}
	copied.BindWorldWithCheckpointBinding(nil, a)
	if w.AttachmentObserver() != nil {
		t.Fatal("leaving exact world failed to clear observer")
	}
	var absent *System
	absent.BindWorldWithCheckpointBinding(w, a)
}

// One authored model and tiny returning script exercise units' actual observer
// proof validation without a running unit or any retail assets [fmt cob].
func movementAuxiliaryCatalog(t *testing.T, fs *vfs.FS) *content.Catalog {
	t.Helper()
	root := t.TempDir()
	model, err := formats.EncodeThreeDO(&formats.ThreeDO{Root: 0, Objects: []formats.ThreeDOObject{{Version: 1, Name: "base", Selection: -1, Parent: -1, FirstChild: -1, NextSibling: -1, Vertices: []formats.ThreeDOVertex{{}}}}})
	if err != nil {
		t.Fatal(err)
	}
	script := make([]byte, 63)
	for i, word := range []uint32{4, 1, 0, 1, 0, 0, 44, 48, 0, 52, 56, 0, 56, 0x10065000} {
		binary.LittleEndian.PutUint32(script[i*4:], word)
	}
	copy(script[56:], "Create\x00")
	program, err := cob.Load(script)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{"objects3d/auxiliary.3do", model}, {"scripts/auxiliary.cob", script}} {
		path := filepath.Join(root, filepath.FromSlash(file.name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, file.data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := fs.MountDirectory(root, 10); err != nil {
		t.Fatal(err)
	}
	defs := map[string]*content.UnitDef{"auxiliary": {DefinitionHeader: content.DefinitionHeader{CanonicalKey: "auxiliary"}, UnitName: "auxiliary", ObjectName: "auxiliary", Script: program, Limit: -1}}
	if _, err := content.CompileCategories(defs); err != nil {
		t.Fatal(err)
	}
	return &content.Catalog{Units: defs, Hash: "authored-auxiliary-fixture"}
}
